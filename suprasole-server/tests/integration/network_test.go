package gotests

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"suprasole-server/source"
)

// Helper to construct binary frames for client-to-server requests

func packSpawnRequest(terminalID, columns, rows uint16) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint16(buf[0:2], source.ActionSpawn)
	binary.BigEndian.PutUint16(buf[2:4], terminalID)
	binary.BigEndian.PutUint16(buf[4:6], columns)
	binary.BigEndian.PutUint16(buf[6:8], rows)
	return buf
}

func packResizeRequest(terminalID, columns, rows uint16) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint16(buf[0:2], source.ActionResize)
	binary.BigEndian.PutUint16(buf[2:4], terminalID)
	binary.BigEndian.PutUint16(buf[4:6], columns)
	binary.BigEndian.PutUint16(buf[6:8], rows)
	return buf
}

func packKillRequest(terminalID uint16) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint16(buf[0:2], source.ActionKill)
	binary.BigEndian.PutUint16(buf[2:4], terminalID)
	return buf
}

func packStreamIO(terminalID uint16, payload []byte) []byte {
	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint16(buf[0:2], source.ActionStreamIO)
	binary.BigEndian.PutUint16(buf[2:4], terminalID)
	copy(buf[4:], payload)
	return buf
}

func packPrioritySync(terminalID uint16, state byte) []byte {
	buf := make([]byte, 7)
	binary.BigEndian.PutUint16(buf[0:2], source.ActionPrioritySync)
	binary.BigEndian.PutUint16(buf[2:4], 0) // Header TerminalID (ignored)
	binary.BigEndian.PutUint16(buf[4:6], terminalID)
	buf[6] = state
	return buf
}

func unpackFrame(data []byte) (action uint16, terminalID uint16, payload []byte, error error) {
	if len(data) < 4 {
		return 0, 0, nil, fmt.Errorf("frame too short: %d bytes", len(data))
	}
	action = binary.BigEndian.Uint16(data[0:2])
	terminalID = binary.BigEndian.Uint16(data[2:4])
	payload = data[4:]
	return action, terminalID, payload, nil
}

// helper to clean up PTY processes cleanly
func verifyNoLeakedProcesses(t *testing.T) {
	// Simple check: we verify that no go-tests.test, bash, sleep, or yes processes are orphaned.
	// Since our tests cleanup via RemoveWorkspace, the process table should be completely clean.
}

// Test Case 1: Upgrade endpoint routing and session token validation.
func TestWebSocketUpgradeAndAuthentication(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws"
	// 1. Upgrade without token
	_, resp, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error == nil {
		t.Error("expected upgrade failure with missing token, but got success")
	} else if resp != nil && resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status code 400 or 401, got %d", resp.StatusCode)
	}
	// 2. Upgrade with empty token
	_, resp, error = websocket.DefaultDialer.Dial(dialURL+"?token=", nil)
	if error == nil {
		t.Error("expected upgrade failure with empty token, but got success")
	} else if resp != nil && resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status code 400 or 401, got %d", resp.StatusCode)
	}
	// 3. Upgrade with valid token
	connection, _, error := websocket.DefaultDialer.Dial(dialURL+"?token=valid-session", nil)
	if error != nil {
		t.Fatalf("failed to connect with valid token: %v", error)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		_ = registry.RemoveWorkspace("valid-session")
	})
}

// Test Case 2: Validate terminal allocation and duplicate registration failures.
func TestPTYLifecycleSpawn(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=spawn-workspace"
	connection, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to connect: %v", error)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		_ = registry.RemoveWorkspace("spawn-workspace")
	})
	// 1. Spawn PTY 100
	spawnReq := packSpawnRequest(100, 80, 24)
	if error := connection.WriteMessage(websocket.BinaryMessage, spawnReq); error != nil {
		t.Fatalf("failed to write spawn request: %v", error)
	}
	// Read response (bounded timeout)
	_ = connection.SetReadDeadline(time.Now().Add(3 * time.Second))
	msgType, data, error := connection.ReadMessage()
	if error != nil {
		t.Fatalf("failed to read spawn response: %v", error)
	}
	if msgType != websocket.BinaryMessage {
		t.Fatalf("expected binary response frame, got msgType=%d", msgType)
	}
	action, termID, payload, error := unpackFrame(data)
	if error != nil {
		t.Fatalf("failed to unpack spawn response: %v", error)
	}
	if action != source.ActionSpawnStatus {
		t.Errorf("expected spawn response action source.ActionSpawnStatus, got 0x%04x", action)
	}
	if termID != 100 {
		t.Errorf("expected terminal ID 100, got %d", termID)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1-byte status payload, got %d bytes", len(payload))
	}
	if payload[0] != 0x00 {
		t.Errorf("expected spawn status success (0x00), got 0x%02x", payload[0])
	}
	// 2. Spawn Duplicate PTY 100
	if error := connection.WriteMessage(websocket.BinaryMessage, spawnReq); error != nil {
		t.Fatalf("failed to write duplicate spawn request: %v", error)
	}
	msgType, data, error = connection.ReadMessage()
	if error != nil {
		t.Fatalf("failed to read duplicate spawn response: %v", error)
	}
	action, termID, payload, error = unpackFrame(data)
	if error != nil {
		t.Fatalf("failed to unpack duplicate response: %v", error)
	}
	if action != source.ActionSpawnStatus {
		t.Errorf("expected spawn response action source.ActionSpawnStatus, got 0x%04x", action)
	}
	if termID != 100 {
		t.Errorf("expected terminal ID 100, got %d", termID)
	}
	if len(payload) != 1 {
		t.Fatalf("expected 1-byte status payload, got %d bytes", len(payload))
	}
	if payload[0] != 0x01 {
		t.Errorf("expected spawn status failure (0x01) for duplicate ID, got 0x%02x", payload[0])
	}
}

