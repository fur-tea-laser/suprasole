package source

import (
	"context"
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
		id:                workspaceID,
		ptys:              newOrderedPTYMap(),
		spawningPTYs:      make(map[uint16]bool),
		terminatedPTYs:    make(map[uint16]bool),
		spawning:          make(map[uint16]*spawningInstance),
		pendingPriorities: make(map[uint16]byte),
		pendingCount:      make(map[uint16]int),
		schedulerSignal:   make(chan struct{}, 1),
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
	ActionTerminalExit uint16 = 0x0005
	ActionRemove       uint16 = 0x0006
	ActionInput        uint16 = 0x0007
	ActionOutput       uint16 = 0x0008
	ActionPrioritySync uint16 = 0x0009
	ActionReset        uint16 = 0x000a
)

type TerminalState int

const (
	StateActive TerminalState = iota
	StateSpawning
	StateTerminated
)

func (s TerminalState) String() string {
	switch s {
	case StateSpawning:
		return "Spawning"
	case StateActive:
		return "Active"
	case StateTerminated:
		return "Terminated"
	default:
		return "Unknown"
	}
}

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
	ptys *orderedPTYMap
	// Thread signaling
	schedulerSignal chan struct{} // Buffered channel (size 1) for non-blocking wakeups
	// Active WebSocket socket writer (guarded by workspace.mutex)
	socketWriter SocketWriter
	// Pacing Barrier State
	pacingDeadline time.Time // Epoch until which low-priority draining is paused
	// Replay Phase Tracking
	pendingReplays int // Counts remaining historical replay frames to prevent starvation
	// Global backpressure condition variable
	backpressureCond *sync.Cond
	// Pre-existing metadata fields
	id                string
	spawningPTYs      map[uint16]bool
	terminatedPTYs    map[uint16]bool
	spawning          map[uint16]*spawningInstance // Redesign spawning context cancellation
	pendingPriorities map[uint16]byte
	isTornDown        bool
	waitGroup         sync.WaitGroup
	queueGeneration   uint64
}

type spawningInstance struct {
	cancel    context.CancelFunc
	done      chan struct{}
	isRemoved bool
}

