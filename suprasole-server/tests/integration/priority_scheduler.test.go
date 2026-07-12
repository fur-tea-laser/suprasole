package gotests

import (
	"encoding/binary"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"suprasole-server/source"
)

// mockTestSocketWriter captures enqueued frames with robust leak protection.
type mockTestSocketWriter struct {
	mutex     sync.Mutex
	frames chan source.OutboundFrame
	done   chan struct{}
	once   sync.Once
	error    error
	block  chan struct{}
}

func (m *mockTestSocketWriter) WriteFrame(action uint16, terminalID uint16, payload []byte) error {
	m.mutex.Lock()
	error := m.error
	blockChan := m.block
	m.mutex.Unlock()

	if error != nil {
		return error
	}

	if blockChan != nil {
		select {
		case <-blockChan:
		case <-m.done:
			return fmt.Errorf("writer closed")
		}
	}

	select {
	case <-m.done:
		return fmt.Errorf("writer closed")
	default:
	}
	select {
	case m.frames <- source.OutboundFrame{Action: action, TerminalID: terminalID, Payload: payload}:
	case <-m.done:
		return fmt.Errorf("writer closed")
	default:
	}
	return nil
}

func (m *mockTestSocketWriter) Close() {
	m.once.Do(func() {
		close(m.done)
	})
}

func newMockTestSocketWriter() *mockTestSocketWriter {
	return &mockTestSocketWriter{
		frames: make(chan source.OutboundFrame, 20000),
		done:   make(chan struct{}),
	}
}

// 1. TestStarvationAndStrictPriorityDraining
func TestStarvationAndStrictPriorityDraining(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, error := registry.GetOrCreateWorkspace("starve-workspace")
	if error != nil {
		t.Fatalf("failed to create workspace: %v", error)
	}
	defer func() {
		_ = registry.RemoveWorkspace("starve-workspace")
	}()

	// Spawn T1 and T2
	if error := workspace.SpawnPTY(1, 80, 24); error != nil {
		t.Fatalf("failed to spawn PTY 1: %v", error)
	}
	if error := workspace.SpawnPTY(2, 80, 24); error != nil {
		t.Fatalf("failed to spawn PTY 2: %v", error)
	}

	// T1 = High Priority (0x01), T2 = Low Priority (0x00)
	_ = workspace.SetPTYPriority(1, 0x01)
	_ = workspace.SetPTYPriority(2, 0x00)

	// Programmatically enqueue 10 frames into T2 and 10 frames into T1
	for i := 0; i < 10; i++ {
		workspace.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       2,
			Payload:          []byte("LOW_VAL"),
			DrainingPriority: source.PriorityLow,
		})
		workspace.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("HIGH_VAL"),
			DrainingPriority: source.PriorityHigh,
		})
	}

	// Bind mock writer
	mockWriter := newMockTestSocketWriter()
	defer mockWriter.Close()
	workspace.SetSocketWriter(mockWriter)

	// Read first 10 frames; assert all are from T1 (strictly starved T2)
	deadline := time.Now().Add(2 * time.Second)
	for i := 0; i < 10; i++ {
		select {
		case frame := <-mockWriter.frames:
			if frame.Action == source.ActionStreamIO {
				if frame.TerminalID == 2 {
					t.Fatal("low priority terminal 2 output received during active high-priority flow (starvation failure)")
				}
			}
		case <-time.After(time.Until(deadline)):
			t.Fatal("timeout waiting for High priority frames")
		}
	}

	// Now read next 10 frames; they should be from T2
	for i := 0; i < 10; i++ {
		select {
		case frame := <-mockWriter.frames:
			if frame.Action == source.ActionStreamIO {
				if frame.TerminalID != 2 {
					t.Fatalf("expected low priority frame from terminal 2, got from terminal %d", frame.TerminalID)
				}
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for Low priority frames")
		}
	}
}

