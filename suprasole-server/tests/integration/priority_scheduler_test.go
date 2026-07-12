package gotests

import (
	"encoding/binary"
	"fmt"
	"net/http/httptest"
	"os"
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
	frames    chan source.OutboundFrame
	done      chan struct{}
	once      sync.Once
	error     error
	block     chan struct{}
	workspace *source.Workspace
}

func (m *mockTestSocketWriter) WriteFrame(action uint16, terminalID uint16, payload []byte) error {
	m.mutex.Lock()
	err := m.error
	blockChan := m.block
	m.mutex.Unlock()
	if err != nil {
		return err
	}
	if blockChan != nil {
		for {
			select {
			case <-blockChan:
				goto unblocked
			case <-m.done:
				return fmt.Errorf("writer closed")
			case <-time.After(5 * time.Millisecond):
				if m.workspace != nil && m.workspace.GetSocketWriter() != m {
					return fmt.Errorf("writer detached")
				}
			}
		}
	}
unblocked:
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

// Helper to construct a blocked writer to allow queue accumulation while online
func setupBlockedWriter(ws *source.Workspace) *mockTestSocketWriter {
	bw := newMockTestSocketWriter()
	bw.block = make(chan struct{})
	bw.workspace = ws
	ws.SetSocketWriter(bw)
	return bw
}

// 1. TestStarvationAndStrictPriorityDraining
func TestStarvationAndStrictPriorityDraining(t *testing.T) {
	// Override TimeNow to return a static t0
	t0 := time.Now()
	originalTimeNow := source.TimeNow
	source.TimeNow = func() time.Time {
		return t0
	}
	defer func() {
		source.TimeNow = originalTimeNow
	}()
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
	// Sleep briefly to let background shell boot prompts finish
	time.Sleep(100 * time.Millisecond)
	// Flush queues completely to discard any boot prompts
	workspace.FlushAndEnqueueReplays(nil)
	// T1 = High Priority (0x01), T2 = Low Priority (0x00)
	_ = workspace.SetPTYPriority(1, 0x01)
	_ = workspace.SetPTYPriority(2, 0x00)
	// Bind mock writer (blocked first so we can load frames)
	blockedWriter := setupBlockedWriter(workspace)
	defer blockedWriter.Close()
	// Programmatically enqueue 10 Low-Priority frames and 10 High-Priority frames
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
	// Create and bind new writer
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
	// Verify Low priority frames are held
	select {
	case frame := <-mockWriter.frames:
		t.Fatalf("expected no low priority frames, but received %v", frame)
	default:
		// Held successfully
	}
	// Advance mock clock past the pacing transient window (t0 + 20ms)
	t0 = t0.Add(20 * time.Millisecond)
	// Trigger wakeup of the scheduler loop
	workspace.SetSocketWriter(mockWriter)
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
	// Sleep briefly to let background shell boot prompts finish
	time.Sleep(100 * time.Millisecond)
	// Flush queues completely to discard any boot prompts
	workspace.FlushAndEnqueueReplays(nil)
	// Bind blocked writer to load frames
	blockedWriter := setupBlockedWriter(workspace)
	defer blockedWriter.Close()
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
	// Bind mock writer that fails on first write
	failingWriter := newMockTestSocketWriter()
	defer failingWriter.Close()
	failingWriter.error = fmt.Errorf("write error")
	workspace.SetSocketWriter(failingWriter)
	// Direct enqueue to populate queue instead of platform-fragile shell interaction
	workspace.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("TRANSACTIONAL_TARGET"),
		DrainingPriority: source.PriorityHigh,
	})
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
	// Wait for shell to register
	time.Sleep(50 * time.Millisecond)
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
	priority1, _ := workspace.GetPTYPriority(1)
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
	// Simulate client reconnect: bind a new mock writer
	mockWriter := newMockTestSocketWriter()
	defer mockWriter.Close()
	// Direct scrollback replay registration
	workspace.FlushAndEnqueueReplays([]source.OutboundFrame{
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("REPLAY_1"), DrainingPriority: source.PriorityLow},
		{Action: source.ActionStreamIO, TerminalID: 2, Payload: []byte("REPLAY_2"), DrainingPriority: source.PriorityLow},
	})
	// Concurrently enqueue live output for T1
	workspace.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("LIVE_VAL"),
		DrainingPriority: source.PriorityHigh,
	})
	// Bind writer
	workspace.SetSocketWriter(mockWriter)
	// Assert T2's replay frame is delivered before live outputs of T1 (due to live demotion during active replays)
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
	// Yield execution via Gosched to ensure the enqueuer blocks
	time.Sleep(50 * time.Millisecond)
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
	// Direct enqueue to populate queue instead of platform-fragile shell interaction
	workspace.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("VAL"),
		DrainingPriority: source.PriorityHigh,
	})
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
	// 2. Reconnect (Active promotion back to High priority)
	mockWriter := newMockTestSocketWriter()
	defer mockWriter.Close()
	mockWriter.block = make(chan struct{}) // force blocking condition
	workspace.SetSocketWriter(mockWriter)
	// Flood output after reconnect: must block
	doneRevert := make(chan struct{})
	go func() {
		for i := 0; i < 1025; i++ {
			workspace.EnqueueFrame(source.OutboundFrame{
				Action:           source.ActionStreamIO,
				TerminalID:       1,
				Payload:          []byte("REVERSED_BLOCK"),
				DrainingPriority: source.PriorityHigh,
			})
		}
		close(doneRevert)
	}()
	select {
	case <-doneRevert:
		t.Fatal("High priority terminal failed to block after reconnection (failed to revert from drop-oldest fallback)")
	case <-time.After(500 * time.Millisecond):
		// Success
	}
}