type ptyInstance struct {
	terminalID uint16
	master     *os.File
	command    *exec.Cmd
	buffer     *ringBuffer
	mutex      sync.Mutex // protects priority, exitStatus, and ring buffer writes
	priority   byte
	exitStatus byte
	state      TerminalState // Redesign terminal lifecycle state
	waitDone   chan struct{}
	readDone   chan struct{}
	next       *ptyInstance
	prev       *ptyInstance
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
func (workspace *Workspace) SpawnPTY(terminalID uint16, columns uint16, rows uint16, command string, args ...string) error {
	if columns == 0 || rows == 0 {
		workspace.EnqueueControlFrame(OutboundFrame{
			Action:     ActionSpawnStatus,
			TerminalID: terminalID,
			Payload:    []byte{0x01},
		})
		return fmt.Errorf("dimensions cannot be zero")
	}
	if command == "" {
		workspace.EnqueueControlFrame(OutboundFrame{
			Action:     ActionSpawnStatus,
			TerminalID: terminalID,
			Payload:    []byte{0x01},
		})
		return fmt.Errorf("command path cannot be empty")
	}
	resolvedPath := command
	if !strings.Contains(command, "/") {
		var err error
		resolvedPath, err = exec.LookPath(command)
		if err != nil {
			workspace.EnqueueControlFrame(OutboundFrame{
				Action:     ActionSpawnStatus,
				TerminalID: terminalID,
				Payload:    []byte{0x01},
			})
			return err
		}
	} else {
		if info, err := os.Stat(command); err != nil || info.IsDir() {
			workspace.EnqueueControlFrame(OutboundFrame{
				Action:     ActionSpawnStatus,
				TerminalID: terminalID,
				Payload:    []byte{0x01},
			})
			if err != nil {
				return err
			}
			return fmt.Errorf("command path is a directory")
		}
	}
	workspace.mutex.Lock()
	if workspace.isTornDown {
		workspace.mutex.Unlock()
		workspace.EnqueueControlFrame(OutboundFrame{
			Action:     ActionSpawnStatus,
			TerminalID: terminalID,
			Payload:    []byte{0x01},
		})
		return errors.New("workspace is torn down")
	}
	if _, exists := workspace.ptys.Get(terminalID); exists {
		workspace.mutex.Unlock()
		workspace.EnqueueControlFrame(OutboundFrame{
			Action:     ActionSpawnStatus,
			TerminalID: terminalID,
			Payload:    []byte{0x01},
		})
		return fmt.Errorf("terminal %d already exists", terminalID)
	}
	if _, exists := workspace.spawning[terminalID]; exists {
		workspace.mutex.Unlock()
		workspace.EnqueueControlFrame(OutboundFrame{
			Action:     ActionSpawnStatus,
			TerminalID: terminalID,
			Payload:    []byte{0x01},
		})
		return fmt.Errorf("terminal %d is spawning", terminalID)
	}
	ctx, cancel := context.WithCancel(context.Background())
	doneChan := make(chan struct{})
	inst := &spawningInstance{cancel: cancel, done: doneChan}
	workspace.spawning[terminalID] = inst
	workspace.mutex.Unlock()
	workspace.EnqueueControlFrame(OutboundFrame{
		Action:     ActionSpawnStatus,
		TerminalID: terminalID,
		Payload:    []byte{0x02}, // Spawning (0x02)
	})
	go func() {
		defer close(doneChan)

		var cmd *exec.Cmd
		var master *os.File
		var slave *os.File
		var wasRegistered bool
		var isFailed bool

		defer func() {
			if !wasRegistered {
				// 1. If it failed, register a terminated dummy PTY so its exit status is queryable
				if isFailed {
					workspace.mutex.Lock()
					if _, exists := workspace.ptys.Get(terminalID); !exists {
						failedPTY := &ptyInstance{
							terminalID: terminalID,
							state:      StateTerminated,
							exitStatus: 255,
							waitDone:   make(chan struct{}),
							readDone:   make(chan struct{}),
						}
						close(failedPTY.waitDone)
						close(failedPTY.readDone)
						workspace.ptys.Put(terminalID, failedPTY)
						workspace.backpressureCond.Broadcast()
					}
					workspace.mutex.Unlock()
				}

				// 2. Clean up spawning maps
				workspace.mutex.Lock()
				wasSpawning := (workspace.spawning[terminalID] == inst)
				if wasSpawning {
					delete(workspace.spawning, terminalID)
					delete(workspace.pendingPriorities, terminalID)
				}
				workspace.backpressureCond.Broadcast()
				workspace.mutex.Unlock()

				// 3. Clean up OS processes
				if cmd != nil && cmd.Process != nil {
					killDescendants(cmd.Process.Pid)
					_ = syscall.Kill(cmd.Process.Pid, syscall.SIGKILL)
					_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
					_ = cmd.Wait()
				}

				// 4. Close PTY file descriptors
				if slave != nil {
					_ = slave.Close()
				}
				if master != nil {
					_ = master.Close()
				}

				// 5. Enqueue control frames
				if wasSpawning {
					status := byte(0x03) // Canceled (0x03)
					if isFailed {
						status = 0x01 // Failure (0x01)
					}
					workspace.EnqueueControlFrame(OutboundFrame{
						Action:     ActionSpawnStatus,
						TerminalID: terminalID,
						Payload:    []byte{status},
					})
					if status == 0x01 {
						workspace.EnqueueControlFrame(OutboundFrame{
							Action:     ActionTerminalExit,
							TerminalID: terminalID,
							Payload:    []byte{255},
						})
					}
				}
			}
		}()

		var err error
		master, slave, err = openPty()
		if err != nil {
			isFailed = true
			return
		}

		if ctx.Err() != nil {
			return
		}

		if err := setSize(int(slave.Fd()), columns, rows); err != nil {
			isFailed = true
			return
		}

		shell := resolvedPath
		shellArgs := args
		cmd = exec.Command(shell, shellArgs...)
		cmd.Stdin = slave
		cmd.Stdout = slave
		cmd.Stderr = slave
		cmd.SysProcAttr = &syscall.SysProcAttr{
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
		cmd.Env = env

		if ctx.Err() != nil {
			return
		}

		if err := cmd.Start(); err != nil {
			isFailed = true
			return
		}

		// Slave side is closed after start.
		_ = slave.Close()
		slave = nil

		if ctx.Err() != nil {
			return
		}

		workspace.mutex.Lock()
		if ctx.Err() != nil {
			workspace.mutex.Unlock()
			return
		}

		if workspace.spawning[terminalID] == inst {
			delete(workspace.spawning, terminalID)
		}

		if workspace.isTornDown {
			workspace.mutex.Unlock()
			return
		}

		priority := PriorityLow
		if p, ok := workspace.pendingPriorities[terminalID]; ok {
			priority = p
			delete(workspace.pendingPriorities, terminalID)
		}

		terminal := &ptyInstance{
			terminalID: terminalID,
			master:     master,
			command:    cmd,
			buffer:     newRingBuffer(256 * 1024),
			priority:   priority,
			state:      StateActive,
			waitDone:   make(chan struct{}),
			readDone:   make(chan struct{}),
		}
		workspace.ptys.Put(terminalID, terminal)
		workspace.pacingDeadline = TimeNow().Add(PacingInterval)
		workspace.waitGroup.Add(1)
		workspace.backpressureCond.Broadcast()
		workspace.mutex.Unlock()

		wasRegistered = true

		go func() {
			defer func() {
				close(terminal.waitDone)
				workspace.handleProcessExit(terminalID)
			}()
			_ = cmd.Wait()
			killDescendants(cmd.Process.Pid)
			_ = syscall.Kill(cmd.Process.Pid, syscall.SIGKILL)
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			var status byte = 0
			if cmd.ProcessState != nil {
				if waitStatus, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
					status = byte(waitStatus.ExitStatus())
					if waitStatus.Signaled() {
						status = byte(128 + waitStatus.Signal())
					}
				} else {
					status = byte(cmd.ProcessState.ExitCode())
				}
			}
			terminal.mutex.Lock()
			terminal.exitStatus = status
			terminal.state = StateTerminated
			terminal.mutex.Unlock()
			select {
			case <-terminal.readDone:
			case <-time.After(20 * time.Millisecond):
			}
			_ = master.Close()
		}()
		go workspace.startReadLoop(terminal)
		workspace.EnqueueControlFrame(OutboundFrame{
			Action:     ActionSpawnStatus,
			TerminalID: terminalID,
			Payload:    []byte{0x00}, // Success (0x00)
		})
	}()
	return nil
}

func (workspace *Workspace) startReadLoop(terminal *ptyInstance) {
	defer close(terminal.readDone)
	buf := make([]byte, 32*1024)
	for {
		n, _ := terminal.master.Read(buf)
		if n <= 0 {
			break
		}
		terminal.mutex.Lock()
		terminal.buffer.Write(buf[:n])
		priority := terminal.priority
		terminal.mutex.Unlock()
		copied := make([]byte, n)
		copy(copied, buf[:n])
		workspace.EnqueueFrame(OutboundFrame{
			Action:           ActionOutput,
			TerminalID:       terminal.terminalID,
			Payload:          copied,
			DrainingPriority: priority,
		})
	}
}

func (workspace *Workspace) handleProcessExit(terminalID uint16) {
	defer workspace.waitGroup.Done()
	workspace.mutex.Lock()
	terminal, exists := workspace.ptys.Get(terminalID)
	workspace.mutex.Unlock()
	if !exists {
		return
	}
	<-terminal.readDone
	// Wait for the egress queue to flush to the socket
	workspace.mutex.Lock()
	for workspace.pendingCount[terminalID] > 0 && workspace.socketWriter != nil && !workspace.isTornDown {
		workspace.backpressureCond.Wait()
	}
	workspace.mutex.Unlock()
	terminal.mutex.Lock()
	exitStatus := terminal.exitStatus
	terminal.state = StateTerminated
	terminal.mutex.Unlock()
	workspace.EnqueueControlFrame(OutboundFrame{
		Action:     ActionTerminalExit,
		TerminalID: terminalID,
		Payload:    []byte{exitStatus},
	})
	workspace.mutex.Lock()
	_ = terminal.master.Close()
	workspace.mutex.Unlock()
}

func (workspace *Workspace) ResizePTY(terminalID uint16, columns uint16, rows uint16) error {
	if columns == 0 {
		columns = 1
	}
	if rows == 0 {
		rows = 1
	}
	workspace.mutex.Lock()
	if _, spawning := workspace.spawning[terminalID]; spawning {
		workspace.mutex.Unlock()
		return fmt.Errorf("terminal %d is not ready (spawning)", terminalID)
	}
	terminal, exists := workspace.ptys.Get(terminalID)
	workspace.mutex.Unlock()
	if !exists {
		return fmt.Errorf("terminal %d not found", terminalID)
	}
	terminal.mutex.Lock()
	defer terminal.mutex.Unlock()
	if terminal.state == StateTerminated {
		return nil
	}
	return setSize(int(terminal.master.Fd()), columns, rows)
}

func (workspace *Workspace) WritePTYInput(terminalID uint16, data []byte) error {
	workspace.mutex.Lock()
	if _, spawning := workspace.spawning[terminalID]; spawning {
		workspace.mutex.Unlock()
		return fmt.Errorf("terminal %d is not ready (spawning)", terminalID)
	}
	terminal, exists := workspace.ptys.Get(terminalID)
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
	if terminal.state == StateTerminated {
		return fmt.Errorf("terminal %d is terminated", terminalID)
	}
	if len(data) == 0 {
		return nil
	}
	_, err := terminal.master.Write(data)
	return err
}

// GetScrollbackBuffer retrieves the current scrollback buffer and its truncation status.
func (workspace *Workspace) GetScrollbackBuffer(terminalID uint16) (buffer []byte, isTruncated bool, error error) {
	workspace.mutex.Lock()
	terminal, exists := workspace.ptys.Get(terminalID)
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
	for pty := workspace.ptys.head; pty != nil; pty = pty.next {
		pty.mutex.Lock()
		active := (pty.state == StateActive)
		pty.mutex.Unlock()
		if active {
			ids = append(ids, pty.terminalID)
		}
	}
	return ids
}

// GetPTYPriority retrieves the priority state of an active terminal.
func (workspace *Workspace) GetPTYPriority(terminalID uint16) (byte, error) {
	workspace.mutex.Lock()
	terminal, exists := workspace.ptys.Get(terminalID)
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
	terminal, exists := workspace.ptys.Get(terminalID)
	if !exists {
		if _, spawning := workspace.spawning[terminalID]; spawning {
			workspace.pendingPriorities[terminalID] = priority
			return fmt.Errorf("terminal %d is not ready (spawning)", terminalID)
		}
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
				if queueIndex == QueueIndexLow && workspace.pendingReplays > 0 {
					workspace.pendingReplays--
				}
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
	terminal, exists := workspace.ptys.Get(frame.TerminalID)
	if !exists {
		return
	}
	queueIndex := QueueIndexLow
	if frame.DrainingPriority == PriorityHigh {
		queueIndex = QueueIndexHigh
	}
	if workspace.pendingCount[frame.TerminalID] >= 1024 {
		if queueIndex == QueueIndexLow {
			workspace.dropOldestFrame(frame.TerminalID)
		} else {
			for {
				terminal.mutex.Lock()
				p := terminal.priority
				isTerminated := (terminal.state == StateTerminated)
				terminal.mutex.Unlock()
				if !(workspace.pendingCount[frame.TerminalID] >= 1024 && !workspace.isTornDown && !isTerminated && workspace.socketWriter != nil && p == PriorityHigh) {
					break
				}
				workspace.backpressureCond.Wait()
			}
			terminal.mutex.Lock()
			isTerminated := (terminal.state == StateTerminated)
			terminal.mutex.Unlock()
			if workspace.isTornDown || isTerminated {
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
	if workspace.pendingReplays > 0 {
		queue := workspace.centralizedQueues[QueueIndexLow]
		if len(queue) > 0 {
			return queue[0], QueueIndexLow, workspace.queueGeneration, true
		}
		return OutboundFrame{}, -1, 0, false
	}
	for queueIndex := QueueIndexControl; queueIndex <= QueueIndexLow; queueIndex++ {
		queue := workspace.centralizedQueues[queueIndex]
		if len(queue) > 0 {
			return queue[0], queueIndex, workspace.queueGeneration, true
		}
	}
	return OutboundFrame{}, -1, 0, false
}

func (workspace *Workspace) popNextFrame(queueIndex int, gen uint64) {
	if gen != workspace.queueGeneration {
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
	workspace.queueGeneration++
	workspace.centralizedQueues[QueueIndexHigh] = nil
	workspace.centralizedQueues[QueueIndexLow] = nil
	workspace.centralizedQueues[QueueIndexControl] = nil
	workspace.pendingCount = make(map[uint16]int)
	workspace.pacingDeadline = time.Time{}
	workspace.pendingReplays = len(replays)
	for _, frame := range replays {
		workspace.centralizedQueues[QueueIndexLow] = append(workspace.centralizedQueues[QueueIndexLow], frame)
		workspace.pendingCount[frame.TerminalID]++
	}
	workspace.backpressureCond.Broadcast()
	workspace.notifyScheduler()
}

func (workspace *Workspace) hasActivePTYs() bool {
	for pty := workspace.ptys.head; pty != nil; pty = pty.next {
		pty.mutex.Lock()
		active := (pty.state == StateActive)
		pty.mutex.Unlock()
		if active {
			return true
		}
	}
	return false
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
		frame, queueIndex, gen, ok := workspace.peekNextFrame()
		if !ok {
			workspace.mutex.Unlock()
			continue
		}
		if workspace.pendingReplays == 0 &&
			queueIndex == QueueIndexLow &&
			frame.Action == ActionOutput &&
			TimeNow().Before(workspace.pacingDeadline) &&
			workspace.hasActivePTYs() {
			sleepDuration := workspace.pacingDeadline.Sub(TimeNow())
			if sleepDuration < 0 {
				sleepDuration = -sleepDuration
			}
			workspace.mutex.Unlock()
			select {
			case <-workspace.schedulerSignal:
			case <-time.After(sleepDuration):
			}
			continue
		}
		writer := workspace.socketWriter
		workspace.mutex.Unlock()
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
		workspace.popNextFrame(queueIndex, gen)
		workspace.mutex.Unlock()
	}
}

func (workspace *Workspace) teardown() {
	workspace.mutex.Lock()
	workspace.isTornDown = true
	workspace.backpressureCond.Broadcast()
	for terminalID, inst := range workspace.spawning {
		inst.isRemoved = true
		inst.cancel()
		delete(workspace.spawning, terminalID)
	}
	if workspace.ptys != nil {
		for pty := workspace.ptys.head; pty != nil; pty = pty.next {
			pty.mutex.Lock()
			isTerminated := (pty.state == StateTerminated)
			pty.state = StateTerminated
			pty.mutex.Unlock()
			if !isTerminated {
				if pty.command != nil && pty.command.Process != nil {
					processID := pty.command.Process.Pid
					killDescendants(processID)
					_ = syscall.Kill(processID, syscall.SIGKILL)
					_ = syscall.Kill(-processID, syscall.SIGKILL)
					var processGroupID int32
					if pty.master != nil {
						ioctlError := ioctl(int(pty.master.Fd()), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&processGroupID)))
						if ioctlError == nil && processGroupID > 0 {
							_ = syscall.Kill(int(-processGroupID), syscall.SIGKILL)
						}
					}
				}
			}
			if pty.master != nil {
				_ = pty.master.Close()
			}
		}
	}
	workspace.mutex.Unlock()
	workspace.notifyScheduler()
	workspace.waitGroup.Wait()
}

func (workspace *Workspace) TerminatePTY(terminalID uint16) error {
	workspace.mutex.Lock()
	if inst, spawning := workspace.spawning[terminalID]; spawning {
		inst.isRemoved = true
		inst.cancel()
		delete(workspace.spawning, terminalID)
		delete(workspace.pendingPriorities, terminalID)
		workspace.backpressureCond.Broadcast()
		workspace.mutex.Unlock()
		workspace.EnqueueControlFrame(OutboundFrame{
			Action:     ActionSpawnStatus,
			TerminalID: terminalID,
			Payload:    []byte{0x03}, // Canceled (0x03)
		})
		return nil
	}
	terminal, exists := workspace.ptys.Get(terminalID)
	workspace.backpressureCond.Broadcast()
	workspace.mutex.Unlock()
	if !exists {
		return fmt.Errorf("terminal %d not found", terminalID)
	}
	terminal.mutex.Lock()
	isTerminated := (terminal.state == StateTerminated)
	terminal.state = StateTerminated
	terminal.mutex.Unlock()
	if !isTerminated {
		if terminal.command != nil && terminal.command.Process != nil {
			processID := terminal.command.Process.Pid
			killDescendants(processID)
			_ = syscall.Kill(processID, syscall.SIGKILL)
			_ = syscall.Kill(-processID, syscall.SIGKILL)
			var processGroupID int32
			ioctlError := ioctl(int(terminal.master.Fd()), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&processGroupID)))
			if ioctlError == nil && processGroupID > 0 {
				_ = syscall.Kill(int(-processGroupID), syscall.SIGKILL)
			}
		}
	}
	_ = terminal.master.Close()
	return nil
}

func (workspace *Workspace) GetPTYState(terminalID uint16) (TerminalState, bool) {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	t, exists := workspace.ptys.Get(terminalID)
	if !exists {
		if _, spawning := workspace.spawning[terminalID]; spawning {
			return StateSpawning, true
		}
		return StateTerminated, false
	}
	t.mutex.Lock()
	state := t.state
	t.mutex.Unlock()
	return state, true
}

func (workspace *Workspace) GetPTYExitStatus(terminalID uint16) (byte, error) {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	t, exists := workspace.ptys.Get(terminalID)
	if !exists {
		return 0, fmt.Errorf("terminal %d not found", terminalID)
	}
	t.mutex.Lock()
	exitStatus := t.exitStatus
	t.mutex.Unlock()
	return exitStatus, nil
}

func (workspace *Workspace) RemovePTY(terminalID uint16) error {
	workspace.mutex.Lock()
	if inst, spawning := workspace.spawning[terminalID]; spawning {
		inst.isRemoved = true
		inst.cancel()
		delete(workspace.spawning, terminalID)
		delete(workspace.pendingPriorities, terminalID)
		workspace.backpressureCond.Broadcast()
		workspace.mutex.Unlock()
		workspace.EnqueueControlFrame(OutboundFrame{
			Action:     ActionSpawnStatus,
			TerminalID: terminalID,
			Payload:    []byte{0x03}, // Canceled (0x03)
		})
		return nil
	}
	terminal, exists := workspace.ptys.Get(terminalID)
	if !exists {
		workspace.mutex.Unlock()
		return fmt.Errorf("terminal %d not found", terminalID)
	}
	workspace.ptys.Delete(terminalID)
	delete(workspace.pendingCount, terminalID)
	workspace.backpressureCond.Broadcast()
	workspace.mutex.Unlock()
	terminal.mutex.Lock()
	isTerminated := (terminal.state == StateTerminated)
	terminal.state = StateTerminated
	terminal.mutex.Unlock()
	if !isTerminated {
		if terminal.command != nil && terminal.command.Process != nil {
			processID := terminal.command.Process.Pid
			killDescendants(processID)
			_ = syscall.Kill(processID, syscall.SIGKILL)
			_ = syscall.Kill(-processID, syscall.SIGKILL)
			var processGroupID int32
			ioctlError := ioctl(int(terminal.master.Fd()), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&processGroupID)))
			if ioctlError == nil && processGroupID > 0 {
				_ = syscall.Kill(int(-processGroupID), syscall.SIGKILL)
			}
		}
	}
	if terminal.master != nil {
		_ = terminal.master.Close()
	}
	return nil
}

