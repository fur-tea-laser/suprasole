package source

import (
	"sync"
	"testing"
	"time"
)

// TestCase11: Popped/Dropped Slice GC Zero-Out
func TestGCZeroOut(t *testing.T) {
	ws := &Workspace{
		pendingCount: make(map[uint16]int),
	}
	ws.backpressureCond = sync.NewCond(&ws.mutex)
	// Enqueue a frame with a large payload
	frame := OutboundFrame{
		Action:           0x0005,
		TerminalID:       1,
		Payload:          []byte("large_payload_garbage_collection_test"),
		DrainingPriority: PriorityHigh,
	}
	ws.centralizedQueues[QueueIndexHigh] = append(ws.centralizedQueues[QueueIndexHigh], frame)
	ws.pendingCount[1]++
	// Verify it is present
	if len(ws.centralizedQueues[QueueIndexHigh]) != 1 {
		t.Fatalf("expected 1 frame in high queue")
	}
	// Pop the frame
	ws.popNextFrame(QueueIndexHigh, ws.queueGeneration)
	// Assert that index 0 of the backing array is now empty (GC zeroed out)
	// Under our shift-left pop, index len(queue)-1 of the original slice is zeroed out.
	// Since we truncated the slice to len 0, we check the underlying slice array cap.
	underlyingSlice := ws.centralizedQueues[QueueIndexHigh][:1] // slice up to capacity 1
	f0 := underlyingSlice[0]
	if f0.Action != 0 || f0.TerminalID != 0 || len(f0.Payload) != 0 {
		t.Errorf("expected popped frame slot to be zeroed out to release GC reference, got %v", underlyingSlice[0])
	}
}

// TestCase24: isEmpty() Queue State Verification
func TestIsEmpty(t *testing.T) {
	ws := &Workspace{}
	if !ws.isEmpty() {
		t.Errorf("expected new workspace to be empty")
	}
	ws.centralizedQueues[QueueIndexLow] = append(ws.centralizedQueues[QueueIndexLow], OutboundFrame{TerminalID: 1})
	if ws.isEmpty() {
		t.Errorf("expected workspace with enqueued frame to not be empty")
	}
}

// TestCase25: dropOldestFrame() Search Order
func TestDropOldestSearchOrder(t *testing.T) {
	ws := &Workspace{
		pendingCount: make(map[uint16]int),
	}
	// Add to High queue
	ws.centralizedQueues[QueueIndexHigh] = append(ws.centralizedQueues[QueueIndexHigh], OutboundFrame{
		TerminalID: 1,
		Payload:    []byte("HIGH_1"),
	})
	ws.pendingCount[1]++
	// Add to Low queue
	ws.centralizedQueues[QueueIndexLow] = append(ws.centralizedQueues[QueueIndexLow], OutboundFrame{
		TerminalID: 1,
		Payload:    []byte("LOW_1"),
	})
	ws.pendingCount[1]++
	// Drop oldest frame for terminal 1
	dropped := ws.dropOldestFrame(1)
	if !dropped {
		t.Fatalf("expected frame to be dropped")
	}
	// Low queue must be searched and dropped first
	if len(ws.centralizedQueues[QueueIndexLow]) != 0 {
		t.Errorf("expected Low queue frame to be dropped first")
	}
	if len(ws.centralizedQueues[QueueIndexHigh]) != 1 {
		t.Errorf("expected High queue frame to be preserved")
	}
}

// TestCase26: dropOldestFrame() Array Shifting and GC Clearing
func TestDropOldestShifting(t *testing.T) {
	ws := &Workspace{
		pendingCount: make(map[uint16]int),
	}
	// Enqueue 3 frames
	ws.centralizedQueues[QueueIndexLow] = append(ws.centralizedQueues[QueueIndexLow],
		OutboundFrame{TerminalID: 1, Payload: []byte("F1")},
		OutboundFrame{TerminalID: 1, Payload: []byte("F2")},
		OutboundFrame{TerminalID: 1, Payload: []byte("F3")},
	)
	ws.pendingCount[1] = 3
	// Drop oldest frame (F1 at index 0)
	ws.dropOldestFrame(1)
	// Remaining elements must shift left
	q := ws.centralizedQueues[QueueIndexLow]
	if len(q) != 2 {
		t.Fatalf("expected length to be 2, got %d", len(q))
	}
	if string(q[0].Payload) != "F2" || string(q[1].Payload) != "F3" {
		t.Errorf("expected shifting left: F2, F3. Got %s, %s", q[0].Payload, q[1].Payload)
	}
	// Verify trailing element in backing array is zeroed out for GC
	underlying := q[:3]
	f2 := underlying[2]
	if f2.Action != 0 || f2.TerminalID != 0 || len(f2.Payload) != 0 {
		t.Errorf("expected trailing backing array slot to be zeroed out for GC, got %v", underlying[2])
	}
}