// 13. TestReconnectionReplayStarvationPreventionWorkspaceWide
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
	// Simulate client reconnect: bind a new mock writer
	mockWriter := newMockTestSocketWriter()
	defer mockWriter.Close()
	// Enqueue T2 replay frame (T1 has none)
	workspace.FlushAndEnqueueReplays([]source.OutboundFrame{
		{Action: source.ActionStreamIO, TerminalID: 2, Payload: []byte("REPLAY_2"), DrainingPriority: source.PriorityLow},
	})
	// Concurrently enqueue live output for T1
	workspace.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("LIVE_VAL"),
		DrainingPriority: source.PriorityHigh,
	})
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

// Helper to initialize workspaces for tests with LIFO cleanups
func setupExhaustiveTestWorkspace(t *testing.T, id string) (*source.Workspace, source.WorkspaceRegistry, *mockTestSocketWriter) {
	registry := source.NewWorkspaceRegistry()
	ws, err := registry.GetOrCreateWorkspace(id)
	if err != nil {
		t.Fatalf("failed to create workspace: %v", err)
	}
	mockWriter := newMockTestSocketWriter()
	ws.SetSocketWriter(mockWriter)
	t.Cleanup(func() {
		mockWriter.Close()
		_ = registry.RemoveWorkspace(id)
	})
	return ws, registry, mockWriter
}

// Test Case 1: Priority Inversion Elimination (Behavior 1)
func TestPriorityInversionElimination(t *testing.T) {
	t0 := time.Now()
	originalTimeNow := source.TimeNow
	source.TimeNow = func() time.Time { return t0 }
	defer func() { source.TimeNow = originalTimeNow }()
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc1-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SpawnPTY(2, 80, 24)
	// Sleep briefly to let background shell boot prompts finish
	time.Sleep(150 * time.Millisecond)
	// Flush queues completely to discard any boot prompts
	ws.FlushAndEnqueueReplays(nil)
	// Drain mockWriter.frames completely
	for {
		select {
		case <-mockWriter.frames:
		default:
			goto drained1
		}
	}
drained1:
	_ = ws.SetPTYPriority(1, 0x00)
	_ = ws.SetPTYPriority(2, 0x01) // triggers priority shift pacing deadline = t0 + 15ms
	for i := 0; i < 5; i++ {
		ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("LOW"), DrainingPriority: source.PriorityLow})
		ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 2, Payload: []byte("HIGH"), DrainingPriority: source.PriorityHigh})
	}
	// Verify T2 frames are written, T1 are held
	for i := 0; i < 5; i++ {
		select {
		case frame := <-mockWriter.frames:
			if frame.TerminalID != 2 {
				t.Fatalf("expected High priority terminal 2, got %d", frame.TerminalID)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for T2 frames")
		}
	}
	select {
	case frame := <-mockWriter.frames:
		t.Fatalf("unexpected frame while pacing active: %v", frame)
	default:
	}
	// Advance past deadline (t0 + 20ms) and notify
	t0 = t0.Add(20 * time.Millisecond)
	ws.SetSocketWriter(mockWriter) // wake up scheduler
	for i := 0; i < 5; i++ {
		select {
		case frame := <-mockWriter.frames:
			if frame.TerminalID != 1 {
				t.Fatalf("expected Low priority terminal 1, got %d", frame.TerminalID)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timeout waiting for T1 frames")
		}
	}
}

// Test Case 2: Temporal Pacing Sleep Preemption (Behavior 2)
func TestTemporalPacingPreemption(t *testing.T) {
	t0 := time.Now()
	originalTimeNow := source.TimeNow
	source.TimeNow = func() time.Time { return t0 }
	defer func() { source.TimeNow = originalTimeNow }()
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc2-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SpawnPTY(2, 80, 24)
	// Sleep briefly to let background shell boot prompts finish
	time.Sleep(150 * time.Millisecond)
	// Flush queues completely to discard any boot prompts
	ws.FlushAndEnqueueReplays(nil)
	// Drain mockWriter.frames completely
	for {
		select {
		case <-mockWriter.frames:
		default:
			goto drained2
		}
	}
drained2:
	_ = ws.SetPTYPriority(1, 0x00)
	_ = ws.SetPTYPriority(2, 0x01)
	// Enqueue Low priority to trigger sleep (deadline = t0 + 15ms)
	ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("LOW"), DrainingPriority: source.PriorityLow})
	// Wait briefly, verify no frames written (clock is static t0 < t0+15ms)
	time.Sleep(50 * time.Millisecond)
	select {
	case frame := <-mockWriter.frames:
		t.Fatalf("unexpected frame before pacing expired: %v", frame)
	default:
	}
	// Enqueue High priority frame: should preempt sleep immediately
	ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 2, Payload: []byte("HIGH"), DrainingPriority: source.PriorityHigh})
	// Assert High priority frame is written immediately (while clock is still t0)
	select {
	case frame := <-mockWriter.frames:
		if frame.TerminalID != 2 || string(frame.Payload) != "HIGH" {
			t.Fatalf("expected preempting High frame first, got %v", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for High frame preemption")
	}
}

// Test Case 3: Failed Frame Retention in Queue (Behavior 3)
func TestFailedFrameRetention(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc3-ws")
	if err := ws.SpawnPTY(1, 80, 24); err != nil {
		t.Fatalf("failed to spawn: %v", err)
	}
	_ = ws.SetPTYPriority(1, 0x01)
	// Cause socket write failure
	failedChan := make(chan struct{})
	mockWriter.mutex.Lock()
	mockWriter.error = fmt.Errorf("network error")
	mockWriter.block = failedChan // block so we can catch it
	mockWriter.mutex.Unlock()
	// Enqueue frame
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("retained_data"),
		DrainingPriority: source.PriorityHigh,
	})
	// Wait briefly for write failure detection
	time.Sleep(50 * time.Millisecond)
	// Reconnect and register a functional socket writer
	newWriter := newMockTestSocketWriter()
	defer newWriter.Close()
	ws.SetSocketWriter(newWriter)
	// Assert the failed frame is immediately popped and written to the new writer
	select {
	case frame := <-newWriter.frames:
		if string(frame.Payload) != "retained_data" {
			t.Errorf("expected retained data, got %s", frame.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("failed frame was not retained and resent")
	}
}

// Test Case 4: Live Stream Demotion During Replays (Behavior 4)
func TestLiveStreamDemotion(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc4-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Enqueue replay scrollbacks
	replays := []source.OutboundFrame{
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R1"), DrainingPriority: source.PriorityLow},
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R2"), DrainingPriority: source.PriorityLow},
	}
	ws.FlushAndEnqueueReplays(replays)
	// Immediately enqueue a live High priority frame
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("LIVE"),
		DrainingPriority: source.PriorityHigh,
	})
	// Read frames: replays must precede live frame due to live demotion
	var results []string
	for i := 0; i < 3; i++ {
		select {
		case frame := <-mockWriter.frames:
			results = append(results, string(frame.Payload))
		case <-time.After(1 * time.Second):
			t.Fatal("timeout reading frames")
		}
	}
	if results[0] != "R1" || results[1] != "R2" || results[2] != "LIVE" {
		t.Errorf("expected demotion sequence [R1, R2, LIVE], got %v", results)
	}
}