func (workspace *Workspace) ResetWorkspace() error {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()

	// 1. Gather all spawning terminals to cancel
	var spawningIDs []uint16
	for terminalID, inst := range workspace.spawning {
		inst.isRemoved = true
		inst.cancel()
		spawningIDs = append(spawningIDs, terminalID)
	}
	workspace.spawning = make(map[uint16]*spawningInstance)

	// 2. Tear down active PTYs
	if workspace.ptys != nil {
		for pty := workspace.ptys.head; pty != nil; pty = pty.next {
			pty.mutex.Lock()
			isTerminated := (pty.state == StateTerminated)
			pty.state = StateTerminated
			pty.mutex.Unlock()
			if !isTerminated {
				if pty.command != nil && pty.command.Process != nil {
					processID := pty.command.Process.Pid
					killDescendants(processID)
					_ = syscall.Kill(processID, syscall.SIGKILL)
					_ = syscall.Kill(-processID, syscall.SIGKILL)
					var processGroupID int32
					if pty.master != nil {
						ioctlError := ioctl(int(pty.master.Fd()), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&processGroupID)))
						if ioctlError == nil && processGroupID > 0 {
							_ = syscall.Kill(int(-processGroupID), syscall.SIGKILL)
						}
					}
				}
			}
			if pty.master != nil {
				_ = pty.master.Close()
			}
		}
	}

	// 3. Clear/Reinitialize workspace structures
	workspace.ptys = newOrderedPTYMap()
	workspace.queueGeneration++
	workspace.centralizedQueues[0] = nil
	workspace.centralizedQueues[1] = nil
	workspace.centralizedQueues[2] = nil
	workspace.pendingCount = make(map[uint16]int)
	workspace.pendingPriorities = make(map[uint16]byte)
	workspace.pendingReplays = 0

	// 4. Enqueue Canceled status codes for cancelled spawning PTYs
	for _, terminalID := range spawningIDs {
		workspace.centralizedQueues[QueueIndexControl] = append(workspace.centralizedQueues[QueueIndexControl], OutboundFrame{
			Action:     ActionSpawnStatus,
			TerminalID: terminalID,
			Payload:    []byte{0x03}, // Canceled (0x03)
		})
	}

	// 5. Enqueue global Reset frame
	workspace.centralizedQueues[QueueIndexControl] = append(workspace.centralizedQueues[QueueIndexControl], OutboundFrame{
		Action:     ActionReset,
		TerminalID: 0,
		Payload:    nil,
	})

	workspace.backpressureCond.Broadcast()
	workspace.notifyScheduler()
	return nil
}

