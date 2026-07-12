package source

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// WorkspaceRegistry coordinates multiple tenant workspaces.
type WorkspaceRegistry interface {
	GetOrCreateWorkspace(workspaceID string) (*Workspace, error)
	RemoveWorkspace(workspaceID string) error
}

// defaultRegistry implements WorkspaceRegistry.
type defaultRegistry struct {
	mutex      sync.RWMutex
	workspaces map[string]*Workspace
}

func (registry *defaultRegistry) GetOrCreateWorkspace(workspaceID string) (*Workspace, error) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	workspace, exists := registry.workspaces[workspaceID]
	if exists {
		return workspace, nil
	}
	workspace = &Workspace{
		id:              workspaceID,
		ptys:            make(map[uint16]*ptyInstance),
		spawningPTYs:    make(map[uint16]bool),
		terminatedPTYs:  make(map[uint16]bool),
		schedulerSignal: make(chan struct{}, 1),
	}
	registry.workspaces[workspaceID] = workspace
	go workspace.startScheduler()
	return workspace, nil
}

func (registry *defaultRegistry) RemoveWorkspace(workspaceID string) error {
	registry.mutex.Lock()
	workspace, exists := registry.workspaces[workspaceID]
	if !exists {
		registry.mutex.Unlock()
		return fmt.Errorf("workspace %s not found", workspaceID)
	}
	delete(registry.workspaces, workspaceID)
	registry.mutex.Unlock()
	// Perform teardown (blocks until all PIDs are reaped)
	workspace.teardown()
	return nil
}

// NewWorkspaceRegistry creates a new default WorkspaceRegistry instance.
func NewWorkspaceRegistry() WorkspaceRegistry {
	return &defaultRegistry{
		workspaces: make(map[string]*Workspace),
	}
}

const (
	ActionSpawn        uint16 = 0x0001
	ActionSpawnStatus  uint16 = 0x0002
	ActionResize       uint16 = 0x0003
	ActionKill         uint16 = 0x0004
	ActionStreamIO     uint16 = 0x0005
	ActionPrioritySync uint16 = 0x0006
)
const (
	PriorityLow  byte = 0x00
	PriorityHigh byte = 0x01
)

// SocketWriter coordinates outbound framing on socket layers.
type SocketWriter interface {
	WriteFrame(action uint16, terminalID uint16, payload []byte) error
}

// OutboundFrame represents a structured outbound message.
type OutboundFrame struct {
	Action           uint16
	TerminalID       uint16
	Payload          []byte
	DrainingPriority byte // PriorityHigh = High, PriorityLow = Low
}

// Workspace represents a collection of PTY instances belonging to a tenant.
type Workspace struct {
	mutex          sync.RWMutex
	id             string
	ptys           map[uint16]*ptyInstance
	spawningPTYs   map[uint16]bool
	terminatedPTYs map[uint16]bool
	isTornDown     bool
	waitGroup      sync.WaitGroup
	// centralized priority scheduler fields
	socketWriter    SocketWriter
	writerMutex     sync.Mutex
	schedulerSignal chan struct{}
	controlQueue    []OutboundFrame
	activeReplays   int
}
type ptyInstance struct {
	terminalID     uint16
	master         *os.File
	command        *exec.Cmd
	buffer         *ringBuffer
	priority       byte // PriorityHigh = High, PriorityLow = Low
	replayActive   bool
	queue          []OutboundFrame
	queueCondition *sync.Cond
	mutex          sync.Mutex // protects priority, queue, and ring buffer writes
	exitStatus     byte
	waitDone       chan struct{}
}

// ringBuffer implements an in-memory FIFO ring buffer with eviction flagging.
type ringBuffer struct {
	data        []byte
	capacity    int
	start       int
	size        int
	isTruncated bool
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{
		data:     make([]byte, capacity),
		capacity: capacity,
	}
}

