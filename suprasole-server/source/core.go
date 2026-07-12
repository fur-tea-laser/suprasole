package source

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"runtime"
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
		pendingCount:    make(map[uint16]int),
		schedulerSignal: make(chan struct{}, 1),
	}
	workspace.backpressureCond = sync.NewCond(&workspace.mutex)
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

const (
	QueueIndexControl = 0
	QueueIndexHigh    = 1
	QueueIndexLow     = 2
)

// PacingInterval aligns pacing sleeps with standard OS scheduling time slices.
const PacingInterval = 15 * time.Millisecond

// TimeNow allows tests to override the clock for deterministic pacing tests.
var TimeNow = time.Now

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
	mutex sync.Mutex
	// Centralized tiered FIFO queues (0 = Control, 1 = High, 2 = Low)
	centralizedQueues [3][]OutboundFrame
	// Backpressure: Tracks number of pending frames in the queue per terminal ID
	pendingCount map[uint16]int
	// Terminals map
	ptys map[uint16]*ptyInstance
	// Thread signaling
	schedulerSignal chan struct{} // Buffered channel (size 1) for non-blocking wakeups
	// Active WebSocket socket writer (guarded by workspace.mutex)
	socketWriter SocketWriter
	// Pacing Barrier State
	pacingDeadline time.Time // Epoch until which low-priority draining is paused
	// Replay Phase Tracking
	pendingReplays int // Counts remaining historical replay frames to prevent starvation
	// Queue Versioning Guard: Prevents stale pops on reconnection flushes
	queueGeneration uint64
	// Global backpressure condition variable
	backpressureCond *sync.Cond
	// Pre-existing metadata fields
	id             string
	spawningPTYs   map[uint16]bool
	terminatedPTYs map[uint16]bool
	isTornDown     bool
	waitGroup      sync.WaitGroup
}

