package gotests

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"suprasole-server/source"
)

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

func waitUntilReady(t *testing.T, workspace *source.Workspace, termID uint16) {
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
				if frame.TerminalID == termID && frame.Action == source.ActionStreamIO {
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
				if frame.TerminalID == termID && frame.Action == source.ActionStreamIO {
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
	error := wsA.SpawnPTY(termIDA, 80, 24)
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
			if frame.TerminalID == termIDA && frame.Action == source.ActionStreamIO {
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
				if frame.Action == source.ActionStreamIO {
					outputBuffer.Write(frame.Payload)
				} else if frame.Action == source.ActionKill && len(frame.Payload) == 1 {
					select {
					case terminated <- frame.Payload[0]:
					default:
					}
				}
			}
		}
	}()

	// Spawn PTY with 132x43
	error := workspace.SpawnPTY(termID, 132, 43)
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}

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
	error := workspace.SpawnPTY(termID, 80, 24)
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
			if frame.TerminalID == termID && frame.Action == source.ActionStreamIO {
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
				if frame.Action == source.ActionStreamIO {
					accum.Write(frame.Payload)
					if strings.Contains(accum.String(), "quick_exit") && outputTimestamp.IsZero() {
						outputTimestamp = time.Now()
					}
				} else if frame.Action == source.ActionKill {
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

	error := workspace.SpawnPTY(termID, 80, 24)
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}

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
			if frame.Action == source.ActionKill && len(frame.Payload) == 1 {
				mutex.Lock()
				exits[frame.TerminalID] = frame.Payload[0]
				mutex.Unlock()
				exitsDone <- frame.TerminalID
			}
		}
	}()

	// Spawn normal exit process
	error := workspace.SpawnPTY(termID501, 80, 24)
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}
	waitUntilReady(t, workspace, termID501)
	pid501 := extractPID(t, workspace, termID501)

	// Spawn signal exit process
	error = workspace.SpawnPTY(termID502, 80, 24)
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}
	waitUntilReady(t, workspace, termID502)
	pid502 := extractPID(t, workspace, termID502)

	// Spawn crash exit process
	error = workspace.SpawnPTY(termID503, 80, 24)
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}
	waitUntilReady(t, workspace, termID503)
	pid503 := extractPID(t, workspace, termID503)

	// Trigger exits
	_ = workspace.WritePTYInput(termID501, []byte("exit 77\n"))
	_ = workspace.WritePTYInput(termID503, []byte("kill -11 $$\n")) // SIGSEGV
	
	// Wait a moment for processing before calling TerminatePTY on 502
	time.Sleep(100 * time.Millisecond)
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
	for i := 0; i < 20; i++ {
		error = syscall.Kill(processID, 0)
		if errors.Is(error, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
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
		error := workspace.SpawnPTY(termID, 80, 24)
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

	// Run stress test for 1.5 seconds
	time.Sleep(1500 * time.Millisecond)
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
	time.Sleep(200 * time.Millisecond)

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
			if frame.TerminalID == termID && frame.Action == source.ActionKill {
				select {
				case <-terminated:
				default:
					close(terminated)
				}
			}
		}
	}()

	error := workspace.SpawnPTY(termID, 80, 24)
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
	waitGroup.Add(3)
	go func() {
		defer waitGroup.Done()
		_ = workspace.SpawnPTY(termID, 80, 24)
	}()
	go func() {
		defer waitGroup.Done()
		_ = workspace.ResizePTY(termID, 120, 40)
	}()
	go func() {
		defer waitGroup.Done()
		_ = workspace.TerminatePTY(termID)
	}()

	waitGroup.Wait()
	time.Sleep(100 * time.Millisecond)

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
	error := workspace.SpawnPTY(termID, 80, 24)
	if error != nil {
		t.Fatalf("failed to spawn PTY: %v", error)
	}

	// Set raw mode using stty raw -echo before piping binary data through cat
	waitUntilReady(t, workspace, termID)

	var outputBuffer bytes.Buffer
	done := make(chan struct{})

	// Sequence containing null, invalid UTF-8 bytes, and escape codes
	payload := []byte{0x00, 0xFF, 0xFE, 0x01, 0x1B, 0x5B, 0x48, 0x02, 0x0A}

	mockWriter := newMockSocketWriter()
	workspace.SetSocketWriter(mockWriter)

	go func() {
		for frame := range mockWriter.frames {
			if frame.TerminalID == termID && frame.Action == source.ActionStreamIO {
				outputBuffer.Write(frame.Payload)
				if bytes.Contains(outputBuffer.Bytes(), payload) {
					close(done)
					return
				}
			}
		}
	}()

	// Run cat
	error = workspace.WritePTYInput(termID, []byte("cat\n"))
	if error != nil {
		t.Fatalf("failed to write cat: %v", error)
	}
	time.Sleep(100 * time.Millisecond)

	// Write raw binary payload
	error = workspace.WritePTYInput(termID, payload)
	if error != nil {
		t.Fatalf("failed to write binary: %v", error)
	}

	select {
	case <-done:
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
			if frame.TerminalID == termID && frame.Action == source.ActionKill {
				select {
				case <-terminated:
				default:
					close(terminated)
				}
			}
		}
	}()

	error := workspace.SpawnPTY(termID, 80, 24)
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}

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

	// Server should remain alive, no SIGPIPE crash
	time.Sleep(100 * time.Millisecond)
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
		error = workspace.SpawnPTY(id, 80, 24)
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
	error := workspace.SpawnPTY(termID, 80, 24)
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
			if frame.TerminalID == termID && frame.Action == source.ActionStreamIO {
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

	// Wait a moment for the nested sleep process to be spawned by the sub-shell
	time.Sleep(100 * time.Millisecond)

	// Retrieve the grandchild (sleep) PID by reading procfs
	var grandchildPID int
	childrenPath := fmt.Sprintf("/proc/%d/task/%d/children", subPID, subPID)
	// Try a few times in case of scheduler latency
	for attempt := 0; attempt < 5; attempt++ {
		if data, error := os.ReadFile(childrenPath); error == nil {
			parts := strings.Fields(string(data))
			if len(parts) > 0 {
				if value, error := strconv.Atoi(parts[0]); error == nil {
					grandchildPID = value
					break
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
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