// 2. TestRoundRobinFairShareDraining
func TestRoundRobinFairShareDraining(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, error := registry.GetOrCreateWorkspace("rr-workspace")
	if error != nil {
		t.Fatalf("failed to create workspace: %v", error)
	}
	defer func() {
		_ = registry.RemoveWorkspace("rr-workspace")
	}()

	// Spawn three PTYs
	for i := uint16(1); i <= 3; i++ {
		if error := workspace.SpawnPTY(i, 80, 24); error != nil {
			t.Fatalf("failed to spawn PTY %d: %v", i, error)
		}
		_ = workspace.SetPTYPriority(i, 0x01) // All High Priority
	}

	// Programmatically enqueue 10 frames in each PTY
	for j := 0; j < 10; j++ {
		for i := uint16(1); i <= 3; i++ {
			workspace.EnqueueFrame(source.OutboundFrame{
				Action:           source.ActionStreamIO,
				TerminalID:       i,
				Payload:          []byte("STREAM"),
				DrainingPriority: source.PriorityHigh,
			})
		}
	}

	mockWriter := newMockTestSocketWriter()
	defer mockWriter.Close()
	workspace.SetSocketWriter(mockWriter)

	// Capture a slice of 30 frames and assert representation (no starvation)
	counts := make(map[uint16]int)
	deadline := time.Now().Add(2 * time.Second)
	for i := 0; i < 30; i++ {
		select {
		case frame := <-mockWriter.frames:
			if frame.Action == source.ActionStreamIO {
				counts[frame.TerminalID]++
			}
		case <-time.After(time.Until(deadline)):
			t.Fatal("timeout waiting for mixed stream frames")
		}
	}

	// Verify all three active High priority PTYs are represented in the stream
	for i := uint16(1); i <= 3; i++ {
		if counts[i] == 0 {
			t.Errorf("PTY %d was starved in the visible/High priority round-robin stream", i)
		}
	}
}

// 3. TestDifferentiatedBackpressure
func TestDifferentiatedBackpressure(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, error := registry.GetOrCreateWorkspace("backpressure-workspace")
	if error != nil {
		t.Fatalf("failed to create workspace: %v", error)
	}
	defer func() {
		_ = registry.RemoveWorkspace("backpressure-workspace")
	}()

	// Setup blocked writer
	blockedWriter := newMockTestSocketWriter()
	defer blockedWriter.Close()
	blockedWriter.block = make(chan struct{})
	workspace.SetSocketWriter(blockedWriter)

	// Setup PTY 1 (High) and PTY 2 (Low)
	if error := workspace.SpawnPTY(1, 80, 24); error != nil {
		t.Fatalf("spawn failed: %v", error)
	}
	_ = workspace.SetPTYPriority(1, 0x01)

	if error := workspace.SpawnPTY(2, 80, 24); error != nil {
		t.Fatalf("spawn failed: %v", error)
	}
	_ = workspace.SetPTYPriority(2, 0x00)

	// Test Low Priority (0x00) evicts (drop-oldest), does not block
	doneLow := make(chan struct{})
	go func() {
		for i := 0; i < 1050; i++ {
			workspace.EnqueueFrame(source.OutboundFrame{
				Action:           source.ActionStreamIO,
				TerminalID:       2,
				Payload:          []byte("LOW_VAL"),
				DrainingPriority: source.PriorityLow,
			})
		}
		close(doneLow)
	}()

	select {
	case <-doneLow:
		// Succeeded (did not block)
	case <-time.After(1 * time.Second):
		t.Fatal("Low priority terminal blocked under congestion (eviction policy failure)")
	}

	// Test High Priority (0x01) blocks
	doneHigh := make(chan struct{})
	go func() {
		for i := 0; i < 1024; i++ {
			workspace.EnqueueFrame(source.OutboundFrame{
				Action:           source.ActionStreamIO,
				TerminalID:       1,
				Payload:          []byte("HIGH_VAL"),
				DrainingPriority: source.PriorityHigh,
			})
		}
		// The 1025th frame should block
		workspace.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("HIGH_VAL_BLOCKING"),
			DrainingPriority: source.PriorityHigh,
		})
		close(doneHigh)
	}()

	select {
	case <-doneHigh:
		t.Fatal("expected High priority terminal to block on full queue under congestion, but writes completed without blocking")
	case <-time.After(500 * time.Millisecond):
		// High priority successfully blocked.
	}
}