func (ringBuffer *ringBuffer) Write(p []byte) {
	if len(p) == 0 {
		return
	}
	if len(p) >= ringBuffer.capacity {
		copy(ringBuffer.data, p[len(p)-ringBuffer.capacity:])
		ringBuffer.start = 0
		ringBuffer.size = ringBuffer.capacity
		ringBuffer.isTruncated = true
		return
	}
	if ringBuffer.size+len(p) > ringBuffer.capacity {
		ringBuffer.isTruncated = true
		evict := (ringBuffer.size + len(p)) - ringBuffer.capacity
		ringBuffer.start = (ringBuffer.start + evict) % ringBuffer.capacity
		ringBuffer.size = ringBuffer.capacity - len(p)
	}
	end := (ringBuffer.start + ringBuffer.size) % ringBuffer.capacity
	writeLen := len(p)
	if end+writeLen <= ringBuffer.capacity {
		copy(ringBuffer.data[end:], p)
	} else {
		firstPart := ringBuffer.capacity - end
		copy(ringBuffer.data[end:], p[:firstPart])
		copy(ringBuffer.data[0:], p[firstPart:])
	}
	ringBuffer.size += writeLen
}

func (ringBuffer *ringBuffer) Bytes() []byte {
	if ringBuffer.size == 0 {
		return nil
	}
	result := make([]byte, ringBuffer.size)
	if ringBuffer.start+ringBuffer.size <= ringBuffer.capacity {
		copy(result, ringBuffer.data[ringBuffer.start:ringBuffer.start+ringBuffer.size])
	} else {
		firstPart := ringBuffer.capacity - ringBuffer.start
		copy(result[:firstPart], ringBuffer.data[ringBuffer.start:])
		copy(result[firstPart:], ringBuffer.data[:ringBuffer.size-firstPart])
	}
	return result
}

// Unix winsize struct for ioctl
type winsize struct {
	ws_row    uint16
	ws_col    uint16
	ws_xpixel uint16
	ws_ypixel uint16
}

func ioctl(fileDescriptor int, command uintptr, value uintptr) error {
	_, _, errorNumber := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fileDescriptor), command, value)
	if errorNumber != 0 {
		return errorNumber
	}
	return nil
}

func openPty() (master *os.File, slave *os.File, error error) {
	masterFileDescriptor, error := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if error != nil {
		return nil, nil, fmt.Errorf("failed to open /dev/ptmx: %w", error)
	}
	// Configure master FD to be non-blocking to register it with Go's netpoller.
	// This allows Close() to forcefully interrupt any active Read() calls.
	if error := syscall.SetNonblock(masterFileDescriptor, true); error != nil {
		syscall.Close(masterFileDescriptor)
		return nil, nil, fmt.Errorf("failed to set master PTY non-blocking: %w", error)
	}
	var ptyNumber uintptr
	if error := ioctl(masterFileDescriptor, syscall.TIOCGPTN, uintptr(unsafe.Pointer(&ptyNumber))); error != nil {
		syscall.Close(masterFileDescriptor)
		return nil, nil, fmt.Errorf("ioctl TIOCGPTN failed: %w", error)
	}
	var lock uintptr = 0
	if error := ioctl(masterFileDescriptor, syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&lock))); error != nil {
		syscall.Close(masterFileDescriptor)
		return nil, nil, fmt.Errorf("ioctl TIOCSPTLCK failed: %w", error)
	}
	slavePath := fmt.Sprintf("/dev/pts/%d", ptyNumber)
	slaveFileDescriptor, error := syscall.Open(slavePath, syscall.O_RDWR|syscall.O_NOCTTY, 0)
	if error != nil {
		syscall.Close(masterFileDescriptor)
		return nil, nil, fmt.Errorf("failed to open slave PTY %s: %w", slavePath, error)
	}
	return os.NewFile(uintptr(masterFileDescriptor), "/dev/ptmx"), os.NewFile(uintptr(slaveFileDescriptor), slavePath), nil
}

func setSize(fileDescriptor int, columns uint16, rows uint16) error {
	workspace := winsize{
		ws_row: rows,
		ws_col: columns,
	}
	return ioctl(fileDescriptor, syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(&workspace)))
}