// TestCase41: peekNextFrame() Priority Scanning Order
func TestPeekNextScanningOrder(t *testing.T) {
	ws := &Workspace{}
	ws.centralizedQueues[QueueIndexLow] = append(ws.centralizedQueues[QueueIndexLow], OutboundFrame{Payload: []byte("LOW")})
	ws.centralizedQueues[QueueIndexHigh] = append(ws.centralizedQueues[QueueIndexHigh], OutboundFrame{Payload: []byte("HIGH")})
	ws.centralizedQueues[QueueIndexControl] = append(ws.centralizedQueues[QueueIndexControl], OutboundFrame{Payload: []byte("CTRL")})
	// Peek next frame: Control should take absolute priority
	frame, qIdx, gen, ok := ws.peekNextFrame()
	if !ok || qIdx != QueueIndexControl || string(frame.Payload) != "CTRL" {
		t.Errorf("expected Control frame to be peeked first, got %v, qIdx=%d", frame, qIdx)
	}
	// Pop control
	ws.popNextFrame(QueueIndexControl, gen)
	// High priority should take priority over Low
	frame, qIdx, gen, ok = ws.peekNextFrame()
	if !ok || qIdx != QueueIndexHigh || string(frame.Payload) != "HIGH" {
		t.Errorf("expected High priority frame to be peeked next, got %v, qIdx=%d", frame, qIdx)
	}
}

// TestCase42: popNextFrame() Slice Shifting and Offset Truncation
func TestPopNextShifting(t *testing.T) {
	ws := &Workspace{
		pendingCount: make(map[uint16]int),
	}
	ws.backpressureCond = sync.NewCond(&ws.mutex)
	ws.centralizedQueues[QueueIndexHigh] = append(ws.centralizedQueues[QueueIndexHigh],
		OutboundFrame{TerminalID: 1, Payload: []byte("H1")},
		OutboundFrame{TerminalID: 1, Payload: []byte("H2")},
	)
	ws.pendingCount[1] = 2
	ws.popNextFrame(QueueIndexHigh, ws.queueGeneration)
	q := ws.centralizedQueues[QueueIndexHigh]
	if len(q) != 1 {
		t.Fatalf("expected length 1")
	}
	if string(q[0].Payload) != "H2" {
		t.Errorf("expected element shifting: H2, got %s", q[0].Payload)
	}
	// Verify trailing element in backing array is zeroed out for GC
	underlying := q[:2]
	f1 := underlying[1]
	if f1.Action != 0 || f1.TerminalID != 0 || len(f1.Payload) != 0 {
		t.Errorf("expected trailing slot to be zeroed out for GC, got %v", underlying[1])
	}
}

// TestCase62: Steady-State Zero-Allocation Slice Shifting
func TestZeroAllocationShifting(t *testing.T) {
	ws := &Workspace{
		pendingCount: make(map[uint16]int),
	}
	ws.backpressureCond = sync.NewCond(&ws.mutex)
	// Pre-allocate queue capacity to 1024 to simulate steady-state capacity saturation
	ws.centralizedQueues[QueueIndexHigh] = make([]OutboundFrame, 0, 1024)
	// Measure allocations of popping and pushing in steady state
	allocs := testing.AllocsPerRun(100, func() {
		// Push frame
		ws.centralizedQueues[QueueIndexHigh] = append(ws.centralizedQueues[QueueIndexHigh], OutboundFrame{TerminalID: 1})
		// Pop frame
		workspace := ws // use local variable for clarity
		workspace.popNextFrame(QueueIndexHigh, workspace.queueGeneration)
	})
	if allocs > 0 {
		t.Errorf("expected 0 heap allocations during steady-state pop/push cycles, got %f", allocs)
	}
}