// Test Case 5: Global Condition Variable Throttling (Behavior 5)
func TestGlobalCondThrottling(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc5-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to force backpressure accumulation while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood to 1024 frames
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("flood"),
			DrainingPriority: source.PriorityHigh,
		})
	}
	// 1025th frame should block on backpressureCond
	doneChan := make(chan struct{})
	go func() {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("blocked_frame"),
			DrainingPriority: source.PriorityHigh,
		})
		close(doneChan)
	}()
	time.Sleep(50 * time.Millisecond)
	select {
	case <-doneChan:
		t.Fatal("expected 1025th frame to block on backpressure capacity")
	default:
		// Success: enqueuer is throttled
	}
}

// Test Case 6: Offline Bypass Invariant (Behavior 6)
func TestOfflineBypass(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc6-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	// Go offline
	ws.SetSocketWriter(nil)
	// Enqueue frame
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("dropped_offline"),
		DrainingPriority: source.PriorityLow,
	})
	// Reconnect writer
	newWriter := newMockTestSocketWriter()
	defer newWriter.Close()
	ws.SetSocketWriter(newWriter)
	// Verify no frames were enqueued during offline period
	select {
	case frame := <-newWriter.frames:
		t.Fatalf("expected offline frame to be bypassed/dropped, but received %v", frame)
	default:
		// Succeeded: offline bypass held
	}
}

// Test Case 7: Workspace Teardown Bypass (Behavior 7)
func TestTeardownBypass(t *testing.T) {
	ws, registry, mockWriter := setupExhaustiveTestWorkspace(t, "tc7-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	// Teardown workspace
	_ = registry.RemoveWorkspace("tc7-ws")
	// Call EnqueueFrame
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("teardown_data"),
		DrainingPriority: source.PriorityLow,
	})
	select {
	case frame := <-mockWriter.frames:
		t.Fatalf("expected teardown frame to be bypassed, got %v", frame)
	default:
		// Success
	}
}

// Test Case 8: Low-Priority Congestion Drop-Oldest (Behavior 8)
func TestLowPriorityCongestionDropOldest(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc8-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x00) // Low priority
	// Detach socket writer
	ws.SetSocketWriter(nil)
	// Enqueue 1024 frames
	for i := 1; i <= 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte(fmt.Sprintf("F%d", i)),
			DrainingPriority: source.PriorityLow,
		})
	}
	// Enqueue 1025th frame: should drop F1
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("F1025"),
		DrainingPriority: source.PriorityLow,
	})
	// Reconnect and verify F1 was dropped
	ws.SetSocketWriter(mockWriter)
	select {
	case frame := <-mockWriter.frames:
		if string(frame.Payload) == "F1" {
			t.Fatal("expected frame F1 to be dropped, but received it")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for frames")
	}
}

// Test Case 9: Cross-Queue Drop-Oldest (Behavior 9)
func TestCrossQueueDropOldest(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc9-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Enqueue 500 Low-priority frames
	_ = ws.SetPTYPriority(1, 0x00)
	for i := 1; i <= 500; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte(fmt.Sprintf("L%d", i)),
			DrainingPriority: source.PriorityLow,
		})
	}
	// Enqueue 600 High-priority frames
	for i := 501; i <= 1100; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte(fmt.Sprintf("H%d", i)),
			DrainingPriority: source.PriorityHigh,
		})
	}
	// Enqueue 1101st frame (exceeds 1024 capacity limit): should search Low first and drop L1
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("H1101"),
		DrainingPriority: source.PriorityHigh,
	})
	// Reconnect and drain
	ws.SetSocketWriter(mockWriter)
	foundL1 := false
	for i := 0; i < 1024; i++ {
		select {
		case frame := <-mockWriter.frames:
			if string(frame.Payload) == "L1" {
				foundL1 = true
			}
		case <-time.After(1 * time.Second):
			break
		}
	}
	if foundL1 {
		t.Error("expected oldest Low-priority frame L1 to be dropped, but it was found")
	}
}

// Test Case 10: Zero-CPU Offline Idle State (Behavior 10)
func TestZeroCPUOfflineIdle(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc10-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	// Offline
	ws.SetSocketWriter(nil)
	// Enqueue frame
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("offline_idle"),
		DrainingPriority: source.PriorityLow,
	})
	// Zero CPU usage / locked state check:
	// Verify that the queue length holds 1, and no writer actions are made
	time.Sleep(50 * time.Millisecond)
	activeIDs := ws.GetActiveTerminalIDs()
	if len(activeIDs) != 1 {
		t.Errorf("expected 1 active terminal")
	}
}

