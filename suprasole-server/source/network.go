package source

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

var connectionSequence uint32

type wsConnection struct {
	token      string
	connection *websocket.Conn
	mutex      sync.Mutex // Serializes concurrent WriteMessage and WriteControl calls
	closed     chan struct{}
	done       chan struct{}
	once       sync.Once
	sequence   uint32
}

func (connection *wsConnection) WriteFrame(action uint16, terminalID uint16, payload []byte) error {
	connection.mutex.Lock()
	defer connection.mutex.Unlock()
	// Enforce strict write deadline of 3 seconds
	_ = connection.connection.SetWriteDeadline(time.Now().Add(3 * time.Second))
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
		connection.mutex.Lock()
		conn := connection.connection
		connection.mutex.Unlock()
		if conn != nil {
			if code != websocket.CloseMessageTooBig {
				connection.mutex.Lock()
				// Write WebSocket close frame cleanly
				_ = conn.WriteControl(
					websocket.CloseMessage,
					websocket.FormatCloseMessage(code, text),
					time.Now().Add(1*time.Second),
				)
				connection.mutex.Unlock()
			}
		}
	})
}

type connectionRegistry struct {
	mutex       sync.Mutex
	connections map[string]*wsConnection
}

func newConnectionRegistry() *connectionRegistry {
	return &connectionRegistry{
		connections: make(map[string]*wsConnection),
	}
}

func (registry *connectionRegistry) getActiveConnection(token string) *wsConnection {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	return registry.connections[token]
}

func (registry *connectionRegistry) commitConnection(token string, newConn *wsConnection) *wsConnection {
	registry.mutex.Lock()
	if active, exists := registry.connections[token]; exists {
		if active.sequence > newConn.sequence {
			registry.mutex.Unlock()
			go newConn.closeWithCode(4000, "Session Taken Over")
			return nil
		}
	}
	type eviction struct {
		conn *wsConnection
		code int
		text string
	}
	var toEvict []eviction
	var evictedConn *wsConnection
	for otherToken, otherConn := range registry.connections {
		if otherToken == token {
			evictedConn = otherConn
		} else {
			toEvict = append(toEvict, eviction{conn: otherConn, code: 1000, text: "Singleton connection takeover"})
		}
		delete(registry.connections, otherToken)
	}
	registry.connections[token] = newConn
	registry.mutex.Unlock()
	for _, ev := range toEvict {
		ev.conn.closeWithCode(ev.code, ev.text)
	}
	return evictedConn
}

func (registry *connectionRegistry) unregisterConnection(token string, connection *wsConnection) {
	registry.mutex.Lock()
	defer registry.mutex.Unlock()
	existing, exists := registry.connections[token]
	if exists && existing == connection {
		delete(registry.connections, token)
	}
}

