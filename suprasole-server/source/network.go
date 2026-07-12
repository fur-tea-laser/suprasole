package source

import (
	"encoding/binary"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var DefaultSweeperDuration = 5 * time.Minute

type wsConnection struct {
	token      string
	connection *websocket.Conn
	mutex      sync.Mutex // Serializes concurrent WriteMessage and WriteControl calls
	closed     chan struct{}
	once       sync.Once
}

func (connection *wsConnection) WriteFrame(action uint16, terminalID uint16, payload []byte) error {
	connection.mutex.Lock()
	defer connection.mutex.Unlock()
	// Enforce strict write deadline of 5 seconds
	_ = connection.connection.SetWriteDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint16(buf[0:2], action)
	binary.BigEndian.PutUint16(buf[2:4], terminalID)
	copy(buf[4:], payload)
	error := connection.connection.WriteMessage(websocket.BinaryMessage, buf)
	if error != nil {
		go connection.closeWithCode(websocket.CloseAbnormalClosure, "Write timeout/error")
	}
	return error
}

func (connection *wsConnection) closeWithCode(code int, text string) {
	connection.once.Do(func() {
		close(connection.closed)
		// Write WebSocket close frame cleanly
		_ = connection.connection.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(code, text),
			time.Now().Add(1*time.Second),
		)
		_ = connection.connection.Close()
	})
}

type connectionRegistry struct {
	mutex       sync.Mutex
	connections map[string]*wsConnection
	sweepers    map[string]*time.Timer
}

func newConnectionRegistry() *connectionRegistry {
	return &connectionRegistry{
		connections: make(map[string]*wsConnection),
		sweepers:    make(map[string]*time.Timer),
	}
}

func (registry *connectionRegistry) getActiveConnection(token string) *wsConnection {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	return registry.connections[token]
}

func (registry *connectionRegistry) registerOrHijack(token string, newConn *wsConnection) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	for otherToken, otherConn := range registry.connections {
		if t, exists := registry.sweepers[otherToken]; exists {
			t.Stop()
			delete(registry.sweepers, otherToken)
		}
		if otherToken == token {
			otherConn.closeWithCode(4000, "Session Taken Over")
		} else {
			otherConn.closeWithCode(1000, "Singleton connection takeover")
		}
		delete(registry.connections, otherToken)
	}
	if t, exists := registry.sweepers[token]; exists {
		t.Stop()
		delete(registry.sweepers, token)
	}
	registry.connections[token] = newConn
}

func (registry *connectionRegistry) unregisterAndSweep(token string, connection *wsConnection, sweepDuration time.Duration, sweepAction func()) {
	registry.mutex.Lock()
	existing, exists := registry.connections[token]
	if exists && existing != connection {
		registry.mutex.Unlock()
		return
	}
	_, hasSweeper := registry.sweepers[token]
	if !exists && hasSweeper {
		registry.mutex.Unlock()
		return
	}
	if exists {
		delete(registry.connections, token)
	}
	var timerStarted bool
	if sweepDuration > 0 {
		if t, exists := registry.sweepers[token]; exists {
			t.Stop()
		}
		registry.sweepers[token] = time.AfterFunc(sweepDuration, func() {
			registry.mutex.Lock()
			delete(registry.sweepers, token)
			activeConn := registry.connections[token]
			registry.mutex.Unlock()
			if activeConn == nil {
				sweepAction()
			}
		})
		timerStarted = true
	}
	registry.mutex.Unlock()
	if !timerStarted && sweepDuration <= 0 {
		sweepAction()
	}
}

func (registry *connectionRegistry) closeAndRemove(token string, code int, text string) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	if t, exists := registry.sweepers[token]; exists {
		t.Stop()
		delete(registry.sweepers, token)
	}
	connection, exists := registry.connections[token]
	if exists {
		connection.closeWithCode(code, text)
		delete(registry.connections, token)
	}
}

type interceptedRegistry struct {
	WorkspaceRegistry
	netRegistry *connectionRegistry
}