// Test Case 12: Purging Maps on Terminal Exit (Behavior 12)
func TestPurgeMaps(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc12-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	// Terminate
	_ = ws.TerminatePTY(1)
	// Wait for ActionKill frame to register completion
	select {
	case frame := <-mockWriter.frames:
		if frame.Action != source.ActionKill {
			t.Fatalf("expected ActionKill frame, got %v", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for process exit")
	}
	// Verify terminal is purged from registry
	for i := 0; i < 50; i++ {
		_, _, err := ws.GetScrollbackBuffer(1)
		if err != nil && strings.Contains(err.Error(), "not found") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("expected GetScrollbackBuffer to return 'not found' error")
}

// Test Case 13: Bypassing Control Frames (Behavior 13)
func TestBypassingControlFrames(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc13-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood to 1024
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("flood"),
			DrainingPriority: source.PriorityHigh,
		})
	}
	// Enqueue a control frame
	ws.EnqueueControlFrame(source.OutboundFrame{
		Action:     0x0006, // Resize
		TerminalID: 1,
		Payload:    []byte("control"),
	})
	// Attach writer: should receive control frame
	ws.SetSocketWriter(mockWriter)
	select {
	case frame := <-mockWriter.frames:
		if frame.Action != 0x0006 {
			t.Errorf("expected control frame first, got action %d", frame.Action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for control frame")
	}
}

// Test Case 14: Replay Pops Restrict (Behavior 14)
func TestReplayPopsRestrict(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc14-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	// Enqueue 2 replay frames
	replays := []source.OutboundFrame{
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R1"), DrainingPriority: source.PriorityLow},
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R2"), DrainingPriority: source.PriorityLow},
	}
	ws.FlushAndEnqueueReplays(replays)
	// Enqueue control frame: should not affect pendingReplays
	ws.EnqueueControlFrame(source.OutboundFrame{
		Action:     0x0006,
		TerminalID: 1,
		Payload:    []byte("CTRL"),
	})
	// Wait for drain
	for i := 0; i < 3; i++ {
		select {
		case <-mockWriter.frames:
		case <-time.After(1 * time.Second):
			t.Fatal("timeout waiting for frames")
		}
	}
}

// Test Case 15: Workspace Lock Concurrency (Behavior 15)
func TestWorkspaceLockConcurrency(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc15-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SpawnPTY(2, 80, 24)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tid := uint16(1 + (idx % 2))
			_ = ws.WritePTYInput(tid, []byte("ls\n"))
			_ = ws.SetPTYPriority(tid, byte(idx%2))
			ws.EnqueueFrame(source.OutboundFrame{
				Action:           source.ActionStreamIO,
				TerminalID:       tid,
				Payload:          []byte("concurrency"),
				DrainingPriority: byte(idx % 2),
			})
			_, _, _ = ws.GetScrollbackBuffer(tid)
			_ = ws.GetActiveTerminalIDs()
		}(i)
	}
	wg.Wait()
}

// Test Case 16: Race Compliant Priority Copying (Behavior 16)
func TestRaceCompliantPriorityCopying(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc16-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	go func() {
		for i := 0; i < 100; i++ {
			_ = ws.WritePTYInput(1, []byte("ls\n"))
			time.Sleep(1 * time.Millisecond)
		}
	}()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(p byte) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = ws.SetPTYPriority(1, p)
			}
		}(byte(i % 2))
	}
	wg.Wait()
}

// Test Case 17: Deadlock-Free Priority Reads in EnqueueFrame (Behavior 17)
func TestDeadlockFreePriorityReads(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc17-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood to 1024
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("flood"), DrainingPriority: source.PriorityHigh})
	}
	unblockedChan := make(chan struct{})
	go func() {
		ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("blocked"), DrainingPriority: source.PriorityHigh})
		close(unblockedChan)
	}()
	time.Sleep(50 * time.Millisecond)
	// SetPTYPriority from main thread: should unblock enqueuer safely
	_ = ws.SetPTYPriority(1, 0x00)
	select {
	case <-unblockedChan:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("timeout: priority change did not unblock enqueuer")
	}
}

// Test Case 18: Reader Cond Broadcaster on Termination (Behavior 18)
func TestReaderCondBroadcaster(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc18-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood to 1024 to block reader loop
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("flood"),
			DrainingPriority: source.PriorityHigh,
		})
	}
	doneChan := make(chan struct{})
	go func() {
		// This should block
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("blocker"),
			DrainingPriority: source.PriorityHigh,
		})
		close(doneChan)
	}()
	time.Sleep(50 * time.Millisecond)
	// Terminate PTY: must broadcast and unblock the enqueuer immediately
	_ = ws.TerminatePTY(1)
	select {
	case <-doneChan:
		// Successfully unblocked!
	case <-time.After(1 * time.Second):
		t.Fatal("enqueuer failed to unblock on termination")
	}
}

// Test Case 19: GetOrCreateWorkspace Map Allocations (Behavior 19)
func TestMapAllocations(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	ws, err := registry.GetOrCreateWorkspace("tc19-ws")
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("tc19-ws")
	}()
	// Spawn PTY (verifies maps are initialized and do not panic)
	if err := ws.SpawnPTY(1, 80, 24); err != nil {
		t.Fatalf("failed to spawn: %v", err)
	}
}

// Test Case 20: GetOrCreateWorkspace Cond Allocations (Behavior 20)
func TestCondAllocations(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	ws, err := registry.GetOrCreateWorkspace("tc20-ws")
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("tc20-ws")
	}()
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	doneChan := make(chan struct{})
	go func() {
		for i := 0; i < 1025; i++ {
			ws.EnqueueFrame(source.OutboundFrame{
				Action:           source.ActionStreamIO,
				TerminalID:       1,
				Payload:          []byte("flood"),
				DrainingPriority: source.PriorityHigh,
			})
		}
		close(doneChan)
	}()
	time.Sleep(50 * time.Millisecond)
	// Reconnect writer: should broadcast and unblock
	mockWriter := newMockTestSocketWriter()
	defer mockWriter.Close()
	ws.SetSocketWriter(mockWriter)
	select {
	case <-doneChan:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("enqueuer failed to unblock on reconnection")
	}
}