// killDescendants recursively scans /proc and kills all child/grandchild processes.
func killDescendants(processID int) {
	childrenPath := fmt.Sprintf("/proc/%d/task/%d/children", processID, processID)
	data, error := os.ReadFile(childrenPath)
	if error == nil {
		parts := strings.Fields(string(data))
		for _, part := range parts {
			if childProcessID, error := strconv.Atoi(part); error == nil && childProcessID > 0 {
				killDescendants(childProcessID)
				_ = syscall.Kill(childProcessID, syscall.SIGKILL)
				_ = syscall.Kill(-childProcessID, syscall.SIGKILL)
			}
		}
	}
}

// SpawnPTY spawns a target executable directly under a PTY for this workspace.
func (workspace *Workspace) SpawnPTY(terminalID uint16, columns uint16, rows uint16) error {
	workspace.mutex.Lock()
	if workspace.isTornDown {
		workspace.mutex.Unlock()
		return errors.New("workspace is torn down")
	}
	if _, exists := workspace.ptys[terminalID]; exists {
		workspace.mutex.Unlock()
		return fmt.Errorf("terminal %d already exists", terminalID)
	}
	if workspace.terminatedPTYs[terminalID] {
		workspace.mutex.Unlock()
		return fmt.Errorf("terminal %d was already terminated", terminalID)
	}
	workspace.spawningPTYs[terminalID] = true
	workspace.mutex.Unlock()
	master, slave, error := openPty()
	if error != nil {
		workspace.mutex.Lock()
		delete(workspace.spawningPTYs, terminalID)
		workspace.mutex.Unlock()
		return fmt.Errorf("failed to allocate PTY: %w", error)
	}
	// Set initial geometry on PTY slave descriptor
	if error := setSize(int(slave.Fd()), columns, rows); error != nil {
		master.Close()
		slave.Close()
		workspace.mutex.Lock()
		delete(workspace.spawningPTYs, terminalID)
		workspace.mutex.Unlock()
		return fmt.Errorf("failed to set geometry: %w", error)
	}
	// Target resolution: default to /bin/bash, fallback to /bin/sh
	shell := "/bin/bash"
	if _, error := os.Stat(shell); error != nil {
		shell = "/bin/sh"
	}
	command := exec.Command(shell)
	command.Stdin = slave
	command.Stdout = slave
	command.Stderr = slave
	// Session leader attributes and controlling terminal assignment
	command.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    1,
	}
	// Environment variable seeding
	var env []string
	hasPath := false
	for _, e := range os.Environ() {
		if strings.HasPrefix(e, "TERM=") || strings.HasPrefix(e, "LANG=") {
			continue
		}
		if strings.HasPrefix(e, "PATH=") {
			hasPath = true
		}
		env = append(env, e)
	}
	env = append(env, "TERM=xterm-256color")
	env = append(env, "LANG=en_US.UTF-8")
	if !hasPath {
		env = append(env, "PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin")
	}
	command.Env = env
	if error := command.Start(); error != nil {
		master.Close()
		slave.Close()
		workspace.mutex.Lock()
		delete(workspace.spawningPTYs, terminalID)
		workspace.mutex.Unlock()
		return fmt.Errorf("failed to start process: %w", error)
	}
	// EOF Bug Safeguard: Close the parent process's copy of the PTY slave descriptor
	slave.Close()
	terminal := &ptyInstance{
		terminalID: terminalID,
		master:     master,
		command:    command,
		buffer:     newRingBuffer(256 * 1024), // 256KB ring buffer capacity
		priority:   0x01,                      // Default to High Priority
		waitDone:   make(chan struct{}),
	}
	terminal.queueCondition = sync.NewCond(&terminal.mutex)
	workspace.mutex.Lock()
	delete(workspace.spawningPTYs, terminalID)
	// Check if this spawn was aborted via TerminatePTY or RemoveWorkspace mid-launch
	if workspace.isTornDown || workspace.terminatedPTYs[terminalID] {
		workspace.mutex.Unlock()
		killDescendants(command.Process.Pid)
		_ = syscall.Kill(command.Process.Pid, syscall.SIGKILL)
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		_ = master.Close()
		_ = command.Wait()
		return fmt.Errorf("terminal %d terminated mid-spawn", terminalID)
	}
	workspace.ptys[terminalID] = terminal
	workspace.waitGroup.Add(1)
	workspace.mutex.Unlock()
	// Asynchronous Process Exit Monitoring Goroutine
	go func() {
		defer close(terminal.waitDone)
		_ = command.Wait()
		var status byte = 0
		if command.ProcessState == nil {
			// No process state
		} else if waitStatus, ok := command.ProcessState.Sys().(syscall.WaitStatus); ok {
			status = byte(waitStatus.ExitStatus())
			if waitStatus.Signaled() {
				status = byte(128 + waitStatus.Signal())
			}
		} else {
			status = byte(command.ProcessState.ExitCode())
		}
		terminal.mutex.Lock()
		terminal.exitStatus = status
		terminal.mutex.Unlock()
		// Unblock the reader loop by closing the PTY master
		_ = master.Close()
	}()
	// Start asynchronous background reader loop
	go workspace.startReadLoop(terminal)
	return nil
}

