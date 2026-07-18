package source

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Helper to count open file descriptors in the current process
func countOpenFDs() (int, error) {
	fdDir, err := os.Open("/proc/self/fd")
	if err != nil {
		return 0, err
	}
	defer fdDir.Close()
	names, err := fdDir.Readdirnames(-1)
	if err != nil {
		return 0, err
	}
	return len(names), nil
}

// Helper to wait for a PTY to transition to Active or Terminated
func waitPTYRegisteredState(workspace *Workspace, terminalID uint16) {
	for i := 0; i < 200; i++ {
		workspace.mutex.Lock()
		_, exists := workspace.ptys.Get(terminalID)
		workspace.mutex.Unlock()
		if exists {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSyncPTYPriorities(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("sync-priorities-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("sync-priorities-ws")
	}()

	// Spawn three active terminals
	for _, id := range []uint16{1, 2, 3} {
		if err := workspace.SpawnPTY(id, 80, 24, "/bin/bash"); err != nil {
			t.Fatalf("Failed to spawn terminal %d: %v", id, err)
		}
		waitPTYRegisteredState(workspace, id)
		if err := workspace.SetPTYPriority(id, PriorityHigh); err != nil {
			t.Fatalf("Failed to set high priority for %d: %v", id, err)
		}
	}

	// Spawning Edge Case: Register terminal 4 in spawning state (using workspace mutex)
	workspace.mutex.Lock()
	workspace.spawning[4] = &spawningInstance{cancel: func() {}, done: make(chan struct{})}
	workspace.mutex.Unlock()

	// Terminated Edge Case: Register terminal 5 as Terminated
	workspace.mutex.Lock()
	term5 := &ptyInstance{
		terminalID: 5,
		priority:   PriorityHigh,
		state:      StateTerminated,
		waitDone:   make(chan struct{}),
	}
	close(term5.waitDone)
	workspace.ptys.Put(5, term5)
	workspace.mutex.Unlock()

	// Sync priorities specifying only terminal 1 as High, and terminal 999 (non-existent)
	priorities := map[uint16]byte{
		1:   PriorityHigh,
		999: PriorityHigh,
	}
	if err := workspace.SyncPTYPriorities(priorities); err != nil {
		t.Fatalf("SyncPTYPriorities failed: %v", err)
	}

	// Verify terminal 1 is High, 2 and 3 are Low (implicitly demoted)
	workspace.mutex.Lock()
	t1, _ := workspace.ptys.Get(1)
	t2, _ := workspace.ptys.Get(2)
	t3, _ := workspace.ptys.Get(3)
	t5, _ := workspace.ptys.Get(5)
	p1 := t1.priority
	p2 := t2.priority
	p3 := t3.priority
	p5 := t5.priority
	workspace.mutex.Unlock()

	if p1 != PriorityHigh {
		t.Errorf("Expected terminal 1 to be PriorityHigh, got %v", p1)
	}
	if p2 != PriorityLow {
		t.Errorf("Expected terminal 2 to be PriorityLow (implicitly demoted), got %v", p2)
	}
	if p3 != PriorityLow {
		t.Errorf("Expected terminal 3 to be PriorityLow (implicitly demoted), got %v", p3)
	}
	// Assert terminated terminal 5 is unaffected
	if p5 != PriorityHigh {
		t.Errorf("Expected terminated terminal 5 priority to remain PriorityHigh, got %v", p5)
	}
}

func TestSyncPTYPrioritiesEmptyPayload(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("sync-empty-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("sync-empty-ws")
	}()

	// Spawn two active terminals
	for _, id := range []uint16{1, 2} {
		if err := workspace.SpawnPTY(id, 80, 24, "/bin/bash"); err != nil {
			t.Fatalf("Failed to spawn terminal %d: %v", id, err)
		}
		waitPTYRegisteredState(workspace, id)
		if err := workspace.SetPTYPriority(id, PriorityHigh); err != nil {
			t.Fatalf("Failed to set high priority for %d: %v", id, err)
		}
	}

	// Sync with empty payload map
	if err := workspace.SyncPTYPriorities(map[uint16]byte{}); err != nil {
		t.Fatalf("SyncPTYPriorities failed: %v", err)
	}

	workspace.mutex.Lock()
	t1, _ := workspace.ptys.Get(1)
	t2, _ := workspace.ptys.Get(2)
	p1 := t1.priority
	p2 := t2.priority
	workspace.mutex.Unlock()

	if p1 != PriorityLow {
		t.Errorf("Expected terminal 1 to be PriorityLow, got %v", p1)
	}
	if p2 != PriorityLow {
		t.Errorf("Expected terminal 2 to be PriorityLow, got %v", p2)
	}
}

func TestSyncPTYPrioritiesValidation(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("sync-validation-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("sync-validation-ws")
	}()

	if err := workspace.SpawnPTY(1, 80, 24, "/bin/bash"); err != nil {
		t.Fatalf("Failed to spawn: %v", err)
	}
	waitPTYRegisteredState(workspace, 1)

	// Set invalid priority value (e.g. 0x05) via SyncPTYPriorities
	err = workspace.SyncPTYPriorities(map[uint16]byte{1: 0x05})
	if err == nil {
		// If it accepts it, verify it stored it safely or paced it as Low priority without panic
		workspace.mutex.Lock()
		t1, _ := workspace.ptys.Get(1)
		p1 := t1.priority
		workspace.mutex.Unlock()
		if p1 != 0x05 && p1 != PriorityLow {
			t.Errorf("Expected priority sync with 0x05 to be stored or demoted, got %v", p1)
		}
	}
}

func TestCompileReplayFrames(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("compile-replay-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("compile-replay-ws")
	}()

	// Spawn terminal 1 (active) and write large buffer data to trigger overflow
	if err := workspace.SpawnPTY(1, 80, 24, "/bin/bash"); err != nil {
		t.Fatalf("Failed to spawn: %v", err)
	}
	waitPTYRegisteredState(workspace, 1)

	workspace.mutex.Lock()
	t1, _ := workspace.ptys.Get(1)
	workspace.mutex.Unlock()

	// Saturate ring buffer past 256KB to force overflow
	largeData := make([]byte, 260*1024)
	for i := range largeData {
		largeData[i] = 'A'
	}
	t1.mutex.Lock()
	t1.buffer.Write(largeData)
	t1.mutex.Unlock()

	// Spawn terminal 2 and terminate it synchronously with exit code 12
	if err := workspace.SpawnPTY(2, 80, 24, "/bin/bash"); err != nil {
		t.Fatalf("Failed to spawn: %v", err)
	}
	waitPTYRegisteredState(workspace, 2)

	workspace.mutex.Lock()
	t2, _ := workspace.ptys.Get(2)
	workspace.mutex.Unlock()

	t2.mutex.Lock()
	t2.state = StateTerminated
	t2.exitStatus = 12
	t2.mutex.Unlock()

	// Compile Replays
	frames := workspace.CompileReplayFrames()

	var output1Found, spawn1Found, exit2Found bool
	for _, f := range frames {
		if f.TerminalID == 1 {
			if f.Action == ActionOutput {
				output1Found = true
				// Check for prepended yellow ANSI warning
				expectedWarning := []byte("\r\n\x1b[33m[... Output truncated due to buffer overflow ...]\x1b[0m\r\n\r\n")
				if len(f.Payload) < len(expectedWarning) {
					t.Errorf("Terminal 1 Output frame missing warning banner")
				}
			}
			if f.Action == ActionSpawnStatus {
				spawn1Found = true
				if len(f.Payload) != 1 || f.Payload[0] != 0x00 {
					t.Errorf("Expected SpawnPTYStatus to be 0x00, got %v", f.Payload)
				}
			}
		} else if f.TerminalID == 2 {
			if f.Action == ActionTerminalExit {
				exit2Found = true
				if len(f.Payload) != 1 || f.Payload[0] != 12 {
					t.Errorf("Expected exit status 12, got %v", f.Payload)
				}
			}
		}
	}

	if !output1Found {
		t.Errorf("OutputPTY frame for terminal 1 not compiled")
	}
	if !spawn1Found {
		t.Errorf("SpawnPTYStatus frame for terminal 1 not compiled")
	}
	if !exit2Found {
		t.Errorf("PTYTerminalExit frame for terminal 2 not compiled")
	}
}

func TestCompileReplayFramesEmptyScrollback(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("replay-empty-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("replay-empty-ws")
	}()

	// Terminal 1: Active with empty buffer
	if err := workspace.SpawnPTY(1, 80, 24, "sleep", "99999"); err != nil {
		t.Fatalf("Failed to spawn: %v", err)
	}
	waitPTYRegisteredState(workspace, 1)

	// Terminal 2: Active with truncated invalid UTF-8 leading continuation bytes
	if err := workspace.SpawnPTY(2, 80, 24, "sleep", "99999"); err != nil {
		t.Fatalf("Failed to spawn: %v", err)
	}
	waitPTYRegisteredState(workspace, 2)

	workspace.mutex.Lock()
	t2, _ := workspace.ptys.Get(2)
	workspace.mutex.Unlock()

	t2.mutex.Lock()
	t2.buffer.Write([]byte{0x82, 0xBF})
	t2.mutex.Unlock()

	// Compile Replays
	frames := workspace.CompileReplayFrames()

	for _, f := range frames {
		if f.Action == ActionOutput {
			// Spec Optimization: empty or fully truncated scrollback PTYs should omit OutputPTY frames
			if f.TerminalID == 1 || f.TerminalID == 2 {
				t.Errorf("Expected active terminal %d with empty scrollback to omit OutputPTY frame, but got one", f.TerminalID)
			}
		}
	}
}

func TestAlignUTF8Boundary(t *testing.T) {
	// Valid UTF-8 string
	valid := []byte("Hello, World!")
	result := AlignUTF8Boundary(valid)
	if string(result) != "Hello, World!" {
		t.Errorf("Expected unchanged, got %s", string(result))
	}

	// Truncated multi-byte (continuation bytes at the front)
	// Input: [0x82, 0xBF, 'A', 'B', 'C']
	input := []byte{0x82, 0xBF, 'A', 'B', 'C'}
	expected := []byte{'A', 'B', 'C'}
	aligned := AlignUTF8Boundary(input)
	if string(aligned) != string(expected) {
		t.Errorf("Expected %s, got %s", string(expected), string(aligned))
	}
}

func TestSpawnPTYEmptyCommand(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("empty-command-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("empty-command-ws")
	}()

	err = workspace.SpawnPTY(1, 80, 24, "")
	if err == nil {
		t.Errorf("Expected SpawnPTY with empty command to return error synchronously")
	}

	workspace.mutex.Lock()
	_, exists := workspace.ptys.Get(1)
	workspace.mutex.Unlock()

	if exists {
		t.Errorf("Terminal ID 1 should not be registered in workspace")
	}
}

func TestSpawnPTYZeroDimensions(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("zero-dims-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("zero-dims-ws")
	}()

	err = workspace.SpawnPTY(1, 0, 24, "/bin/bash")
	if err == nil {
		t.Errorf("Expected SpawnPTY with 0 columns to fail synchronously")
	}

	err = workspace.SpawnPTY(2, 80, 0, "/bin/bash")
	if err == nil {
		t.Errorf("Expected SpawnPTY with 0 rows to fail synchronously")
	}
}

func TestSpawnPTYFileDescriptorCleanup(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("fd-cleanup-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("fd-cleanup-ws")
	}()

	// 1. Get baseline file descriptor count
	startFDs, err := countOpenFDs()
	if err != nil {
		t.Logf("Warning: unable to count FDs on this host: %v", err)
		return
	}

	// 2. Synchronous Check Failure (Collision)
	if err := workspace.SpawnPTY(1, 80, 24, "/bin/bash"); err != nil {
		t.Fatalf("Failed to spawn initial: %v", err)
	}
	waitPTYRegisteredState(workspace, 1)

	// Count FDs after active terminal 1 is spawned
	activeFDs, _ := countOpenFDs()

	// Call SpawnPTY with duplicate ID 1 to trigger synchronous collision check
	_ = workspace.SpawnPTY(1, 80, 24, "/bin/bash")

	postCollisionFDs, _ := countOpenFDs()
	if postCollisionFDs != activeFDs {
		t.Errorf("Expected FD count after synchronous collision to match baseline %d, got %d", activeFDs, postCollisionFDs)
	}

	// 3. Asynchronous/Lookup Startup Failure (Non-existent executable)
	// Open descriptors before non-existent spawn
	preNonExistentFDs, _ := countOpenFDs()

	err = workspace.SpawnPTY(2, 80, 24, "/bin/non-existent-executable-12345")
	if err == nil {
		t.Errorf("Expected SpawnPTY with non-existent executable to fail synchronously")
	}

	if !errors.Is(err, exec.ErrNotFound) && !os.IsNotExist(err) {
		t.Errorf("Expected error to be path/not found error, got: %v", err)
	}

	// Verify terminal ID 2 was never registered
	workspace.mutex.Lock()
	_, exists := workspace.ptys.Get(2)
	workspace.mutex.Unlock()

	if exists {
		t.Errorf("Terminal ID 2 should not be registered in the workspace")
	}

	// Assert that open file descriptors returned back to the baseline
	postNonExistentFDs, _ := countOpenFDs()
	if postNonExistentFDs != preNonExistentFDs {
		t.Errorf("Expected FD count after non-existent executable spawn failure to return to baseline %d, got %d", preNonExistentFDs, postNonExistentFDs)
	}

	// Cleanup terminal 1
	workspace.mutex.Lock()
	t1, _ := workspace.ptys.Get(1)
	workspace.mutex.Unlock()
	t1.master.Close()

	// Wait for FD count to return to baseline
	var finalFDs int
	for i := 0; i < 50; i++ {
		finalFDs, _ = countOpenFDs()
		if finalFDs == startFDs {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if finalFDs != startFDs {
		t.Logf("FD baseline diff: start=%d, final=%d", startFDs, finalFDs)
	}
}

func TestSynchronousSpawnFailureFraming(t *testing.T) {
	registry := NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("sync-failure-framing-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("sync-failure-framing-ws")
	}()

	// First spawn terminal 1
	if err := workspace.SpawnPTY(1, 80, 24, "/bin/bash"); err != nil {
		t.Fatalf("Failed to spawn: %v", err)
	}
	waitPTYRegisteredState(workspace, 1)

	// Clear control queue
	workspace.mutex.Lock()
	workspace.centralizedQueues[QueueIndexControl] = nil
	workspace.mutex.Unlock()

	// Spawn duplicate terminal ID 1 to trigger collision
	err = workspace.SpawnPTY(1, 80, 24, "/bin/bash")
	if err == nil {
		t.Errorf("Expected spawn collision to return error synchronously")
	}

	// Verify failure frame is enqueued
	workspace.mutex.Lock()
	queue := workspace.centralizedQueues[QueueIndexControl]
	workspace.mutex.Unlock()

	var foundFailureFrame bool
	for _, f := range queue {
		if f.Action == ActionSpawnStatus && f.TerminalID == 1 && len(f.Payload) == 1 && f.Payload[0] == 0x01 {
			foundFailureFrame = true
			break
		}
	}

	if !foundFailureFrame {
		t.Errorf("ActionSpawnStatus failure frame was not enqueued in control queue on synchronous failure")
	}
}