func (interceptedRegistry *interceptedRegistry) RemoveWorkspace(workspaceID string) error {
	interceptedRegistry.netRegistry.closeAndRemove(workspaceID, websocket.CloseNormalClosure, "Workspace removed")
	return interceptedRegistry.WorkspaceRegistry.RemoveWorkspace(workspaceID)
}

type networkHandler struct {
	registry    WorkspaceRegistry
	netRegistry *connectionRegistry
	upgrader    websocket.Upgrader
}

// NewHandler creates a new HTTP handler that upgrades incoming WebSocket requests
// on path "/ws" and binds them to the provided WorkspaceRegistry.
func NewHandler(registry WorkspaceRegistry) http.Handler {
	netRegistry := newConnectionRegistry()
	wrappedRegistry := &interceptedRegistry{
		WorkspaceRegistry: registry,
		netRegistry:       netRegistry,
	}
	return &networkHandler{
		registry:    wrappedRegistry,
		netRegistry: netRegistry,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  32768,
			WriteBufferSize: 32768,
			CheckOrigin: func(request *http.Request) bool {
				return true
			},
		},
	}
}

func (handler *networkHandler) ServeHTTP(responseWriter http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/ws" {
		http.NotFound(responseWriter, request)
		return
	}
	token := request.URL.Query().Get("token")
	if token == "" {
		http.Error(responseWriter, "Missing token query parameter", http.StatusBadRequest)
		return
	}
	// Upgrade connection to WebSocket
	connection, upgradeError := handler.upgrader.Upgrade(responseWriter, request, nil)
	if upgradeError != nil {
		return
	}
	// Set small write buffer to allow write deadline tests to saturate TCP buffers fast
	if tcpConn, ok := connection.UnderlyingConn().(*net.TCPConn); ok {
		_ = tcpConn.SetWriteBuffer(4096)
	}
	// Set maximum message size constraint (64KB payload + 4 bytes header)
	connection.SetReadLimit(65536 + 4)
	// Fetch/Create core workspace state
	workspace, workspaceError := handler.registry.GetOrCreateWorkspace(token)
	if workspaceError != nil {
		_ = connection.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to resolve workspace"),
			time.Now().Add(1*time.Second),
		)
		connection.Close()
		return
	}
	wsConn := &wsConnection{
		token:      token,
		connection: connection,
		closed:     make(chan struct{}),
	}
	// Session Takeover / Hijacking Registration
	handler.netRegistry.registerOrHijack(token, wsConn)
	// 2. Perform State Replay by enqueuing into the PTY queues atomically
	var replayFrames []OutboundFrame
	activeTerminalIDs := workspace.GetActiveTerminalIDs()
	for _, termID := range activeTerminalIDs {
		buf, isTruncated, scrollbackError := workspace.GetScrollbackBuffer(termID)
		if scrollbackError != nil {
			continue
		}
		replayPayload := buf
		if isTruncated {
			warning := []byte("\r\n\x1b[33m[... Output truncated due to buffer overflow ...]\x1b[0m\r\n\r\n")
			replayPayload = make([]byte, len(warning)+len(buf))
			copy(replayPayload, warning)
			copy(replayPayload[len(warning):], buf)
		}
		replayFrames = append(replayFrames, OutboundFrame{
			Action:           ActionStreamIO,
			TerminalID:       termID,
			Payload:          replayPayload,
			DrainingPriority: PriorityHigh, // High Priority
		})
	}
	// Send spawn success status for all active terminals to synchronize client terminal states
	for _, termID := range activeTerminalIDs {
		replayFrames = append(replayFrames, OutboundFrame{
			Action:           ActionSpawnStatus,
			TerminalID:       termID,
			Payload:          []byte{0x00},
			DrainingPriority: PriorityHigh, // High Priority
		})
	}
	workspace.FlushAndEnqueueReplays(replayFrames)
	// Bind wsConn as the socket writer -> Scheduler starts draining enqueued replays
	workspace.SetSocketWriter(wsConn)
	// Configure deadlines and ping tickers
	_ = connection.SetReadDeadline(time.Now().Add(40 * time.Second))
	connection.SetPongHandler(func(appData string) error {
		_ = connection.SetReadDeadline(time.Now().Add(40 * time.Second))
		return nil
	})
	ticker := time.NewTicker(30 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				wsConn.mutex.Lock()
				_ = connection.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(2*time.Second))
				wsConn.mutex.Unlock()
			case <-wsConn.closed:
				return
			}
		}
	}()
	defer func() {
		wsConn.closeWithCode(websocket.CloseNormalClosure, "Connection closing")
		workspace.ClearSocketWriter(wsConn)
		handler.netRegistry.unregisterAndSweep(token, wsConn, DefaultSweeperDuration, func() {
			_ = handler.registry.RemoveWorkspace(token)
		})
	}()
	// Main binary frame reading and dispatch loop
	for {
		msgType, message, error := connection.ReadMessage()
		if error != nil {
			break
		}
		if msgType != websocket.BinaryMessage {
			wsConn.closeWithCode(websocket.CloseUnsupportedData, "Unsupported frame type")
			break
		}
		if len(message) < 4 {
			wsConn.closeWithCode(websocket.CloseProtocolError, "Frame too short")
			break
		}
		action := binary.BigEndian.Uint16(message[0:2])
		terminalID := binary.BigEndian.Uint16(message[2:4])
		payload := message[4:]
		switch action {
		case ActionSpawn: // Spawn Request
			if len(payload) != 4 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Invalid spawn request layout")
				return
			}
			columns := binary.BigEndian.Uint16(payload[0:2])
			rows := binary.BigEndian.Uint16(payload[2:4])
			if columns == 0 || rows == 0 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Invalid terminal dimensions")
				return
			}
			// Spawn asynchronously to keep the main read loop non-blocking
			go func(terminalID uint16, columns uint16, rows uint16) {
				spawnError := workspace.SpawnPTY(terminalID, columns, rows)
				status := byte(0x00)
				if spawnError != nil {
					status = byte(0x01)
				}
				workspace.EnqueueControlFrame(OutboundFrame{
					Action:           ActionSpawnStatus,
					TerminalID:       terminalID,
					Payload:          []byte{status},
					DrainingPriority: PriorityHigh, // High Priority
				})
			}(terminalID, columns, rows)
		case ActionResize: // Resize Request
			if len(payload) != 4 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Invalid resize request layout")
				return
			}
			columns := binary.BigEndian.Uint16(payload[0:2])
			rows := binary.BigEndian.Uint16(payload[2:4])
			if columns == 0 || rows == 0 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Invalid terminal dimensions")
				return
			}
			_ = workspace.ResizePTY(terminalID, columns, rows)
		case ActionKill: // Kill Request
			if len(payload) != 0 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Invalid termination request layout")
				return
			}
			_ = workspace.TerminatePTY(terminalID)
		case ActionStreamIO: // Stream I/O
			_ = workspace.WritePTYInput(terminalID, payload)
		case ActionPrioritySync: // Priority Sync (Whole set of terminals)
			if len(payload)%3 != 0 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Invalid priority state layout")
				return
			}
			numPTYs := len(payload) / 3
			specified := make(map[uint16]byte)
			for i := 0; i < numPTYs; i++ {
				termID := binary.BigEndian.Uint16(payload[i*3 : i*3+2])
				state := payload[i*3+2]
				if state != PriorityLow && state != PriorityHigh {
					wsConn.closeWithCode(websocket.CloseProtocolError, "Invalid priority state value")
					return
				}
				specified[termID] = state
			}
			// Update all active terminals in the workspace
			activeIDs := workspace.GetActiveTerminalIDs()
			for _, termID := range activeIDs {
				state := specified[termID]
				_ = workspace.SetPTYPriority(termID, state)
			}
		default:
			wsConn.closeWithCode(websocket.CloseProtocolError, "Unrecognized Action ID")
			return
		}
	}
}