// 4. TestOrphanedDropOldestFallback
func TestOrphanedDropOldestFallback(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, error := registry.GetOrCreateWorkspace("orphaned-workspace")
	if error != nil {
		t.Fatalf("failed to create workspace: %v", error)
	}
	defer func() {
		_ = registry.RemoveWorkspace("orphaned-workspace")
	}()

	if error := workspace.SpawnPTY(1, 80, 24); error != nil {
		t.Fatalf("spawn failed: %v", error)
	}
	_ = workspace.SetPTYPriority(1, 0x01) // High priority normally

	// Disconnect client WebSocket (SetSocketWriter to nil, simulating Orphaned state)
	workspace.SetSocketWriter(nil)

	// Flood output while offline
	doneOffline := make(chan struct{})
	go func() {
		for i := 0; i < 1050; i++ {
			workspace.EnqueueFrame(source.OutboundFrame{
				Action:           source.ActionStreamIO,
				TerminalID:       1,
				Payload:          []byte("OFFLINE_VAL"),
				DrainingPriority: source.PriorityHigh, // enqueued priority is High, but offline makes effective priority Low
			})
		}
		close(doneOffline)
	}()

	// Assert that offline High priority does not block (reverted to drop-oldest fallback)
	select {
	case <-doneOffline:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("High priority terminal blocked while offline (orphaned drop-oldest fallback failure)")
	}
}

// 5. TestSocketWriteErrorTransactionalHold
func TestSocketWriteErrorTransactionalHold(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, error := registry.GetOrCreateWorkspace("error-hold-workspace")
	if error != nil {
		t.Fatalf("failed to create workspace: %v", error)
	}
	defer func() {
		_ = registry.RemoveWorkspace("error-hold-workspace")
	}()

	if error := workspace.SpawnPTY(1, 80, 24); error != nil {
		t.Fatalf("spawn failed: %v", error)
	}
	_ = workspace.SetPTYPriority(1, 0x01)

	// Ingest a frame
	_ = workspace.WritePTYInput(1, []byte("echo 'TRANSACTIONAL_TARGET'\n"))

	// Wait for stdout to populate queue
	time.Sleep(50 * time.Millisecond)

	// Bind mock writer that fails on first write
	failingWriter := newMockTestSocketWriter()
	defer failingWriter.Close()
	failingWriter.error = fmt.Errorf("write error")
	workspace.SetSocketWriter(failingWriter)

	// Sleep briefly; the scheduler thread will attempt write, fail, and set writer to nil
	time.Sleep(50 * time.Millisecond)

	// Attach a new successful writer
	reconnectWriter := newMockTestSocketWriter()
	defer reconnectWriter.Close()
	workspace.SetSocketWriter(reconnectWriter)

	// Verify the frame was held on failure and successfully delivered to the new socket
	deadline := time.Now().Add(2 * time.Second)
	foundTarget := false
	for !foundTarget {
		select {
		case frame := <-reconnectWriter.frames:
			if frame.TerminalID == 1 && frame.Action == source.ActionStreamIO {
				if strings.Contains(string(frame.Payload), "TRANSACTIONAL_TARGET") {
					foundTarget = true
				}
			}
		case <-time.After(time.Until(deadline)):
			t.Fatal("timeout waiting for transactional hold recovery of failed write frame")
		}
	}
}

