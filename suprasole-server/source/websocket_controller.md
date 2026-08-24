# WebsocketController Architectural Specification & Lifecycle

## Macro Lifecycle Architecture

This section documents the architectural design principles, race-prevention invariants, and thread-synchronization mechanics behind WebsocketController.

### 1. Ingress Concurrency Decoupling (HandleGetPtyRequest)
* **HTTP Multi-Threading vs. Single-Worker Lifecycle Ownership**: Go's net/http handles incoming client requests across arbitrary worker goroutines. Allowing HTTP handlers to directly mutate socket pointers or execute upgrades triggers data races and split-brain session states.
* **Invariant: Single-Source Takeover Signalling**: When a new connection arrives while an active session exists (CONNECTED__WebsocketConnectionStatus), HandleGetPtyRequest does not perform socket teardown directly on the HTTP handler thread. Instead, it flags IsTakeoverPending = true under lock and sends a Close control frame. This forces the active read loop to exit cleanly and delegates teardown exclusively to the background worker thread, eliminating teardown-versus-upgrade race conditions.
* **Invariant: Bounded Queue Backpressure & Resiliency**: Bounding SubmissionQueue via non-blocking select fallback prevents server memory exhaustion during reconnect storms. Synchronous reply channels paired with context monitoring guarantee that client cancellations and obsolete requests release HTTP worker threads cleanly.

### 2. The Single-Threaded Core & Thread-Safe State Machine Isolation (RunLifecycleLoop)
* **Thread-Safe State Machine Isolation & Deadlock Avoidance**:
  * **Elimination of AB-BA Circular Wait Deadlocks**: Domain controllers maintain internal locks. If state transitions and callback dispatches executed under a broad proxy lock across arbitrary HTTP or application threads, a domain callback attempting to acquire a domain lock while an application thread calls an egress method would trigger AB-BA circular wait deadlocks. Confining state transitions and callback dispatches to a single background worker thread enforces a strict single-direction thread ownership model.
  * **Unlocked Callback Dispatch**: Callbacks are strictly invoked unlocked outside Mutex. Releasing locks before invoking external callbacks prevents lock coupling and avoids deadlocks if callbacks execute synchronous operations against the controller.
  * **Read-Loop Concurrency Gate & Memory Consistency**: The blocking ReadMessage() loop acts as a natural control-flow gate. While active, it physically prevents the worker thread from processing subsequent queue submissions, enforcing single-active-connection exclusivity while guaranteeing sequential memory safety across pointer setup, deadline assignment, and teardown reset.

### 3. Queue Compaction & Flapping Prevention (GetLatestSubmissionAndRejectPreceding_SubmissionQueue)
* **Connection Flapping Prevention**: Rapid client reconnects or browser refresh bursts can enqueue multiple HTTP submissions in milliseconds. Processing them sequentially causes rapid, chaotic connection swapping.
* **Invariant: Atomic Queue Draining**: GetLatestSubmissionAndRejectPreceding_SubmissionQueue non-blockingly drains the submission queue in a single pass. It immediately rejects all preceding stale requests with a superseded error and retains only the newest client intent.

### 4. Handoff Distinction & Status Preservation (UpdateWebsocketConnection)
* **Granular Handoff Telemetry**: Provides downstream controllers with granular lifecycle hooks to distinguish cold connections from live session takeovers.
* **Invariant: Preserved State Intent Across Upgrades**: By explicitly tracking TAKEOVER_CONNECTING__WebsocketConnectionStatus state across the upgrade window, the worker deterministically fires takeover-specific callbacks (OnTakeoverConnected / OnTakeoverDisconnected) versus standard callbacks (OnConnected / OnDisconnected), and transitions to TAKEOVER_UPGRADE_FAILED__WebsocketConnectionStatus versus UPGRADE_FAILED__WebsocketConnectionStatus upon error. This guarantees that failure telemetry accurately reflects whether an established session was evicted during a failed takeover attempt.