type ptyInstance struct {
	terminalID uint16
	master     *os.File
	command    *exec.Cmd
	buffer     *ringBuffer
	priority   byte // PriorityHigh = High, PriorityLow = Low
	mutex      sync.Mutex // protects priority, exitStatus, and ring buffer writes
	exitStatus byte
	waitDone   chan struct{}
	drainSignal chan struct{}
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
	if error := setSize(int(slave.Fd()), columns, rows); error != nil {
		master.Close()
		slave.Close()
		workspace.mutex.Lock()
		delete(workspace.spawningPTYs, terminalID)
		workspace.mutex.Unlock()
		return fmt.Errorf("failed to set geometry: %w", error)
	}
	shell := "/bin/bash"
	if _, error := os.Stat(shell); error != nil {
		shell = "/bin/sh"
	}
	command := exec.Command(shell)
	command.Stdin = slave
	command.Stdout = slave
	command.Stderr = slave
	command.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    1,
	}
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
	slave.Close()
	terminal := &ptyInstance{
		terminalID:  terminalID,
		master:      master,
		command:     command,
		buffer:      newRingBuffer(256 * 1024), // 256KB ring buffer capacity
		priority:    0x01,                      // Default to High Priority
		waitDone:    make(chan struct{}),
		drainSignal: make(chan struct{}),
	}
	workspace.mutex.Lock()
	delete(workspace.spawningPTYs, terminalID)
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
	workspace.pacingDeadline = TimeNow().Add(PacingInterval)
	workspace.waitGroup.Add(1)
	workspace.mutex.Unlock()
	go func() {
		defer close(terminal.waitDone)
		_ = command.Wait()
		var status byte = 0
		if command.ProcessState == nil {
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
		for i := 0; i < 10; i++ {
			var bytesAvailable int
			err := ioctl(int(master.Fd()), syscall.TIOCINQ, uintptr(unsafe.Pointer(&bytesAvailable)))
			if err != nil || bytesAvailable == 0 {
				break
			}
			runtime.Gosched()
		}
		_ = master.Close()
	}()
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
		priority := terminal.priority
		terminal.mutex.Unlock()
		copied := make([]byte, n)
		copy(copied, buf[:n])
		workspace.EnqueueFrame(OutboundFrame{
			Action:           0x0005,
			TerminalID:       terminal.terminalID,
			Payload:          copied,
			DrainingPriority: priority,
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
	<-terminal.waitDone
	terminal.mutex.Lock()
	exitStatus := terminal.exitStatus
	terminal.mutex.Unlock()
	workspace.EnqueueFrame(OutboundFrame{
		Action:           0x0004,
		TerminalID:       terminalID,
		Payload:          []byte{exitStatus},
		DrainingPriority: PriorityHigh,
	})
	workspace.mutex.Lock()
	queueLength := workspace.pendingCount[terminalID]
	isTornDown := workspace.isTornDown
	hasWriter := workspace.socketWriter != nil
	workspace.mutex.Unlock()
	if queueLength > 0 && hasWriter && !isTornDown {
		<-terminal.drainSignal
	}
	workspace.mutex.Lock()
	delete(workspace.ptys, terminalID)
	delete(workspace.pendingCount, terminalID)
	workspace.mutex.Unlock()
	_ = terminal.master.Close()
	workspace.waitGroup.Done()
}

// ResizePTY resizes an active PTY process.
func (workspace *Workspace) ResizePTY(terminalID uint16, columns uint16, rows uint16) error {
	workspace.mutex.Lock()
	terminal, exists := workspace.ptys[terminalID]
	workspace.mutex.Unlock()
	if !exists {
		return fmt.Errorf("terminal %d not found", terminalID)
	}
	terminal.mutex.Lock()
	defer terminal.mutex.Unlock()
	return setSize(int(terminal.master.Fd()), columns, rows)
}

// WritePTYInput writes raw bytes directly to the PTY stdin pipe.
func (workspace *Workspace) WritePTYInput(terminalID uint16, data []byte) error {
	workspace.mutex.Lock()
	terminal, exists := workspace.ptys[terminalID]
	if !exists {
		workspace.mutex.Unlock()
		return fmt.Errorf("terminal %d not found", terminalID)
	}
	terminal.mutex.Lock()
	isHigh := terminal.priority == PriorityHigh
	terminal.mutex.Unlock()
	if isHigh {
		workspace.pacingDeadline = TimeNow().Add(PacingInterval)
		workspace.notifyScheduler()
	}
	workspace.mutex.Unlock()
	terminal.mutex.Lock()
	defer terminal.mutex.Unlock()
	_, error := terminal.master.Write(data)
	return error
}

// GetScrollbackBuffer retrieves the current scrollback buffer and its truncation status.
func (workspace *Workspace) GetScrollbackBuffer(terminalID uint16) (buffer []byte, isTruncated bool, error error) {
	workspace.mutex.Lock()
	terminal, exists := workspace.ptys[terminalID]
	workspace.mutex.Unlock()
	if !exists {
		return nil, false, fmt.Errorf("terminal %d not found", terminalID)
	}
	terminal.mutex.Lock()
	defer terminal.mutex.Unlock()
	return terminal.buffer.Bytes(), terminal.buffer.isTruncated, nil
}

// GetActiveTerminalIDs lists all active Terminal IDs inside this workspace.
func (workspace *Workspace) GetActiveTerminalIDs() []uint16 {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	var ids []uint16
	for id := range workspace.ptys {
		ids = append(ids, id)
	}
	return ids
}

// GetPTYPriority retrieves the priority state of an active terminal.
func (workspace *Workspace) GetPTYPriority(terminalID uint16) (byte, error) {
	workspace.mutex.Lock()
	terminal, exists := workspace.ptys[terminalID]
	workspace.mutex.Unlock()
	if !exists {
		return 0, fmt.Errorf("terminal %d not found", terminalID)
	}
	terminal.mutex.Lock()
	defer terminal.mutex.Unlock()
	return terminal.priority, nil
}

// SetPTYPriority updates the priority state of an active terminal.
func (workspace *Workspace) SetPTYPriority(terminalID uint16, priority byte) error {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	terminal, exists := workspace.ptys[terminalID]
	if !exists {
		return fmt.Errorf("terminal %d not found", terminalID)
	}
	terminal.mutex.Lock()
	terminal.priority = priority
	terminal.mutex.Unlock()
	workspace.backpressureCond.Broadcast()
	workspace.pacingDeadline = TimeNow().Add(PacingInterval)
	workspace.notifyScheduler()
	return nil
}

// SetSocketWriter binds the active WebSocket writer.
func (workspace *Workspace) SetSocketWriter(socketWriter SocketWriter) {
	workspace.mutex.Lock()
	workspace.socketWriter = socketWriter
	workspace.backpressureCond.Broadcast()
	workspace.mutex.Unlock()
	workspace.notifyScheduler()
}

// ClearSocketWriter detaches the active writer only if it matches the passed writer.
func (workspace *Workspace) ClearSocketWriter(socketWriter SocketWriter) {
	workspace.mutex.Lock()
	if workspace.socketWriter == socketWriter {
		workspace.socketWriter = nil
		for _, pty := range workspace.ptys {
			if pty.drainSignal != nil {
				select {
				case <-pty.drainSignal:
				default:
					close(pty.drainSignal)
				}
			}
		}
	}
	workspace.mutex.Unlock()
	workspace.notifyScheduler()
}

// GetSocketWriter retrieves the current active socket writer.
func (workspace *Workspace) GetSocketWriter() SocketWriter {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	return workspace.socketWriter
}

func (workspace *Workspace) isEmpty() bool {
	return len(workspace.centralizedQueues[QueueIndexControl]) == 0 &&
		len(workspace.centralizedQueues[QueueIndexHigh]) == 0 &&
		len(workspace.centralizedQueues[QueueIndexLow]) == 0
}

func (workspace *Workspace) dropOldestFrame(terminalID uint16) bool {
	for _, queueIndex := range []int{QueueIndexLow, QueueIndexHigh} {
		queue := workspace.centralizedQueues[queueIndex]
		for i, f := range queue {
			if f.TerminalID == terminalID {
				copy(queue[i:], queue[i+1:])
				queue[len(queue)-1] = OutboundFrame{}
				workspace.centralizedQueues[queueIndex] = queue[:len(queue)-1]
				workspace.pendingCount[terminalID]--
				return true
			}
		}
	}
	return false
}

// EnqueueFrame registers outbound frames with flow controls.
func (workspace *Workspace) EnqueueFrame(frame OutboundFrame) {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	if workspace.socketWriter == nil || workspace.isTornDown {
		return
	}
	terminal, exists := workspace.ptys[frame.TerminalID]
	if !exists {
		return
	}
	queueIndex := QueueIndexLow
	if frame.DrainingPriority == PriorityHigh {
		queueIndex = QueueIndexHigh
	}
	if workspace.pendingReplays > 0 {
		queueIndex = QueueIndexLow
	}
	if workspace.pendingCount[frame.TerminalID] >= 1024 {
		if queueIndex == QueueIndexLow {
			workspace.dropOldestFrame(frame.TerminalID)
		} else {
			for {
				terminal.mutex.Lock()
				p := terminal.priority
				terminal.mutex.Unlock()
				if !(workspace.pendingCount[frame.TerminalID] >= 1024 && !workspace.isTornDown && !workspace.terminatedPTYs[frame.TerminalID] && workspace.socketWriter != nil && p == PriorityHigh) {
					break
				}
				workspace.backpressureCond.Wait()
			}
			if workspace.isTornDown || workspace.terminatedPTYs[frame.TerminalID] {
				return
			}
			if workspace.socketWriter == nil {
				return
			}
			if workspace.pendingCount[frame.TerminalID] >= 1024 {
				workspace.dropOldestFrame(frame.TerminalID)
			}
		}
	}
	workspace.centralizedQueues[queueIndex] = append(workspace.centralizedQueues[queueIndex], frame)
	workspace.pendingCount[frame.TerminalID]++
	if queueIndex == QueueIndexHigh {
		workspace.pacingDeadline = TimeNow().Add(PacingInterval)
	}
	workspace.notifyScheduler()
}

// EnqueueControlFrame enqueues a workspace-level control frame.
func (workspace *Workspace) EnqueueControlFrame(frame OutboundFrame) {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	if workspace.isTornDown {
		return
	}
	workspace.centralizedQueues[QueueIndexControl] = append(workspace.centralizedQueues[QueueIndexControl], frame)
	workspace.notifyScheduler()
}

func (workspace *Workspace) notifyScheduler() {
	select {
	case workspace.schedulerSignal <- struct{}{}:
	default:
	}
}

func (workspace *Workspace) peekNextFrame() (OutboundFrame, int, uint64, bool) {
	for queueIndex := QueueIndexControl; queueIndex <= QueueIndexLow; queueIndex++ {
		queue := workspace.centralizedQueues[queueIndex]
		if len(queue) > 0 {
			return queue[0], queueIndex, workspace.queueGeneration, true
		}
	}
	return OutboundFrame{}, -1, 0, false
}

func (workspace *Workspace) popNextFrame(queueIndex int, generation uint64) {
	if generation != workspace.queueGeneration {
		return
	}
	queue := workspace.centralizedQueues[queueIndex]
	if len(queue) > 0 {
		frame := queue[0]
		copy(queue[0:], queue[1:])
		queue[len(queue)-1] = OutboundFrame{}
		workspace.centralizedQueues[queueIndex] = queue[:len(queue)-1]
		if queueIndex != QueueIndexControl {
			workspace.pendingCount[frame.TerminalID]--
			workspace.backpressureCond.Broadcast()
			if workspace.pendingCount[frame.TerminalID] == 0 {
				if pty, exists := workspace.ptys[frame.TerminalID]; exists && pty.drainSignal != nil {
					select {
					case <-pty.drainSignal:
					default:
						close(pty.drainSignal)
					}
				}
			}
		}
		if queueIndex == QueueIndexLow && workspace.pendingReplays > 0 {
			workspace.pendingReplays--
		}
	}
}

// FlushAndEnqueueReplays flushes all queues and enqueues the state replays atomically.
func (workspace *Workspace) FlushAndEnqueueReplays(replays []OutboundFrame) {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	workspace.centralizedQueues[QueueIndexHigh] = nil
	workspace.centralizedQueues[QueueIndexLow] = nil
	workspace.queueGeneration++
	workspace.pendingCount = make(map[uint16]int)
	workspace.pacingDeadline = time.Time{}
	workspace.pendingReplays = len(replays)
	for _, frame := range replays {
		workspace.centralizedQueues[QueueIndexLow] = append(workspace.centralizedQueues[QueueIndexLow], frame)
		workspace.pendingCount[frame.TerminalID]++
	}
	// Reset any pending drainSignals because the queues are flushed/reset
	for termID, pty := range workspace.ptys {
		if workspace.pendingCount[termID] == 0 && pty.drainSignal != nil {
			select {
			case <-pty.drainSignal:
			default:
				close(pty.drainSignal)
			}
		}
	}
	workspace.backpressureCond.Broadcast()
	workspace.notifyScheduler()
}

func (workspace *Workspace) startScheduler() {
	for {
		workspace.mutex.Lock()
		for workspace.isEmpty() && !workspace.isTornDown {
			workspace.mutex.Unlock()
			<-workspace.schedulerSignal
			workspace.mutex.Lock()
		}
		if workspace.isTornDown {
			workspace.mutex.Unlock()
			return
		}
		for workspace.pendingReplays == 0 &&
			len(workspace.centralizedQueues[QueueIndexControl]) == 0 &&
			len(workspace.centralizedQueues[QueueIndexHigh]) == 0 &&
			TimeNow().Before(workspace.pacingDeadline) {
			sleepDuration := workspace.pacingDeadline.Sub(TimeNow())
			if sleepDuration < 0 {
				sleepDuration = -sleepDuration
			}
			select {
			case <-workspace.schedulerSignal:
			default:
			}
			workspace.mutex.Unlock()
			select {
			case <-workspace.schedulerSignal:
			case <-time.After(sleepDuration):
			}
			workspace.mutex.Lock()
		}
		frame, queueIndex, generation, ok := workspace.peekNextFrame()
		writer := workspace.socketWriter
		workspace.mutex.Unlock()
		if !ok {
			continue
		}
		if writer == nil {
			workspace.mutex.Lock()
			for workspace.socketWriter == nil && !workspace.isTornDown {
				workspace.mutex.Unlock()
				<-workspace.schedulerSignal
				workspace.mutex.Lock()
			}
			workspace.mutex.Unlock()
			continue
		}
		err := writer.WriteFrame(frame.Action, frame.TerminalID, frame.Payload)
		if err != nil {
			workspace.ClearSocketWriter(writer)
			continue
		}
		workspace.mutex.Lock()
		workspace.popNextFrame(queueIndex, generation)
		workspace.mutex.Unlock()
	}
}

func (workspace *Workspace) teardown() {
	workspace.mutex.Lock()
	workspace.isTornDown = true
	workspace.backpressureCond.Broadcast()
	for _, terminal := range workspace.ptys {
		if terminal.drainSignal != nil {
			select {
			case <-terminal.drainSignal:
			default:
				close(terminal.drainSignal)
			}
		}
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

// TerminatePTY programmatically terminates an active PTY process.
func (workspace *Workspace) TerminatePTY(terminalID uint16) error {
	workspace.mutex.Lock()
	workspace.terminatedPTYs[terminalID] = true
	terminal, exists := workspace.ptys[terminalID]
	isSpawning := workspace.spawningPTYs[terminalID]
	workspace.backpressureCond.Broadcast()
	workspace.mutex.Unlock()
	if !exists && !isSpawning {
		return fmt.Errorf("terminal %d not found", terminalID)
	}
	if !exists {
		return nil
	}
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
	return nil
}