// 6. TestImplicitDemotionOnPrioritySync
func TestImplicitDemotionOnPrioritySync(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	token := "implicit-demote-workspace"
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=" + token

	connection, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to connect: %v", error)
	}
	defer func() {
		_ = connection.Close()
		_ = registry.RemoveWorkspace(token)
	}()

	// Spawn T1 and T2
	_ = connection.WriteMessage(websocket.BinaryMessage, packSpawnRequest(1, 80, 24))
	_, _, _ = connection.ReadMessage()
	_ = connection.WriteMessage(websocket.BinaryMessage, packSpawnRequest(2, 80, 24))
	_, _, _ = connection.ReadMessage()

	// Wait for shell outputs
	time.Sleep(100 * time.Millisecond)

	// Sync priorities: Set only PTY 1 to High (0x01), omitting PTY 2 entirely
	syncFrame := make([]byte, 7)
	binary.BigEndian.PutUint16(syncFrame[0:2], source.ActionPrioritySync)
	binary.BigEndian.PutUint16(syncFrame[2:4], 0)
	binary.BigEndian.PutUint16(syncFrame[4:6], 1)
	syncFrame[6] = 0x01

	_ = connection.WriteMessage(websocket.BinaryMessage, syncFrame)
	time.Sleep(50 * time.Millisecond)

	workspace, _ := registry.GetOrCreateWorkspace(token)

	// Get PTY 1 priority (must be High 0x01)
	priority1, _ := workspace.GetPTYPriority(1) // mapped to GetPTYPriority / SetPTYPriority internally
	if priority1 != 0x01 {
		t.Errorf("expected PTY 1 priority to be High 0x01, got %x", priority1)
	}

	// Get PTY 2 priority (must be Low 0x00 due to implicit demotion)
	priority2, _ := workspace.GetPTYPriority(2)
	if priority2 != 0x00 {
		t.Errorf("expected omitted PTY 2 priority to default/demote to Low 0x00, got %x", priority2)
	}
}

// 7. TestReconnectionReplayStarvationPrevention
func TestReconnectionReplayStarvationPrevention(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, error := registry.GetOrCreateWorkspace("replay-starve-workspace")
	if error != nil {
		t.Fatalf("failed to create workspace: %v", error)
	}
	defer func() {
		_ = registry.RemoveWorkspace("replay-starve-workspace")
	}()

	// Spawn T1 and T2
	_ = workspace.SpawnPTY(1, 80, 24)
	_ = workspace.SpawnPTY(2, 80, 24)

	_ = workspace.SetPTYPriority(1, 0x01) // High
	_ = workspace.SetPTYPriority(2, 0x00) // Low

	// Populate scrollbacks
	_ = workspace.WritePTYInput(1, []byte("echo 'REPLAY_1'\n"))
	_ = workspace.WritePTYInput(2, []byte("echo 'REPLAY_2'\n"))
	time.Sleep(100 * time.Millisecond)

	// Simulate client reconnect: bind a new mock writer
	mockWriter := newMockTestSocketWriter()
	defer mockWriter.Close()
	
	buf1, _, _ := workspace.GetScrollbackBuffer(1)
	buf2, _, _ := workspace.GetScrollbackBuffer(2)
	workspace.FlushAndEnqueueReplays([]source.OutboundFrame{
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: buf1, DrainingPriority: source.PriorityHigh},
		{Action: source.ActionStreamIO, TerminalID: 2, Payload: buf2, DrainingPriority: source.PriorityHigh},
	})

	// Concurrently flood T1 with live output
	_ = workspace.WritePTYInput(1, []byte("while true; do echo 'LIVE_VAL'; done\n"))

	// Bind writer
	workspace.SetSocketWriter(mockWriter)

	// Assert that T2's replay frame is delivered before any live outputs of T1
	deadline := time.Now().Add(3 * time.Second)
	foundT2Replay := false
	for !foundT2Replay {
		select {
		case frame := <-mockWriter.frames:
			if frame.TerminalID == 2 && strings.Contains(string(frame.Payload), "REPLAY_2") {
				foundT2Replay = true
			}
			if frame.Action == source.ActionStreamIO && frame.TerminalID == 1 && strings.Contains(string(frame.Payload), "LIVE_VAL") {
				if !foundT2Replay {
					t.Fatal("T1 live output was delivered before T2's scrollback replay finished (replay starvation)")
				}
			}
		case <-time.After(time.Until(deadline)):
			t.Fatal("timeout waiting for T2 scrollback replay")
		}
	}
}

