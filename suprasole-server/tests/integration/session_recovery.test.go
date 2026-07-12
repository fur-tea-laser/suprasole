package gotests

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"suprasole-server/source"
)

// Test Case 1: PTY Continuity & Always-On Ingestion
func TestWorkspaceOrphanStateAndPTYContinuity(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	token := "orphan-continuity-workspace"
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=" + token

	// 1. First connection
	connA, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to connect A: %v", error)
	}

	// Spawn PTY 1
	_ = connA.WriteMessage(websocket.BinaryMessage, packSpawnRequest(1, 80, 24))
	_, _, error = connA.ReadMessage()
	if error != nil {
		t.Fatalf("failed to spawn: %v", error)
	}

	// Wait deterministically for the initial shell prompt before writing commands
	for {
		_, msg, error := connA.ReadMessage()
		if error != nil {
			t.Fatalf("failed to read prompt: %v", error)
		}
		action, termID, _, _ := unpackFrame(msg)
		if action == source.ActionStreamIO && termID == 1 {
			break
		}
	}

	// Write input that prints after connection close
	cmdInput := packStreamIO(1, []byte("sleep 0.3 && echo 'OFFLINE_OUTPUT'\n"))
	_ = connA.WriteMessage(websocket.BinaryMessage, cmdInput)

	// Close connA forcefully
	_ = connA.Close()

	// Poll core workspace scrollback buffer deterministically until 'OFFLINE_OUTPUT' is present
	workspace, error := registry.GetOrCreateWorkspace(token)
	if error != nil {
		t.Fatalf("failed to resolve workspace: %v", error)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		buf, _, _ := workspace.GetScrollbackBuffer(1)
		if strings.Contains(string(buf), "OFFLINE_OUTPUT") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for offline output to be buffered")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 2. Reconnect client
	connB, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to reconnect: %v", error)
	}
	t.Cleanup(func() {
		_ = connB.Close()
		_ = registry.RemoveWorkspace(token)
	})

	// Read replayed scrollback stream
	_ = connB.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, replayData, error := connB.ReadMessage()
	if error != nil {
		t.Fatalf("failed to read replay: %v", error)
	}

	replayAction, replayTermID, replayPayload, error := unpackFrame(replayData)
	if error != nil {
		t.Fatalf("unpack failed: %v", error)
	}

	if replayAction != source.ActionStreamIO || replayTermID != 1 {
		t.Fatalf("expected replayed stream source.ActionStreamIO for PTY 1, got action %x term %d", replayAction, replayTermID)
	}

	if !strings.Contains(string(replayPayload), "OFFLINE_OUTPUT") {
		t.Errorf("expected replayed output to contain 'OFFLINE_OUTPUT', got: %q", string(replayPayload))
	}

	// Negative assertion: non-evicted buffers must NOT contain the truncation warning
	warning := []byte("[... Output truncated due to buffer overflow ...]")
	if bytes.Contains(replayPayload, warning) {
		t.Error("expected non-truncated terminal stream to NOT contain the truncation warning prefix")
	}
}