func (workspace *Workspace) startReadLoop(terminal *ptyInstance) {
	defer func() {
		workspace.handleProcessExit(terminal.terminalID)
	}()
	buf := make([]byte, 32*1024)
	for {
		n, error := terminal.master.Read(buf)
		if n <= 0 {
			if error != nil {
				break
			}
			continue
		}
		terminal.mutex.Lock()
		terminal.buffer.Write(buf[:n])
		terminal.mutex.Unlock()
		copied := make([]byte, n)
		copy(copied, buf[:n])
		workspace.EnqueueFrame(OutboundFrame{
			Action:           0x0005,
			TerminalID:       terminal.terminalID,
			Payload:          copied,
			DrainingPriority: terminal.priority,
		})
		if error != nil {
			break
		}
	}
}

func (workspace *Workspace) handleProcessExit(terminalID uint16) {
	workspace.mutex.Lock()
	terminal, exists := workspace.ptys[terminalID]
	workspace.mutex.Unlock()
	if !exists {
		workspace.waitGroup.Done()
		return
	}
	// Wait for the exit status monitoring goroutine to finish and populate the exitStatus
	<-terminal.waitDone
	terminal.mutex.Lock()
	exitStatus := terminal.exitStatus
	terminal.mutex.Unlock()
	// Enqueue termination frame
	workspace.EnqueueFrame(OutboundFrame{
		Action:           0x0004,
		TerminalID:       terminalID,
		Payload:          []byte{exitStatus},
		DrainingPriority: PriorityHigh,
	})
	// Wait for the scheduler to fully drain the PTY queue, up to 1 second
	deadline := time.Now().Add(1 * time.Second)
	for {
		terminal.mutex.Lock()
		queueLength := len(terminal.queue)
		terminal.mutex.Unlock()
		workspace.writerMutex.Lock()
		hasWriter := workspace.socketWriter != nil
		workspace.writerMutex.Unlock()
		if queueLength == 0 || !hasWriter || workspace.isTornDown || time.Now().After(deadline) {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	// Delete from active list
	workspace.mutex.Lock()
	if terminal.replayActive {
		terminal.replayActive = false
		workspace.activeReplays--
	}
	delete(workspace.ptys, terminalID)
	workspace.mutex.Unlock()
	_ = terminal.master.Close()
	workspace.waitGroup.Done()
}

// ResizePTY resizes an active PTY process.
func (workspace *Workspace) ResizePTY(terminalID uint16, columns uint16, rows uint16) error {
	workspace.mutex.RLock()
	terminal, exists := workspace.ptys[terminalID]
	workspace.mutex.RUnlock()
	if !exists {
		return fmt.Errorf("terminal %d not found", terminalID)
	}
	terminal.mutex.Lock()
	defer terminal.mutex.Unlock()
	return setSize(int(terminal.master.Fd()), columns, rows)
}

// WritePTYInput writes raw bytes directly to the PTY stdin pipe.
func (workspace *Workspace) WritePTYInput(terminalID uint16, data []byte) error {
	workspace.mutex.RLock()
	terminal, exists := workspace.ptys[terminalID]
	workspace.mutex.RUnlock()
	if !exists {
		return fmt.Errorf("terminal %d not found", terminalID)
	}
	terminal.mutex.Lock()
	defer terminal.mutex.Unlock()
	_, error := terminal.master.Write(data)
	return error
}

// TerminatePTY programmatically terminates an active PTY process.
func (workspace *Workspace) TerminatePTY(terminalID uint16) error {
	workspace.mutex.Lock()
	workspace.terminatedPTYs[terminalID] = true
	terminal, exists := workspace.ptys[terminalID]
	isSpawning := workspace.spawningPTYs[terminalID]
	workspace.mutex.Unlock()
	if !exists && !isSpawning {
		return fmt.Errorf("terminal %d not found", terminalID)
	}
	if !exists {
		return nil
	}
	terminal.mutex.Lock()
	terminal.queueCondition.Broadcast()
	terminal.mutex.Unlock()
	processID := terminal.command.Process.Pid
	killDescendants(processID)
	_ = syscall.Kill(processID, syscall.SIGKILL)
	_ = syscall.Kill(-processID, syscall.SIGKILL)
	// Query terminal driver for active foreground process group and kill it directly
	var processGroupID int32
	ioctlError := ioctl(int(terminal.master.Fd()), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&processGroupID)))
	if ioctlError == nil && processGroupID > 0 {
		_ = syscall.Kill(int(-processGroupID), syscall.SIGKILL)
	}
	_ = terminal.master.Close()
	return nil
}