// 8. TestPrioritySyncWakeup
func TestPrioritySyncWakeup(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, error := registry.GetOrCreateWorkspace("wakeup-workspace")
	if error != nil {
		t.Fatalf("failed to create workspace: %v", error)
	}
	defer func() {
		_ = registry.RemoveWorkspace("wakeup-workspace")
	}()

	// Setup blocked writer
	blockedWriter := newMockTestSocketWriter()
	defer blockedWriter.Close()
	blockedWriter.block = make(chan struct{})
	workspace.SetSocketWriter(blockedWriter)

	if error := workspace.SpawnPTY(1, 80, 24); error != nil {
		t.Fatalf("spawn failed: %v", error)
	}
	_ = workspace.SetPTYPriority(1, 0x01)

	// Block the PTY reader loop
	doneWrite := make(chan struct{})
	go func() {
		for i := 0; i < 1024; i++ {
			workspace.EnqueueFrame(source.OutboundFrame{
				Action:           source.ActionStreamIO,
				TerminalID:       1,
				Payload:          []byte("BLOCK_VAL"),
				DrainingPriority: source.PriorityHigh,
			})
		}
		// The 1025th frame should block
		workspace.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("BLOCK_VAL_WAKEUP"),
			DrainingPriority: source.PriorityHigh,
		})
		close(doneWrite)
	}()

	// Wait to ensure it blocks
	time.Sleep(100 * time.Millisecond)

	select {
	case <-doneWrite:
		t.Fatal("expected enqueue to block, but it returned immediately")
	default:
		// Succeeded in blocking
	}

	// Demote to Low Priority (0x00) -> must broadcast and unblock
	_ = workspace.SetPTYPriority(1, 0x00)

	select {
	case <-doneWrite:
		// Successfully unblocked!
	case <-time.After(1 * time.Second):
		t.Fatal("blocked PTY reader loop failed to unblock upon demotion to Low Priority")
	}
}

// 9. TestGlobalConnectionSingletonEviction
func TestGlobalConnectionSingletonEviction(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	defer func() {
		_ = registry.RemoveWorkspace("token-A")
		_ = registry.RemoveWorkspace("token-B")
	}()

	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	// Override DefaultSweeperDuration to a short duration for fast tests
	originalDuration := source.DefaultSweeperDuration
	source.DefaultSweeperDuration = 30 * time.Millisecond
	defer func() {
		source.DefaultSweeperDuration = originalDuration
	}()

	dialURLA := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=token-A"
	dialURLB := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=token-B"

	// Establish connection A
	connA, _, error := websocket.DefaultDialer.Dial(dialURLA, nil)
	if error != nil {
		t.Fatalf("failed to connect A: %v", error)
	}
	defer connA.Close()

	// Send Spawn Request from connA to ensure it is fully registered and active
	spawnReq := make([]byte, 8)
	binary.BigEndian.PutUint16(spawnReq[0:2], source.ActionSpawn)
	binary.BigEndian.PutUint16(spawnReq[2:4], 1)
	binary.BigEndian.PutUint16(spawnReq[4:6], 80)
	binary.BigEndian.PutUint16(spawnReq[6:8], 24)
	if err := connA.WriteMessage(websocket.BinaryMessage, spawnReq); err != nil {
		t.Fatalf("failed to send spawn request on A: %v", err)
	}

	// Read the spawn status response to ensure connection A is fully registered and active
	_ = connA.SetReadDeadline(time.Now().Add(3 * time.Second))
	msgType, resp, err := connA.ReadMessage()
	if err != nil || msgType != websocket.BinaryMessage || len(resp) < 5 {
		t.Fatalf("failed to read spawn response on A: %v (msgType=%d, len=%d)", err, msgType, len(resp))
	}

	// Establish connection B
	connB, _, error := websocket.DefaultDialer.Dial(dialURLB, nil)
	if error != nil {
		t.Fatalf("failed to connect B: %v", error)
	}
	defer connB.Close()

	// Connection A must be forcefully closed due to global singleton eviction
	_ = connA.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _, error = connA.ReadMessage()
	if error == nil {
		t.Fatal("expected connection A to be evicted/closed upon connection B upgrade, but it remained open")
	}

	// Wait for the cleanup sweeper countdown to finish for token-A
	time.Sleep(200 * time.Millisecond)

	// Verify that token-A workspace was reaped by checking that its active terminal ID list is empty
	workspaceANew, _ := registry.GetOrCreateWorkspace("token-A")
	activeIDs := workspaceANew.GetActiveTerminalIDs()
	if len(activeIDs) > 0 {
		t.Errorf("expected evicted workspace token-A to be swept and cleaned up, but it still contains active terminal IDs: %v", activeIDs)
	}
}