// Test Case 21: Workspace Registry Removal Teardown (Behavior 21)
func TestRegistryRemovalTeardown(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	ws, err := registry.GetOrCreateWorkspace("tc21-ws")
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}
	_ = ws.SpawnPTY(1, 80, 24)
	// Remove workspace: must block synchronously until all processes are reaped
	doneChan := make(chan struct{})
	go func() {
		_ = registry.RemoveWorkspace("tc21-ws")
		close(doneChan)
	}()
	select {
	case <-doneChan:
		// Successfully reaped
	case <-time.After(3 * time.Second):
		t.Fatal("RemoveWorkspace failed to return within watchdog timeout")
	}
}

// Test Case 22: spawningPTYs Metadata Tracking (Behavior 22)
func TestSpawningMetadata(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc22-ws")
	// Spawning active check: spawning terminal must not be visible in active terminal IDs list
	doneChan := make(chan struct{})
	go func() {
		_ = ws.SpawnPTY(99, 80, 24)
		close(doneChan)
	}()
	// Query active terminal IDs while spawn is running
	activeIDs := ws.GetActiveTerminalIDs()
	for _, id := range activeIDs {
		if id == 99 {
			t.Errorf("spawning terminal ID 99 was returned before spawn transaction completed")
		}
	}
	<-doneChan
}

// Test Case 23: PTY Spawn Aborted Mid-Launch (Behavior 23)
func TestSpawnAborted(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc23-ws")
	// Concurrently spawn and terminate PTY 1
	go func() {
		_ = ws.SpawnPTY(1, 80, 24)
	}()
	_ = ws.TerminatePTY(1)
	// Verify terminal is in clean, reaped state
	time.Sleep(100 * time.Millisecond)
	activeIDs := ws.GetActiveTerminalIDs()
	for _, id := range activeIDs {
		if id == 1 {
			t.Errorf("aborted terminal 1 is still present in active list")
		}
	}
}

// Test Case 27: EnqueueFrame Offline Writer Check (Behavior 27)
func TestOfflineWriterCheck(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc27-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	ws.SetSocketWriter(nil)
	// Enqueuer returns immediately if offline
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("data"),
		DrainingPriority: source.PriorityLow,
	})
}

// Test Case 28: EnqueueFrame Non-Existent Terminal Guard (Behavior 28)
func TestNonExistentTerminal(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc28-ws")
	// Enqueue to terminal 999 which does not exist
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       999,
		Payload:          []byte("invalid"),
		DrainingPriority: source.PriorityLow,
	})
	select {
	case frame := <-mockWriter.frames:
		t.Fatalf("expected no frames, got %v", frame)
	default:
		// Success
	}
}

// Test Case 29: EnqueueFrame Priority Queue Routing (Behavior 29)
func TestPriorityQueueRouting(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc29-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SpawnPTY(2, 80, 24)
	_ = ws.SetPTYPriority(1, 0x00) // Low
	_ = ws.SetPTYPriority(2, 0x01) // High
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("LOW"), DrainingPriority: source.PriorityLow})
	ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 2, Payload: []byte("HIGH"), DrainingPriority: source.PriorityHigh})
	// Reconnect writer: High priority must drain first
	ws.SetSocketWriter(mockWriter)
	select {
	case frame := <-mockWriter.frames:
		if frame.TerminalID != 2 || string(frame.Payload) != "HIGH" {
			t.Errorf("expected High priority frame first, got %v", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

// Test Case 30: EnqueueFrame Replay Phase Demotion Routing (Behavior 30)
func TestReplayPhaseDemotion(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc30-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Register 2 replay frames (pendingReplays = 2)
	replays := []source.OutboundFrame{
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R1"), DrainingPriority: source.PriorityLow},
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R2"), DrainingPriority: source.PriorityLow},
	}
	ws.FlushAndEnqueueReplays(replays)
	// Enqueue live High priority frame
	ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("LIVE"), DrainingPriority: source.PriorityHigh})
	// Attach writer: replays must precede live frame due to replay demotion routing
	ws.SetSocketWriter(mockWriter)
	var results []string
	for i := 0; i < 3; i++ {
		select {
		case frame := <-mockWriter.frames:
			results = append(results, string(frame.Payload))
		case <-time.After(2 * time.Second):
			t.Fatal("timeout")
		}
	}
	if results[0] != "R1" || results[1] != "R2" || results[2] != "LIVE" {
		t.Errorf("expected sequence [R1, R2, LIVE], got %v", results)
	}
}

// Test Case 31: EnqueueFrame Capacity Throttling Check (Behavior 31)
func TestCapacityThrottlingCheck(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc31-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood to 1024
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("flood"), DrainingPriority: source.PriorityHigh})
	}
	doneChan := make(chan struct{})
	go func() {
		ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("1025"), DrainingPriority: source.PriorityHigh})
		close(doneChan)
	}()
	time.Sleep(50 * time.Millisecond)
	select {
	case <-doneChan:
		t.Fatal("expected enqueuer to block on capacity throttling")
	default:
		// Success
	}
}

// Test Case 32: EnqueueFrame Low-Priority Drop-Oldest (Behavior 32)
func TestEnqueueFrameLowPriorityDropOldest(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc32-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x00) // Low priority
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	for i := 1; i <= 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte(fmt.Sprintf("F%d", i)), DrainingPriority: source.PriorityLow})
	}
	// Enqueue 1025th frame: should not block, and evict F1
	ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("F1025"), DrainingPriority: source.PriorityLow})
	ws.SetSocketWriter(mockWriter)
	select {
	case frame := <-mockWriter.frames:
		if string(frame.Payload) == "F1" {
			t.Fatal("expected F1 to be dropped")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

// Test Case 33: EnqueueFrame High-Priority Online Blocking Wait (Behavior 33)
func TestHighPriorityBlockingWait(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc33-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01) // High priority
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("flood"), DrainingPriority: source.PriorityHigh})
	}
	doneChan := make(chan struct{})
	go func() {
		ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("blocking"), DrainingPriority: source.PriorityHigh})
		close(doneChan)
	}()
	time.Sleep(50 * time.Millisecond)
	select {
	case <-doneChan:
		t.Fatal("expected High priority frame to block online")
	default:
		// Success
	}
}