func (registry *connectionRegistry) closeAndRemove(token string, code int, text string) {
	registry.mutex.Lock()
	connection, exists := registry.connections[token]
	if exists {
		delete(registry.connections, token)
	}
	registry.mutex.Unlock()
	if exists {
		connection.closeWithCode(code, text)
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
	wsConn := &wsConnection{
		token:    token,
		closed:   make(chan struct{}),
		done:     make(chan struct{}),
		sequence: atomic.AddUint32(&connectionSequence, 1),
	}
	defer func() {
		select {
		case <-wsConn.done:
		default:
			close(wsConn.done)
		}
	}()
	// Upgrade connection to WebSocket concurrently (returns 101 Switching Protocols to client, unblocking Dial)
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
	wsConn.mutex.Lock()
	wsConn.connection = connection
	wsConn.mutex.Unlock()
	// Commit and evict the old connection atomically
	oldConn := handler.netRegistry.commitConnection(token, wsConn)
	if oldConn != nil {
		oldConn.closeWithCode(4000, "Session Taken Over")
	}
	// Check if we were immediately evicted during/after the commit process
	select {
	case <-wsConn.closed:
		_ = connection.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(4000, "Session Taken Over"),
			time.Now().Add(1*time.Second),
		)
		return
	default:
	}
	// Fetch/Create core workspace state
	workspace, workspaceError := handler.registry.GetOrCreateWorkspace(token)
	if workspaceError != nil {
		_ = connection.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseInternalServerErr, "Failed to resolve workspace"),
			time.Now().Add(1*time.Second),
		)
		connection.Close()
		handler.netRegistry.unregisterConnection(token, wsConn)
		return
	}
	// 2. Perform State Replay and bind wsConn atomically under the registry lock
	handler.netRegistry.mutex.Lock()
	if handler.netRegistry.connections[token] == wsConn {
		workspace.PerformReplayTakeover(wsConn)
	}
	handler.netRegistry.mutex.Unlock()
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
	closeCode := websocket.CloseNormalClosure
	closeText := "Connection closing"
	defer func() {
		wsConn.closeWithCode(closeCode, closeText)
		workspace.ClearSocketWriter(wsConn)
		handler.netRegistry.unregisterConnection(token, wsConn)
		wsConn.mutex.Lock()
		conn := wsConn.connection
		wsConn.mutex.Unlock()
		if conn != nil {
			if tcpConn, ok := conn.UnderlyingConn().(*net.TCPConn); ok {
				_ = tcpConn.SetLinger(2)
				_ = tcpConn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
				buf := make([]byte, 1024)
				for {
					_, err := tcpConn.Read(buf)
					if err != nil {
						break
					}
				}
			}
			_ = conn.Close()
		}
		select {
		case <-wsConn.done:
		default:
			close(wsConn.done)
		}
	}()
	// Main binary frame reading and dispatch loop
	for {
		msgType, message, error := connection.ReadMessage()
		if error != nil {
			if error == websocket.ErrReadLimit || error.Error() == "websocket: read limit exceeded" {
				closeCode = websocket.CloseMessageTooBig
				closeText = "Message size limit exceeded"
			}
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
			reader := bytes.NewReader(payload)
			columns, err1 := readUint16(reader)
			rows, err2 := readUint16(reader)
			command, err3 := readString16(reader)
			argCount, err4 := readUint8(reader)
			if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Invalid spawn request layout")
				return
			}
			if columns == 0 || rows == 0 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Dimensions cannot be zero")
				return
			}
			if command == "" {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Command path cannot be empty")
				return
			}
			var args []string
			for i := 0; i < int(argCount); i++ {
				argVal, err := readString16(reader)
				if err != nil {
					wsConn.closeWithCode(websocket.CloseProtocolError, "Spawn request payload truncated before argument value")
					return
				}
				args = append(args, argVal)
			}
			if reader.Len() > 0 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Malformed spawn request payload (trailing bytes)")
				return
			}
			// Dispatch to core SpawnPTY. The core handles all synchronous/asynchronous error framing internally.
			_ = workspace.SpawnPTY(terminalID, columns, rows, command, args...)
		case ActionResize: // Resize Request
			if len(payload) != 4 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Invalid resize request layout")
				return
			}
			columns := binary.BigEndian.Uint16(payload[0:2])
			rows := binary.BigEndian.Uint16(payload[2:4])
			if columns == 0 || rows == 0 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Dimensions cannot be zero")
				return
			}
			_ = workspace.ResizePTY(terminalID, columns, rows)
		case ActionKill: // Kill Request
			if len(payload) != 0 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Invalid termination request layout")
				return
			}
			_ = workspace.TerminatePTY(terminalID)
		case ActionRemove: // Remove Request
			if len(payload) != 0 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Invalid remove request layout")
				return
			}
			_ = workspace.RemovePTY(terminalID)
		case ActionInput: // Input (0x0007)
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
			_ = workspace.SyncPTYPriorities(specified)
		case ActionReset: // Reset Request
			if len(payload) != 0 {
				wsConn.closeWithCode(websocket.CloseProtocolError, "Invalid reset request layout")
				return
			}
			_ = workspace.ResetWorkspace()
		default:
			wsConn.closeWithCode(websocket.CloseProtocolError, "Unrecognized Action ID")
			return
		}
	}
}

func readUint16(reader *bytes.Reader) (uint16, error) {
	var b [2]byte
	if _, err := io.ReadFull(reader, b[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint16(b[:]), nil
}

func readUint8(reader *bytes.Reader) (uint8, error) {
	var b [1]byte
	if _, err := io.ReadFull(reader, b[:]); err != nil {
		return 0, err
	}
	return b[0], nil
}

func readString16(reader *bytes.Reader) (string, error) {
	length, err := readUint16(reader)
	if err != nil {
		return "", err
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(reader, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}