func AlignUTF8Boundary(data []byte) []byte {
	skipped := 0
	for skipped < len(data) && skipped < 3 {
		b := data[skipped]
		if (b & 0xC0) == 0x80 {
			skipped++
		} else {
			break
		}
	}
	return data[skipped:]
}

// SyncPTYPriorities updates the priority schedule for specified active terminals
// and implicitly demotes any active terminals omitted from the map.
func (workspace *Workspace) SyncPTYPriorities(priorities map[uint16]byte) error {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	var modified bool
	for pty := workspace.ptys.head; pty != nil; pty = pty.next {
		pty.mutex.Lock()
		active := (pty.state == StateActive)
		pty.mutex.Unlock()
		if !active {
			continue
		}
		tid := pty.terminalID
		newPriority := PriorityLow
		if state, specified := priorities[tid]; specified {
			newPriority = state
		}
		pty.mutex.Lock()
		if pty.priority != newPriority {
			pty.priority = newPriority
			modified = true
		}
		pty.mutex.Unlock()
	}

	// Synchronize pending priorities for spawning PTYs
	for tid := range workspace.spawning {
		if priority, specified := priorities[tid]; specified {
			workspace.pendingPriorities[tid] = priority
		} else {
			delete(workspace.pendingPriorities, tid)
		}
	}

	if modified {
		workspace.backpressureCond.Broadcast()
		workspace.pacingDeadline = TimeNow().Add(PacingInterval)
		workspace.notifyScheduler()
	}
	return nil
}