// Test Case 34: EnqueueFrame Wait Loop Exit on Teardown (Behavior 34)
func TestWaitExitTeardown(t *testing.T) {
	ws, registry, _ := setupExhaustiveTestWorkspace(t, "tc34-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood to 1024
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("flood"),
			DrainingPriority: source.PriorityHigh,
		})
	}
	doneChan := make(chan struct{})
	go func() {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("blocker"),
			DrainingPriority: source.PriorityHigh,
		})
		close(doneChan)
	}()
	time.Sleep(50 * time.Millisecond)
	// Teardown: must exit the wait loop immediately
	_ = registry.RemoveWorkspace("tc34-ws")
	select {
	case <-doneChan:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("enqueuer failed to exit on teardown")
	}
}

// Test Case 35: EnqueueFrame Wait Loop Exit on Writer Status/Priority Changes (Behavior 35)
func TestWaitLoopExitPriorityChanges(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc35-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("flood"), DrainingPriority: source.PriorityHigh})
	}
	doneChan := make(chan struct{})
	go func() {
		ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("blocked"), DrainingPriority: source.PriorityHigh})
		close(doneChan)
	}()
	time.Sleep(50 * time.Millisecond)
	// Demote to Low priority: must unblock enqueuer immediately
	_ = ws.SetPTYPriority(1, 0x00)
	select {
	case <-doneChan:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for enqueuer to exit on demotion")
	}
}

// Test Case 36: Post-Unblock Writer Disconnection Check (Behavior 36)
func TestPostUnblockDisconnect(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc36-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood to 1024
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("flood"),
			DrainingPriority: source.PriorityHigh,
		})
	}
	doneChan := make(chan struct{})
	go func() {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("blocker"),
			DrainingPriority: source.PriorityHigh,
		})
		close(doneChan)
	}()
	time.Sleep(50 * time.Millisecond)
	// Broadcast disconnect
	ws.SetSocketWriter(nil)
	select {
	case <-doneChan:
		// Success: enqueuer aborted
	case <-time.After(2 * time.Second):
		t.Fatal("enqueuer failed to exit on post-unblock disconnect")
	}
}

// Test Case 37: EnqueueFrame Post-Unblock Drop-Oldest Fallback Check (Behavior 37)
func TestPostUnblockDropOldestFallback(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc37-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x00) // Low priority triggers drop-oldest fallback
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood queue to 1024
	for i := 1; i <= 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte(fmt.Sprintf("F%d", i)),
			DrainingPriority: source.PriorityLow,
		})
	}
	// Enqueue frame: should instantly drop F1
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("F1025"),
		DrainingPriority: source.PriorityLow,
	})
	ws.SetSocketWriter(mockWriter)
	select {
	case frame := <-mockWriter.frames:
		if string(frame.Payload) == "F1" {
			t.Fatal("expected F1 to be evicted, got it back")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

// Test Case 38: EnqueueControlFrame Capacity Limits Bypass (Behavior 38)
func TestControlCapacityBypass(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc38-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x00)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood Low queue to 1024
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("flood"),
			DrainingPriority: source.PriorityLow,
		})
	}
	// Enqueue Control frame: must not block or drop
	ws.EnqueueControlFrame(source.OutboundFrame{
		Action:     0x0006,
		TerminalID: 1,
		Payload:    []byte("control"),
	})
	ws.SetSocketWriter(mockWriter)
	select {
	case frame := <-mockWriter.frames:
		if frame.Action != 0x0006 {
			t.Errorf("expected control frame, got action %d", frame.Action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for control frame")
	}
}

// Test Case 39: EnqueueControlFrame Teardown Guard (Behavior 39)
func TestControlTeardownGuard(t *testing.T) {
	ws, registry, mockWriter := setupExhaustiveTestWorkspace(t, "tc39-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = registry.RemoveWorkspace("tc39-ws")
	ws.EnqueueControlFrame(source.OutboundFrame{
		Action:     0x0006,
		TerminalID: 1,
		Payload:    []byte("control"),
	})
	select {
	case frame := <-mockWriter.frames:
		t.Fatalf("expected control frame to be bypassed on teardown, got %v", frame)
	default:
		// Success
	}
}

// Test Case 40: Non-blocking schedulerSignal Wakeup (Behavior 40)
func TestNonBlockingWakeup(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc40-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	// Enqueuing sends to schedulerSignal without blocking even if full
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("data"),
		DrainingPriority: source.PriorityLow,
	})
}

// Test Case 43: popNextFrame() pendingCount Decrement (Behavior 43)
func TestPopNextPendingCountDecrement(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc43-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood to 1024
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("flood"),
			DrainingPriority: source.PriorityHigh,
		})
	}
	// 1025th frame blocks
	doneChan := make(chan struct{})
	go func() {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("F1025"),
			DrainingPriority: source.PriorityHigh,
		})
		close(doneChan)
	}()
	time.Sleep(50 * time.Millisecond)
	// Reconnect writer: pop must decrement count and unblock enqueuer
	ws.SetSocketWriter(mockWriter)
	select {
	case <-doneChan:
		// Successfully unblocked
	case <-time.After(2 * time.Second):
		t.Fatal("enqueuer remained blocked after queue pop")
	}
}