// Test Case 3: Ring Buffer Eviction & Warning Injection
func TestRingBufferEvictionAndTruncationWarning(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	token := "eviction-warning-workspace"
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=" + token

	connA, _, dialError := websocket.DefaultDialer.Dial(dialURL, nil)
	if dialError != nil {
		t.Fatalf("failed to connect A: %v", dialError)
	}

	// Spawn PTY 1
	_ = connA.WriteMessage(websocket.BinaryMessage, packSpawnRequest(1, 80, 24))
	_, _, readError := connA.ReadMessage()
	if readError != nil {
		t.Fatalf("failed to read spawn: %v", readError)
	}

	// Wait deterministically for the initial shell prompt before writing commands
	for {
		_, msg, readError := connA.ReadMessage()
		if readError != nil {
			t.Fatalf("failed to read prompt: %v", readError)
		}
		action, termID, _, _ := unpackFrame(msg)
		if action == source.ActionStreamIO && termID == 1 {
			break
		}
	}

	// Output > 256KB of data (we output 600KB to guarantee overflow)
	cmdInput := packStreamIO(1, []byte("seq 1 200000 | head -c 600000\n"))
	_ = connA.WriteMessage(websocket.BinaryMessage, cmdInput)

	// Disconnect Client A immediately so that background drainage runs without socket blocking/backpressure
	_ = connA.Close()

	// Poll core workspace scrollback buffer status deterministically until isTruncated is true
	workspace, _ := registry.GetOrCreateWorkspace(token)
	deadline := time.Now().Add(5 * time.Second)
	var coreBuf []byte
	var coreTrunc bool
	var coreErr error
	for {
		coreBuf, coreTrunc, coreErr = workspace.GetScrollbackBuffer(1)
		if coreTrunc {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for ring buffer truncation flag to set")
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Logf("CORE BUFFER BEFORE RECONNECTION: size=%d, isTruncated=%t, error=%v", len(coreBuf), coreTrunc, coreErr)

	// Reconnect
	connB, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to reconnect: %v", error)
	}
	t.Cleanup(func() {
		_ = connB.Close()
		_ = registry.RemoveWorkspace(token)
	})

	// Read replayed scrollback stream
	_ = connB.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, replayData, error := connB.ReadMessage()
	if error != nil {
		t.Fatalf("failed to read replay: %v", error)
	}

	action, termID, payload, error := unpackFrame(replayData)
	if error != nil {
		t.Fatalf("unpack failed: %v", error)
	}

	if action != source.ActionStreamIO || termID != 1 {
		t.Fatalf("expected stream replay, got action %x term %d", action, termID)
	}

	warning := []byte("\r\n\x1b[33m[... Output truncated due to buffer overflow ...]\x1b[0m\r\n\r\n")
	safeLen := len(payload)
	if safeLen > 200 {
		safeLen = 200
	}
	t.Logf("REPLAYED PAYLOAD LEN: %d, FIRST CHARS: %q", len(payload), string(payload[:safeLen]))
	if !bytes.HasPrefix(payload, warning) {
		t.Fatal("expected replayed payload to start with the truncation warning prefix")
	}

	// Buffer capacity is 256KB (262144 bytes)
	expectedLength := len(warning) + 262144
	if len(payload) != expectedLength {
		t.Errorf("expected total payload length to be %d, got %d", expectedLength, len(payload))
	}
}

// Test Case 4: Cleanup Sweeper Lifecycle
func TestCleanupSweeperLifecycle(t *testing.T) {
	// Scenario A: Expiration, Teardown & Non-Blocking Registry
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	// Set a very short sweeper duration
	source.DefaultSweeperDuration = 200 * time.Millisecond
	defer func() {
		source.DefaultSweeperDuration = 5 * time.Minute
	}()

	tokenA := "sweeper-workspace-a"
	dialURLA := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=" + tokenA

	connA1, _, error := websocket.DefaultDialer.Dial(dialURLA, nil)
	if error != nil {
		t.Fatalf("failed to connect A1: %v", error)
	}

	// Spawn PTY 1
	_ = connA1.WriteMessage(websocket.BinaryMessage, packSpawnRequest(1, 80, 24))
	_, _, error = connA1.ReadMessage()
	if error != nil {
		t.Fatalf("failed to read spawn: %v", error)
	}

	// Write command to dump PIDs to file and start a background sleep process
	parentFile := fmt.Sprintf("/tmp/parent-%s.processID", tokenA)
	childFile := fmt.Sprintf("/tmp/child-%s.processID", tokenA)
	_ = os.Remove(parentFile)
	_ = os.Remove(childFile)

	cmdInput := packStreamIO(1, []byte(fmt.Sprintf("echo $$ > %s && sleep 100 & echo $! > %s\n", parentFile, childFile)))
	_ = connA1.WriteMessage(websocket.BinaryMessage, cmdInput)

	// Poll until parent PID and child PID files exist and are not empty
	deadline := time.Now().Add(5 * time.Second)
	var parentBytes, childBytes []byte
	for {
		parentBytes, _ = os.ReadFile(parentFile)
		childBytes, _ = os.ReadFile(childFile)
		if len(bytes.TrimSpace(parentBytes)) > 0 && len(bytes.TrimSpace(childBytes)) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for processID files to be written")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Read PIDs
	parentPid, error := strconv.Atoi(strings.TrimSpace(string(parentBytes)))
	if error != nil {
		t.Fatalf("invalid parent processID: %v", error)
	}

	childProcessID, error := strconv.Atoi(strings.TrimSpace(string(childBytes)))
	if error != nil {
		t.Fatalf("invalid child processID: %v", error)
	}

	// Clean up temp files
	_ = os.Remove(parentFile)
	_ = os.Remove(childFile)

	// Disconnect Client A1 (initiating sweeper)
	_ = connA1.Close()

	// Concurrently connect Client B1 to a different workspace to verify it is non-blocking
	tokenB := "sweeper-workspace-b"
	dialURLB := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=" + tokenB
	
	startUpgrade := time.Now()
	connB1, _, error := websocket.DefaultDialer.Dial(dialURLB, nil)
	if error != nil {
		t.Fatalf("failed to connect B1: %v", error)
	}
	t.Cleanup(func() {
		_ = connB1.Close()
		_ = registry.RemoveWorkspace(tokenB)
	})
	
	upgradeDuration := time.Since(startUpgrade)
	if upgradeDuration > 100*time.Millisecond {
		t.Errorf("concurrent upgrade was blocked, took %v", upgradeDuration)
	}

	// Poll registry deterministically until workspace A is removed (sweeper is 200ms)
	deadline = time.Now().Add(5 * time.Second)
	for {
		error = registry.RemoveWorkspace(tokenA)
		if error != nil && strings.Contains(error.Error(), "not found") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for workspace to be swept")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Assert parent shell and background daemon are reaped
	error = syscall.Kill(parentPid, 0)
	if error == nil {
		t.Errorf("parent shell PID %d is still alive after sweeper teardown", parentPid)
	} else if error != syscall.ESRCH {
		t.Errorf("unexpected kill error for parent: %v", error)
	}

	error = syscall.Kill(childProcessID, 0)
	if error == nil {
		t.Errorf("background daemon PID %d is still alive after sweeper teardown", childProcessID)
	} else if error != syscall.ESRCH {
		t.Errorf("unexpected kill error for daemon: %v", error)
	}

	// Scenario B: Cancellation on Reconnection
	tokenC := "sweeper-workspace-c"
	dialURLC := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=" + tokenC

	connC1, _, error := websocket.DefaultDialer.Dial(dialURLC, nil)
	if error != nil {
		t.Fatalf("failed to connect C1: %v", error)
	}

	// Spawn PTY 2
	_ = connC1.WriteMessage(websocket.BinaryMessage, packSpawnRequest(2, 80, 24))
	_, _, _ = connC1.ReadMessage()

	// Poll until parent PID file exists and is not empty
	parentFileC := fmt.Sprintf("/tmp/parent-%s.processID", tokenC)
	_ = os.Remove(parentFileC)
	_ = connC1.WriteMessage(websocket.BinaryMessage, packStreamIO(2, []byte(fmt.Sprintf("echo $$ > %s\n", parentFileC))))
	
	deadline = time.Now().Add(5 * time.Second)
	var parentBytesC []byte
	for {
		parentBytesC, _ = os.ReadFile(parentFileC)
		if len(bytes.TrimSpace(parentBytesC)) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for parent processID file to be written")
		}
		time.Sleep(10 * time.Millisecond)
	}
	parentPidC, _ := strconv.Atoi(strings.TrimSpace(string(parentBytesC)))
	_ = os.Remove(parentFileC)

	// Disconnect client
	_ = connC1.Close()

	// Wait 50ms (before sweeper expires at 200ms)
	time.Sleep(50 * time.Millisecond)

	// Reconnect
	connC2, _, error := websocket.DefaultDialer.Dial(dialURLC, nil)
	if error != nil {
		t.Fatalf("failed to reconnect C2: %v", error)
	}
	t.Cleanup(func() {
		_ = connC2.Close()
		_ = registry.RemoveWorkspace(tokenC)
	})

	// Wait 300ms (exceeding original 200ms sweeper duration)
	time.Sleep(300 * time.Millisecond)

	// Assert workspace and parent PID are still alive
	error = syscall.Kill(parentPidC, 0)
	if error != nil {
		t.Errorf("parent shell C PID %d died, expected it to remain alive: %v", parentPidC, error)
	}

	// Scenario C: Rescheduling Chain (Disconnect -> Takeover/Reconnect -> Disconnect -> Sweep)
	tokenD := "sweeper-workspace-d"
	dialURLD := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=" + tokenD

	connD1, _, error := websocket.DefaultDialer.Dial(dialURLD, nil)
	if error != nil {
		t.Fatalf("failed to connect D1: %v", error)
	}

	// Spawn PTY 3
	_ = connD1.WriteMessage(websocket.BinaryMessage, packSpawnRequest(3, 80, 24))
	_, _, _ = connD1.ReadMessage()

	// Disconnect client D1 (starts first sweeper)
	_ = connD1.Close()

	// Wait 50ms (timer is 200ms)
	time.Sleep(50 * time.Millisecond)

	// Reconnect/Takeover D2 (cancels first sweeper)
	connD2, _, error := websocket.DefaultDialer.Dial(dialURLD, nil)
	if error != nil {
		t.Fatalf("failed to reconnect D2: %v", error)
	}

	// Disconnect client D2 (starts second sweeper)
	_ = connD2.Close()

	// Poll registry deterministically until workspace D is removed (sweeper is 200ms)
	deadline = time.Now().Add(5 * time.Second)
	for {
		error = registry.RemoveWorkspace(tokenD)
		if error != nil && strings.Contains(error.Error(), "not found") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for workspace D to be swept in reschedule chain")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Test Case 5: Mutex-Synchronized Playback & Concurrent Inputs
func TestMutexSynchronizedPlaybackAndConcurrentInput(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	token := "mutex-playback-workspace"
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=" + token

	connA, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to connect A: %v", error)
	}

	// Spawn PTY 1
	_ = connA.WriteMessage(websocket.BinaryMessage, packSpawnRequest(1, 80, 24))
	_, _, error = connA.ReadMessage()
	if error != nil {
		t.Fatalf("failed to read spawn: %v", error)
	}

	// Write command that outputs part 1, sleeps, then outputs part 2
	cmdInput := packStreamIO(1, []byte("echo 'PART1' && sleep 0.3 && echo 'PART2'\n"))
	_ = connA.WriteMessage(websocket.BinaryMessage, cmdInput)

	// Read until PART1 is printed and buffered
	for {
		_, msg, error := connA.ReadMessage()
		if error != nil {
			t.Fatalf("failed to read PART1: %v", error)
		}
		action, termID, payload, error := unpackFrame(msg)
		if error == nil && action == source.ActionStreamIO && termID == 1 {
			if strings.Contains(string(payload), "PART1") {
				break
			}
		}
	}

	// Disconnect client
	_ = connA.Close()

	// Reconnect Client B immediately (starts replay of PART1) while shell is sleeping
	connB, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to reconnect B: %v", error)
	}
	t.Cleanup(func() {
		_ = connB.Close()
		_ = registry.RemoveWorkspace(token)
	})

	// Concurrently write client inputs (Stream I/O and Resize) during playback phase
	// Write concurrent input command: "echo 'CONCURRENT' && stty size\n"
	_ = connB.WriteMessage(websocket.BinaryMessage, packStreamIO(1, []byte("echo 'CONCURRENT' && stty size\n")))
	
	// Write concurrent resize command: 100x30
	resizeFrame := make([]byte, 8)
	binary.BigEndian.PutUint16(resizeFrame[0:2], source.ActionResize) // Resize Action ID
	binary.BigEndian.PutUint16(resizeFrame[2:4], 1)      // Terminal ID
	binary.BigEndian.PutUint16(resizeFrame[4:6], 100)    // Cols
	binary.BigEndian.PutUint16(resizeFrame[6:8], 30)     // Rows
	_ = connB.WriteMessage(websocket.BinaryMessage, resizeFrame)

	// Poll connB until all expected strings are in the output or timeout occurs
	var received []string
	deadline := time.Now().Add(5 * time.Second)
	_ = connB.SetReadDeadline(deadline)
	for {
		_, msg, error := connB.ReadMessage()
		if error != nil {
			t.Fatalf("failed to read message: %v", error)
		}
		action, termID, payload, error := unpackFrame(msg)
		if error == nil && action == source.ActionStreamIO && termID == 1 {
			received = append(received, string(payload))
			fullText := strings.Join(received, "")
			if strings.Contains(fullText, "PART1") &&
				strings.Contains(fullText, "PART2") &&
				strings.Contains(fullText, "CONCURRENT") &&
				strings.Contains(fullText, "30 100") {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for expected outputs")
		}
	}

	fullText := strings.Join(received, "")
	
	// 1. Must contain PART1 (from scrollback)
	if !strings.Contains(fullText, "PART1") {
		t.Errorf("missing PART1 in output: %q", fullText)
	}
	// 2. Must contain PART2 (from live stream)
	if !strings.Contains(fullText, "PART2") {
		t.Errorf("missing PART2 in output: %q", fullText)
	}
	// 3. Must contain CONCURRENT (from concurrent input)
	if !strings.Contains(fullText, "CONCURRENT") {
		t.Errorf("missing CONCURRENT in output: %q", fullText)
	}
	// 4. Verify PTY window size was updated to 100x30 (stty size prints rows columns, i.e., "30 100")
	if !strings.Contains(fullText, "30 100") {
		t.Errorf("missing dimension change '30 100' in output: %q", fullText)
	}

	// 5. Verify chronological order: PART1 (replay) must precede CONCURRENT and PART2
	idxPart1 := strings.Index(fullText, "PART1")
	idxConcurrent := strings.Index(fullText, "CONCURRENT")
	idxPart2 := strings.Index(fullText, "PART2")
	if idxPart1 > idxConcurrent {
		t.Errorf("chronological order failure: replay PART1 was not delivered before concurrent command output. fullText: %q", fullText)
	}
	if idxPart1 > idxPart2 {
		t.Errorf("chronological order failure: replay PART1 was not delivered before live PART2. fullText: %q", fullText)
	}
}

// Test Case 6: Valid Visibility Shifts State Sync (Whole state API)
func TestValidPrioritySync(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	token := "visibility-sync-workspace"
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=" + token

	connection, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to connect: %v", error)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		_ = registry.RemoveWorkspace(token)
	})

	// Spawn PTY 1 and PTY 2
	_ = connection.WriteMessage(websocket.BinaryMessage, packSpawnRequest(1, 80, 24))
	_, _, _ = connection.ReadMessage()
	_ = connection.WriteMessage(websocket.BinaryMessage, packSpawnRequest(2, 80, 24))
	_, _, _ = connection.ReadMessage()

	// Wait deterministically for prompts to avoid typeahead issues
	for {
		_, msg, error := connection.ReadMessage()
		if error != nil {
			t.Fatalf("failed to read output: %v", error)
		}
		action, termID, _, _ := unpackFrame(msg)
		if action == source.ActionStreamIO && termID == 2 {
			break
		}
	}

	// Pack priority sync frame:
	// Set PTY 1 to High (0x01) and PTY 2 to Low (0x00)
	// Layout: Action (2B: source.ActionPrioritySync) [ignored Header TermID (2B: 0)] [TermID 1 (2B)] [State 1 (1B)] [TermID 2 (2B)] [State 2 (1B)]
	syncFrame := make([]byte, 10)
	binary.BigEndian.PutUint16(syncFrame[0:2], source.ActionPrioritySync)
	binary.BigEndian.PutUint16(syncFrame[2:4], 0)
	binary.BigEndian.PutUint16(syncFrame[4:6], 1)
	syncFrame[6] = 0x01
	binary.BigEndian.PutUint16(syncFrame[7:9], 2)
	syncFrame[9] = 0x00
 
	_ = connection.WriteMessage(websocket.BinaryMessage, syncFrame)
 
	// Sleep briefly to let server state update
	time.Sleep(50 * time.Millisecond)
 
	// Verify workspace states directly
	workspace, error := registry.GetOrCreateWorkspace(token)
	if error != nil {
		t.Fatalf("failed to resolve workspace: %v", error)
	}
 
	// Get PTY 1 priority (must be High 0x01)
	priority1, error := workspace.GetPTYPriority(1)
	if error != nil {
		t.Fatalf("failed to get PTY 1 priority: %v", error)
	}
	if priority1 != 0x01 {
		t.Errorf("expected PTY 1 priority to be 0x01, got %x", priority1)
	}
 
	// Get PTY 2 priority (must be Low 0x00)
	priority2, error := workspace.GetPTYPriority(2)
	if error != nil {
		t.Fatalf("failed to get PTY 2 priority: %v", error)
	}
	if priority2 != 0x00 {
		t.Errorf("expected PTY 2 priority to be 0x00, got %x", priority2)
	}
 
	// Send another sync frame omitting PTY 1 (must default/demote PTY 1 to Low 0x00, and set PTY 2 to High 0x01)
	syncFrame2 := make([]byte, 7)
	binary.BigEndian.PutUint16(syncFrame2[0:2], source.ActionPrioritySync)
	binary.BigEndian.PutUint16(syncFrame2[2:4], 0)
	binary.BigEndian.PutUint16(syncFrame2[4:6], 2)
	syncFrame2[6] = 0x01
 
	_ = connection.WriteMessage(websocket.BinaryMessage, syncFrame2)
 
	// Sleep briefly to let server state update
	time.Sleep(50 * time.Millisecond)
 
	priority1, _ = workspace.GetPTYPriority(1)
	if priority1 != 0x00 {
		t.Errorf("expected omitted PTY 1 priority to default to Low 0x00, got %x", priority1)
	}
 
	priority2, _ = workspace.GetPTYPriority(2)
	if priority2 != 0x01 {
		t.Errorf("expected PTY 2 priority to update to High 0x01, got %x", priority2)
	}
}

// TestSweeperTimerCallbackRaceRegression verifies that if a client reconnects
// at the exact moment the sweeper timer fires, the workspace is NOT reaped.
func TestSweeperTimerCallbackRaceRegression(t *testing.T) {
	// Override DefaultSweeperDuration for testing
	oldDuration := source.DefaultSweeperDuration
	source.DefaultSweeperDuration = 30 * time.Millisecond
	defer func() {
		source.DefaultSweeperDuration = oldDuration
	}()

	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=race-token"

	// 1. Connect Client A
	connA, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to connect A: %v", error)
	}

	workspace, error := registry.GetOrCreateWorkspace("race-token")
	if error != nil {
		t.Fatalf("failed to get workspace: %v", error)
	}

	// Spawn a PTY to keep the workspace active
	if error := workspace.SpawnPTY(1, 80, 24); error != nil {
		t.Fatalf("failed to spawn PTY: %v", error)
	}

	// 2. Disconnect Client A (starts the 30ms sweeper timer)
	_ = connA.Close()

	// Wait exactly 30ms (until the timer fires/expires and the callback goroutine is scheduled)
	time.Sleep(30 * time.Millisecond)

	// 3. Immediately connect Client B (takeover/reconnect)
	connB, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to connect B: %v", error)
	}
	defer connB.Close()

	// Wait a moment to allow the sweeper goroutine (which was already scheduled) to execute
	time.Sleep(100 * time.Millisecond)

	// 4. Verify that:
	// - The workspace still exists in the registry
	_, error = registry.GetOrCreateWorkspace("race-token")
	if error != nil {
		t.Errorf("workspace was incorrectly reaped by the sweeper callback race: %v", error)
	}

	// - Client B's connection is still active and has not been closed by the sweeper
	_ = connB.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	_, _, error = connB.ReadMessage()
	if error != nil && !strings.Contains(error.Error(), "i/o timeout") {
		t.Errorf("Client B's connection was closed unexpectedly: %v", error)
	}
}