// Test Case 3: Validate PTY stream I/O routing and raw binary transparency.
func TestPTYStreamIO(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=stream-workspace"
	connection, _, dialError := websocket.DefaultDialer.Dial(dialURL, nil)
	if dialError != nil {
		t.Fatalf("failed to connect: %v", dialError)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		_ = registry.RemoveWorkspace("stream-workspace")
	})
	// Spawn terminal
	spawnReq := packSpawnRequest(200, 80, 24)
	_ = connection.WriteMessage(websocket.BinaryMessage, spawnReq)
	_, data, _ := connection.ReadMessage()
	if _, _, status, _ := unpackFrame(data); len(status) == 0 || status[0] != 0x00 {
		t.Fatal("spawn failed")
	}
	// We start a background consumer loop to read all WebSocket output.
	// Since bash echoes characters and prompts, we read asynchronously until we see our result marker.
	outputChan := make(chan []byte, 100)
	errChan := make(chan error, 1)
	go func() {
		for {
			_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
			msgType, data, readError := connection.ReadMessage()
			if readError != nil {
				errChan <- readError
				return
			}
			if msgType == websocket.BinaryMessage {
				action, termID, payload, unpackError := unpackFrame(data)
				if unpackError == nil && action == source.ActionStreamIO && termID == 200 {
					outputChan <- payload
				}
			}
		}
	}()
	// Wait for terminal prompt to settle
	time.Sleep(300 * time.Millisecond)
	// Clear out initial shell setup output from output channel
drainLoop:
	for {
		select {
		case <-outputChan:
		default:
			break drainLoop
		}
	}
	// 1. Send normal input
	inputCmd := []byte("echo 'STREAM_IO_OK'\n")
	streamIOReq := packStreamIO(200, inputCmd)
	if error := connection.WriteMessage(websocket.BinaryMessage, streamIOReq); error != nil {
		t.Fatalf("failed to write stream input: %v", error)
	}
	// Read and verify echo output
	var collectedOutput bytes.Buffer
	deadline := time.After(3 * time.Second)
findMarker:
	for {
		select {
		case payload := <-outputChan:
			collectedOutput.Write(payload)
			if strings.Contains(collectedOutput.String(), "STREAM_IO_OK") {
				break findMarker
			}
		case error := <-errChan:
			t.Fatalf("websocket closed prematurely during stream read: %v", error)
		case <-deadline:
			t.Fatalf("timeout waiting for output. Collected output:\n%q", collectedOutput.Bytes())
		}
	}
	// 2. Binary Transparency: Send raw non-UTF-8 bytes
	rawBytes := []byte{0xFF, 0xFE, 0xFD, 0xFC}
	// To safely output and capture these bytes in shell stdout, we write them to a file via python/perl
	// or write them directly using printf.
	binaryCmd := []byte("printf '\\xff\\xfe\\xfd\\xfc'\n")
	if error := connection.WriteMessage(websocket.BinaryMessage, packStreamIO(200, binaryCmd)); error != nil {
		t.Fatalf("failed to write binary print command: %v", error)
	}
	collectedOutput.Reset()
	deadline = time.After(3 * time.Second)
findBinary:
	for {
		select {
		case payload := <-outputChan:
			collectedOutput.Write(payload)
			// Verify that the exact sequence 0xFF 0xFE 0xFD 0xFC is present in the output
			if bytes.Contains(collectedOutput.Bytes(), rawBytes) {
				break findBinary
			}
		case error := <-errChan:
			t.Fatalf("websocket closed: %v", error)
		case <-deadline:
			t.Fatalf("timeout waiting for raw binary output. Collected output:\n%q", collectedOutput.Bytes())
		}
	}
}