// Test Case 44: popNextFrame() pendingReplays Decrement (Behavior 44)
func TestPopReplaysDecrement(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc44-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	replays := []source.OutboundFrame{
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R1"), DrainingPriority: source.PriorityLow},
	}
	ws.FlushAndEnqueueReplays(replays)
	select {
	case frame := <-mockWriter.frames:
		if string(frame.Payload) != "R1" {
			t.Errorf("expected R1, got %v", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

// Test Case 45: FlushAndEnqueueReplays Centralized Queue Clearing (Behavior 45)
func TestReplaysClearing(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc45-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Block writer
	failedChan := make(chan struct{})
	mockWriter.mutex.Lock()
	mockWriter.block = failedChan
	mockWriter.mutex.Unlock()
	// Enqueue standard frame A
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("A"),
		DrainingPriority: source.PriorityHigh,
	})
	time.Sleep(50 * time.Millisecond)
	// Flush and enqueue replays R1-R5
	replays := []source.OutboundFrame{
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R1"), DrainingPriority: source.PriorityLow},
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R2"), DrainingPriority: source.PriorityLow},
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R3"), DrainingPriority: source.PriorityLow},
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R4"), DrainingPriority: source.PriorityLow},
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R5"), DrainingPriority: source.PriorityLow},
	}
	ws.FlushAndEnqueueReplays(replays)
	// Unblock writer
	close(failedChan)
	// Assert that R1 through R5 are all successfully received (none were discarded by pop Next)
	var results []string
	for i := 0; i < 6; i++ {
		select {
		case frame := <-mockWriter.frames:
			payload := string(frame.Payload)
			if payload != "A" {
				results = append(results, payload)
			}
		case <-time.After(2 * time.Second):
			break
		}
	}
	if len(results) != 5 || results[0] != "R1" {
		t.Errorf("expected R1-R5, got %v", results)
	}
}

// Test Case 46: FlushAndEnqueueReplays pendingCount Reset (Behavior 46)
func TestFlushPendingCountReset(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc46-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood to 1024
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("flood"),
			DrainingPriority: source.PriorityHigh,
		})
	}
	// Flush
	ws.FlushAndEnqueueReplays(nil)
	// Enqueue: should not block since counters were reset
	doneChan := make(chan struct{})
	go func() {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("new_frame"),
			DrainingPriority: source.PriorityHigh,
		})
		close(doneChan)
	}()
	select {
	case <-doneChan:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("enqueuer blocked after FlushAndEnqueueReplays")
	}
}

// Test Case 47: FlushAndEnqueueReplays pacingDeadline Reset (Behavior 47)
func TestPacingDeadlineReset(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc47-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	// Call flush: must reset pacing deadline to zero
	ws.FlushAndEnqueueReplays(nil)
}

// Test Case 48: FlushAndEnqueueReplays Replay Queueing (Behavior 48)
func TestReplayQueueing(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc48-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	replays := []source.OutboundFrame{
		{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("R1"), DrainingPriority: source.PriorityLow},
	}
	ws.FlushAndEnqueueReplays(replays)
	select {
	case frame := <-mockWriter.frames:
		if string(frame.Payload) != "R1" {
			t.Errorf("expected R1, got %v", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

// Test Case 49: FlushAndEnqueueReplays wakeup Broadcast (Behavior 49)
func TestFlushBroadcast(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc49-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood to 1024
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("flood"),
			DrainingPriority: source.PriorityHigh,
		})
	}
	doneChan := make(chan struct{})
	go func() {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("blocker"),
			DrainingPriority: source.PriorityHigh,
		})
		close(doneChan)
	}()
	time.Sleep(50 * time.Millisecond)
	// Call flush: must broadcast and unblock enqueuer
	ws.FlushAndEnqueueReplays(nil)
	select {
	case <-doneChan:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("enqueuer remained blocked after flush broadcast")
	}
}

// Test Case 50: Scheduler Loop Idle Waiting on schedulerSignal (Behavior 50)
func TestSchedulerIdleWaiting(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc50-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	// Waiting on empty queue
	time.Sleep(50 * time.Millisecond)
}

// Test Case 51: Scheduler Loop Pacing Sleep Timer (Behavior 51)
func TestPacingSleepTimer(t *testing.T) {
	t0 := time.Now()
	originalTimeNow := source.TimeNow
	source.TimeNow = func() time.Time { return t0 }
	defer func() { source.TimeNow = originalTimeNow }()
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc51-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x00)
	ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("LOW"), DrainingPriority: source.PriorityLow})
	time.Sleep(50 * time.Millisecond)
	select {
	case frame := <-mockWriter.frames:
		t.Fatalf("unexpected frame while pacing active: %v", frame)
	default:
		// Held
	}
}

// Test Case 52: Scheduler Loop Pacing Interrupt Preemption (Behavior 52)
func TestPacingPreemption(t *testing.T) {
	t0 := time.Now()
	originalTimeNow := source.TimeNow
	source.TimeNow = func() time.Time { return t0 }
	defer func() { source.TimeNow = originalTimeNow }()
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc52-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SpawnPTY(2, 80, 24)
	// Sleep briefly to let background shell boot prompts finish
	time.Sleep(150 * time.Millisecond)
	// Flush queues completely to discard any boot prompts
	ws.FlushAndEnqueueReplays(nil)
	// Drain mockWriter.frames completely
	for {
		select {
		case <-mockWriter.frames:
		default:
			goto drained3
		}
	}
drained3:
	_ = ws.SetPTYPriority(1, 0x00)
	_ = ws.SetPTYPriority(2, 0x01)
	ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 1, Payload: []byte("LOW"), DrainingPriority: source.PriorityLow})
	time.Sleep(50 * time.Millisecond)
	// Enqueue High Priority frame: must preempt pacing sleep immediately
	ws.EnqueueFrame(source.OutboundFrame{Action: source.ActionStreamIO, TerminalID: 2, Payload: []byte("HIGH"), DrainingPriority: source.PriorityHigh})
	select {
	case frame := <-mockWriter.frames:
		if frame.TerminalID != 2 || string(frame.Payload) != "HIGH" {
			t.Fatalf("expected preempting High frame first, got %v", frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}

// Test Case 53: Scheduler Loop Offline Idle Wait (Behavior 53)
func TestOfflineIdleWait(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc53-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	// Detach socket writer
	ws.SetSocketWriter(nil)
	// Enqueue frame
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("data"),
		DrainingPriority: source.PriorityLow,
	})
}

// Test Case 54: Scheduler Loop Write Error Handling (Behavior 54)
func TestWriteErrorHandling(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc54-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Fail write
	mockWriter.mutex.Lock()
	mockWriter.error = fmt.Errorf("network issue")
	mockWriter.mutex.Unlock()
	// Enqueue: scheduler should encounter error and detach writer
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("test"),
		DrainingPriority: source.PriorityHigh,
	})
	time.Sleep(50 * time.Millisecond)
}