// GetScrollbackBuffer retrieves the current scrollback buffer and its truncation status.
func (workspace *Workspace) GetScrollbackBuffer(terminalID uint16) (buffer []byte, isTruncated bool, error error) {
	workspace.mutex.RLock()
	terminal, exists := workspace.ptys[terminalID]
	workspace.mutex.RUnlock()
	if !exists {
		return nil, false, fmt.Errorf("terminal %d not found", terminalID)
	}
	terminal.mutex.Lock()
	defer terminal.mutex.Unlock()
	return terminal.buffer.Bytes(), terminal.buffer.isTruncated, nil
}

// GetActiveTerminalIDs lists all active Terminal IDs inside this workspace.
func (workspace *Workspace) GetActiveTerminalIDs() []uint16 {
	workspace.mutex.RLock()
	defer workspace.mutex.RUnlock()
	var ids []uint16
	for id := range workspace.ptys {
		ids = append(ids, id)
	}
	return ids
}

// SetPTYPriority updates the priority state of an active terminal.
func (workspace *Workspace) SetPTYPriority(terminalID uint16, priority byte) error {
	workspace.mutex.RLock()
	terminal, exists := workspace.ptys[terminalID]
	workspace.mutex.RUnlock()
	if !exists {
		return fmt.Errorf("terminal %d not found", terminalID)
	}
	terminal.mutex.Lock()
	terminal.priority = priority
	terminal.mutex.Unlock()
	// Wake up any blocked reader loops and the scheduler pop loop
	terminal.queueCondition.Broadcast()
	workspace.notifyScheduler()
	return nil
}

// GetPTYPriority retrieves the priority state of an active terminal.
func (workspace *Workspace) GetPTYPriority(terminalID uint16) (byte, error) {
	workspace.mutex.RLock()
	terminal, exists := workspace.ptys[terminalID]
	workspace.mutex.RUnlock()
	if !exists {
		return 0, fmt.Errorf("terminal %d not found", terminalID)
	}
	terminal.mutex.Lock()
	defer terminal.mutex.Unlock()
	return terminal.priority, nil
}

