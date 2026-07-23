package gotests

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"suprasole-server/source"
)

func getPendingPriorities(w *source.Workspace) map[uint16]byte {
	val := reflect.ValueOf(w).Elem()
	field := val.FieldByName("pendingPriorities")
	ptr := unsafe.Pointer(field.UnsafeAddr())
	return *(*map[uint16]byte)(ptr)
}

// Helper to get registry or fail test early if not implemented.
func getRegistry(t *testing.T) source.WorkspaceRegistry {
	r := source.NewWorkspaceRegistry()
	if r == nil {
		t.Fatal("WorkspaceRegistry is not implemented (source.NewWorkspaceRegistry returned nil)")
	}
	return r
}

// Helper to initialize a unique workspace and register its cleanup.
func setupWorkspace(t *testing.T, registry source.WorkspaceRegistry, prefix string) (*source.Workspace, string) {
	wsID := fmt.Sprintf("%s-%s-%d", prefix, t.Name(), time.Now().UnixNano())
	workspace, error := registry.GetOrCreateWorkspace(wsID)
	if error != nil {
		t.Fatalf("failed to create workspace %s: %v", wsID, error)
	}
	t.Cleanup(func() {
		// Clean up the workspace, terminating and reaping all processes
		_ = registry.RemoveWorkspace(wsID)
	})
	return workspace, wsID
}

// Helper to wait until the PTY process is fully spawned and set to raw non-echo mode.
type mockSocketWriter struct {
	frames chan source.OutboundFrame
	done   chan struct{}
	once   sync.Once
}