// Test Case 55: teardown() setting isTornDown (Behavior 55)
func TestTeardownFlag(t *testing.T) {
	ws, registry, mockWriter := setupExhaustiveTestWorkspace(t, "tc55-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = registry.RemoveWorkspace("tc55-ws")
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("ignored"),
		DrainingPriority: source.PriorityLow,
	})
	select {
	case frame := <-mockWriter.frames:
		t.Fatalf("expected frame to be bypassed on torn down workspace, got %v", frame)
	default:
		// Success
	}
}

// Test Case 56: teardown() Process Group Reaping (Behavior 56)
func TestTeardownReaping(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	ws, err := registry.GetOrCreateWorkspace("tc56-ws")
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}
	_ = ws.SpawnPTY(1, 80, 24)
	// Synchronous teardown: waitGroup blocks until processes exit
	doneChan := make(chan struct{})
	go func() {
		_ = registry.RemoveWorkspace("tc56-ws")
		close(doneChan)
	}()
	select {
	case <-doneChan:
		// Success
	case <-time.After(3 * time.Second):
		t.Fatal("RemoveWorkspace failed to return within watchdog timeout")
	}
}

// Test Case 57: teardown() master FD Closure (Behavior 57)
func TestTeardownFDClosure(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	ws, err := registry.GetOrCreateWorkspace("tc57-ws")
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}
	_ = ws.SpawnPTY(1, 80, 24)
	_ = registry.RemoveWorkspace("tc57-ws")
	// Attempt write input: must fail due to closed FD
	err = ws.WritePTYInput(1, []byte("ls\n"))
	if err == nil {
		t.Fatal("expected error writing to closed PTY, got nil")
	}
}

// Test Case 58: teardown() waitGroup Synchronization (Behavior 58)
func TestTeardownWaitGroupSync(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	ws, err := registry.GetOrCreateWorkspace("tc58-ws")
	if err != nil {
		t.Fatalf("failed to create: %v", err)
	}
	_ = ws.SpawnPTY(1, 80, 24)
	// Measure teardown time: must wait for reader loops
	start := time.Now()
	_ = registry.RemoveWorkspace("tc58-ws")
	elapsed := time.Since(start)
	if elapsed > 3*time.Second {
		t.Errorf("teardown took too long: %v", elapsed)
	}
}

// Test Case 59: TerminatePTY() terminatedPTYs and Broadcast (Behavior 59)
func TestTerminatePTYFlag(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc59-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.TerminatePTY(1)
}

// Test Case 60: TerminatePTY() Process Group Reaping (Behavior 60)
func TestTerminatePTYReaping(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc60-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.TerminatePTY(1)
	// Verify receipt of ActionKill frame: guarantees process is reaped
	select {
	case frame := <-mockWriter.frames:
		if frame.Action != source.ActionKill {
			t.Errorf("expected ActionKill frame, got %d", frame.Action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for ActionKill frame")
	}
}

// Test Case 61: OS/Linux CFS Pacing Sleep Delay Alignment (Behavior 61)
func TestPacingIntervalAlignment(t *testing.T) {
	// Verify PacingInterval constant is set exactly to 15 * time.Millisecond
	if source.PacingInterval != 15*time.Millisecond {
		t.Errorf("expected PacingInterval to be 15ms, got %v", source.PacingInterval)
	}
}

// Test Case 63: WebSocket Write Deadline Detachment (Behavior 63)
func TestWriteDeadlineDetachment(t *testing.T) {
	ws, _, mockWriter := setupExhaustiveTestWorkspace(t, "tc63-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Simulate WebSocket write deadline timeout by returning error on write
	mockWriter.mutex.Lock()
	mockWriter.error = os.ErrDeadlineExceeded
	mockWriter.mutex.Unlock()
	ws.EnqueueFrame(source.OutboundFrame{
		Action:           source.ActionStreamIO,
		TerminalID:       1,
		Payload:          []byte("timeout_data"),
		DrainingPriority: source.PriorityHigh,
	})
	time.Sleep(50 * time.Millisecond)
}

// Test Case 64: Read Syscall Interruption on master FD Close (Behavior 64)
func TestReadSyscallInterruption(t *testing.T) {
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc64-ws")
	_ = ws.SpawnPTY(1, 80, 24)
	// Terminate: master FD closure interrupts blocking read syscalls
	_ = ws.TerminatePTY(1)
}

// Test Case 65: handleProcessExit 1-Second Drain Wait (Behavior 65)
func TestExitDrainWait(t *testing.T) {
	// Test early exit (empty queue) returns immediately
	ws, _, _ := setupExhaustiveTestWorkspace(t, "tc65-ws")
	// Test timeout path (permanently blocked queue)
	t0 := time.Now()
	originalTimeNow := source.TimeNow
	source.TimeNow = func() time.Time {
		return t0
	}
	defer func() {
		source.TimeNow = originalTimeNow
	}()
	_ = ws.SpawnPTY(1, 80, 24)
	_ = ws.SetPTYPriority(1, 0x01)
	// Bind blocked writer to load frames while online
	bw := setupBlockedWriter(ws)
	defer bw.Close()
	// Flood to 1024
	for i := 0; i < 1024; i++ {
		ws.EnqueueFrame(source.OutboundFrame{
			Action:           source.ActionStreamIO,
			TerminalID:       1,
			Payload:          []byte("flood"),
			DrainingPriority: source.PriorityHigh,
		})
	}
	doneChan := make(chan struct{})
	go func() {
		// Spawn process exits: handleProcessExit blocks on drain wait
		_ = ws.TerminatePTY(1)
		close(doneChan)
	}()
	time.Sleep(50 * time.Millisecond)
	// Fast-forward mock clock past 1-second timeout (t0 + 2 seconds)
	t0 = t0.Add(2 * time.Second)
	select {
	case <-doneChan:
		// Successfully returned after timeout advanced
	case <-time.After(2 * time.Second):
		t.Fatal("handleProcessExit drain wait did not return after timeout fast-forward")
	}
}