// CompileReplayFrames compiles the current state of all active and terminated PTYs
// into a slice of OutboundFrames to be replayed upon client connection.
func (workspace *Workspace) CompileReplayFrames() []OutboundFrame {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	return workspace.compileReplayFramesLocked()
}

func (workspace *Workspace) compileReplayFramesLocked() []OutboundFrame {
	var replayFrames []OutboundFrame

	// 1. Replay Spawning status frames for all currently spawning PTYs
	for id := range workspace.spawning {
		replayFrames = append(replayFrames, OutboundFrame{
			Action:           ActionSpawnStatus,
			TerminalID:       id,
			Payload:          []byte{0x02}, // Spawning (0x02)
			DrainingPriority: PriorityLow,
		})
	}

	// 2. Replay active and terminated PTY outputs and trailing statuses
	for pty := workspace.ptys.head; pty != nil; pty = pty.next {
		pty.mutex.Lock()
		state := pty.state
		exitStatus := pty.exitStatus
		buf := pty.buffer.Bytes()
		isTruncated := pty.buffer.isTruncated
		pty.mutex.Unlock()
		id := pty.terminalID

		if state == StateActive || state == StateTerminated {
			buf = AlignUTF8Boundary(buf)
			if len(buf) > 0 || isTruncated {
				replayPayload := buf
				if isTruncated {
					warning := []byte("\r\n\x1b[33m[... Output truncated due to buffer overflow ...]\x1b[0m\r\n\r\n")
					replayPayload = make([]byte, len(warning)+len(buf))
					copy(replayPayload, warning)
					copy(replayPayload[len(warning):], buf)
				}
				replayFrames = append(replayFrames, OutboundFrame{
					Action:           ActionOutput,
					TerminalID:       id,
					Payload:          replayPayload,
					DrainingPriority: PriorityLow,
				})
			}
		}

		if state == StateActive {
			replayFrames = append(replayFrames, OutboundFrame{
				Action:           ActionSpawnStatus,
				TerminalID:       id,
				Payload:          []byte{0x00}, // Success (0x00)
				DrainingPriority: PriorityLow,
			})
		} else if state == StateTerminated {
			replayFrames = append(replayFrames, OutboundFrame{
				Action:           ActionTerminalExit,
				TerminalID:       id,
				Payload:          []byte{exitStatus},
				DrainingPriority: PriorityLow,
			})
		}
	}
	return replayFrames
}