### 5. Single-Source Teardown & Callback Ordering (HandleConnectionTeardown)
* **Teardown Race Elimination**: Teardowns originate from administrative eviction, client closure, or network drops. Executing teardown logic across multiple threads could trigger double-close panics or corrupted callback ordering.
* **Invariant: Single-Path Teardown Execution**: All socket terminations funnel strictly through HandleConnectionTeardown inside the worker loop.

### 6. Ingress Callback Non-Blocking Invariant (OnBinaryMessage)
* **Single-Worker Execution Boundary**: Ingress binary frames are dispatched synchronously to OnBinaryMessage on the single worker loop thread (RunLifecycleLoop).
* **Invariant: Zero-Latency Callback Handshake**: OnBinaryMessage MUST remain strictly non-blocking and lightweight. Blocking calls, heavy CPU work, or long OS system calls inside OnBinaryMessage freeze RunLifecycleLoop, halting ReadMessage(), disabling Gorilla Ping/Pong control frame processing, and stalling incoming HTTP takeover submissions. Any operation involving latency (PTY process spawning, ioctl window resizing, PTY stdin writing) MUST be offloaded upstream to decoupled background goroutines or worker queues.

---

## Inherent System Realities & Architectural Trade-Offs

### 1. Ingress Byte Discarding on Administrative Socket Closure
* **Mechanism**: When an administrative teardown or session takeover occurs, HandleGetPtyRequest flags IsTakeoverPending = true and invokes CloseWithCode (or Close).
* **Inherent System Reality**: Any WebSocket binary message frames (e.g., client stdin keypresses or control frames) that were successfully transmitted across the TCP network and received into the OS kernel socket receive buffer (or Gorilla's internal buffer), but **not yet popped by ReadMessage()**, are discarded when Close() invalidates the underlying OS net descriptor.
* **Network Delivery vs. Application Processing**: Even though network transmission succeeded, closing the descriptor terminates kernel receive queues (recv()) before RunLifecycleLoop can invoke ReadMessage() to process the queued frames into OnBinaryMessage.
* **Library vs. Kernel Bounds**: Replacing Gorilla WebSocket with a custom WebSocket implementation would not prevent this, as closing the underlying socket connection at the OS level inherently purges unread kernel receive buffers.

### 2. Pre-Upgrade Eviction During Session Takeover
* **Mechanism**: When HandleGetPtyRequest receives a takeover connection request while an active session exists (CONNECTED__WebsocketConnectionStatus), it immediately sends a Close frame (4000, "Session Taken Over") to evict the active connection **before** the incoming HTTP request undergoes its WebSocket upgrade.
* **Inherent System Reality**: If the incoming takeover request fails its HTTP 101 WebSocket upgrade (e.g., due to an aborted HTTP handshake, invalid headers, or client network drop during upgrade), the pre-existing healthy session has already been closed.
* **Telemetry & State Outcome**: The worker transitions to TAKEOVER_UPGRADE_FAILED__WebsocketConnectionStatus, leaving the system with no active connection. The controller prioritizes immediate takeover responsiveness over optimistic pre-validation; completely eliminating this risk would require performing HTTP upgrade validation *before* signaling eviction on the active session.

### 3. 1:1 Exclusive Connection Pipe vs. Multi-Client Fan-Out
* **Mechanism**: WebsocketController strictly enforces a 1:1 single-active-session model per controller instance.
* **Inherent System Reality**: The controller does not provide pub/sub broadcast or multi-client fan-out capability. Opening a secondary browser tab or terminal client to the same endpoint forcibly evicts the active connection via takeover eviction.
* **Architectural Reality**: Multi-client session multiplexing or passive viewer broadcast is not supported or envisioned; the system is fundamentally designed exclusively for a single-user experience to prevent split-brain stdin input scrambling and complex multi-client session state management.

### 4. Egress Flushing vs. Ingress Truncation Asymmetry
* Egress messages (stdout PTY output) are serialized under EgressMutex and written to the socket via WriteMessage / WriteControl, ensuring Close control frames are transmitted cleanly to the network peer.
* Ingress frames, conversely, rely on ReadMessage() execution within the single-threaded worker loop. Once a Close frame or descriptor invalidation occurs, un-drained ingress frames remaining in the kernel buffer are discarded upon teardown.

---

## Permutations of Websocket Status Flows

1. **Cold Start Connection & Disconnection**: STANDBY__WebsocketConnectionStatus -> CONNECTING__WebsocketConnectionStatus -> CONNECTED__WebsocketConnectionStatus -> DISCONNECTED__WebsocketConnectionStatus
2. **Cold Start Handshake Failure**: STANDBY__WebsocketConnectionStatus -> CONNECTING__WebsocketConnectionStatus -> UPGRADE_FAILED__WebsocketConnectionStatus
3. **Successful Session Takeover**: CONNECTED__WebsocketConnectionStatus -> TAKEOVER_CONNECTING__WebsocketConnectionStatus -> CONNECTED__WebsocketConnectionStatus -> DISCONNECTED__WebsocketConnectionStatus
4. **Failed Session Takeover**: CONNECTED__WebsocketConnectionStatus -> TAKEOVER_CONNECTING__WebsocketConnectionStatus -> TAKEOVER_UPGRADE_FAILED__WebsocketConnectionStatus
5. **Multiple Sequential Session Takeovers**: CONNECTED__WebsocketConnectionStatus -> TAKEOVER_CONNECTING__WebsocketConnectionStatus -> CONNECTED__WebsocketConnectionStatus -> TAKEOVER_CONNECTING__WebsocketConnectionStatus -> CONNECTED__WebsocketConnectionStatus -> DISCONNECTED__WebsocketConnectionStatus
6. **Reconnection After Normal Disconnect**: DISCONNECTED__WebsocketConnectionStatus -> CONNECTING__WebsocketConnectionStatus -> CONNECTED__WebsocketConnectionStatus -> DISCONNECTED__WebsocketConnectionStatus
7. **Failed Reconnection After Disconnect**: DISCONNECTED__WebsocketConnectionStatus -> CONNECTING__WebsocketConnectionStatus -> UPGRADE_FAILED__WebsocketConnectionStatus
8. **Reconnection After Handshake Failure**: UPGRADE_FAILED__WebsocketConnectionStatus -> CONNECTING__WebsocketConnectionStatus -> CONNECTED__WebsocketConnectionStatus -> DISCONNECTED__WebsocketConnectionStatus
9. **Reconnection After Takeover Failure**: TAKEOVER_UPGRADE_FAILED__WebsocketConnectionStatus -> CONNECTING__WebsocketConnectionStatus -> CONNECTED__WebsocketConnectionStatus -> DISCONNECTED__WebsocketConnectionStatus
10. **Repeat Handshake Failure after UPGRADE_FAILED__WebsocketConnectionStatus**: UPGRADE_FAILED__WebsocketConnectionStatus -> CONNECTING__WebsocketConnectionStatus -> UPGRADE_FAILED__WebsocketConnectionStatus
11. **Failed Retry after TAKEOVER_UPGRADE_FAILED__WebsocketConnectionStatus**: TAKEOVER_UPGRADE_FAILED__WebsocketConnectionStatus -> CONNECTING__WebsocketConnectionStatus -> UPGRADE_FAILED__WebsocketConnectionStatus

---

## Method Specifications & Technical Implementation Rationale

### WriteBinaryMessage

WriteBinaryMessage safely serializes outbound binary payload frames over the active WebSocket connection with write deadline protection and single-writer mutex serialization (EgressMutex).

#### Parameters
1. **binaryMessageData**: Raw binary slice ([]byte) transmitted as a WEBSOCKET.BinaryMessage frame over the wire (e.g., stdout bytes from the pseudo-terminal process).

#### Whitelisting & Guard Invariants
* **Positive Status & Connection Whitelisting**: Performs a combined assertion under lock: ConnectionStatus == CONNECTED__WebsocketConnectionStatus && WebsocketConnection != nil. Egress is allowed if and only if the session is fully established and non-nil.
* **Error Handling**: If the socket is offline or transitioning (STANDBY__WebsocketConnectionStatus, CONNECTING__WebsocketConnectionStatus, TAKEOVER_CONNECTING__WebsocketConnectionStatus, or DISCONNECTED__WebsocketConnectionStatus), it returns error "websocket is not connected" without attempting socket egress.

#### SetWriteDeadline Error Discarding Rationale (_ = ...)
* **Immediate Fallthrough to WriteMessage**: SetWriteDeadline configures an auxiliary write deadline on the net socket immediately prior to WriteMessage. If SetWriteDeadline fails due to a closed or broken socket (net.ErrClosed), calling WriteMessage on the very next line will instantly fail with the exact same underlying socket error.
* **Redundant Error Handling**: Catching SetWriteDeadline errors would merely preempt WriteMessage by a single instruction without changing return behavior. Allowing execution to proceed directly to WriteMessage ensures that the frame write operation returns the authoritative error directly to the caller.

#### conn.WriteMessage Egress Failure Modes
* **Write Deadline Timeout (i/o timeout)**: Client network congestion or slow consumption prevents TCP socket buffers from accepting bytes before SetWriteDeadline expires (*net.OpError wrapping os.ErrDeadlineExceeded). [*Also same for conn.WriteControl*]
* **Broken TCP Socket / Peer Disconnect**: Client closes browser tab, drops network connectivity, or resets connection (syscall.EPIPE, syscall.ECONNRESET, or net.ErrClosed). [*Also same for conn.WriteControl*]
* **Closed Socket Egress**: Attempting egress after conn.Close() was executed during socket eviction or server shutdown (net.ErrClosed). [*Also same for conn.WriteControl*]
* **Post-Close Control Frame Egress (websocket: close sent)**: Attempting binary egress after CloseWithCode wrote a Close frame to the wire (websocket.ErrCloseSent). [*Also same for conn.WriteControl*]
* **TLS Layer Record Failures**: MAC checksum or record header failure when running over secure WebSockets (wss://) (tls.alertBadRecordMac). [*Also same for conn.WriteControl*]
* **OS Kernel Buffer Exhaustion**: Fails if host OS kernel runs out of network socket memory buffers (syscall.ENOBUFS). [*Also same for conn.WriteControl*]
* **Invalid Message Opcode (websocket: invalid message type)**: Attempting egress with an unsupported message opcode integer. (Prevented by hardcoding WEBSOCKET.BinaryMessage)
* **Unclosed Previous Frame Writer (websocket: write buffer full)**: Calling WriteMessage while a raw NextWriter stream is open or frame buffer limits are exceeded. (Prevented by using atomic WriteMessage)
* **Per-Message Deflate Compression Failure**: Payload compression failure when WebSocket deflate extension is active. (N/A: compression is disabled in suprasole-server)
* **Concurrent Writer Corruptions**: Concurrent un-synchronized writes corrupt frame headers or trigger Go data races. (Prevented by acquiring EgressMutex) [*Also same for conn.WriteControl*]

---

### CloseWithCode

#### Parameters
1. **websocketCloseCode**: WebSocket Close status codes are 2-byte unsigned integers (RFC 6455 Section 7.4 & IANA Registry) categorized into protocol ranges:
   * **0–999 (Unused Range)**: Reserved and forbidden for use in WebSocket Close frames.
   * **1000 (Normal Closure)**: Purpose of connection was fulfilled; clean shutdown.
   * **1001 (Going Away)**: Endpoint is going away (e.g., server shutdown or browser navigation).
   * **1002 (Protocol Error)**: Endpoint terminated connection due to a protocol error.
   * **1003 (Unsupported Data)**: Endpoint received unsupported data type (e.g., text frame when binary expected).
   * **1004 (Reserved)**: Reserved by RFC 6455 for future definition.
   * **1005 (No Status Received)**: (Local Only — MUST NOT be sent on wire) Pseudo-code for expecting a status code when none was provided.
   * **1006 (Abnormal Closure)**: (Local Only — MUST NOT be sent on wire) Pseudo-code for TCP connection drops without a Close frame.
   * **1007 (Invalid Frame Payload Data)**: Received data inconsistent with message type (e.g., non-UTF-8 in text frame).
   * **1008 (Policy Violation)**: Endpoint terminated connection due to policy violation.
   * **1009 (Message Too Big)**: Received message too large to process.
   * **1010 (Mandatory Extension)**: Client expected server to negotiate one or more extensions.
   * **1011 (Internal Server Error)**: Server encountered an unexpected condition preventing request fulfillment.
   * **1012 (Service Restart)**: Server restarting; client may reconnect.
   * **1013 (Try Again Later)**: Server overloaded; client should reconnect later.
   * **1014 (Bad Gateway)**: Server acting as gateway received invalid response from upstream.
   * **1015 (TLS Handshake Failure)**: (Local Only — MUST NOT be sent on wire) Pseudo-code for TLS handshake failure.
   * **1016–2999 (Reserved IETF Range)**: Reserved for definition by future IETF specifications, RFCs, and WebSocket standard extensions.
   * **3000–3999 (Registered Framework Range)**: Reserved for definition by libraries, frameworks, and specifications (registered with IANA).
   * **4000–4999 (Private Application Range)**: Reserved for custom private application status codes.
     * **4000 (Session Takeover)**: Custom application code used in WebsocketController when a new client session evicts an existing session.

   * **Current Application Close Codes**:
     * **1001 (Going Away)**: Sent during graceful server termination.
     * **4000 (Session Takeover)**: Custom application code sent during session takeover eviction in HandleGetPtyRequest when a newer client connects.

2. **websocketCloseReason**:
   * **Payload Size Limit**: WebSocket Close control frames are subject to RFC 6455 Section 5.5.1, capping total control frame payloads at 125 bytes. Subtracting the 2-byte websocketCloseCode leaves a strict maximum of **123 bytes** for the UTF-8 encoded websocketCloseReason string.
   * **Encoding Requirements**: Must consist of valid UTF-8 text data.
   * **Current Application Close Reasons**:
     * **"Server Shutting Down"**: Sent during graceful server termination (paired with status code 1001).
     * **"Session Taken Over"**: Sent during session takeover eviction in HandleGetPtyRequest when a newer client connects (paired with status code 4000).

#### conn.WriteControl Egress Failure Modes
* **Write Deadline Timeout (i/o timeout)**: Client network congestion or slow consumption prevents TCP socket buffers from accepting bytes before deadline expires (*net.OpError wrapping os.ErrDeadlineExceeded). [*Also same for conn.WriteMessage*]
* **Broken TCP Socket / Peer Disconnect**: Client closes browser tab, drops network connectivity, or resets connection (syscall.EPIPE, syscall.ECONNRESET, or net.ErrClosed). [*Also same for conn.WriteMessage*]
* **Closed Socket Egress**: Attempting WriteControl after conn.Close() was executed (net.ErrClosed). [*Also same for conn.WriteMessage*]
* **Post-Close Control Frame Egress (websocket: close sent)**: Attempting WriteControl after a Close control frame was already written to the wire (websocket.ErrCloseSent). [*Also same for conn.WriteMessage*]
* **TLS Layer Record Failures**: MAC checksum or record header failure when running over secure WebSockets (wss://) (tls.alertBadRecordMac). [*Also same for conn.WriteMessage*]
* **OS Kernel Buffer Exhaustion**: Host OS kernel runs out of network socket memory buffers (syscall.ENOBUFS). [*Also same for conn.WriteMessage*]
* **Payload Limit Exceeded (websocket: invalid control frame payload length)**: Control frame payload exceeds RFC 6455 125-byte limit. (Prevented by capping websocketCloseReason at 123 bytes)
* **Invalid Control Frame Opcode**: Attempting WriteControl with a non-control opcode. (Prevented in CloseWithCode by hardcoding WEBSOCKET.CloseMessage (8))
* **Concurrent Writer Corruptions**: Concurrent un-synchronized writes corrupt frame headers or trigger Go data races. (Prevented by acquiring EgressMutex) [*Also same for conn.WriteMessage*]

#### Gorilla Egress Concurrency & Mutex Protection
* **Full-Duplex Reading & Writing**: Gorilla WebSocket permits one reader goroutine (conn.ReadMessage) and one writer goroutine to execute concurrently without a lock because TCP kernel RX and TX buffers operate independently.
* **Single-Writer Constraint**: Gorilla strictly forbids multiple goroutines from calling write methods (WriteMessage, WriteControl, NextWriter) concurrently. Simultaneous writes corrupt frame headers on the wire and trigger Go runtime data race panics.
* **Control Frame Egress Protection**: Outbound control frame calls (WriteControl in CloseWithCode) compete directly with normal binary data frame writes (WriteMessage in WriteBinaryMessage). Holding EgressMutex during the entire execution of both WriteBinaryMessage and CloseWithCode enforces the single-writer invariant, serializing all outbound socket writes.

#### Close With Code Teardown Dynamics

CloseWithCode executes a two-phase terminal teardown sequence that separates Application Layer (Layer 7) protocol notification from Transport Layer (Layer 4) socket resource reclamation.

1. **Layer 7 vs. Layer 4 Interplay & Responsibilities**:
   * **WriteControl (Layer 7 Application Layer)**: Transmits an RFC 6455 Close control frame (Opcode 0x08) containing the 2-byte status code (websocketCloseCode, e.g., 1001 or 4000) and the UTF-8 websocketCloseReason string over the wire. Assembles the 8-byte control frame payload and enqueues it into the host OS kernel TCP TX send buffer. It serves as a best-effort graceful notification allowing the client browser engine to fire its onclose handler with exact status metadata.
   * **conn.Close() (Layer 4 Transport Layer)**: Executes the OS kernel close(fd) system call to destroy the underlying socket file descriptor and reclaim TCP memory buffers. Operates purely at the transport layer with zero awareness of WebSocket frames, opcodes, or close codes. It guarantees host OS kernel resource reclamation regardless of whether WriteControl succeeded or failed.

2. **Sequential Outbound (TX Egress) Guarantees**:
   * **Strict Egress Ordering**: Holding EgressMutex throughout CloseWithCode guarantees that WriteControl completes its TX buffer enqueue before conn.Close() is called, while preventing concurrent WriteMessage calls from intervening.
   * **Kernel Flush & TCP FIN**: Once conn.Close() is called, the OS network stack flushes remaining queued outbound bytes (including the Close frame) and transmits a **TCP FIN segment** to initiate TCP teardown.

3. **Inbound (RX Ingress) Interruption & Unread Byte Discarding**:
   * **Reader Goroutine Unblocking**: Calling conn.Close() immediately interrupts the background reader goroutine (conn.ReadMessage() in RunLifecycleLoop), forcing it to return net.ErrClosed ("use of closed network connection").
   * **Purging Unread RX Queues**: If the client sent binary frames that arrived at the server's network interface and are sitting in the OS kernel TCP RX buffer, executing conn.Close() destroys the socket context. Any unread inbound bytes are discarded by the OS kernel and never processed by the application.

4. **Kernel TCP Reset (RST) vs. Graceful Teardown (FIN)**:
   * **TCP RST Generation**: Under Linux socket semantics, if close(fd) is invoked while unread bytes remain in the kernel TCP RX buffer, the OS kernel sends a **TCP RST (Reset)** segment to the client instead of a clean FIN.
   * **Client Signal**: Receiving a TCP RST causes the client network stack to fail ongoing reads/writes with ECONNRESET ("Connection reset by peer") and triggers WebSocket close code 1006 (Abnormal Closure). This explicitly notifies the client that the connection was aborted and unacknowledged in-flight bytes were dropped at the socket boundary.

5. **Error Discarding Rationale (_ = ...)**:
   Both WriteControl and Close discard return errors (_ = ...) because CloseWithCode is a non-failing, idempotent terminal teardown function:
   * **Why WriteControl Error is Discarded**: If WriteControl fails due to network partition, client abrupt disconnect, or write timeout, execution must still proceed immediately to conn.Close() to ensure OS kernel file descriptor leaks are prevented.
   * **Why Close() Error is Discarded**: conn.Close() returns an error primarily when the underlying network connection is already closed (net.ErrClosed / "use of closed network connection" or syscall.EBADF). Since CloseWithCode is terminal and idempotent, a failure during Close() means the socket file descriptor has already been released or destroyed by a concurrent network drop. The desired end state (socket release) is achieved regardless of whether Close() returns nil or net.ErrClosed.