// 10. TestQueueCleanupOnPTYTermination
func TestQueueCleanupOnPTYTermination(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, error := registry.GetOrCreateWorkspace("cleanup-workspace")
	if error != nil {
		t.Fatalf("failed to create workspace: %v", error)
	}
	defer func() {
		_ = registry.RemoveWorkspace("cleanup-workspace")
	}()

	_ = workspace.SpawnPTY(1, 80, 24)
	_ = workspace.SetPTYPriority(1, 0x01)

	// Populate queue
	_ = workspace.WritePTYInput(1, []byte("echo 'VAL'\n"))
	time.Sleep(50 * time.Millisecond)

	// Terminate PTY 1 (deletes from workspace map)
	_ = workspace.TerminatePTY(1)

	// Bind mock writer
	mockWriter := newMockTestSocketWriter()
	defer mockWriter.Close()
	workspace.SetSocketWriter(mockWriter)

	// Verify scheduler loop continues running cleanly (no nil pointer dereferences or panics)
	time.Sleep(100 * time.Millisecond)
}

// 11. TestPTYTerminationChronologicalSequence
func TestPTYTerminationChronologicalSequence(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, error := registry.GetOrCreateWorkspace("seq-workspace")
	if error != nil {
		t.Fatalf("failed to create workspace: %v", error)
	}
	defer func() {
		_ = registry.RemoveWorkspace("seq-workspace")
	}()

	_ = workspace.SpawnPTY(1, 80, 24)
	_ = workspace.SetPTYPriority(1, 0x01)

	mockWriter := newMockTestSocketWriter()
	defer mockWriter.Close()
	workspace.SetSocketWriter(mockWriter)

	// Write and terminate
	_ = workspace.WritePTYInput(1, []byte("echo 'TERMINATION_TEST_OUTPUT'\nexit 42\n"))

	// Assert that exit notification frame (source.ActionKill) arrives after stdout and is final
	deadline := time.Now().Add(3 * time.Second)
	foundExit := false
	foundOutput := false
	for !foundExit {
		select {
		case frame := <-mockWriter.frames:
			if frame.TerminalID == 1 {
				if frame.Action == source.ActionStreamIO && strings.Contains(string(frame.Payload), "TERMINATION_TEST_OUTPUT") {
					foundOutput = true
				}
				if frame.Action == source.ActionKill {
					if !foundOutput {
						t.Fatal("PTY exit notification received before stdout was fully drained")
					}
					if len(frame.Payload) != 1 || frame.Payload[0] != 42 {
						t.Errorf("expected exit status 42, got %v", frame.Payload)
					}
					foundExit = true
				}
			}
		case <-time.After(time.Until(deadline)):
			t.Fatal("timeout waiting for exit notification")
		}
	}
}

