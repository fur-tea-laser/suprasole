package gotests

import (
	"bytes"
	"encoding/binary"
	"net/http/httptest"
	"strings"
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
		if action == source.ActionOutput && termID == 1 {
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
	if replayAction != source.ActionOutput || replayTermID != 1 {
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
		if action == source.ActionOutput && termID == 1 {
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
	if action != source.ActionOutput || termID != 1 {
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
	for i := 0; i < 2; i++ {
		_, _, error = connA.ReadMessage()
		if error != nil {
			t.Fatalf("failed to read spawn status: %v", error)
		}
	}
	// Wait for the initial shell prompt before writing commands
	for {
		_, msg, error := connA.ReadMessage()
		if error != nil {
			t.Fatalf("failed to read prompt: %v", error)
		}
		action, termID, _, _ := unpackFrame(msg)
		if action == source.ActionOutput && termID == 1 {
			break
		}
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
		if error == nil && action == source.ActionOutput && termID == 1 {
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
	binary.BigEndian.PutUint16(resizeFrame[2:4], 1)                   // Terminal ID
	binary.BigEndian.PutUint16(resizeFrame[4:6], 100)                 // Cols
	binary.BigEndian.PutUint16(resizeFrame[6:8], 30)                  // Rows
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
		if error == nil && action == source.ActionOutput && termID == 1 {
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