// TestSchedulerPacingDeadlineFlow verifies that pacing pauses low-priority draining under active deadline transients.
func TestSchedulerPacingDeadlineFlow(t *testing.T) {
	// Mock TimeNow for deterministic clock control
	var mockTime = time.Now()
	TimeNow = func() time.Time {
		return mockTime
	}
	defer func() {
		TimeNow = time.Now
	}()
	ws := &Workspace{
		pendingCount:    make(map[uint16]int),
		schedulerSignal: make(chan struct{}, 10),
		ptys:            newOrderedPTYMap(),
		spawningPTYs:    make(map[uint16]bool),
		terminatedPTYs:  make(map[uint16]bool),
	}
	ws.backpressureCond = sync.NewCond(&ws.mutex)
	ws.ptys.Put(1, &ptyInstance{terminalID: 1, priority: PriorityLow})
	ws.ptys.Put(2, &ptyInstance{terminalID: 2, priority: PriorityHigh})
	// Register a mock socket writer that records writes
	var written []OutboundFrame
	var writtenMutex sync.Mutex
	mockWriter := &mockSocketWriter{
		writeFunc: func(action uint16, terminalID uint16, payload []byte) error {
			writtenMutex.Lock()
			defer writtenMutex.Unlock()
			written = append(written, OutboundFrame{Action: action, TerminalID: terminalID, Payload: payload})
			return nil
		},
	}
	ws.socketWriter = mockWriter
	// Start scheduler thread
	go ws.startScheduler()
	// 1. Initial State: Enqueue a Low priority frame. It should drain immediately since pacing deadline is not set.
	ws.EnqueueFrame(OutboundFrame{Action: 8, TerminalID: 1, DrainingPriority: PriorityLow, Payload: []byte("L1")})
	time.Sleep(10 * time.Millisecond) // Give scheduler time to run
	writtenMutex.Lock()
	if len(written) != 1 || string(written[0].Payload) != "L1" {
		writtenMutex.Unlock()
		t.Fatalf("expected Low priority frame to drain immediately in initial state, got %v", written)
	}
	written = nil // Reset
	writtenMutex.Unlock()
	// 2. Set pacing deadline manually (representing a priority Sync shift transient)
	ws.mutex.Lock()
	ws.pacingDeadline = mockTime.Add(PacingInterval)
	ws.mutex.Unlock()
	// Enqueue a Low priority frame.
	ws.EnqueueFrame(OutboundFrame{Action: 8, TerminalID: 1, DrainingPriority: PriorityLow, Payload: []byte("L2")})
	time.Sleep(10 * time.Millisecond) // Give scheduler time to run
	// Verify that the frame is NOT drained because pacing deadline is active
	writtenMutex.Lock()
	if len(written) != 0 {
		writtenMutex.Unlock()
		t.Fatalf("expected Low priority draining to be paused while pacing deadline is active, got %v", written)
	}
	writtenMutex.Unlock()
	// 3. Enqueue a High priority frame while paced. It should drain immediately (preemption).
	ws.EnqueueFrame(OutboundFrame{Action: 8, TerminalID: 2, DrainingPriority: PriorityHigh, Payload: []byte("H1")})
	time.Sleep(10 * time.Millisecond) // Give scheduler time to run
	// Verify that H1 is drained, but L2 is still paused.
	writtenMutex.Lock()
	if len(written) != 1 || string(written[0].Payload) != "H1" {
		writtenMutex.Unlock()
		t.Fatalf("expected High priority frame to preempt and drain immediately, got %v", written)
	}
	written = nil // Reset
	writtenMutex.Unlock()
	// 4. Advance mock clock past the deadline
	mockTime = mockTime.Add(PacingInterval + 1*time.Millisecond)
	ws.notifyScheduler()              // Wake up pacing loop
	time.Sleep(10 * time.Millisecond) // Give scheduler time to run
	// Verify that L2 is now drained because pacing deadline expired
	writtenMutex.Lock()
	if len(written) != 1 || string(written[0].Payload) != "L2" {
		writtenMutex.Unlock()
		t.Fatalf("expected Low priority frame L2 to drain after pacing deadline expires, got %v", written)
	}
	written = nil
	writtenMutex.Unlock()
	// Clean up workspace
	ws.mutex.Lock()
	ws.ptys = nil
	ws.mutex.Unlock()
	ws.teardown()
}

type mockSocketWriter struct {
	writeFunc func(action uint16, terminalID uint16, payload []byte) error
}

func (m *mockSocketWriter) WriteFrame(action uint16, terminalID uint16, payload []byte) error {
	return m.writeFunc(action, terminalID, payload)
}