// PerformReplayTakeover compiles the state replays, flushes the active queues,
// and binds the new WebSocket writer atomically under a single lock session.
func (workspace *Workspace) PerformReplayTakeover(socketWriter SocketWriter) {
	workspace.mutex.Lock()
	defer workspace.mutex.Unlock()
	replays := workspace.compileReplayFramesLocked()
	workspace.queueGeneration++
	workspace.centralizedQueues[QueueIndexHigh] = nil
	workspace.centralizedQueues[QueueIndexLow] = nil
	workspace.centralizedQueues[QueueIndexControl] = nil
	workspace.pendingCount = make(map[uint16]int)
	workspace.pacingDeadline = time.Time{}
	workspace.pendingReplays = len(replays)
	for _, frame := range replays {
		workspace.centralizedQueues[QueueIndexLow] = append(workspace.centralizedQueues[QueueIndexLow], frame)
		workspace.pendingCount[frame.TerminalID]++
	}
	workspace.socketWriter = socketWriter
	workspace.backpressureCond.Broadcast()
	workspace.notifyScheduler()
}

type orderedPTYMap struct {
	head   *ptyInstance
	tail   *ptyInstance
	values map[uint16]*ptyInstance
}

func newOrderedPTYMap() *orderedPTYMap {
	return &orderedPTYMap{
		values: make(map[uint16]*ptyInstance),
	}
}

func (m *orderedPTYMap) Get(key uint16) (*ptyInstance, bool) {
	val, exists := m.values[key]
	return val, exists
}

func (m *orderedPTYMap) Put(key uint16, val *ptyInstance) {
	if _, exists := m.values[key]; exists {
		m.Delete(key)
	}
	m.values[key] = val
	if m.head == nil {
		m.head = val
		m.tail = val
	} else {
		m.tail.next = val
		val.prev = m.tail
		m.tail = val
	}
}

func (m *orderedPTYMap) Delete(key uint16) {
	val, exists := m.values[key]
	if !exists {
		return
	}
	delete(m.values, key)
	if val.prev != nil {
		val.prev.next = val.next
	} else {
		m.head = val.next
	}
	if val.next != nil {
		val.next.prev = val.prev
	} else {
		m.tail = val.prev
	}
	val.next = nil
	val.prev = nil
}