// 12. TestOrphanedPriorityReversionOnReconnect
func TestOrphanedPriorityReversionOnReconnect(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, error := registry.GetOrCreateWorkspace("revert-workspace")
	if error != nil {
		t.Fatalf("failed to create workspace: %v", error)
	}
	defer func() {
		_ = registry.RemoveWorkspace("revert-workspace")
	}()

	_ = workspace.SpawnPTY(1, 80, 24)
	_ = workspace.SetPTYPriority(1, 0x01)

	// 1. Disconnect (Orphaned fallback to Low priority)
	workspace.SetSocketWriter(nil)

	// Ingest 1024 frames while offline (should not block)
	for i := 0; i < 1024; i++ {
		workspace.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("OFFLINE_VAL"),
			DrainingPriority: source.PriorityHigh,
		})
	}

	// 2. Reconnect (Active promotion back to High priority)
	// We bind a socket writer, but we do NOT drain the queue (mock writer blocks/errors)
	mockWriter := newMockTestSocketWriter()
	defer mockWriter.Close()
	mockWriter.block = make(chan struct{}) // force blocking condition
	workspace.SetSocketWriter(mockWriter)

	// Flood output: must block
	doneRevert := make(chan struct{})
	go func() {
		workspace.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("REVERSED_BLOCK"),
			DrainingPriority: source.PriorityHigh,
		})
		close(doneRevert)
	}()

	select {
	case <-doneRevert:
		t.Fatal("High priority terminal failed to block after reconnection (failed to revert from drop-oldest fallback)")
	case <-time.After(500 * time.Millisecond):
		// Success
	}
}

// TestReconnectionReplayStarvationPreventionWorkspaceWide verifies that if T1 has no replay frames
// but T2 does, T1's live outputs do not starve T2's replay during the active replay phase.
func TestReconnectionReplayStarvationPreventionWorkspaceWide(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, error := registry.GetOrCreateWorkspace("replay-starve-ww-workspace")
	if error != nil {
		t.Fatalf("failed to create workspace: %v", error)
	}
	defer func() {
		_ = registry.RemoveWorkspace("replay-starve-ww-workspace")
	}()

	// Spawn T1 and T2
	_ = workspace.SpawnPTY(1, 80, 24)
	_ = workspace.SpawnPTY(2, 80, 24)

	_ = workspace.SetPTYPriority(1, 0x01) // High
	_ = workspace.SetPTYPriority(2, 0x00) // Low

	// Populate T2 scrollback (T1 remains empty)
	_ = workspace.WritePTYInput(2, []byte("echo 'REPLAY_2'\n"))
	time.Sleep(100 * time.Millisecond)

	// Simulate client reconnect: bind a new mock writer
	mockWriter := newMockTestSocketWriter()
	defer mockWriter.Close()

	buf2, _, _ := workspace.GetScrollbackBuffer(2)

	// Enqueue T2 replay frame (T1 has none)
	workspace.FlushAndEnqueueReplays([]source.OutboundFrame{
		{Action: source.ActionStreamIO, TerminalID: 2, Payload: buf2, DrainingPriority: source.PriorityHigh},
	})

	// Concurrently flood T1 (no replays, High priority) with live output
	_ = workspace.WritePTYInput(1, []byte("while true; do echo 'LIVE_VAL'; done\n"))

	// Bind writer
	workspace.SetSocketWriter(mockWriter)

	// Assert T2's replay frame is delivered before T1's live output
	deadline := time.Now().Add(3 * time.Second)
	foundT2Replay := false
	for !foundT2Replay {
		select {
		case frame := <-mockWriter.frames:
			if frame.TerminalID == 2 && strings.Contains(string(frame.Payload), "REPLAY_2") {
				foundT2Replay = true
			}
			if frame.Action == source.ActionStreamIO && frame.TerminalID == 1 && strings.Contains(string(frame.Payload), "LIVE_VAL") {
				if !foundT2Replay {
					t.Fatal("T1 live output was delivered before T2's scrollback replay finished (workspace-wide starvation)")
				}
			}
		case <-time.After(time.Until(deadline)):
			t.Fatal("timeout waiting for T2 scrollback replay")
		}
	}
}