// SetSocketWriter binds the active WebSocket writer.
func (workspace *Workspace) SetSocketWriter(socketWriter SocketWriter) {
	workspace.writerMutex.Lock()
	workspace.socketWriter = socketWriter
	workspace.writerMutex.Unlock()
	if socketWriter != nil {
		workspace.mutex.RLock()
		for _, terminal := range workspace.ptys {
			terminal.mutex.Lock()
			terminal.queueCondition.Broadcast()
			terminal.mutex.Unlock()
		}
		workspace.mutex.RUnlock()
	}
	workspace.notifyScheduler()
}

// FlushAndEnqueueReplays flushes all queues and enqueues the state replays atomically.
func (workspace *Workspace) FlushAndEnqueueReplays(replays []OutboundFrame) {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	workspace.controlQueue = nil
	workspace.activeReplays = 0
	for _, terminal := range workspace.ptys {
		terminal.mutex.Lock()
		terminal.queue = nil
		terminal.queueCondition.Broadcast()
		terminal.replayActive = false
		terminal.mutex.Unlock()
	}
	for _, frame := range replays {
		terminal, exists := workspace.ptys[frame.TerminalID]
		if !exists {
			continue
		}
		terminal.mutex.Lock()
		if !terminal.replayActive {
			terminal.replayActive = true
			workspace.activeReplays++
		}
		terminal.queue = append(terminal.queue, frame)
		terminal.mutex.Unlock()
	}
	workspace.notifyScheduler()
}

// ClearSocketWriter detaches the active writer only if it matches the passed writer.
func (workspace *Workspace) ClearSocketWriter(socketWriter SocketWriter) {
	workspace.writerMutex.Lock()
	if workspace.socketWriter == socketWriter {
		workspace.socketWriter = nil
	}
	workspace.writerMutex.Unlock()
	workspace.notifyScheduler()
}

// GetSocketWriter retrieves the current active socket writer.
func (workspace *Workspace) GetSocketWriter() SocketWriter {
	workspace.writerMutex.Lock()
	defer workspace.writerMutex.Unlock()
	return workspace.socketWriter
}

// EnqueueFrame registers outbound frames with flow controls.
func (workspace *Workspace) EnqueueFrame(frame OutboundFrame) {
	workspace.mutex.RLock()
	hasActiveReplays := workspace.activeReplays > 0
	terminal, exists := workspace.ptys[frame.TerminalID]
	workspace.mutex.RUnlock()
	if !exists {
		return
	}
	workspace.writerMutex.Lock()
	hasWriter := workspace.socketWriter != nil
	workspace.writerMutex.Unlock()
	terminal.mutex.Lock()
	defer terminal.mutex.Unlock()
	effectivePriority := terminal.priority
	if !hasWriter || terminal.replayActive || hasActiveReplays {
		effectivePriority = PriorityLow // Fallback to Low priority drop-oldest when offline or during replay phase
	}
	if terminal.replayActive || hasActiveReplays {
		frame.DrainingPriority = PriorityLow // Force low draining priority during replay phase to prevent starvation
	}
	if len(terminal.queue) < 1024 {
		terminal.queue = append(terminal.queue, frame)
		workspace.notifyScheduler()
		return
	}
	if effectivePriority == PriorityLow {
		copy(terminal.queue, terminal.queue[1:])
		terminal.queue[len(terminal.queue)-1] = frame
		workspace.notifyScheduler()
		return
	}
	// High Priority: block until queue space clears
	for len(terminal.queue) >= 1024 && !workspace.isTornDown {
		terminal.queueCondition.Wait()
		workspace.writerMutex.Lock()
		hasWriter = workspace.socketWriter != nil
		workspace.writerMutex.Unlock()
		effectivePriority = terminal.priority
		if !hasWriter || terminal.replayActive {
			effectivePriority = PriorityLow
		}
		if effectivePriority == PriorityLow {
			break
		}
	}
	if len(terminal.queue) >= 1024 {
		copy(terminal.queue, terminal.queue[1:])
		terminal.queue[len(terminal.queue)-1] = frame
	} else {
		terminal.queue = append(terminal.queue, frame)
	}
	workspace.notifyScheduler()
}

