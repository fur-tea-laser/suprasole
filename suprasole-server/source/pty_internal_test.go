package source

import (
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReplayUTF8BoundaryAlignment(t *testing.T) {
	// Input: [0x82, 0xBF, 0x41, 0x42, 0x43] (where 0x82 and 0xBF are continuation bytes, followed by ASCII A, B, C)
	// Expected: [0x41, 0x42, 0x43] (continuation bytes stripped)
	input := []byte{0x82, 0xBF, 0x41, 0x42, 0x43}
	expected := []byte{0x41, 0x42, 0x43}

	result := AlignUTF8Boundary(input)
	if len(result) != len(expected) {
		t.Errorf("Expected length %d, got %d", len(expected), len(result))
		return
	}
	for i, b := range result {
		if b != expected[i] {
			t.Errorf("Expected byte at %d to be 0x%02x, got 0x%02x", i, expected[i], b)
		}
	}
}

func TestReplayUTF8BoundaryAlignmentCorruptLimit(t *testing.T) {
	// Input: [0x82, 0x82, 0x82, 0x82, 0x41] (4 continuation bytes followed by ASCII A)
	// Expected: [0x82, 0x41] (skips maximum of 3 continuation bytes)
	input := []byte{0x82, 0x82, 0x82, 0x82, 0x41}
	expected := []byte{0x82, 0x41}

	result := AlignUTF8Boundary(input)
	if len(result) != len(expected) {
		t.Errorf("Expected length %d, got %d", len(expected), len(result))
		return
	}
	for i, b := range result {
		if b != expected[i] {
			t.Errorf("Expected byte at %d to be 0x%02x, got 0x%02x", i, expected[i], b)
		}
	}
}

func waitPTYRegistered(workspace *Workspace, terminalID uint16) *ptyInstance {
	for i := 0; i < 200; i++ {
		workspace.mutex.Lock()
		t, exists := workspace.ptys.Get(terminalID)
		workspace.mutex.Unlock()
		if exists {
			return t
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil
}

func TestPTYJobControlSessionLeadership(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("job-control-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("job-control-ws")
	}()

	err = workspace.SpawnPTY(210, 80, 24, "/bin/bash")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	terminal := waitPTYRegistered(workspace, 210)
	if terminal == nil {
		t.Fatalf("PTY 210 not found in registry")
	}

	pid := terminal.command.Process.Pid
	sid, _, errno := syscall.Syscall(syscall.SYS_GETSID, uintptr(pid), 0, 0)
	if errno != 0 {
		t.Fatalf("Failed to get SID of process %d: %v", pid, errno)
	}

	if int(sid) != pid {
		t.Errorf("Expected SID %d to match PID %d (not a session leader)", sid, pid)
	}

	// Send SIGINT to pgid and assert signal propagation works
	err = syscall.Kill(-pid, syscall.SIGINT)
	if err != nil {
		t.Errorf("Failed to send SIGINT to PGID -%d: %v", pid, err)
	}
}

func TestPTYEnvironmentVariables(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("env-inject-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("env-inject-ws")
	}()

	err = workspace.SpawnPTY(211, 80, 24, "/bin/bash")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	terminal := waitPTYRegistered(workspace, 211)
	if terminal == nil {
		t.Fatalf("PTY 211 not found in registry")
	}

	var hasTerm bool
	for _, env := range terminal.command.Env {
		if env == "TERM=xterm-256color" {
			hasTerm = true
			break
		}
	}
	if !hasTerm {
		t.Errorf("Expected TERM=xterm-256color in child environment, got %v", terminal.command.Env)
	}
}

func TestPTYRaceConditionDrain(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("race-drain-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("race-drain-ws")
	}()

	expectedText := "hello-race-test-drain-12345"
	err = workspace.SpawnPTY(999, 80, 24, "printf", "%s", expectedText)
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	terminal := waitPTYRegistered(workspace, 999)
	if terminal == nil {
		t.Fatalf("PTY 999 not found in registry")
	}

	<-terminal.waitDone
	<-terminal.readDone

	bufBytes := terminal.buffer.Bytes()
	if !strings.Contains(string(bufBytes), expectedText) {
		t.Errorf("Expected buffer to contain %q, but got %q", expectedText, string(bufBytes))
	}
}

func TestPTYSpawnCancellationStatus(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("spawn-cancel-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("spawn-cancel-ws")
	}()

	terminalID := uint16(998)

	// Spawn the PTY. SpawnPTY is synchronous up to starting the background goroutine,
	// so the terminal is guaranteed to be in the spawning map when it returns.
	err = workspace.SpawnPTY(terminalID, 80, 24, "sleep", "10")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	// Wait until it is registered as active in workspace.ptys to ensure it has started
	var activeTerm *ptyInstance
	for i := 0; i < 100; i++ {
		workspace.mutex.Lock()
		tInstance, exists := workspace.ptys.Get(terminalID)
		workspace.mutex.Unlock()
		if exists && tInstance.state == StateActive {
			activeTerm = tInstance
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if activeTerm == nil {
		t.Fatalf("PTY was not registered as active before cancellation")
	}

	// Now terminate it
	err = workspace.TerminatePTY(terminalID)
	if err != nil {
		t.Fatalf("Failed to terminate spawning PTY: %v", err)
	}

	// Verify that the spawning goroutine cleanly exited, deleted itself from spawning,
	// and registered as a terminated PTY in workspace.ptys
	var term *ptyInstance
	for i := 0; i < 100; i++ {
		workspace.mutex.Lock()
		tInstance, exists := workspace.ptys.Get(terminalID)
		workspace.mutex.Unlock()
		if exists {
			select {
			case <-tInstance.waitDone:
				term = tInstance
			default:
			}
		}
		if term != nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if term == nil {
		t.Fatalf("PTY was not registered as terminated after cancellation")
	}

	// Verify channels are closed cleanly
	select {
	case <-term.waitDone:
	default:
		t.Errorf("waitDone channel was not closed")
	}

	select {
	case <-term.readDone:
		// Passed
	default:
		t.Errorf("readDone channel was not closed")
	}
}

func TestPTYSpawnCancellationEvictionRace(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("spawn-evict-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("spawn-evict-ws")
	}()

	terminalID := uint16(997)

	// Spawn a PTY (spawns in a background goroutine)
	err = workspace.SpawnPTY(terminalID, 80, 24, "sleep", "10")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	workspace.mutex.Lock()
	inst := workspace.spawning[terminalID]
	workspace.mutex.Unlock()

	// Immediately call RemovePTY.
	err = workspace.RemovePTY(terminalID)
	if err != nil {
		t.Fatalf("Failed to remove spawning PTY: %v", err)
	}

	if inst != nil {
		<-inst.done
	}

	// Assert that we can re-spawn a new PTY on the same ID immediately without collisions
	err = workspace.SpawnPTY(terminalID, 80, 24, "echo", "hello")
	if err != nil {
		t.Fatalf("Failed to re-spawn PTY immediately after RemovePTY: %v", err)
	}

	// Wait for the new spawn to register and complete execution
	term := waitPTYRegistered(workspace, 997)
	if term == nil {
		t.Fatalf("Failed to register new PTY")
	}
	<-term.waitDone
	<-term.readDone
}

func TestPTYChronologicalReplayOrder(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("replay-order-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("replay-order-ws")
	}()

	// Spawn terminals in a specific chronological sequence
	ids := []uint16{991, 992, 993}
	for _, id := range ids {
		err = workspace.SpawnPTY(id, 80, 24, "echo", "hello")
		if err != nil {
			t.Fatalf("Failed to spawn terminal %d: %v", id, err)
		}
		// Wait for it to register in active map
		term := waitPTYRegistered(workspace, id)
		if term == nil {
			t.Fatalf("Terminal %d did not register", id)
		}
		// Wait for the echo process to finish so it has output/state
		<-term.waitDone
		<-term.readDone
	}

	// Compile replay frames multiple times to verify 100% deterministic chronological order
	for run := 0; run < 50; run++ {
		frames := workspace.CompileReplayFrames()

		// Map frame actions/terminal IDs back to spawn order
		var observedOrder []uint16
		for _, frame := range frames {
			// We track when we see a terminal's exit or output status
			if len(observedOrder) == 0 || observedOrder[len(observedOrder)-1] != frame.TerminalID {
				observedOrder = append(observedOrder, frame.TerminalID)
			}
		}

		// Verify the order of terminals in the replay matches the exact chronological spawn order [991, 992, 993]
		if len(observedOrder) != len(ids) {
			t.Fatalf("Expected replay frames for %d terminals, got %d", len(ids), len(observedOrder))
		}
		for i, expectedID := range ids {
			if observedOrder[i] != expectedID {
				t.Errorf("Run %d: Expected terminal at index %d to be %d, but got %d (non-deterministic ordering)",
					run, i, expectedID, observedOrder[i])
			}
		}
	}
}

func TestSchedulerReplayGateControlDemotion(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("replay-gate-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("replay-gate-ws")
	}()

	// 1. Manually set pendingReplays and load a low-priority replay frame to simulate replay phase
	workspace.mutex.Lock()
	workspace.pendingReplays = 1
	workspace.centralizedQueues[QueueIndexLow] = append(workspace.centralizedQueues[QueueIndexLow], OutboundFrame{
		Action:     ActionOutput,
		TerminalID: 100,
		Payload:    []byte("replay"),
	})
	workspace.mutex.Unlock()

	// 2. Enqueue a control frame. Under our new design, it goes to Control queue and is not demoted.
	workspace.EnqueueControlFrame(OutboundFrame{
		Action:     ActionSpawnStatus,
		TerminalID: 100,
		Payload:    []byte{0x00},
	})

	workspace.mutex.Lock()
	controlLen := len(workspace.centralizedQueues[QueueIndexControl])
	lowLen := len(workspace.centralizedQueues[QueueIndexLow])

	// Assert it was routed to Control queue and not demoted
	if controlLen != 1 {
		t.Errorf("Expected control frame to be in Control queue under new design, got length %d", controlLen)
	}
	if lowLen != 1 {
		t.Errorf("Expected only replay frame in Low queue, got length %d", lowLen)
	}

	// 3. Peek frame: because pendingReplays > 0, it should peek from QueueIndexLow, NOT QueueIndexControl!
	frame, qIdx, _, ok := workspace.peekNextFrame()
	if !ok || qIdx != QueueIndexLow || string(frame.Payload) != "replay" {
		t.Errorf("Expected to peek low-priority replay frame first due to replay gating, got %v, qIdx=%d", frame, qIdx)
	}

	// 4. Pop the replay frame to finish replay phase
	workspace.popNextFrame(QueueIndexLow, workspace.queueGeneration)
	pendingReplays := workspace.pendingReplays
	workspace.mutex.Unlock()

	if pendingReplays != 0 {
		t.Errorf("Expected pendingReplays to decrement to 0, got %d", pendingReplays)
	}

	// 5. Peek frame: now that pendingReplays is 0, priority scheduling is restored. It should peek the Control frame.
	workspace.mutex.Lock()
	frame, qIdx, _, ok = workspace.peekNextFrame()
	workspace.mutex.Unlock()
	if !ok || qIdx != QueueIndexControl || frame.Action != ActionSpawnStatus {
		t.Errorf("Expected to peek control frame after replay phase completed, got %v, qIdx=%d", frame, qIdx)
	}
}

func TestPTYSpawnCancellationQueueCleanSlate(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("spawn-cancel-clean-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("spawn-cancel-clean-ws")
	}()

	terminalID := uint16(990)

	// 1. Spawn a PTY (spawns in a background goroutine)
	err = workspace.SpawnPTY(terminalID, 80, 24, "sleep", "10")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	workspace.mutex.Lock()
	inst := workspace.spawning[terminalID]
	workspace.mutex.Unlock()

	// 2. Call RemovePTY
	err = workspace.RemovePTY(terminalID)
	if err != nil {
		t.Fatalf("Failed to remove spawning PTY: %v", err)
	}

	if inst != nil {
		<-inst.done
	}

	// 3. Assert that both Spawning (0x02) and Canceled (0x03) frames for 990 are enqueued in centralizedQueues
	workspace.mutex.Lock()
	var step3Payloads []byte
	for _, frame := range workspace.centralizedQueues[QueueIndexControl] {
		if frame.TerminalID == terminalID && frame.Action == ActionSpawnStatus {
			if len(frame.Payload) > 0 {
				step3Payloads = append(step3Payloads, frame.Payload[0])
			}
		}
	}
	workspace.mutex.Unlock()
	if len(step3Payloads) != 2 || step3Payloads[0] != 0x02 || step3Payloads[1] != 0x03 {
		t.Errorf("Expected [0x02, 0x03] SpawnStatus frames in control queue, got %v", step3Payloads)
	}

	// 4. Verify we can spawn 990 again and get a success status
	err = workspace.SpawnPTY(terminalID, 80, 24, "echo", "hello")
	if err != nil {
		t.Fatalf("Failed to re-spawn PTY immediately: %v", err)
	}

	// Wait for the new spawn to register and complete execution
	term := waitPTYRegistered(workspace, terminalID)
	if term == nil {
		t.Fatalf("Failed to register new PTY")
	}

	// Check that the full sequence of SpawnStatus frames for 990 is: 0x02, 0x03, 0x02, 0x00
	workspace.mutex.Lock()
	var finalPayloads []byte
	for _, frame := range workspace.centralizedQueues[QueueIndexControl] {
		if frame.TerminalID == terminalID && frame.Action == ActionSpawnStatus {
			if len(frame.Payload) > 0 {
				finalPayloads = append(finalPayloads, frame.Payload[0])
			}
		}
	}
	workspace.mutex.Unlock()
	if len(finalPayloads) != 4 || finalPayloads[0] != 0x02 || finalPayloads[1] != 0x03 || finalPayloads[2] != 0x02 || finalPayloads[3] != 0x00 {
		t.Errorf("Expected [0x02, 0x03, 0x02, 0x00] SpawnStatus frames, got %v", finalPayloads)
	}
}

func TestTakeoverReplayGapAtomicity(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("gap-atomicity-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("gap-atomicity-ws")
	}()

	terminalID := uint16(888)

	// Run multiple iterations to catch any potential race timing
	for i := 0; i < 50; i++ {
		// Spawn a PTY
		err = workspace.SpawnPTY(terminalID, 80, 24, "echo", "hello")
		if err != nil {
			t.Fatalf("Failed to spawn PTY on iteration %d: %v", i, err)
		}

		// Concurrently call PerformReplayTakeover
		go func() {
			workspace.PerformReplayTakeover(nil)
		}()

		// Wait for the PTY to be fully registered/active
		var term *ptyInstance
		for j := 0; j < 200; j++ {
			workspace.mutex.Lock()
			tInstance, exists := workspace.ptys.Get(terminalID)
			workspace.mutex.Unlock()
			if exists {
				term = tInstance
				break
			}
			time.Sleep(1 * time.Millisecond)
		}

		if term == nil {
			t.Fatalf("PTY did not register in time")
		}

		// Wait for process wait done
		<-term.waitDone

		// Assert that the success status frame (0x00) was NOT lost.
		workspace.mutex.Lock()
		foundStatus := false

		// Check QueueIndexLow (for replayed spawn status)
		for _, frame := range workspace.centralizedQueues[QueueIndexLow] {
			if frame.TerminalID == terminalID && frame.Action == ActionSpawnStatus && len(frame.Payload) > 0 && frame.Payload[0] == 0x00 {
				foundStatus = true
				break
			}
		}

		// Check QueueIndexControl (for normally enqueued spawn status)
		if !foundStatus {
			for _, frame := range workspace.centralizedQueues[QueueIndexControl] {
				if frame.TerminalID == terminalID && frame.Action == ActionSpawnStatus && len(frame.Payload) > 0 && frame.Payload[0] == 0x00 {
					foundStatus = true
					break
				}
			}
		}
		workspace.mutex.Unlock()

		if !foundStatus {
			t.Errorf("Spawn status frame (0x00) for terminal %d was completely lost on iteration %d", terminalID, i)
		}

		// Clean up the PTY before the next iteration
		err = workspace.RemovePTY(terminalID)
		if err != nil {
			t.Fatalf("Failed to remove PTY on iteration %d: %v", i, err)
		}
	}
}

func TestPTYSpawnIDRecyclingLateCleanup(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("test-late-cleanup")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("test-late-cleanup")
	}()

	// 1. Initiate first spawn for terminal ID 999 (a slow command)
	err = workspace.SpawnPTY(999, 80, 24, "sleep", "99")
	if err != nil {
		t.Fatalf("Failed to start first spawn: %v", err)
	}

	workspace.mutex.Lock()
	inst := workspace.spawning[999]
	workspace.mutex.Unlock()

	// 2. Immediately remove the terminal to trigger abort/cleanup
	err = workspace.RemovePTY(999)
	if err != nil {
		t.Fatalf("Failed to remove first spawn: %v", err)
	}

	if inst != nil {
		<-inst.done
	}

	// 3. Immediately initiate second spawn for recycled terminal ID 999 (a quick command)
	err = workspace.SpawnPTY(999, 80, 24, "echo", "recycled")
	if err != nil {
		t.Fatalf("Failed to start second spawn: %v", err)
	}

	// Wait for the second spawn goroutine itself to finish spawning
	workspace.mutex.Lock()
	secondInst := workspace.spawning[999]
	workspace.mutex.Unlock()
	if secondInst != nil {
		<-secondInst.done
	}

	// 4. Wait for all background spawning/monitoring goroutines to complete
	workspace.waitGroup.Wait()

	// 5. Assert the active PTY state
	// In the buggy code: The late cleanup of the first spawn would have run AFTER the second
	// spawn registered itself, evicting terminal 999 completely from workspace.ptys.
	// In the fixed code: Terminal 999 must still exist and be intact.
	terminal, exists := workspace.ptys.Get(999)
	if !exists {
		t.Fatalf("Regression: terminal 999 was evicted from workspace.ptys by late cleanup of the first spawn")
	}

	if terminal.state != StateTerminated {
		t.Errorf("Expected terminal to be StateTerminated, got %v", terminal.state)
	}
}

func TestRemovePTYNonBlocking(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("non-blocking-remove-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("non-blocking-remove-ws")
	}()

	terminalID := uint16(888)
	err = workspace.SpawnPTY(terminalID, 80, 24, "sleep", "10")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	workspace.mutex.Lock()
	inst := workspace.spawning[terminalID]
	workspace.mutex.Unlock()

	start := time.Now()
	err = workspace.RemovePTY(terminalID)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Failed to remove spawning PTY: %v", err)
	}

	// RemovePTY must return immediately (non-blocking).
	// We assert it took less than 10 milliseconds to avoid scheduling variance.
	if duration > 10*time.Millisecond {
		t.Errorf("RemovePTY blocked for %v, expected it to return immediately (non-blocking)", duration)
	}

	// Wait for the spawning map to clear asynchronously and event-driven using inst.done
	if inst != nil {
		<-inst.done
	}

	workspace.mutex.Lock()
	_, spawning := workspace.spawning[terminalID]
	workspace.mutex.Unlock()
	if spawning {
		t.Errorf("Expected spawning to be cleared after inst.done closed")
	}
}

func TestPTYExitDrainRace(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("exit-drain-race-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("exit-drain-race-ws")
	}()

	terminalID := uint16(777)
	// Spawn a fast-exiting printf command to test that output is never truncated on exit
	err = workspace.SpawnPTY(terminalID, 80, 24, "printf", "exit-drain-race-success\n")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	term := waitPTYRegistered(workspace, terminalID)
	if term == nil {
		t.Fatalf("Failed to register PTY")
	}

	<-term.waitDone
	<-term.readDone

	term.mutex.Lock()
	output := term.buffer.Bytes()
	term.mutex.Unlock()

	expected := "exit-drain-race-success\r\n"
	if string(output) != expected {
		t.Errorf("Expected output %q, got %q (data truncated)", expected, string(output))
	}
}

func TestPTYDetachedDaemonExit(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("daemon-exit-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("daemon-exit-ws")
	}()

	terminalID := uint16(991)
	// Spawn a bash shell that launches a background sleep and exits immediately.
	// This tests that our exit handler correctly handles surviving daemon processes.
	err = workspace.SpawnPTY(terminalID, 80, 24, "/bin/bash", "-c", "sleep 100 & exit 0")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	term := waitPTYRegistered(workspace, terminalID)
	if term == nil {
		t.Fatalf("Failed to register PTY")
	}

	// The wait-goroutine must complete and close waitDone quickly without blocking on the daemon
	select {
	case <-term.waitDone:
		// Passed!
	case <-time.After(1 * time.Second):
		t.Errorf("Timeout waiting for waitDone (suspected block on daemon process)")
	}
}

func TestTakeoverHandshakeAtomicity(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("takeover-atomic-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("takeover-atomic-ws")
	}()

	// Verify that PerformReplayTakeover atomically binds the socketWriter
	writer := &mockSocketWriter{
		writeFunc: func(action uint16, terminalID uint16, payload []byte) error {
			return nil
		},
	}
	workspace.PerformReplayTakeover(writer)

	if workspace.GetSocketWriter() != writer {
		t.Errorf("Expected socketWriter to be bound to the mock writer under the single lock session")
	}
}