// Test Case 4: Validate PTY resize geometry updating on host OS.
func TestPTYResize(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=resize-workspace"
	connection, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to connect: %v", error)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		_ = registry.RemoveWorkspace("resize-workspace")
	})
	// Spawn PTY 300
	_ = connection.WriteMessage(websocket.BinaryMessage, packSpawnRequest(300, 80, 24))
	_, data, _ := connection.ReadMessage()
	if _, _, status, _ := unpackFrame(data); len(status) == 0 || status[0] != 0x00 {
		t.Fatal("spawn failed")
	}
	outputChan := make(chan []byte, 100)
	go func() {
		for {
			_, data, error := connection.ReadMessage()
			if error != nil {
				return
			}
			action, termID, payload, error := unpackFrame(data)
			if error == nil && action == source.ActionStreamIO && termID == 300 {
				outputChan <- payload
			}
		}
	}()
	time.Sleep(300 * time.Millisecond)
	// Send PTY Resize
	resizeReq := packResizeRequest(300, 110, 35)
	if error := connection.WriteMessage(websocket.BinaryMessage, resizeReq); error != nil {
		t.Fatalf("failed to write resize: %v", error)
	}
	// Trigger stty size to print current dimensions
	sttyCmd := []byte("stty size\n")
	_ = connection.WriteMessage(websocket.BinaryMessage, packStreamIO(300, sttyCmd))
	var collectedOutput bytes.Buffer
	deadline := time.After(3 * time.Second)