// EnqueueControlFrame enqueues a workspace-level control frame.
func (workspace *Workspace) EnqueueControlFrame(frame OutboundFrame) {
	workspace.mutex.Lock()
	workspace.controlQueue = append(workspace.controlQueue, frame)
	workspace.mutex.Unlock()
	workspace.notifyScheduler()
}

func (workspace *Workspace) notifyScheduler() {
	select {
	case workspace.schedulerSignal <- struct{}{}:
	default:
	}
}

// startScheduler runs the central queue draining loop.
func (workspace *Workspace) startScheduler() {
	for {
		workspace.writerMutex.Lock()
		writer := workspace.socketWriter
		workspace.writerMutex.Unlock()
		if workspace.isTornDown {
			return
		}
		var frame OutboundFrame
		var isControl bool
		var found bool
		if writer != nil {
			frame, isControl, found = workspace.popHighestPriorityFrame()
		}
		if !found {
			select {
			case <-workspace.schedulerSignal:
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		error := writer.WriteFrame(frame.Action, frame.TerminalID, frame.Payload)
		if error != nil {
			workspace.SetSocketWriter(nil)
			continue
		}
		workspace.popFrame(isControl, frame.TerminalID)
	}
}

func (workspace *Workspace) popHighestPriorityFrame() (frame OutboundFrame, isControl bool, found bool) {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	// 1. Control Queue has absolute priority
	if len(workspace.controlQueue) > 0 {
		return workspace.controlQueue[0], true, true
	}
	// 2. High Priority PTY queues (randomized order iteration for fair RR)
	for _, terminal := range workspace.ptys {
		terminal.mutex.Lock()
		if len(terminal.queue) > 0 && terminal.queue[0].DrainingPriority == 0x01 {
			frame = terminal.queue[0]
			terminal.mutex.Unlock()
			return frame, false, true
		}
		terminal.mutex.Unlock()
	}
	// 3. Low Priority PTY queues
	for _, terminal := range workspace.ptys {
		terminal.mutex.Lock()
		if len(terminal.queue) > 0 && terminal.queue[0].DrainingPriority == 0x00 {
			frame = terminal.queue[0]
			terminal.mutex.Unlock()
			return frame, false, true
		}
		terminal.mutex.Unlock()
	}
	return OutboundFrame{}, false, false
}

func (workspace *Workspace) popFrame(isControl bool, terminalID uint16) {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	if isControl {
		if len(workspace.controlQueue) > 0 {
			workspace.controlQueue = workspace.controlQueue[1:]
		}
		return
	}
	terminal, exists := workspace.ptys[terminalID]
	if !exists {
		return
	}
	terminal.mutex.Lock()
	if len(terminal.queue) > 0 {
		terminal.queue = terminal.queue[1:]
		terminal.queueCondition.Broadcast()
	}
	if len(terminal.queue) == 0 && terminal.replayActive {
		terminal.replayActive = false
		workspace.activeReplays--
	}
	terminal.mutex.Unlock()
}

func (workspace *Workspace) teardown() {
	workspace.mutex.Lock()
	workspace.isTornDown = true
	for _, terminal := range workspace.ptys {
		terminal.mutex.Lock()
		terminal.queueCondition.Broadcast()
		terminal.mutex.Unlock()
		processID := terminal.command.Process.Pid
		killDescendants(processID)
		_ = syscall.Kill(processID, syscall.SIGKILL)
		_ = syscall.Kill(-processID, syscall.SIGKILL)
		var processGroupID int32
		ioctlError := ioctl(int(terminal.master.Fd()), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&processGroupID)))
		if ioctlError == nil && processGroupID > 0 {
			_ = syscall.Kill(int(-processGroupID), syscall.SIGKILL)
		}
		_ = terminal.master.Close()
	}
	workspace.mutex.Unlock()
	workspace.notifyScheduler()
	workspace.waitGroup.Wait()
}