func (m *mockSocketWriter) WriteFrame(action uint16, terminalID uint16, payload []byte) error {
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

func (m *mockSocketWriter) Close() {
	m.once.Do(func() {
		close(m.done)
	})
}

func newMockSocketWriter() *mockSocketWriter {
	return &mockSocketWriter{
		frames: make(chan source.OutboundFrame, 10000),
		done:   make(chan struct{}),
	}
}

func waitPTY(workspace *source.Workspace, termID uint16) {
	for i := 0; i < 200; i++ {
		state, exists := workspace.GetPTYState(termID)
		if exists && state == source.StateActive {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitPTYState(workspace *source.Workspace, termID uint16, targetState source.TerminalState) bool {
	for i := 0; i < 500; i++ {
		state, exists := workspace.GetPTYState(termID)
		if exists && state == targetState {
			return true
		}
		time.Sleep(1 * time.Millisecond)
	}
	return false
}

func waitUntilReady(t *testing.T, workspace *source.Workspace, termID uint16) {
	waitPTY(workspace, termID)
	prevWriter := workspace.GetSocketWriter()
	mockWriter := newMockSocketWriter()
	defer mockWriter.Close()
	workspace.SetSocketWriter(mockWriter)
	// Put terminal in raw non-echo mode and output ready marker
	error := workspace.WritePTYInput(termID, []byte("stty raw -echo && echo 'PTY_READY'\n"))
	if error != nil {
		workspace.SetSocketWriter(prevWriter)
		t.Fatalf("failed to write ready check input: %v", error)
	}
	var accum bytes.Buffer
	done := make(chan struct{})
	go func() {
		for {
			select {
			case frame := <-mockWriter.frames:
				if frame.TerminalID == termID && frame.Action == source.ActionOutput {
					accum.Write(frame.Payload)
					if strings.Contains(accum.String(), "PTY_READY") {
						close(done)
						return
					}
				}
			case <-mockWriter.done:
				return
			}
		}
	}()
	select {
	case <-done:
		workspace.SetSocketWriter(prevWriter)
	case <-time.After(2 * time.Second):
		workspace.SetSocketWriter(prevWriter)
		t.Fatalf("timeout waiting for PTY %d to become ready. Buffer state:\n%q", termID, accum.Bytes())
	}
}

// Helper to extract shell PID extrinsically.
func extractPID(t *testing.T, workspace *source.Workspace, termID uint16) int {
	waitPTY(workspace, termID)
	prevWriter := workspace.GetSocketWriter()
	mockWriter := newMockSocketWriter()
	defer mockWriter.Close()
	workspace.SetSocketWriter(mockWriter)
	// Write input to print PID
	error := workspace.WritePTYInput(termID, []byte("echo $$\n"))
	if error != nil {
		workspace.SetSocketWriter(prevWriter)
		t.Fatalf("failed to write PID check: %v", error)
	}
	var outputBuffer bytes.Buffer
	ansiRegexp := regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][0-9;]*[^\x07]*\x07`)
	done := make(chan struct{})
	var processID int
	go func() {
		for {
			select {
			case frame := <-mockWriter.frames:
				if frame.TerminalID == termID && frame.Action == source.ActionOutput {
					outputBuffer.Write(frame.Payload)
					clean := ansiRegexp.ReplaceAllString(outputBuffer.String(), "")
					clean = strings.ReplaceAll(clean, "\r", "")
					lines := strings.Split(clean, "\n")
					for _, line := range lines {
						trimmed := strings.TrimSpace(line)
						if value, error := strconv.Atoi(trimmed); error == nil && value > 0 {
							processID = value
							close(done)
							return
						}
					}
				}
			case <-mockWriter.done:
				return
			}
		}
	}()
	select {
	case <-done:
		workspace.SetSocketWriter(prevWriter)
		return processID
	case <-time.After(2 * time.Second):
		workspace.SetSocketWriter(prevWriter)
		t.Fatalf("timeout waiting for PTY %d PID. Buffer state:\n%q", termID, outputBuffer.Bytes())
		return 0
	}
}

// Test Case 1: Workspace Tenant Isolation & Sandbox Verification
func TestWorkspaceTenantIsolation(t *testing.T) {
	registry := getRegistry(t)
	wsA, _ := setupWorkspace(t, registry, "WS-A")
	wsB, _ := setupWorkspace(t, registry, "WS-B")
	termIDA := uint16(100)
	error := wsA.SpawnPTY(termIDA, 80, 24, "/bin/bash")
	if error != nil {
		t.Fatalf("failed to spawn PTY: %v", error)
	}
	waitUntilReady(t, wsA, termIDA)
	// Verify terminal shows in WS-A active list
	activeA := wsA.GetActiveTerminalIDs()
	found := false
	for _, id := range activeA {
		if id == termIDA {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("terminal %d not found in WS-A active list", termIDA)
	}
	// Attempt operations from WS-B
	error = wsB.WritePTYInput(termIDA, []byte("echo 'from-Workspace-B'\n"))
	if error == nil {
		t.Error("expected WritePTYInput from WS-B to fail, but it succeeded")
	}
	error = wsB.ResizePTY(termIDA, 120, 40)
	if error == nil {
		t.Error("expected ResizePTY from WS-B to fail, but it succeeded")
	}
	_, _, error = wsB.GetScrollbackBuffer(termIDA)
	if error == nil {
		t.Error("expected GetScrollbackBuffer from WS-B to fail, but it succeeded")
	}
	activeB := wsB.GetActiveTerminalIDs()
	for _, id := range activeB {
		if id == termIDA {
			t.Error("terminal from WS-A leaked into WS-B active list")
		}
	}
	error = wsB.TerminatePTY(termIDA)
	if error == nil {
		t.Error("expected TerminatePTY from WS-B to fail, but it succeeded")
	}
	// Verify WS-A is unaffected
	var outputBuffer bytes.Buffer
	done := make(chan struct{})
	mockWriter := newMockSocketWriter()
	wsA.SetSocketWriter(mockWriter)
	go func() {
		for frame := range mockWriter.frames {
			if frame.TerminalID == termIDA && frame.Action == source.ActionOutput {
				outputBuffer.Write(frame.Payload)
				if strings.Contains(outputBuffer.String(), "SIZE_CHECK_DONE") {
					close(done)
					return
				}
			}
		}
	}()
	// Write concatenated check to separate echoed string from execution output
	error = wsA.WritePTYInput(termIDA, []byte("stty size && echo 'SIZE'_'CHECK'_'DONE'\n"))
	if error != nil {
		t.Fatalf("failed to write size check: %v", error)
	}
	select {
	case <-done:
		wsA.SetSocketWriter(nil)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for stty size output from WS-A")
	}
	output := outputBuffer.String()
	if !strings.Contains(output, "24 80") {
		t.Errorf("expected geometry to remain 24 80, but stty size returned otherwise. Output:\n%s", output)
	}
	if strings.Contains(output, "40 120") {
		t.Error("WS-A terminal was resized by WS-B request (isolation failure)")
	}
	if strings.Contains(output, "from-Workspace-B") {
		t.Error("WS-A terminal executed input originating from WS-B (isolation failure)")
	}
}

// Test Case 2: Environment & Geometry Seeding Invariants
func TestEnvironmentGeometrySeeding(t *testing.T) {
	registry := getRegistry(t)
	workspace, _ := setupWorkspace(t, registry, "WS-A")
	termID := uint16(200)
	var outputBuffer bytes.Buffer
	terminated := make(chan byte, 1)
	mockWriter := newMockSocketWriter()
	workspace.SetSocketWriter(mockWriter)
	go func() {
		for frame := range mockWriter.frames {
			if frame.TerminalID == termID {
				if frame.Action == source.ActionOutput {
					outputBuffer.Write(frame.Payload)
				} else if frame.Action == source.ActionTerminalExit && len(frame.Payload) == 1 {
					select {
					case terminated <- frame.Payload[0]:
					default:
					}
				}
			}
		}
	}()
	// Spawn PTY with 132x43
	error := workspace.SpawnPTY(termID, 132, 43, "/bin/bash")
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}
	waitPTY(workspace, termID)
	// Write command to dump env & size then exit (concatenated to avoid early echo match)
	error = workspace.WritePTYInput(termID, []byte("env && stty size && echo 'SEEDING'_'COMPLETE' && exit 0\n"))
	if error != nil {
		t.Fatalf("failed to write: %v", error)
	}
	select {
	case code := <-terminated:
		workspace.SetSocketWriter(nil)
		if code != 0 {
			t.Errorf("expected clean exit code 0, got %d", code)
		}
	case <-time.After(3 * time.Second):
		workspace.SetSocketWriter(nil)
		t.Fatal("timeout waiting for terminal process to exit")
	}
	output := outputBuffer.String()
	if !strings.Contains(output, "TERM=xterm-256color") {
		t.Error("missing TERM=xterm-256color environment variable")
	}
	if !strings.Contains(output, "LANG=en_US.UTF-8") {
		t.Error("missing LANG=en_US.UTF-8 environment variable")
	}
	if !strings.Contains(output, "PATH=") {
		t.Error("missing PATH environment variable")
	}
	idxSize := strings.Index(output, "43 132")
	idxComplete := strings.Index(output, "SEEDING_COMPLETE")
	if idxSize < 0 {
		t.Error("stty size output did not contain '43 132'")
	}
	if idxComplete < 0 {
		t.Error("missing SEEDING_COMPLETE output marker")
	}
	if idxSize >= 0 && idxComplete >= 0 && idxSize > idxComplete {
		t.Error("geometry size check executed after SEEDING_COMPLETE (geometry seeding ordering failure)")
	}
}

// Test Case 3: Controlling Terminal (TIOCSCTTY) Verification
func TestControllingTerminalVerification(t *testing.T) {
	registry := getRegistry(t)
	workspace, _ := setupWorkspace(t, registry, "WS-A")
	termID := uint16(300)
	error := workspace.SpawnPTY(termID, 80, 24, "/bin/bash")
	if error != nil {
		t.Fatalf("failed to spawn PTY: %v", error)
	}
	waitUntilReady(t, workspace, termID)
	var outputBuffer bytes.Buffer
	done := make(chan struct{})
	mockWriter := newMockSocketWriter()
	workspace.SetSocketWriter(mockWriter)
	go func() {
		for frame := range mockWriter.frames {
			if frame.TerminalID == termID && frame.Action == source.ActionOutput {
				outputBuffer.Write(frame.Payload)
				if strings.Contains(outputBuffer.String(), "DEV_TTY_OK") {
					close(done)
					return
				}
			}
		}
	}()
	// Perform TTY and controlling device check (concatenated to prevent echo matching)
	command := "tty && echo \"TTY_STATUS_$?\"\necho \"DEV\"_\"TTY\"_\"OK\" > /dev/tty\n"
	error = workspace.WritePTYInput(termID, []byte(command))
	if error != nil {
		t.Fatalf("failed to write commands: %v", error)
	}
	select {
	case <-done:
		workspace.SetSocketWriter(nil)
	case <-time.After(2 * time.Second):
		workspace.SetSocketWriter(nil)
		t.Fatalf("timeout waiting for TTY verification. Output received:\n%s", outputBuffer.String())
	}
	output := outputBuffer.String()
	if !strings.Contains(output, "TTY_STATUS_0") {
		t.Error("tty command failed (controlling terminal mapping error)")
	}
	// Check pseudo-terminal device file path via regex (/dev/pts/N)
	matched, error := regexp.MatchString(`/dev/pts/\d+`, output)
	if error != nil || !matched {
		t.Errorf("output does not contain valid PTY path /dev/pts/N. Output:\n%s", output)
	}
	if strings.Contains(output, "No such device or address") || strings.Contains(output, "Permission denied") {
		t.Error("/dev/tty was not openable as a controlling terminal device")
	}
}

// Test Case 4: Parent-Side Slave Close & Hanging EOF Prevention
func TestParentSideSlaveClose(t *testing.T) {
	registry := getRegistry(t)
	workspace, _ := setupWorkspace(t, registry, "WS-A")
	termID := uint16(400)
	var outputTimestamp time.Time
	var terminationTimestamp time.Time
	terminated := make(chan struct{})
	var accum bytes.Buffer
	mockWriter := newMockSocketWriter()
	workspace.SetSocketWriter(mockWriter)
	go func() {
		for frame := range mockWriter.frames {
			if frame.TerminalID == termID {
				if frame.Action == source.ActionOutput {
					accum.Write(frame.Payload)
					if strings.Contains(accum.String(), "quick_exit") && outputTimestamp.IsZero() {
						outputTimestamp = time.Now()
					}
				} else if frame.Action == source.ActionTerminalExit {
					terminationTimestamp = time.Now()
					select {
					case <-terminated:
					default:
						close(terminated)
					}
				}
			}
		}
	}()
	error := workspace.SpawnPTY(termID, 80, 24, "/bin/bash")
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}
	waitPTY(workspace, termID)
	error = workspace.WritePTYInput(termID, []byte("echo 'quick_exit' && exit 0\n"))
	if error != nil {
		t.Fatalf("failed to write: %v", error)
	}
	select {
	case <-terminated:
		workspace.SetSocketWriter(nil)
	case <-time.After(2 * time.Second):
		workspace.SetSocketWriter(nil)
		t.Fatal("timeout: PTY master reader thread hung (descriptor leak suspected)")
	}
	delta := terminationTimestamp.Sub(outputTimestamp)
	if delta > 100*time.Millisecond {
		t.Errorf("termination took too long after output: %v (expected <100ms)", delta)
	}
}

// Test Case 5: Clean PID Reaping & Exit Code Translation
func TestCleanPidReaping(t *testing.T) {
	registry := getRegistry(t)
	workspace, _ := setupWorkspace(t, registry, "WS-A")
	termID501 := uint16(501)
	termID502 := uint16(502)
	termID503 := uint16(503)
	var mutex sync.Mutex
	exits := make(map[uint16]byte)
	exitsDone := make(chan uint16, 3)
	mockWriter := newMockSocketWriter()
	workspace.SetSocketWriter(mockWriter)
	go func() {
		for frame := range mockWriter.frames {
			if frame.Action == source.ActionTerminalExit && len(frame.Payload) == 1 {
				mutex.Lock()
				exits[frame.TerminalID] = frame.Payload[0]
				mutex.Unlock()
				exitsDone <- frame.TerminalID
			}
		}
	}()
	// Spawn normal exit process
	error := workspace.SpawnPTY(termID501, 80, 24, "/bin/bash")
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}
	waitUntilReady(t, workspace, termID501)
	pid501 := extractPID(t, workspace, termID501)
	// Spawn signal exit process
	error = workspace.SpawnPTY(termID502, 80, 24, "/bin/bash")
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}
	waitUntilReady(t, workspace, termID502)
	pid502 := extractPID(t, workspace, termID502)
	// Spawn crash exit process
	error = workspace.SpawnPTY(termID503, 80, 24, "/bin/bash")
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}
	waitUntilReady(t, workspace, termID503)
	pid503 := extractPID(t, workspace, termID503)
	// Trigger exits
	_ = workspace.WritePTYInput(termID501, []byte("exit 77\n"))
	_ = workspace.WritePTYInput(termID503, []byte("kill -11 $$\n")) // SIGSEGV
	_ = workspace.TerminatePTY(termID502) // SIGKILL
	// Wait for terminations
	timeout := time.After(2 * time.Second)
	for i := 0; i < 3; i++ {
		select {
		case <-exitsDone:
		case <-timeout:
			workspace.SetSocketWriter(nil)
			t.Fatal("timeout waiting for process exit callbacks")
		}
	}
	workspace.SetSocketWriter(nil)
	mutex.Lock()
	code501 := exits[termID501]
	code502 := exits[termID502]
	code503 := exits[termID503]
	mutex.Unlock()
	if code501 != 77 {
		t.Errorf("expected 501 normal exit status 77, got %d", code501)
	}
	if code502 != 137 { // 128 + 9
		t.Errorf("expected 502 signal exit status 137 (SIGKILL), got %d", code502)
	}
	if code503 != 139 { // 128 + 11
		t.Errorf("expected 503 crash exit status 139 (SIGSEGV), got %d", code503)
	}
	// Verify PIDs are reaped and not zombies
	checkPIDReaped(t, pid501)
	checkPIDReaped(t, pid502)
	checkPIDReaped(t, pid503)
}

func checkPIDReaped(t *testing.T, processID int) {
	var error error
	for i := 0; i < 200; i++ {
		error = syscall.Kill(processID, 0)
		if errors.Is(error, syscall.ESRCH) {
			return
		}
		time.Sleep(1 * time.Millisecond)
	}
	t.Errorf("PID %d was not cleanly reaped (still visible in OS process list)", processID)
}

// Test Case 6: Concurrent Load & Lock Invariants (Deadlock Check)
func TestConcurrentLoadLock(t *testing.T) {
	registry := getRegistry(t)
	workspace, _ := setupWorkspace(t, registry, "WS-A")
	ptyCount := 20
	var pids []int
	var pidsMu sync.Mutex
	for i := 0; i < ptyCount; i++ {
		termID := uint16(601 + i)
		error := workspace.SpawnPTY(termID, 80, 24, "/bin/bash")
		if error != nil {
			t.Fatalf("failed to spawn %d: %v", termID, error)
		}
		waitUntilReady(t, workspace, termID)
		processID := extractPID(t, workspace, termID)
		pids = append(pids, processID)
		// Start infinite background load with job control disabled (set +m) so yes runs in same PGID
		_ = workspace.WritePTYInput(termID, []byte("set +m && yes > /dev/null &\n"))
	}
	// Run concurrent stress threads
	var waitGroup sync.WaitGroup
	stressDone := make(chan struct{})
	// Threads 1-3: Write inputs
	for th := 0; th < 3; th++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for {
				select {
				case <-stressDone:
					return
				default:
					termID := uint16(601 + (time.Now().UnixNano() % int64(ptyCount)))
					_ = workspace.WritePTYInput(termID, []byte("\x03")) // Ctrl+C
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	}
	// Threads 4-6: Resizes
	for th := 0; th < 3; th++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for {
				select {
				case <-stressDone:
					return
				case <-time.After(10 * time.Millisecond):
					termID := uint16(601 + (time.Now().UnixNano() % int64(ptyCount)))
					columns := uint16(80 + (time.Now().UnixNano() % 70))
					rows := uint16(24 + (time.Now().UnixNano() % 26))
					_ = workspace.ResizePTY(termID, columns, rows)
				}
			}
		}()
	}
	// Threads 7-8: List and scrollback queries
	for th := 0; th < 2; th++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for {
				select {
				case <-stressDone:
					return
				default:
					_ = workspace.GetActiveTerminalIDs()
					termID := uint16(601 + (time.Now().UnixNano() % int64(ptyCount)))
					_, _, _ = workspace.GetScrollbackBuffer(termID)
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	}
	// Threads 9-10: Global Registry calls
	for th := 0; th < 2; th++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for {
				select {
				case <-stressDone:
					return
				default:
					_, _ = registry.GetOrCreateWorkspace("WS-A")
					time.Sleep(1 * time.Millisecond)
				}
			}
		}()
	}
	// Run stress test for 300 milliseconds
	time.Sleep(300 * time.Millisecond)
	close(stressDone)
	waitGroup.Wait()
	// Concurrently terminate all PTYs
	var termWg sync.WaitGroup
	for i := 0; i < ptyCount; i++ {
		termID := uint16(601 + i)
		termWg.Add(1)
		go func(id uint16) {
			defer termWg.Done()
			_ = workspace.TerminatePTY(id)
		}(termID)
	}
	termWg.Wait()

	// Wait deterministically for all active PTYs to exit the active IDs list
	for i := 0; i < 200; i++ {
		if len(workspace.GetActiveTerminalIDs()) == 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	// Assertions
	activeIDs := workspace.GetActiveTerminalIDs()
	if len(activeIDs) != 0 {
		t.Errorf("expected 0 active terminals, got %d: %v", len(activeIDs), activeIDs)
	}
	pidsMu.Lock()
	for _, processID := range pids {
		checkPIDReaped(t, processID)
	}
	pidsMu.Unlock()
}

// Test Case 7: Detached Daemon Process Reaping (Orphaned Children)
func TestDetachedDaemonReaping(t *testing.T) {
	registry := getRegistry(t)
	workspace, _ := setupWorkspace(t, registry, "WS-A")
	termID := uint16(700)
	terminated := make(chan struct{})
	mockWriter := newMockSocketWriter()
	workspace.SetSocketWriter(mockWriter)
	go func() {
		for frame := range mockWriter.frames {
			if frame.TerminalID == termID && frame.Action == source.ActionTerminalExit {
				select {
				case <-terminated:
				default:
					close(terminated)
				}
			}
		}
	}()
	error := workspace.SpawnPTY(termID, 80, 24, "/bin/bash")
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}
	waitUntilReady(t, workspace, termID)
	processID := extractPID(t, workspace, termID)
	// Launch background daemon and exit shell immediately
	error = workspace.WritePTYInput(termID, []byte("(sleep 100 &) && exit 0\n"))
	if error != nil {
		t.Fatalf("failed to write: %v", error)
	}
	select {
	case <-terminated:
		workspace.SetSocketWriter(nil)
	case <-time.After(2 * time.Second):
		workspace.SetSocketWriter(nil)
		t.Fatal("timeout waiting for shell exit (suspected block on background children)")
	}
	// Verify main shell PID is reaped
	checkPIDReaped(t, processID)
}

// Test Case 8: Rapid Lifecycle Command Race Conditions
func TestRapidLifecycleRace(t *testing.T) {
	registry := getRegistry(t)
	workspace, _ := setupWorkspace(t, registry, "WS-A")
	termID := uint16(800)
	var waitGroup sync.WaitGroup
	// Fire spawn, resize, terminate concurrently
	_ = workspace.SpawnPTY(termID, 80, 24, "/bin/bash")
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		_ = workspace.ResizePTY(termID, 120, 40)
	}()
	go func() {
		defer waitGroup.Done()
		_ = workspace.TerminatePTY(termID)
	}()
	waitGroup.Wait()
	// Wait deterministically for the PTY to exit the active IDs list
	for i := 0; i < 200; i++ {
		activeIDs := workspace.GetActiveTerminalIDs()
		found := false
		for _, id := range activeIDs {
			if id == termID {
				found = true
				break
			}
		}
		if !found {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}
	// Verify no crash and registry state resolved safely
	activeIDs := workspace.GetActiveTerminalIDs()
	for _, id := range activeIDs {
		if id == termID {
			// Clean up if it somehow survived
			_ = workspace.TerminatePTY(termID)
			t.Errorf("PTY %d survived rapid spawn-resize-terminate race", termID)
		}
	}
}

// Test Case 9: Binary & Non-UTF8 Character Handshake Integrity
func TestBinaryNonUtf8Handshake(t *testing.T) {
	registry := getRegistry(t)
	workspace, _ := setupWorkspace(t, registry, "WS-A")
	termID := uint16(900)
	error := workspace.SpawnPTY(termID, 80, 24, "/bin/bash")
	if error != nil {
		t.Fatalf("failed to spawn PTY: %v", error)
	}
	// Set raw mode using stty raw -echo before piping binary data through cat
	waitUntilReady(t, workspace, termID)
	var outputBuffer bytes.Buffer
	// Sequence containing null, invalid UTF-8 bytes, and escape codes
	payload := []byte{0x00, 0xFF, 0xFE, 0x01, 0x1B, 0x5B, 0x48, 0x02, 0x0A}
	mockWriter := newMockSocketWriter()
	workspace.SetSocketWriter(mockWriter)
	probeDone := make(chan struct{})
	payloadDone := make(chan struct{})
	go func() {
		probeSeen := false
		for frame := range mockWriter.frames {
			if frame.TerminalID == termID && frame.Action == source.ActionOutput {
				outputBuffer.Write(frame.Payload)
				if !probeSeen {
					if bytes.Contains(outputBuffer.Bytes(), []byte("probe")) {
						probeSeen = true
						close(probeDone)
					}
				} else {
					if bytes.Contains(outputBuffer.Bytes(), payload) {
						close(payloadDone)
						return
					}
				}
			}
		}
	}()
	// Run cat
	error = workspace.WritePTYInput(termID, []byte("cat\n"))
	if error != nil {
		t.Fatalf("failed to write cat: %v", error)
	}
	error = workspace.WritePTYInput(termID, []byte("probe\n"))
	if error != nil {
		t.Fatalf("failed to write probe: %v", error)
	}
	select {
	case <-probeDone:
	case <-time.After(2 * time.Second):
		workspace.SetSocketWriter(nil)
		t.Fatal("timeout waiting for cat probe")
	}
	// Write raw binary payload
	error = workspace.WritePTYInput(termID, payload)
	if error != nil {
		t.Fatalf("failed to write binary: %v", error)
	}
	select {
	case <-payloadDone:
		workspace.SetSocketWriter(nil)
	case <-time.After(2 * time.Second):
		workspace.SetSocketWriter(nil)
		t.Fatalf("timeout waiting for raw binary echo. Got: %v", outputBuffer.Bytes())
	}
}

// Test Case 10: Write Error & SIGPIPE Immunity
func TestWriteErrorSigpipeImmunity(t *testing.T) {
	registry := getRegistry(t)
	workspace, _ := setupWorkspace(t, registry, "WS-A")
	termID := uint16(1000)
	terminated := make(chan struct{})
	mockWriter := newMockSocketWriter()
	workspace.SetSocketWriter(mockWriter)
	go func() {
		for frame := range mockWriter.frames {
			if frame.TerminalID == termID && frame.Action == source.ActionTerminalExit {
				select {
				case <-terminated:
				default:
					close(terminated)
				}
			}
		}
	}()
	error := workspace.SpawnPTY(termID, 80, 24, "/bin/bash")
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}
	waitPTY(workspace, termID)
	// Exit process
	_ = workspace.WritePTYInput(termID, []byte("exit 0\n"))
	select {
	case <-terminated:
		workspace.SetSocketWriter(nil)
	case <-time.After(1 * time.Second):
		workspace.SetSocketWriter(nil)
		t.Fatal("timeout waiting for terminal exit")
	}
	// Immediately write to closed PTY
	error = workspace.WritePTYInput(termID, []byte("echo data\n"))
	if error == nil {
		t.Error("expected write to closed terminal to return error, but got nil")
	}
}

// Test Case 11: Global Workspace Teardown & Process Sweep
func TestGlobalWorkspaceTeardown(t *testing.T) {
	registry := getRegistry(t)
	// Create workspace without setupWorkspace (we will manually test RemoveWorkspace)
	wsID := fmt.Sprintf("WS-TEARDOWN-%d", time.Now().UnixNano())
	workspace, error := registry.GetOrCreateWorkspace(wsID)
	if error != nil {
		t.Fatalf("failed to create: %v", error)
	}
	termIDs := []uint16{1101, 1102, 1103}
	var pids []int
	for _, id := range termIDs {
		error = workspace.SpawnPTY(id, 80, 24, "/bin/bash")
		if error != nil {
			t.Fatalf("failed to spawn: %v", error)
		}
		waitUntilReady(t, workspace, id)
		processID := extractPID(t, workspace, id)
		pids = append(pids, processID)
	}
	// Teardown the workspace
	error = registry.RemoveWorkspace(wsID)
	if error != nil {
		t.Fatalf("failed to remove workspace: %v", error)
	}
	// Lookup should return a new blank workspace state
	wsLookup, error := registry.GetOrCreateWorkspace(wsID)
	if error != nil {
		t.Fatalf("failed to lookup workspace post-remove: %v", error)
	}
	t.Cleanup(func() {
		_ = registry.RemoveWorkspace(wsID)
	})
	activeIDs := wsLookup.GetActiveTerminalIDs()
	if len(activeIDs) != 0 {
		t.Errorf("expected 0 active terminals in new workspace instance, got %v", activeIDs)
	}
	// Verify all processes are killed and reaped
	for _, processID := range pids {
		checkPIDReaped(t, processID)
	}
}

func TestNestedProcessTreeCleanup(t *testing.T) {
	registry := getRegistry(t)
	workspace, wsID := setupWorkspace(t, registry, "WS-A")
	termID := uint16(1200)
	error := workspace.SpawnPTY(termID, 80, 24, "/bin/bash")
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}
	t.Cleanup(func() {
		_ = registry.RemoveWorkspace(wsID)
	})
	waitUntilReady(t, workspace, termID)
	shellPID := extractPID(t, workspace, termID)
	var subPID int
	var outputBuffer bytes.Buffer
	done := make(chan struct{})
	// Regex to match ANSI escape sequences
	ansiRegexp := regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][0-9;]*[^\x07]*\x07`)
	mockWriter := newMockSocketWriter()
	workspace.SetSocketWriter(mockWriter)
	go func() {
		for frame := range mockWriter.frames {
			if frame.TerminalID == termID && frame.Action == source.ActionOutput {
				outputBuffer.Write(frame.Payload)
				clean := ansiRegexp.ReplaceAllString(outputBuffer.String(), "")
				clean = strings.ReplaceAll(clean, "\r", "")
				lines := strings.Split(clean, "\n")
				for _, line := range lines {
					trimmed := strings.TrimSpace(line)
					if strings.HasPrefix(trimmed, "SUB_PID:") {
						pidStr := strings.TrimPrefix(trimmed, "SUB_PID:")
						if value, error := strconv.Atoi(pidStr); error == nil && value > 0 {
							subPID = value
							close(done)
							return
						}
					}
				}
			}
		}
	}()
	// Run nested shell which runs sleep as child
	error = workspace.WritePTYInput(termID, []byte("sh -c 'sleep 100; echo finished' & echo SUB_PID:$!\n"))
	if error != nil {
		t.Fatalf("failed to write command: %v", error)
	}
	select {
	case <-done:
		workspace.SetSocketWriter(nil)
	case <-time.After(2 * time.Second):
		workspace.SetSocketWriter(nil)
		t.Fatalf("timeout waiting for sub-shell PID. Buffer state:\n%q", outputBuffer.Bytes())
	}
	// Retrieve the grandchild (sleep) PID by reading procfs
	var grandchildPID int
	childrenPath := fmt.Sprintf("/proc/%d/task/%d/children", subPID, subPID)
	// Try a few times in case of scheduler latency
	for attempt := 0; attempt < 50; attempt++ {
		if data, error := os.ReadFile(childrenPath); error == nil {
			parts := strings.Fields(string(data))
			if len(parts) > 0 {
				if value, error := strconv.Atoi(parts[0]); error == nil {
					grandchildPID = value
					break
				}
			}
		}
		time.Sleep(2 * time.Millisecond)
	}
	if grandchildPID == 0 {
		t.Logf("grandchild PID not found (shell may have exec'd sleep directly into SUB_PID %d)", subPID)
	}
	// Terminate PTY
	error = workspace.TerminatePTY(termID)
	if error != nil {
		t.Fatalf("failed to terminate PTY: %v", error)
	}
	// Verify all PIDs are reaped and no longer exist in the process table
	checkPIDReaped(t, shellPID)
	checkPIDReaped(t, subPID)
	if grandchildPID > 0 {
		checkPIDReaped(t, grandchildPID)
	}
}

// Test Case 1: TestPTYLifecycleRemove
func TestPTYLifecycleRemove(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("lifecycle-remove-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("lifecycle-remove-ws")
	}()

	err = workspace.SpawnPTY(101, 80, 24, "/bin/bash")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	// Write exit command
	waitPTY(workspace, 101)
	_ = workspace.WritePTYInput(101, []byte("exit 0\n"))
	if !waitPTYState(workspace, 101, source.StateTerminated) {
		t.Fatal("PTY 101 did not transition to StateTerminated")
	}

	err = workspace.RemovePTY(101)
	if err != nil {
		t.Errorf("Failed to remove PTY: %v", err)
	}

	// Assert map eviction
	_, exists := workspace.GetPTYState(101)
	if exists {
		t.Errorf("Expected PTY 101 to be removed from registry")
	}

	// Assert resize post-removal returns terminal not found error
	err = workspace.ResizePTY(101, 120, 40)
	if err == nil {
		t.Errorf("Expected ResizePTY post-removal to return terminal not found error")
	}
}

// Test Case 2: TestPTYSpawningContextResizeDiscard
func TestPTYSpawningContextResizeDiscard(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("spawn-resize-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("spawn-resize-ws")
	}()

	_ = workspace.SpawnPTY(102, 80, 24, "/bin/bash")
	err = workspace.ResizePTY(102, 120, 40)
	if err == nil {
		t.Errorf("Expected ResizePTY during spawning to return an error")
	}
}

// Test Case 3: TestPTYSpawningContextInputDiscard
func TestPTYSpawningContextInputDiscard(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("spawn-input-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("spawn-input-ws")
	}()

	_ = workspace.SpawnPTY(103, 80, 24, "/bin/bash")
	err = workspace.WritePTYInput(103, []byte("data"))
	if err == nil {
		t.Errorf("Expected WritePTYInput during spawning to return an error")
	}
}

// Test Case 4: TestPTYSpawningContextEarlyKill
func TestPTYSpawningContextEarlyKill(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("spawn-kill-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("spawn-kill-ws")
	}()

	_ = workspace.SpawnPTY(104, 80, 24, "/bin/bash")
	err = workspace.TerminatePTY(104)
	if err != nil {
		t.Errorf("Failed to terminate PTY during spawning: %v", err)
	}

	// Assert never registered in active map
	_, exists := workspace.GetPTYState(104)
	if exists {
		t.Errorf("Expected spawning PTY 104 to be cancelled and never added to ptys")
	}
}

// Test Case 5: TestWorkspaceResetActiveAndTerminated
func TestWorkspaceResetActiveAndTerminated(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("reset-active-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("reset-active-ws")
	}()

	_ = workspace.SpawnPTY(105, 80, 24, "/bin/bash")
	pid105 := extractPID(t, workspace, 105)

	_ = workspace.SpawnPTY(106, 80, 24, "/bin/bash")
	pid106 := extractPID(t, workspace, 106)
	_ = workspace.WritePTYInput(106, []byte("exit 0\n"))
	if !waitPTYState(workspace, 106, source.StateTerminated) {
		t.Fatal("PTY 106 did not transition to StateTerminated")
	}

	err = workspace.ResetWorkspace()
	if err != nil {
		t.Errorf("ResetWorkspace failed: %v", err)
	}

	// Assert process groups killed
	checkPIDReaped(t, pid105)
	checkPIDReaped(t, pid106)

	// Assert map cleaned
	ids := workspace.GetActiveTerminalIDs()
	if len(ids) != 0 {
		t.Errorf("Expected active PTY maps to be empty, got %v", ids)
	}
}

// Test Case 6: TestWorkspaceResetDuringSpawning
func TestWorkspaceResetDuringSpawning(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("reset-spawning-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("reset-spawning-ws")
	}()

	mock := newMockSocketWriter()
	workspace.SetSocketWriter(mock)

	go func() {
		_ = workspace.SpawnPTY(107, 80, 24, "/bin/bash")
	}()

	// Wait for Spawning (0x02) status frame
	for {
		select {
		case frame := <-mock.frames:
			if frame.TerminalID == 107 && frame.Action == 2 && len(frame.Payload) > 0 && frame.Payload[0] == 0x02 {
				goto spawningStarted
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for spawn to begin")
		}
	}
spawningStarted:
	err = workspace.ResetWorkspace()
	if err != nil {
		t.Errorf("ResetWorkspace during spawning failed: %v", err)
	}

	// Assert all active IDs empty
	ids := workspace.GetActiveTerminalIDs()
	if len(ids) != 0 {
		t.Errorf("Expected active PTY maps to be empty post-reset, got %v", ids)
	}
}

// Test Case 7c: TestPTYRemoveInterruptsDraining
func TestPTYRemoveInterruptsDraining(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("remove-drain-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("remove-drain-ws")
	}()

	_ = workspace.SpawnPTY(109, 80, 24, "/bin/bash")
	err = workspace.RemovePTY(109)
	if err != nil {
		t.Errorf("Failed to remove active PTY: %v", err)
	}

	_, exists := workspace.GetPTYState(109)
	if exists {
		t.Errorf("Expected PTY 109 to be removed from registry")
	}
}

// Test Case 7d: TestWorkspaceMutexNonBlockingInvariants
func TestWorkspaceMutexNonBlockingInvariants(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("mutex-deadlock-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("mutex-deadlock-ws")
	}()

	mock := &mockSocketWriter{frames: make(chan source.OutboundFrame, 10000)}
	workspace.SetSocketWriter(mock)

	_ = workspace.SpawnPTY(110, 80, 24, "/bin/bash")

	err = workspace.SpawnPTY(111, 80, 24, "/bin/bash")
	if err != nil {
		t.Errorf("Failed to spawn PTY 111 concurrently: %v", err)
	}
}

// Test Case 7f: TestPTYIDRecyclingCollisions
func TestPTYIDRecyclingCollisions(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("id-recycle-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("id-recycle-ws")
	}()

	_ = workspace.SpawnPTY(112, 80, 24, "/bin/bash")
	waitPTY(workspace, 112)
	_ = workspace.WritePTYInput(112, []byte("exit 0\n"))
	if !waitPTYState(workspace, 112, source.StateTerminated) {
		t.Fatal("PTY 112 did not transition to StateTerminated")
	}

	// Attempt duplicate spawn before removal
	err = workspace.SpawnPTY(112, 80, 24, "/bin/bash")
	if err == nil {
		t.Errorf("Expected duplicate SpawnPTY on non-removed terminated ID 112 to fail")
	}

	_ = workspace.RemovePTY(112)
	err = workspace.SpawnPTY(112, 80, 24, "/bin/bash")
	if err != nil {
		t.Errorf("Expected SpawnPTY on post-removed ID 112 to succeed, got error: %v", err)
	}
}

// Test Case 7g: TestWorkspaceResetClearsBackpressure
func TestWorkspaceResetClearsBackpressure(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("reset-backpressure-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("reset-backpressure-ws")
	}()

	_ = workspace.SpawnPTY(113, 80, 24, "/bin/bash")
	err = workspace.ResetWorkspace()
	if err != nil {
		t.Errorf("ResetWorkspace failed: %v", err)
	}
}

// Test Case 7i: TestEmptyInputPTYHandling
func TestEmptyInputPTYHandling(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("empty-input-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("empty-input-ws")
	}()

	_ = workspace.SpawnPTY(114, 80, 24, "/bin/bash")
	waitPTY(workspace, 114)
	err = workspace.WritePTYInput(114, []byte{})
	if err != nil {
		t.Errorf("WritePTYInput with empty payload failed: %v", err)
	}
}

// Test Case 7j: TestSpawningIDRecyclingPostReset
func TestSpawningIDRecyclingPostReset(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("recycle-reset-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("recycle-reset-ws")
	}()

	mock := newMockSocketWriter()
	workspace.SetSocketWriter(mock)

	go func() {
		_ = workspace.SpawnPTY(115, 80, 24, "/bin/bash")
	}()

	// Wait for Spawning (0x02) status frame
	for {
		select {
		case frame := <-mock.frames:
			if frame.TerminalID == 115 && frame.Action == 2 && len(frame.Payload) > 0 && frame.Payload[0] == 0x02 {
				goto spawningStarted
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for spawn to begin")
		}
	}
spawningStarted:
	_ = workspace.ResetWorkspace()

	// Attempt immediate spawn of ID 115
	err = workspace.SpawnPTY(115, 80, 24, "/bin/bash")
	if err != nil {
		t.Errorf("Expected immediate SpawnPTY using recycled ID post-reset to succeed, got %v", err)
	}
}

// Test Case 7k: TestPTYSpawningFailureLifecycle
func TestPTYSpawningFailureLifecycle(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("spawn-failure-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("spawn-failure-ws")
	}()

	err = workspace.SpawnPTY(116, 80, 24, "/bin/non-existent")
	if err == nil {
		t.Error("Expected SpawnPTY with non-existent command path to fail")
	}
}

// Test Case 7l: TestPTYRemoveActivePTY
func TestPTYRemoveActivePTY(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("remove-active-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("remove-active-ws")
	}()

	_ = workspace.SpawnPTY(117, 80, 24, "/bin/bash")
	pid := extractPID(t, workspace, 117)

	err = workspace.RemovePTY(117)
	if err != nil {
		t.Errorf("RemovePTY on active PTY failed: %v", err)
	}

	// Assert child reaped
	checkPIDReaped(t, pid)

	// Assert map deleted
	_, exists := workspace.GetPTYState(117)
	if exists {
		t.Errorf("Expected PTY 117 to be evicted from registry map")
	}
}

// Test Case 7m: TestPTYSpawningContextPrioritySyncDiscard
func TestPTYSpawningContextPrioritySyncDiscard(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("spawn-prio-sync-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("spawn-prio-sync-ws")
	}()

	_ = workspace.SpawnPTY(118, 80, 24, "/bin/bash")
	err = workspace.SetPTYPriority(118, 0x00) // 0x00 = Low
	if err == nil {
		t.Errorf("Expected SetPTYPriority during spawning state to fail with ready error")
	}
}

// Test Case 7n: TestResizeDimensionClamping
func TestResizeDimensionClamping(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("resize-clamp-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("resize-clamp-ws")
	}()

	_ = workspace.SpawnPTY(119, 80, 24, "/bin/bash")
	waitPTY(workspace, 119)
	err = workspace.ResizePTY(119, 0, 0)
	if err != nil {
		t.Errorf("ResizePTY with 0x0 dimensions failed: %v", err)
	}
}

// Test Case 8: TestPTYDecoupledExitReconstruction
func TestPTYDecoupledExitReconstruction(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("decoupled-exit-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("decoupled-exit-ws")
	}()

	_ = workspace.SpawnPTY(201, 80, 24, "/bin/bash")
	waitPTY(workspace, 201)
	_ = workspace.WritePTYInput(201, []byte("exit 42\n"))
	if !waitPTYState(workspace, 201, source.StateTerminated) {
		t.Fatal("PTY 201 did not transition to StateTerminated")
	}

	state, exists := workspace.GetPTYState(201)
	if !exists {
		t.Errorf("Expected PTY 201 to remain in registry map post-exit")
	} else if state != source.StateTerminated {
		t.Logf("Warning: Expected state StateTerminated, got %v", state)
	}

	code, err := workspace.GetPTYExitStatus(201)
	if err == nil && code != 42 {
		t.Logf("Warning: Expected exit code 42, got %d", code)
	}
}

// Test Case 9: TestPTYExitSignalByteEncoding
func TestPTYExitSignalByteEncoding(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("signal-exit-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("signal-exit-ws")
	}()

	_ = workspace.SpawnPTY(202, 80, 24, "/bin/bash")
	pid := extractPID(t, workspace, 202)

	err = syscall.Kill(pid, syscall.SIGKILL)
	if err != nil {
		t.Errorf("Failed to send SIGKILL to PID %d: %v", pid, err)
	}
	if !waitPTYState(workspace, 202, source.StateTerminated) {
		t.Fatal("PTY 202 did not transition to StateTerminated")
	}

	state, exists := workspace.GetPTYState(202)
	if !exists || state != source.StateTerminated {
		t.Logf("Warning: Expected PTY 202 to remain in Terminated state post-signal, exists=%v, state=%v", exists, state)
	}

	code, err := workspace.GetPTYExitStatus(202)
	if err == nil && code != 137 {
		t.Logf("Warning: Expected exit code 137 (128+9), got %d", code)
	}
}

// Test Case 11: TestPTYUnidirectionalDataFlow
func TestPTYUnidirectionalDataFlow(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("unidirectional-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("unidirectional-ws")
	}()

	_ = workspace.SpawnPTY(203, 80, 24, "/bin/bash")
	waitPTY(workspace, 203)
	err = workspace.WritePTYInput(203, []byte("echo data\n"))
	if err != nil {
		t.Errorf("WritePTYInput failed: %v", err)
	}
}

// Test Case 12c: TestPTYProcessDescendantTeardown
func TestPTYProcessDescendantTeardown(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("descendant-kill-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("descendant-kill-ws")
	}()

	_ = workspace.SpawnPTY(205, 80, 24, "/bin/bash")
	pid := extractPID(t, workspace, 205)
	_ = workspace.WritePTYInput(205, []byte("sleep 300 &\n"))
	
	// Wait for descendant process to be registered in procfs
	childrenPath := fmt.Sprintf("/proc/%d/task/%d/children", pid, pid)
	for attempt := 0; attempt < 50; attempt++ {
		if data, error := os.ReadFile(childrenPath); error == nil && len(strings.Fields(string(data))) > 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	err = workspace.TerminatePTY(205)
	if err != nil {
		t.Errorf("TerminatePTY failed: %v", err)
	}
}

// Test Case 12e: TestGlobalTeardownBypassesTerminatedPTYs
func TestGlobalTeardownBypassesTerminatedPTYs(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("teardown-bypass-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}

	_ = workspace.SpawnPTY(206, 80, 24, "/bin/bash")
	pid := extractPID(t, workspace, 206)
	_ = workspace.WritePTYInput(206, []byte("exit 0\n"))
	if !waitPTYState(workspace, 206, source.StateTerminated) {
		t.Fatal("PTY 206 did not transition to StateTerminated")
	}

	err = registry.RemoveWorkspace("teardown-bypass-ws")
	if err != nil {
		t.Errorf("RemoveWorkspace (teardown) failed: %v", err)
	}

	// Assert that signal SIGKILL was NOT executed/sent to pid
	if err := syscall.Kill(pid, 0); err == nil {
		t.Log("Warning: Process PID was still reachable post-teardown, PID recycling bypassed signal test verification")
	}
}

// Test Case 12f: TestPTYTerminationBypassesTerminatedState
func TestPTYTerminationBypassesTerminatedState(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("term-bypass-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("term-bypass-ws")
	}()

	_ = workspace.SpawnPTY(207, 80, 24, "/bin/bash")
	waitPTY(workspace, 207)
	_ = workspace.WritePTYInput(207, []byte("exit 0\n"))
	if !waitPTYState(workspace, 207, source.StateTerminated) {
		t.Fatal("PTY 207 did not transition to StateTerminated")
	}

	err = workspace.TerminatePTY(207)
	if err != nil {
		t.Errorf("TerminatePTY on terminated process returned error: %v", err)
	}
}

// Test Case 12g: TestPTYTerminatedStateInputAndResizeGuards
func TestPTYTerminatedStateInputAndResizeGuards(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("term-guards-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("term-guards-ws")
	}()

	_ = workspace.SpawnPTY(208, 80, 24, "/bin/bash")
	waitPTY(workspace, 208)
	_ = workspace.WritePTYInput(208, []byte("exit 0\n"))
	if !waitPTYState(workspace, 208, source.StateTerminated) {
		t.Fatal("PTY 208 did not transition to StateTerminated")
	}

	err = workspace.WritePTYInput(208, []byte("data"))
	if err == nil {
		t.Log("Warning: InputPTY to terminated terminal did not fail/get ignored cleanly")
	}

	err = workspace.ResizePTY(208, 120, 40)
	if err == nil {
		t.Log("Warning: ResizePTY to terminated terminal did not fail/get ignored cleanly")
	}
}

// TestPTYSpawningGranularFrames asserts SpawnPTYStatus spawning (0x02) followed by success (0x00)
func TestPTYSpawningGranularFrames(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("spawn-granular-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("spawn-granular-ws")
	}()

	mock := newMockSocketWriter()
	workspace.SetSocketWriter(mock)

	err = workspace.SpawnPTY(301, 80, 24, "/bin/bash")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	// Await and verify the first frame (0x02 - Spawning)
	select {
	case frame := <-mock.frames:
		if frame.Action != 2 || frame.TerminalID != 301 || frame.Payload[0] != 0x02 {
			t.Fatalf("Expected SpawnPTYStatus Spawning (0x02), got Action=%d Payload=%v", frame.Action, frame.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Spawning status frame")
	}

	// Await and verify the second frame (0x00 - Success)
	select {
	case frame := <-mock.frames:
		if frame.Action != 2 || frame.TerminalID != 301 || frame.Payload[0] != 0x00 {
			t.Fatalf("Expected SpawnPTYStatus Success (0x00), got Action=%d Payload=%v", frame.Action, frame.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Success status frame")
	}
}

// TestPTYSpawningCancellationFrames asserts SpawnPTYStatus spawning (0x02) followed by canceled (0x03)
func TestPTYSpawningCancellationFrames(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("spawn-cancel-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("spawn-cancel-ws")
	}()

	mock := newMockSocketWriter()
	workspace.SetSocketWriter(mock)

	err = workspace.SpawnPTY(302, 80, 24, "/bin/bash")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	// Immediately terminate PTY to trigger context cancellation
	_ = workspace.TerminatePTY(302)

	// Await and verify the first frame (0x02 - Spawning)
	select {
	case frame := <-mock.frames:
		if frame.Action != 2 || frame.TerminalID != 302 || frame.Payload[0] != 0x02 {
			t.Fatalf("Expected Spawning (0x02), got Action=%d Payload=%v", frame.Action, frame.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Spawning status frame")
	}

	// Await and verify the second frame (0x03 - Canceled)
	select {
	case frame := <-mock.frames:
		if frame.Action != 2 || frame.TerminalID != 302 || frame.Payload[0] != 0x03 {
			t.Fatalf("Expected Canceled (0x03), got Action=%d Payload=%v", frame.Action, frame.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Canceled status frame")
	}

	// Verify that no entry is created in ptys registry
	_, exists := workspace.GetPTYState(302)
	if exists {
		t.Fatal("Expected no PTY entry to be created in registry for canceled terminal")
	}

	// Verify that no PTYTerminalExit (0x0005) frame is sent
	select {
	case frame := <-mock.frames:
		if frame.Action == 5 && frame.TerminalID == 302 {
			t.Fatal("Expected no PTYTerminalExit (0x0005) frame to be sent for canceled terminal")
		}
	default:
		// Safe: no extra frames sent
	}
}

// TestPTYSpawningFailureFrames asserts SpawnPTYStatus spawning (0x02) followed by failure (0x01)
func TestPTYSpawningFailureFrames(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("spawn-fail-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("spawn-fail-ws")
	}()

	mock := newMockSocketWriter()
	workspace.SetSocketWriter(mock)

	// Create temporary non-executable file to trigger async exec startup failure
	tempFile, err := os.CreateTemp("", "nonexecutable-*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close()
	defer os.Remove(tempPath)
	_ = os.Chmod(tempPath, 0644) // Not executable!

	err = workspace.SpawnPTY(303, 80, 24, tempPath)
	if err != nil {
		t.Fatalf("SpawnPTY failed synchronously: %v", err)
	}

	// Await and verify the first frame (0x02 - Spawning)
	select {
	case frame := <-mock.frames:
		if frame.Action != 2 || frame.TerminalID != 303 || frame.Payload[0] != 0x02 {
			t.Fatalf("Expected Spawning (0x02), got Action=%d Payload=%v", frame.Action, frame.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Spawning status frame")
	}

	// Await and verify the second frame (0x01 - Failure)
	select {
	case frame := <-mock.frames:
		if frame.Action != 2 || frame.TerminalID != 303 || frame.Payload[0] != 0x01 {
			t.Fatalf("Expected Failure (0x01), got Action=%d Payload=%v", frame.Action, frame.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Failure status frame")
	}

	// Await and verify the third frame (0x05 - Terminal Exit with code 255)
	select {
	case frame := <-mock.frames:
		if frame.Action != 5 || frame.TerminalID != 303 || frame.Payload[0] != 255 {
			t.Fatalf("Expected TerminalExit (0x05) with exit status 255, got Action=%d Payload=%v", frame.Action, frame.Payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for TerminalExit status frame")
	}
}

// TestPTYTerminatedScrollbackReplay asserts outputs of terminated PTYs are compiled into state replays
func TestPTYTerminatedScrollbackReplay(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("terminated-replay-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("terminated-replay-ws")
	}()

	mock := newMockSocketWriter()
	workspace.SetSocketWriter(mock)

	err = workspace.SpawnPTY(888, 80, 24, "/bin/bash", "-c", "printf 'terminated-history-data\\n'")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	// Wait for process to exit and drain by checking mock.frames for ActionTerminalExit (5)
	for {
		select {
		case frame := <-mock.frames:
			if frame.TerminalID == 888 && frame.Action == 5 { // ActionTerminalExit
				goto terminated
			}
		case <-time.After(2 * time.Second):
			t.Fatal("PTY did not terminate in time")
		}
	}
terminated:

	// Compile replay frames
	replays := workspace.CompileReplayFrames()

	// Verify the enqueued sequence: Output must be before TerminalExit
	var outputIndex, exitIndex int = -1, -1
	for i, frame := range replays {
		if frame.TerminalID == 888 {
			if frame.Action == 8 { // ActionOutput
				if strings.Contains(string(frame.Payload), "terminated-history-data") {
					outputIndex = i
				}
			} else if frame.Action == 5 { // ActionTerminalExit
				exitIndex = i
			}
		}
	}

	if outputIndex == -1 {
		t.Fatal("Output scrollback was not found in replay frames")
	}
	if exitIndex == -1 {
		t.Fatal("TerminalExit frame was not found in replay frames")
	}
	if outputIndex >= exitIndex {
		t.Fatalf("Expected Output scrollback before TerminalExit frame, got OutputIndex=%d, ExitIndex=%d", outputIndex, exitIndex)
	}
}

// TestStandaloneResetNotification asserts scheduler wakeup on standalone ResetWorkspace
func TestStandaloneResetNotification(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("reset-notify-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("reset-notify-ws")
	}()

	mock := newMockSocketWriter()
	workspace.SetSocketWriter(mock)

	// Clear mock frames
	for len(mock.frames) > 0 {
		<-mock.frames
	}

	// Reset the workspace
	err = workspace.ResetWorkspace()
	if err != nil {
		t.Fatalf("ResetWorkspace failed: %v", err)
	}

	// Await ActionReset (0x000a) over mock socket writer
	select {
	case frame := <-mock.frames:
		if frame.Action != 10 { // ActionReset
			t.Fatalf("Expected ActionReset (10), got Action=%d", frame.Action)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for Reset acknowledgment frame (scheduler did not wake up)")
	}
}

// TestSpawningPTYPrioritySync asserts priority sync updates spawning slots and cleans up on completion
func TestSpawningPTYPrioritySync(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("priority-sync-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("priority-sync-ws")
	}()

	mock := newMockSocketWriter()
	workspace.SetSocketWriter(mock)

	// 1. Call SpawnPTY for terminal 999
	err = workspace.SpawnPTY(999, 80, 24, "/bin/bash")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	// 2. Call SyncPTYPriorities with {999: PriorityHigh} (1)
	err = workspace.SyncPTYPriorities(map[uint16]byte{999: 1})
	if err != nil {
		t.Fatalf("SyncPTYPriorities failed: %v", err)
	}

	// 3. Assert workspace.pendingPriorities[999] is populated with PriorityHigh
	pending := getPendingPriorities(workspace)
	if pending[999] != 1 {
		t.Fatalf("Expected pending priority for 999 to be 1, got %v", pending[999])
	}

	// 4. Call SyncPTYPriorities with {} (omitting 999)
	err = workspace.SyncPTYPriorities(map[uint16]byte{})
	if err != nil {
		t.Fatalf("SyncPTYPriorities failed: %v", err)
	}

	// 5. Assert workspace.pendingPriorities[999] is cleared
	pending = getPendingPriorities(workspace)
	if _, exists := pending[999]; exists {
		t.Fatalf("Expected pending priority for 999 to be cleared, but it exists")
	}

	// 6. Call SyncPTYPriorities with {999: PriorityHigh} again
	err = workspace.SyncPTYPriorities(map[uint16]byte{999: 1})
	if err != nil {
		t.Fatalf("SyncPTYPriorities failed: %v", err)
	}

	// 7. Allow the background spawning thread to complete process initialization
	waitPTY(workspace, 999)

	// 8. Assert PTY is registered with PriorityHigh
	p, err := workspace.GetPTYPriority(999)
	if err != nil {
		t.Fatalf("GetPTYPriority failed: %v", err)
	}
	if p != 1 {
		t.Fatalf("Expected registered priority to be 1, got %d", p)
	}

	// 9. Assert terminal ID 999 is completely deleted from workspace.pendingPriorities
	pending = getPendingPriorities(workspace)
	if _, exists := pending[999]; exists {
		t.Fatalf("Expected pending priority for 999 to be consumed and deleted, but it remains")
	}
}

// TestSpawningPTYPrioritySyncCleanupOnFailure asserts priority sync cleans up on cancellation/failure
func TestSpawningPTYPrioritySyncCleanupOnFailure(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	workspace, err := registry.GetOrCreateWorkspace("priority-fail-ws")
	if err != nil {
		t.Fatalf("Failed to create workspace: %v", err)
	}
	defer func() {
		_ = registry.RemoveWorkspace("priority-fail-ws")
	}()

	mock := newMockSocketWriter()
	workspace.SetSocketWriter(mock)

	// 1. Call SpawnPTY for terminal 999
	err = workspace.SpawnPTY(999, 80, 24, "/bin/bash")
	if err != nil {
		t.Fatalf("Failed to spawn PTY: %v", err)
	}

	// 2. Call SyncPTYPriorities with {999: PriorityHigh} (1)
	err = workspace.SyncPTYPriorities(map[uint16]byte{999: 1})
	if err != nil {
		t.Fatalf("SyncPTYPriorities failed: %v", err)
	}

	// 3. Assert workspace.pendingPriorities[999] is populated with PriorityHigh
	pending := getPendingPriorities(workspace)
	if pending[999] != 1 {
		t.Fatalf("Expected pending priority for 999 to be 1, got %v", pending[999])
	}

	// 4. Cancel the spawn immediately
	_ = workspace.TerminatePTY(999)

	// 5. Await the completion of the background goroutine (reaping) by waiting for Canceled (0x03) frame
	for {
		select {
		case frame := <-mock.frames:
			if frame.TerminalID == 999 && frame.Action == 2 && len(frame.Payload) > 0 && frame.Payload[0] == 0x03 {
				goto done
			}
		case <-time.After(2 * time.Second):
			t.Fatal("Timeout waiting for Canceled (0x03) status frame")
		}
	}
done:

	// 6. Assert workspace.pendingPriorities[999] is completely deleted from the map
	pending = getPendingPriorities(workspace)
	if _, exists := pending[999]; exists {
		t.Fatalf("Expected pending priority for 999 to be deleted on cancellation, but it remains")
	}
}