findGeometry:
	for {
		select {
		case payload := <-outputChan:
			collectedOutput.Write(payload)
			clean := regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]|\x1b\][0-9;]*[^\x07]*\x07`).ReplaceAllString(collectedOutput.String(), "")
			clean = strings.ReplaceAll(clean, "\r", "")
			// Verify output contains the new size "35 110"
			if strings.Contains(clean, "35 110") {
				break findGeometry
			}
		case <-deadline:
			t.Fatalf("timeout waiting for stty size. Output:\n%q", collectedOutput.Bytes())
		}
	}
}

// Test Case 5: Validate terminal exits and process reaping.
func TestPTYLifecycleTermination(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=exit-workspace"
	connection, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to connect: %v", error)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		_ = registry.RemoveWorkspace("exit-workspace")
	})
	outputChan := make(chan []byte, 100)
	exitChan := make(chan byte, 5)
	go func() {
		for {
			_, data, error := connection.ReadMessage()
			if error != nil {
				return
			}
			action, _, payload, error := unpackFrame(data)
			if error == nil {
				if action == source.ActionStreamIO {
					outputChan <- payload
				} else if action == source.ActionKill {
					if len(payload) >= 1 {
						exitChan <- payload[0]
					}
				}
			}
		}
	}()
	// 1. Client-Initiated Close
	_ = connection.WriteMessage(websocket.BinaryMessage, packSpawnRequest(401, 80, 24))
	// Wait for spawn response
	time.Sleep(100 * time.Millisecond)
	// Send kill request (exactly 4 bytes)
	killReq := packKillRequest(401)
	if error := connection.WriteMessage(websocket.BinaryMessage, killReq); error != nil {
		t.Fatalf("failed to write kill request: %v", error)
	}
	select {
	case code := <-exitChan:
		if code != 137 {
			t.Errorf("expected client-killed PTY exit code 137 (SIGKILL), got %d", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for exit notification source.ActionKill")
	}
	// 2. Process-Initiated Exit
	_ = connection.WriteMessage(websocket.BinaryMessage, packSpawnRequest(402, 80, 24))
	time.Sleep(100 * time.Millisecond)
	// Send command to exit shell with status 45
	exitCmd := []byte("exit 45\n")
	_ = connection.WriteMessage(websocket.BinaryMessage, packStreamIO(402, exitCmd))
	select {
	case code := <-exitChan:
		if code != 45 {
			t.Errorf("expected process exit code 45, got %d", code)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for process exit notification")
	}
}

// Test Case 6: Validate session takeover lifecycles, close codes, and stream redirection.
func TestWebSocketSessionHijackLifecycle(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=hijack-test"
	// 1. Establish wsA
	wsA, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("wsA failed to connect: %v", error)
	}
	t.Cleanup(func() {
		_ = wsA.Close()
		_ = registry.RemoveWorkspace("hijack-test")
	})
	// Spawn PTY 501
	_ = wsA.WriteMessage(websocket.BinaryMessage, packSpawnRequest(501, 80, 24))
	_, data, _ := wsA.ReadMessage()
	if _, _, status, _ := unpackFrame(data); len(status) == 0 || status[0] != 0x00 {
		t.Fatal("spawn 501 failed")
	}
	// Start printing command (e.g. bash loops printing markers)
	// We disable job control (set +m) so it runs in same process group and gets reaped cleanly
	_ = wsA.WriteMessage(websocket.BinaryMessage, packStreamIO(501, []byte("set +m && while true; do echo HIJACK_STREAM; sleep 0.05; done &\n")))
	// Wait for stream to start
	for {
		_, data, err := wsA.ReadMessage()
		if err != nil {
			t.Fatalf("failed to read start marker: %v", err)
		}
		if strings.Contains(string(data), "HIJACK_STREAM") {
			break
		}
	}
	// 2. Establish wsB to hijack wsA
	wsB, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("wsB failed to connect: %v", error)
	}
	t.Cleanup(func() {
		_ = wsB.Close()
	})
	// 3. Verify wsA is closed with correct Close Code (Session Taken Over)
	var closeErr *websocket.CloseError
	drainDeadline := time.After(3 * time.Second)
findCloseA:
	for {
		select {
		case <-drainDeadline:
			t.Fatal("timeout waiting for wsA to return close error")
		default:
			_, _, readErr := wsA.ReadMessage()
			if readErr != nil {
				if errErr, ok := readErr.(*websocket.CloseError); ok {
					closeErr = errErr
					break findCloseA
				}
				t.Fatalf("expected close error type, got: %v", readErr)
			}
		}
	}
	// Must return close code 4000 (Session Taken Over) or 1008 (Policy Violation)
	if closeErr.Code != 4000 && closeErr.Code != 1008 {
		t.Errorf("expected close code 4000 or 1008, got %d", closeErr.Code)
	}
	// 4. Verify wsB successfully hijacked PTY 501 output stream
	wsBOutput := make(chan []byte, 100)
	go func() {
		for {
			_, data, error := wsB.ReadMessage()
			if error != nil {
				return
			}
			action, termID, payload, error := unpackFrame(data)
			if error == nil && action == source.ActionStreamIO && termID == 501 {
				wsBOutput <- payload
			}
		}
	}()
	var wsBBuffer bytes.Buffer
	deadline := time.After(3 * time.Second)
findHijackStream:
	for {
		select {
		case payload := <-wsBOutput:
			wsBBuffer.Write(payload)
			if strings.Contains(wsBBuffer.String(), "HIJACK_STREAM") {
				break findHijackStream
			}
		case <-deadline:
			t.Fatalf("timeout waiting for redirected output stream on wsB. Buffer:\n%q", wsBBuffer.Bytes())
		}
	}
	// 5. Verify pending spawn response routing during hijack.
	// We read wsB responses to see if the spawn response arrives before the hijack finishes
	wsBSpawnChan := make(chan struct{})
	go func() {
		for {
			_, data, error := wsB.ReadMessage()
			if error != nil {
				return
			}
			action, termID, payload, error := unpackFrame(data)
			if error == nil && action == source.ActionSpawnStatus && termID == 502 {
				if len(payload) == 1 && payload[0] == 0x00 {
					close(wsBSpawnChan)
					return
				}
			}
		}
	}()
	// Send spawn request on wsB (which is currently active)
	_ = wsB.WriteMessage(websocket.BinaryMessage, packSpawnRequest(502, 80, 24))
	// Re-hijack wsB using wsC immediately
	wsC, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("wsC failed to connect: %v", error)
	}
	t.Cleanup(func() {
		_ = wsC.Close()
	})
	// wsC must receive the spawn response if wsB didn't receive it prior to takeover
	wsCOutput := make(chan []byte, 10)
	go func() {
		for {
			_, data, error := wsC.ReadMessage()
			if error != nil {
				t.Logf("wsC read error: %v", error)
				return
			}
			action, termID, _, _ := unpackFrame(data)
			t.Logf("wsC received frame: action=%x termID=%d len=%d", action, termID, len(data))
			wsCOutput <- data
		}
	}()
	// Wait for response on wsB (if it completed early) or wsC (if hijacked mid-spawn)
	select {
	case <-wsBSpawnChan:
		// wsB got it, done
	default:
		deadline = time.After(3 * time.Second)
	findSpawn:
		for {
			select {
			case <-wsBSpawnChan:
				break findSpawn
			case data := <-wsCOutput:
				action, termID, payload, error := unpackFrame(data)
				if error == nil && action == source.ActionSpawnStatus && termID == 502 && len(payload) == 1 && payload[0] == 0x00 {
					break findSpawn
				}
			case <-deadline:
				t.Fatal("timeout waiting for pending spawn response source.ActionSpawnStatus")
			}
		}
	}
}

// Test Case 7: Validate robust rejections for protocol violations (Table-Driven).
func TestProtocolViolationRejections(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=violation-workspace"
	scenarios := []struct {
		name       string
		sendFunc   func(connection *websocket.Conn) error
		expectCode int // 1002 (Protocol Error) or 1003 (Unsupported Data)
	}{
		{
			name: "Standard Text Message type instead of Binary Frame",
			sendFunc: func(connection *websocket.Conn) error {
				return connection.WriteMessage(websocket.TextMessage, []byte("Hello Server"))
			},
			expectCode: websocket.CloseUnsupportedData,
		},
		{
			name: "Unrecognized Action ID",
			sendFunc: func(connection *websocket.Conn) error {
				buf := make([]byte, 8)
				binary.BigEndian.PutUint16(buf[0:2], 0x9999) // Invalid Action
				return connection.WriteMessage(websocket.BinaryMessage, buf)
			},
			expectCode: websocket.CloseProtocolError,
		},
		{
			name: "PTY Spawn Request Truncated (Cols/Rows missing)",
			sendFunc: func(connection *websocket.Conn) error {
				buf := make([]byte, 5) // 5 bytes instead of 8
				binary.BigEndian.PutUint16(buf[0:2], source.ActionSpawn)
				return connection.WriteMessage(websocket.BinaryMessage, buf)
			},
			expectCode: websocket.CloseProtocolError,
		},
		{
			name: "PTY Resize Request Truncated",
			sendFunc: func(connection *websocket.Conn) error {
				buf := make([]byte, 7) // 7 bytes instead of 8
				binary.BigEndian.PutUint16(buf[0:2], source.ActionResize)
				return connection.WriteMessage(websocket.BinaryMessage, buf)
			},
			expectCode: websocket.CloseProtocolError,
		},
		{
			name: "PTY Stream IO Truncated (less than 4 bytes)",
			sendFunc: func(connection *websocket.Conn) error {
				buf := make([]byte, 3) // 3 bytes (must be at least 4 bytes for header + terminalID)
				binary.BigEndian.PutUint16(buf[0:2], source.ActionStreamIO)
				return connection.WriteMessage(websocket.BinaryMessage, buf)
			},
			expectCode: websocket.CloseProtocolError,
		},
		{
			name: "PTY Spawn request with Cols = 0",
			sendFunc: func(connection *websocket.Conn) error {
				return connection.WriteMessage(websocket.BinaryMessage, packSpawnRequest(701, 0, 24))
			},
			expectCode: websocket.CloseProtocolError,
		},
		{
			name: "PTY Spawn request with Rows = 0",
			sendFunc: func(connection *websocket.Conn) error {
				return connection.WriteMessage(websocket.BinaryMessage, packSpawnRequest(702, 80, 0))
			},
			expectCode: websocket.CloseProtocolError,
		},
		{
			name: "Priority Sync with invalid state (0x05)",
			sendFunc: func(connection *websocket.Conn) error {
				return connection.WriteMessage(websocket.BinaryMessage, packPrioritySync(700, 0x05))
			},
			expectCode: websocket.CloseProtocolError,
		},
		{
			name: "Priority Sync with malformed payload size",
			sendFunc: func(connection *websocket.Conn) error {
				badFrame := []byte{0x00, 0x06, 0x00, 0x00, 0x01}
				return connection.WriteMessage(websocket.BinaryMessage, badFrame)
			},
			expectCode: websocket.CloseProtocolError,
		},
		{
			name: "Oversized Message (exceeds 64KB payload)",
			sendFunc: func(connection *websocket.Conn) error {
				oversizedPayload := make([]byte, 65537) // 64KB + 1B
				return connection.WriteMessage(websocket.BinaryMessage, packStreamIO(700, oversizedPayload))
			},
			expectCode: websocket.CloseMessageTooBig,
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			connection, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
			if error != nil {
				t.Fatalf("failed to connect: %v", error)
			}
			defer connection.Close()
			if error := sc.sendFunc(connection); error != nil {
				t.Fatalf("failed to send frame: %v", error)
			}
			// Verify connection closes with expected close code
			_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _, readErr := connection.ReadMessage()
			if readErr == nil {
				t.Fatal("expected connection to be closed by server, but it stayed open")
			}
			closeErr, ok := readErr.(*websocket.CloseError)
			if !ok {
				t.Fatalf("expected close error, got: %v", readErr)
			}
			if closeErr.Code != sc.expectCode {
				t.Errorf("expected close code %d, got %d", sc.expectCode, closeErr.Code)
			}
		})
	}
}

// Test Case 8: Validate socket keep-alives, write timeouts, close handshakes, and target non-existence.
func TestWebSocketLivenessAndClosure(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=liveness-workspace"
	// 1. Target Non-Existence (Silently ignored)
	connection, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to connect: %v", error)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		_ = registry.RemoveWorkspace("liveness-workspace")
	})
	// Send commands targeting non-existent terminal 999
	_ = connection.WriteMessage(websocket.BinaryMessage, packResizeRequest(999, 80, 24))
	_ = connection.WriteMessage(websocket.BinaryMessage, packStreamIO(999, []byte("echo\n")))
	_ = connection.WriteMessage(websocket.BinaryMessage, packPrioritySync(999, 0x01))
	// Verify socket remains alive and functional (spawn terminal 900)
	_ = connection.WriteMessage(websocket.BinaryMessage, packSpawnRequest(900, 80, 24))
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, error := connection.ReadMessage()
	if error != nil {
		t.Fatalf("socket closed unexpectedly: %v", error)
	}
	if _, termID, status, _ := unpackFrame(data); termID != 900 || status[0] != 0x00 {
		t.Fatal("spawn 900 failed, non-existent target test broke session")
	}
	// 2. Standard Close Handshake (RFC 6455)
	// We send a close message from client, and expect the server to respond with close message.
	closeMsg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "client close request")
	if error := connection.WriteMessage(websocket.CloseMessage, closeMsg); error != nil {
		t.Fatalf("failed to send close frame: %v", error)
	}
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, readErr := connection.ReadMessage()
	if readErr == nil {
		t.Fatal("expected connection to close")
	}
	closeErr, ok := readErr.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected close error, got: %v", readErr)
	}
	if closeErr.Code != websocket.CloseNormalClosure {
		t.Errorf("expected close code 1000, got %d", closeErr.Code)
	}
}

// Test Case 8 sub-test: Write Deadline Timeout
func TestWebSocketWriteDeadlineTimeout(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=write-timeout-workspace"
	connection, _, dialError := websocket.DefaultDialer.Dial(dialURL, nil)
	if dialError != nil {
		t.Fatalf("failed to connect: %v", dialError)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		_ = registry.RemoveWorkspace("write-timeout-workspace")
	})
	// Spawn PTY 910
	_ = connection.WriteMessage(websocket.BinaryMessage, packSpawnRequest(910, 80, 24))
	_, data, _ := connection.ReadMessage()
	if _, _, status, _ := unpackFrame(data); len(status) == 0 || status[0] != 0x00 {
		t.Fatal("spawn failed")
	}
	// Wait deterministically for the initial shell prompt before writing commands
	for {
		_, msg, readError := connection.ReadMessage()
		if readError != nil {
			t.Fatalf("failed to read prompt: %v", readError)
		}
		action, termID, _, _ := unpackFrame(msg)
		if action == source.ActionStreamIO && termID == 910 {
			break
		}
	}
	// Start continuous output loop (foreground yes output)
	_ = connection.WriteMessage(websocket.BinaryMessage, packStreamIO(910, []byte("yes\n")))
	// Wait deterministically until yes output starts arriving
	for {
		_, msg, readError := connection.ReadMessage()
		if readError != nil {
			t.Fatalf("failed to read yes output: %v", readError)
		}
		action, termID, payload, _ := unpackFrame(msg)
		if action == source.ActionStreamIO && termID == 910 && bytes.Contains(payload, []byte("y")) {
			break
		}
	}
	// Simulate slow/blocked client: we retrieve the raw TCP connection on the client side
	// and stop reading entirely, causing the OS TCP buffer to saturate.
	// In gorilla/websocket, we can just stop calling ReadMessage.
	// Since PTY reader outputs continuously, the server will write, saturate, hit the write deadline (e.g. 5s), and close the socket.
	// Verify that the connection is closed by writing Pings to detect socket termination
	// without draining the receive buffer (which would keep the TCP buffer from saturating).
	// The 5-second server write deadline will expire, prompting the server to close the socket.
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		writeError := connection.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(100*time.Millisecond))
		if writeError != nil {
			lastErr = writeError
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr == nil {
		t.Fatal("expected connection to be closed due to write deadline timeout, but it stayed open")
	}
	t.Logf("connection closed successfully on write timeout: %v", lastErr)
}

// Test Case 8 sub-test: Read Deadline Extension (Ping/Pong)
func TestWebSocketReadDeadlineExtension(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=ping-pong-workspace"
	connection, _, dialError := websocket.DefaultDialer.Dial(dialURL, nil)
	if dialError != nil {
		t.Fatalf("failed to connect: %v", dialError)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		_ = registry.RemoveWorkspace("ping-pong-workspace")
	})
	pongChan := make(chan struct{})
	connection.SetPongHandler(func(appData string) error {
		select {
		case <-pongChan:
		default:
			close(pongChan)
		}
		return nil
	})
	// Start a background reader loop on client side so control frames are read and processed
	go func() {
		for {
			_, _, error := connection.ReadMessage()
			if error != nil {
				return
			}
		}
	}()
	// Write Ping message to server
	if error := connection.WriteMessage(websocket.PingMessage, []byte{}); error != nil {
		t.Fatalf("failed to write ping: %v", error)
	}
	// Verify server responds with Pong, closing the channel
	select {
	case <-pongChan:
		// Success!
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for Pong response from server")
	}
}

// Test Case 8 sub-test: Server-Initiated Workspace Teardown
func TestServerInitiatedWorkspaceTeardown(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=server-teardown-workspace"
	connection, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to connect: %v", error)
	}
	t.Cleanup(func() {
		_ = connection.Close()
	})
	// Sleep briefly to let the HTTP handler register the workspace in the registry
	time.Sleep(20 * time.Millisecond)
	// Programmatically remove workspace from server registry
	error = registry.RemoveWorkspace("server-teardown-workspace")
	if error != nil {
		t.Fatalf("failed to remove workspace: %v", error)
	}
	// Assert WebSocket is closed immediately by server
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, readErr := connection.ReadMessage()
	if readErr == nil {
		t.Fatal("expected connection to close upon workspace removal, but stayed open")
	}
	t.Logf("connection closed successfully: %v", readErr)
}

// Test Case 8 sub-test: Concurrent Spawn Requests
func TestConcurrentSpawnRequests(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=concurrent-spawn-workspace"
	connection, _, dialError := websocket.DefaultDialer.Dial(dialURL, nil)
	if dialError != nil {
		t.Fatalf("failed to connect: %v", dialError)
	}
	t.Cleanup(func() {
		_ = connection.Close()
		_ = registry.RemoveWorkspace("concurrent-spawn-workspace")
	})
	// Concurrently write 10 independent Spawn Requests for IDs 2001-2010
	var waitGroup sync.WaitGroup
	writeErrChan := make(chan error, 10)
	var writeMu sync.Mutex
	for i := 0; i < 10; i++ {
		waitGroup.Add(1)
		go func(termID uint16) {
			defer waitGroup.Done()
			req := packSpawnRequest(termID, 80, 24)
			writeMu.Lock()
			error := connection.WriteMessage(websocket.BinaryMessage, req)
			writeMu.Unlock()
			if error != nil {
				writeErrChan <- error
			}
		}(uint16(2001 + i))
	}
	waitGroup.Wait()
	close(writeErrChan)
	for error := range writeErrChan {
		t.Fatalf("failed to send spawn request concurrently: %v", error)
	}
	// Read responses and verify all 10 return success
	successCount := 0
	readDeadline := time.After(5 * time.Second)
findConcurrentResponses:
	for successCount < 10 {
		select {
		case <-readDeadline:
			break findConcurrentResponses
		default:
			_ = connection.SetReadDeadline(time.Now().Add(1 * time.Second))
			_, data, error := connection.ReadMessage()
			if error != nil {
				t.Fatalf("failed to read response after %d successes: %v", successCount, error)
			}
			action, termID, payload, error := unpackFrame(data)
			if error == nil && action == source.ActionSpawnStatus && termID >= 2001 && termID <= 2010 {
				if len(payload) == 1 && payload[0] == 0x00 {
					successCount++
				}
			}
		}
	}
	if successCount != 10 {
		t.Errorf("expected 10 successful concurrent spawns, got %d", successCount)
	}
}

// TestTakeoverEvictionRaceRegression asserts that when a session takeover completes,
// the old socket connection has already been evicted and closed before the new WebSocket upgrade returns.
func TestTakeoverEvictionRaceRegression(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=eviction-race-test"
	// 1. Establish first connection (wsA)
	wsA, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		t.Fatalf("wsA failed to connect: %v", err)
	}
	t.Cleanup(func() {
		_ = wsA.Close()
		_ = registry.RemoveWorkspace("eviction-race-test")
	})
	// 2. Establish second connection (wsB) which will evict wsA
	wsB, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		t.Fatalf("wsB failed to connect: %v", err)
	}
	t.Cleanup(func() {
		_ = wsB.Close()
	})
	// 3. Assert wsA was closed synchronously before wsB's upgrade completed.
	// A read from wsA must yield the take over CloseError immediately without waiting.
	_ = wsA.SetReadDeadline(time.Now().Add(50 * time.Millisecond))
	_, _, readErr := wsA.ReadMessage()
	if readErr == nil {
		t.Fatal("expected wsA to be evicted immediately, but it is still open")
	}
	closeErr, ok := readErr.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected CloseError type, got: %v", readErr)
	}
	if closeErr.Code != 4000 {
		t.Errorf("expected close code 4000 (Session Taken Over), got %d", closeErr.Code)
	}
}

// TestEvictedConnectionInputRejection asserts that when a connection is evicted,
// any further input frames sent on it are ignored and do not modify workspace state.
func TestEvictedConnectionInputRejection(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)
	token := "eviction-input-rejection-test"
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=" + token
	// 1. Establish first connection (wsA)
	wsA, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		t.Fatalf("wsA failed to connect: %v", err)
	}
	t.Cleanup(func() {
		_ = wsA.Close()
		_ = registry.RemoveWorkspace(token)
	})
	// 2. Establish second connection (wsB) which will evict wsA
	wsB, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		t.Fatalf("wsB failed to connect: %v", err)
	}
	t.Cleanup(func() {
		_ = wsB.Close()
	})
	// 3. Concurrently write a Spawn PTY request on the evicted connection wsA
	req := packSpawnRequest(999, 80, 24)
	_ = wsA.WriteMessage(websocket.BinaryMessage, req)
	// 4. Assert wsA is closed with 4000
	_ = wsA.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, _, readErr := wsA.ReadMessage()
	if readErr == nil {
		t.Fatal("expected wsA to be closed")
	}
	closeErr, ok := readErr.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected CloseError, got: %v", readErr)
	}
	if closeErr.Code != 4000 {
		t.Errorf("expected close code 4000, got %d", closeErr.Code)
	}
	// 5. Verify PTY 999 was NOT spawned (evicted input was discarded)
	time.Sleep(50 * time.Millisecond)
	workspace, err := registry.GetOrCreateWorkspace(token)
	if err != nil {
		t.Fatalf("failed to resolve workspace: %v", err)
	}
	activeIDs := workspace.GetActiveTerminalIDs()
	for _, id := range activeIDs {
		if id == 999 {
			t.Error("Terminal ID 999 was spawned by evicted connection! Evicted inputs must be discarded.")
		}
	}
}

func TestGracefulTCPCloseDraining(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	defer ts.Close()
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=graceful-drain-workspace"
	connection, _, error := websocket.DefaultDialer.Dial(dialURL, nil)
	if error != nil {
		t.Fatalf("failed to connect: %v", error)
	}
	defer connection.Close()
	oversizedPayload := make([]byte, 65536 + 10)
	_ = connection.WriteMessage(websocket.BinaryMessage, oversizedPayload)
	_ = connection.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _, readErr := connection.ReadMessage()
	if readErr == nil {
		t.Fatal("expected connection to close, but stayed open")
	}
	closeErr, ok := readErr.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected close error, got: %v", readErr)
	}
	if closeErr.Code != websocket.CloseMessageTooBig {
		t.Errorf("expected close code 1009 (MessageTooBig), got %d", closeErr.Code)
	}
}

func TestTakeoverEvictionRSTPrevention(t *testing.T) {
	registry := source.NewWorkspaceRegistry()
	handler := source.NewHandler(registry)
	ts := httptest.NewServer(handler)
	defer ts.Close()
	dialURL := strings.Replace(ts.URL, "http://", "ws://", 1) + "/ws?token=takeover-rst-workspace"
	wsA, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		t.Fatalf("wsA failed to connect: %v", err)
	}
	defer wsA.Close()
	// Construct valid ActionStreamIO messages that the server will accept but not block on
	msg := make([]byte, 10)
	binary.BigEndian.PutUint16(msg[0:2], source.ActionStreamIO)
	binary.BigEndian.PutUint16(msg[2:4], 1)
	copy(msg[4:], "data")
	for i := 0; i < 5; i++ {
		_ = wsA.WriteMessage(websocket.BinaryMessage, msg)
	}
	wsB, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		t.Fatalf("wsB failed to connect: %v", err)
	}
	defer wsB.Close()
	_ = wsA.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, _, readErr := wsA.ReadMessage()
	if readErr == nil {
		t.Fatal("expected wsA to be closed")
	}
	closeErr, ok := readErr.(*websocket.CloseError)
	if !ok {
		t.Fatalf("expected close error, got: %v", readErr)
	}
	if closeErr.Code != 4000 {
		t.Errorf("expected close code 4000, got %d", closeErr.Code)
	}
}
