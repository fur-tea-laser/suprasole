# WorkspaceController Architectural Specification & Lifecycle

## Macro Architectural Role & System Context

`WorkspaceController` is the central aggregate root and operational orchestration hub of `suprasole-server`. It coordinates the lifecycle, concurrency, and message dispatch between four primary subsystems:
 1. **The Network Transport Boundary (`WorkspaceNetwork` / `WebsocketController`)**: Manages the underlying HTTP/1.1-to-WebSocket upgrade lifecycle, TCP socket read/write loops, and generational connection identifier tracking (`id_WebsocketConnection_expected`).
 2. **The Pseudo-Terminal Instance Pool (`PtyPool` holding `_WorkspacePty_`)**: Encapsulates active and exited pseudo-terminal processes (`_PtyProxy_`), headless virtual terminal emulators (`*xterm.Terminal`), background stdout stream readers (`PtyReader`), background paced egress flushers (`PtyFlusher`), and background stdin stream writers (`PtyWriter`).
 3. **The Ingress Layout Reducer (`MessageReducer__LayoutUpdate_PtyProxy`)**: Layout reducing and coalescing engine that buffers, deduplicates, and debounces rapid browser layout updates and coordinates serialized FIFO dispatch for lifecycle events (`Spawn`, `Remove`, `Resize`, `Sync`).
 4. **The Serialized Lifecycle Coordinator (`LifecycleCoordinator_WorkspacePty`)**: Single-threaded FIFO order reconciliation pipeline that orders and compacts asynchronous connection, disconnection, sync, and process exit events.

### The Latent Pressures of Full-Duplex Web Terminal Servers
Operating a browser-based multi-terminal workspace introduces systemic friction across several OS and network boundaries:
 * **Linux OS Kernel Boundaries**: Child processes (`bash`, `zsh`, `htop`, `vim`) execute inside Linux pseudo-terminal slave devices (`/dev/pts/*`) governed by kernel TTY line disciplines, buffer quotas, and asynchronous signal delivery (`SIGWINCH`, `SIGHUP`, `SIGCHLD`).
 * **Network Transport Boundaries**: A single full-duplex TCP/WebSocket pipe multiplexes user input, terminal stdout chunks, window geometry changes, and workspace lifecycle metadata. Network backpressure or slow client consumption must never block internal kernel PTY draining.
 * **DOM & Browser Realities**: Browsers generate hundreds of window resize events during viewport dragging and support multi-tab interfaces where only a fraction of open terminals are visible at any given millisecond.
 * **Concurrency Clashes**: HTTP connection handoffs, client frame reads, child process exits, and stdin writes occur simultaneously across disparate goroutines. Without a central coordinating entity, concurrent mutations would trigger data races, split-brain session states, and circular-wait deadlocks.

`WorkspaceController` resolves these pressures by enforcing strict unidirectional thread ownership, non-blocking handoffs, and an order-reconciliation pipeline.

---

## Ingress Concurrency Decoupling & Wire Protocol Dispatch

### 1. Ingress Callback Non-Blocking Boundary (`HandlePayload_BinaryMessage__PtyWebsocket`)
All incoming client WebSocket frames arrive via `WebsocketController`'s single-threaded reader loop (`conn.ReadMessage()`) and enter `WorkspaceController` through `HandlePayload_BinaryMessage__PtyWebsocket`.

* **The Zero-Latency Invariant**: Because `HandlePayload_BinaryMessage__PtyWebsocket` executes synchronously on the network reader thread, it **MUST NEVER** execute long-running computations, blocking channel operations, filesystem I/O, or OS system calls.
* **Preserving WebSocket Transport Liveness**: If an ingress handler blocks (e.g., waiting for `fork/exec`, acquiring a contended lock, or writing to a full channel), the WebSocket reader loop halts. This freezes TCP socket draining, disables Gorilla WebSocket's internal Ping/Pong keepalive control frame processing, and stalls incoming HTTP connection takeovers.
* **Asynchronous Delegation Pattern**: Every ingress opcode handler in `WorkspaceController` delegates heavy or latency-inducing operations upstream:
  * `SPAWN_PTY`, `BATCH__RESIZE_PTY`, `BATCH__REMOVE_PTY`, and `BATCH__SYNC_PTY` perform a non-blocking enqueue into the layout reducer queue (`MessageReducer__LayoutUpdate_PtyProxy.QueueChannel`), preserving strict FIFO sequencing while coalescing continuous geometry bursts.
  * `WRITE_INPUT__PTY` performs a non-blocking `select` enqueue into the target PTY's decoupled input channel (`PtyWriter.QueueChannel_InputOrder`).
  * `TERMINATE_PTY` enqueues an order into the serialized lifecycle coordinator channel (`LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder`).

### 2. Dynamic Opcode Routing (`__decodeWebsocketPayload_binaryMessage`)
Raw binary payloads are routed via a generic dispatch function parametrized over ingress opcodes:
```go
func __decodeWebsocketPayload_binaryMessage[
    __Code__Message_Ingress__ ~uint16,
    __Message_Ingress__ interface {
        Execute(WorkspaceController_forwarded *_WorkspaceController_, id_WebsocketConnection_expected uint64)
    },
](...)
```
* **Opcode Decoding**: The leading 2 bytes are parsed as a BigEndian `uint16` opcode and indexed into `MAP__DECODE_PAYLOAD___PTY_MESSAGE__INGRESS`.
* **Sticky-Error Protocol Safety**: Decoding functions utilize `_BinaryDecoder_WebsocketMessage_` with sticky error tracking. If a payload is truncated, malformed, or exceeds field bounds, the decoder records the error on `Error_Earliest_maybe` without advancing invalid state. Execution aborts cleanly without panicking the server.
* **Polymorphic Execution**: Successfully decoded structs implement `_PtyMessage_Ingress_` and invoke their `Execute` method, passing the forwarded controller reference and target connection ID.

---

## Asynchronous Process Spawning & Wire Protocol Determinism

### 1. Linux OS Process Creation Pressures
Creating an interactive pseudo-terminal involves heavy operating system resource allocation:
 1. Allocating a UNIX 98 pseudo-terminal master/slave pair (`/dev/ptmx` and `/dev/pts/N`).
 2. Calling `fork()` / `clone()` and `execve()` to spawn the target shell binary (`_EXEC.Command`).
 3. Setting slave descriptor termios attributes (raw mode, echo control).
 4. Establishing process credentials, environment arrays, and working directories.

Under Linux, `fork/exec` is subject to kernel memory paging delays, process table locking, and virtual memory cloning overhead (even with `vfork`/`clone`). Executing `Spawn_PtyProxy` synchronously within the ingress handler would stall the entire server's network pipeline. Therefore, `_SpawnPty__PtyMessage_Ingress_.Execute` spawns a dedicated goroutine to manage process launch.

### 2. Monotonic ID Allocation Under Lock
Before launching the background spawning goroutine, `WorkspaceController` assigns a globally unique, monotonically increasing 32-bit identifier under lock:
```go
WorkspaceController_forwarded.Mutex.Lock()
id_PtyProxy_new := WorkspaceController_forwarded.Id_PtyProxy_next
WorkspaceController_forwarded.Id_PtyProxy_next++
WorkspaceController_forwarded.Mutex.Unlock()
```
Allocating `id_PtyProxy_new` synchronously before entering the background goroutine ensures that PTY IDs remain strictly sequential and deterministic, regardless of the order in which the operating system schedules subsequent `fork/exec` goroutines.

### 3. The Spawning Handshake Contract (`OnSpawned_PtyProxy__`)
A critical race condition exists between process creation, process exit, and stdout streaming:
 * If a child process is fast-failing (e.g., an invalid command, missing executable, or instant `exit 1`), the Linux kernel returns `syscall.EIO` on the master descriptor immediately.
 * If the child process produces immediate banner output (e.g., shell motd or bash initialization), stdout bytes arrive on the master descriptor within microseconds.

To establish wire protocol determinism, `Spawn_PtyProxy` enforces a synchronous handshake hook (`api.OnSpawned_PtyProxy__`, bound to `HandleSpawned_Pty`):
 1. `_PTY.Start` creates the process and binds the master file descriptor.
 2. **Synchronous Registration Hook**: `Spawn_PtyProxy` synchronously executes `HandleSpawned_Pty(ptyProxy_spawned)`:
    * Wraps `_PtyProxy_` in `_WorkspacePty_` with initial `Visibility_Client_current = VISIBLE__Visibility_Client`.
    * Inserts the wrapper into `This.PtyPool` under `This.Mutex`.
    * Transmits `STATUS__SPAWN_PTY` (0x0002) with `SUCCESS__Status_SpawnPty` over WebSocket egress.
 3. **Background Reader, Flusher & Writer Launch**: Only after `HandleSpawned_Pty` completes does `Spawn_PtyProxy` launch `go PtyReader.RunWorker()`, `go PtyFlusher.RunWorker()`, and `go PtyWriter.RunWorker()`.

**Critical Invariant**: This guarantees that `STATUS__SPAWN_PTY` (0x0002) is **always** the first frame the client receives for a newly spawned PTY. Under no circumstances can child process output (`PTY_OUTPUT` 0x0008) or process exit notices (`PTY_EXIT` 0x0005) precede the creation status frame on the wire.

### 4. Spawning Failure Telemetry (`HandleSpawnFailed_Pty`)
If `Spawn_PtyProxy` fails (e.g., binary not found `ENOENT`, execution permission denied `EACCES`, kernel process quota exhausted `EAGAIN`), the spawning goroutine catches the non-nil error and invokes `HandleSpawnFailed_Pty(id_PtyProxy_new)`. This emits `STATUS__SPAWN_PTY` with `FAILURE__Status_SpawnPty` and the reserved ID, notifying the client that session initialization failed without leaking half-initialized state into `PtyPool`.

---

## The Lifecycle Coordinator: Sequential Consistency & Order Compaction

### 1. The Hazard of Concurrent Multi-Threaded State Mutations
In a live workspace, lifecycle events originate from four asynchronous sources:
 1. **HTTP Ingress**: Client connections and session takeovers arriving from `net/http` worker goroutines (`HandleConnected_PtyWebsocket`, `HandleConnected_Takeover__PtyWebsocket`).
 2. **Network Drops**: Sudden TCP resets or WebSocket close frames caught on network reader threads (`HandleDisconnected_PtyWebsocket`).
 3. **Client Session Syncs**: Multi-tab visibility updates and geometry orders arriving over the wire (`HandleSync_PtyPool__Coordinator`).
 4. **Process Teardown**: Child processes dying asynchronously on background `PtyFlusher` exit dispatchers (`HandleExited_*`).

If these events mutated `PtyPool` and proxy operational modes directly across their calling threads, race conditions would corrupt session state (e.g., a disconnect handler attempting to suppress a proxy while a concurrent sync handler transitions it to live, or an exit handler racing a manifest generation).

### 2. Single-Worker Lifecycle Pipeline (`_LifecycleCoordinator_WorkspacePty_`)
`LifecycleCoordinator_WorkspacePty` resolves this by funnelling all lifecycle orders into a single background worker goroutine via `QueueChannel_WorkspaceOrder` (capacity 16).

```
                      [ Client Connect ] ---------\
                      [ Client Disconnect ] -----> \
                      [ Client Sync (0x000a) ] ---> +--> QueueChannel_WorkspaceOrder (Cap 16)
                      [ PTY Exit (EIO/Exit) ] ---> /
                                                  /
                                                 v
                                   +---------------------------+
                                   | Drain Queue Channel       |
                                   +---------------------------+
                                                 |
                                                 v
                                   +---------------------------+
                                   | Reconcile & Compact Batch |
                                   +---------------------------+
                                                 |
                                                 v
                                   +---------------------------+
                                   | Sequential Order Execute  |
                                   +---------------------------+
```

### 3. Atomic Micro-Batch Draining (`Drain__QueueChannel_WorkspaceOrder`)
When the coordinator's select loop wakes on `order_leading := <-this.QueueChannel_WorkspaceOrder`, it immediately invokes `Drain__QueueChannel_WorkspaceOrder`. This non-blockingly drains all subsequent pending orders currently waiting in the channel buffer into a contiguous slice (`orderBatch_pending`). Draining converts isolated event-driven reactions into an atomic micro-batch.

### 4. Reconciled Order Compaction Algorithm (`Reconcile__orderBatch_pending`)
The pending batch is processed through a specialized compaction algorithm that enforces two critical ordering invariants:

```go
func (this *_LifecycleCoordinator_WorkspacePty_) Reconcile__orderBatch_pending(
    orderBatch_pending []_WorkspaceOrder_LifecycleCoordinator_,
) []_WorkspaceOrder_LifecycleCoordinator_
```

#### Invariant A: Exit Order Segregation & Head Elevation
Process exits (`_ExitPty__WorkspaceOrder_LifecycleCoordinator_`) are segregated into `orderBatch_ExitPty_result` and placed at the **front** of the execution list.
* **Rationale**: If a child process died while the client was reconnecting or submitting a sync batch, the system must acknowledge and register the process death before evaluating session visibility or serializing snapshots. Elevating exit orders guarantees that subsequent manifest and sync calculations observe the terminal `ExitOutcome_PtyProxy_maybe` state.

#### Invariant B: Disconnect Preemption & Tail Order Compaction
Session-related orders (`Connect`, `Disconnect`, `Sync`) are reconciled in `orderBatch_Session_result`:
* **Disconnect Preemption**: If a `_Disconnect_` order appears, it completely clobbers all preceding session orders in the batch (`orderBatch_Session_result = []{order}`). If a disconnect occurred, preceding in-flight syncs or connects for that connection are obsolete and discarded.
* **Tail Order Compaction (`upsertTail_Order__orderBatch_Session`)**: Consecutive duplicate orders of the same type (`Connect` followed by `Connect`, or `Sync` followed by `Sync`) replace the tail element. If rapid client reconnects or sync storms enqueue multiple identical orders in the channel buffer, only the newest client intent executes.

This guarantees that the coordinator never executes redundant, flapping, or conflicting state transitions.

---

## Multi-Tab Visibility & Terminal Resynchronization

### 1. The Multi-Tab Economics of Bandwidth and CPU
In modern web development environments, a user may have 10 to 50 terminal tabs open simultaneously. However, the human eye only observes the active tab (or a small set of split panes).
* **Naive Architecture**: Streaming stdout frames across WebSocket for all 50 tabs would overwhelm client DOM rendering engines (`xterm.js`), saturate network bandwidth, and waste CPU serialization time.
* **Suprasole Architecture**: `WorkspaceController` enforces an active/quiescent visibility model:
  * **Focused Tabs (`VISIBLE__Visibility_Client`)**: Mode is `LIVE_RUNNING__Mode_PtyProxy`. Stdout bytes are cloned and emitted live across WebSocket (`PTY_OUTPUT` 0x0008).
  * **Background Tabs (`NOT_VISIBLE__Visibility_Client`)**: Mode is `PRE_SNAPSHOT__RUNNING___Mode_PtyProxy`. Background stdout bytes are consumed exclusively by the headless `*xterm.Terminal` emulator under lock. Live egress is suppressed. **Zero bytes are transmitted over the network.**

### 2. The 5-Way State Transition Matrix (`HandleSync_PtyPool__Coordinator`)
When a client switches tabs, focuses windows, or reconnects, it transmits `BATCH__SYNC_PTY` (0x000a) containing a map of active PTY IDs and their viewport geometries (`ColumnCount`, `RowCount`).

`HandleSync_PtyPool__Coordinator` clones `PtyPool` under lock and evaluates each session against a 5-way state transition matrix:

| Current Visibility | Inbound Sync Order | Process State | State Transition & Action Pipeline |
| :--- | :--- | :--- | :--- |
| **Visible** | Present (`order != nil`) | Running / Exited | **Visible -> Visible (Geometry Update)**<br>Executes `PtyProxy.Resize()`. Proxy remains in `LIVE_RUNNING` or `EXITED`. |
| **Not Visible** | Present (`order != nil`) | Exited (`ExitOutcome != nil`) | **Not Visible -> Visible (Dead Session Snapshot)**<br>1. Resizes VTE grid via `PtyProxy.Resize()`.<br>2. Emits snapshot via `__emitSnapshot_SyncPty(EmitSnapshot_Exited)` inside task brackets.<br>3. Remains in `EXITED` mode (no live streaming). |
| **Not Visible** | Present (`order != nil`) | Running (`ExitOutcome == nil`) | **Not Visible -> Visible (Live Session Snapshot & Hydration)**<br>1. Resizes VTE grid via `PtyProxy.Resize()`.<br>2. Emits snapshot via `__emitSnapshot_SyncPty(TransitionMode_PreToPostSnapshot)` inside task brackets.<br>3. Flushes staging buffer and transitions to live via `TransitionMode_PostSnapshotToLive()`. |
| **Visible** | Absent (`order == nil`) | Running | **Visible -> Not Visible (Egress Suppression)**<br>Executes `PtyProxy.TransitionMode_LiveToPreSnapshot()`. Live stdout egress is suppressed. |
| **Not Visible** | Absent (`order == nil`) | Running / Exited | **Not Visible -> Not Visible (Quiescent No-Op)**<br>No mode change. Session remains suppressed or exited. |

### 3. Task-Bracketing Protocol (`TASK_START__SYNC_PTY` & `TASK_COMPLETE__SYNC_PTY`)
During a `Not Visible -> Visible` transition, delivering an ANSI snapshot is an atomic task:
```go
func __emitSnapshot_SyncPty__WebsocketController_Pty(...) {
    Emit(WebsocketController_Pty, id, _TaskStart_SyncPty_{Id_PtyProxy: id_PtyProxy_target})
    onEmitSnapshot__()
    Emit(WebsocketController_Pty, id, _TaskComplete_SyncPty_{Id_PtyProxy: id_PtyProxy_target})
}
```
* **Client Synchronization Barrier**: Emitting `TASK_START__SYNC_PTY` (0x000b) instructs the client UI to clear its DOM terminal viewport and pause user input.
* **Snapshot Delivery**: `onEmitSnapshot__` serializes the headless VTE grid (`serializeAddon.Serialize`) and transmits `PTY_OUTPUT` (0x0008).
* **Task Completion Barrier**: Emitting `TASK_COMPLETE__SYNC_PTY` (0x000c) notifies the client that baseline hydration is complete. The client unpauses the terminal and resumes live rendering.

---

## High-Frequency DOM Window Resize Debouncing

### 1. Browser UI Dragging vs. Kernel `SIGWINCH` Avalanches
When a user resizes a browser window or drags an interactive split-pane divider:
 * The browser's `ResizeObserver` generates resize events at 60Hz to 120Hz.
 * If transmitted directly to the backend, each resize would invoke `pty.Setsize` on the OS master descriptor (`ioctl(TIOCSWINSZ)`).
 * The Linux kernel responds to `TIOCSWINSZ` by sending `SIGWINCH` (Window Changed) to the child process foreground group.
 * CLI applications (`vim`, `tmux`, `htop`, `zsh`) handle `SIGWINCH` by querying window dimensions and re-rendering their entire screen matrix over stdout.

Synchronously executing 60 full-screen ANSI redraws per second saturates the kernel TTY buffer, spikes server CPU, and floods WebSocket egress with megabytes of redundant intermediate redraws.

### 2. The Ingress Layout Reducer Engine (`MessageReducer__LayoutUpdate_PtyProxy`)
`WorkspaceController` interposes `MessageReducer__LayoutUpdate_PtyProxy` to decouple DOM layout burst frequency while strictly preserving FIFO ordering across all layout-altering commands (`Spawn`, `Remove`, `Resize`, `Sync`):
 1. **Bounded Non-Blocking Queue**: Ingress layout orders are enqueued into `QueueChannel` (capacity 512).
 2. **Sequential FIFO Processing & Geometry Folding**:
    The worker reads orders sequentially from `QueueChannel`.
    * Shared geometry delta reduction (`Coalesce__OrderBatch_LayoutUpdate`) standardizes layout updates across `Resize`, `Spawn`, and `Remove`: updating existing tracked keys or inserting new entries into `batch__LayoutUpdate_PtyProxy__tentative_state`.
    * Intermediate width/height coordinates are overwritten; only the latest coordinates per terminal are retained.
 3. **Latency-Optimized Debounced Execution**:
    * **Pure Resizes**: High-frequency mouse drag window resize bursts reset and wait on a 50ms trailing-edge debounce timer (`_TIME.NewTimer(50 * _TIME.Millisecond)`), coalescing continuous layout adjustments before sending to the PTY.
    * **Discrete Lifecycle Events**: Interactive operations (`Spawn`, `Remove`, `Sync`) bypass the 50ms delay. The worker immediately halts any active debounce timer, folds sibling geometry deltas into `batch__LayoutUpdate_PtyProxy__tentative_state` (or authoritatively resets it on `Sync`), and flushes immediately (0ms delay) directly via dedicated callbacks passing typecasted orders.
 4. **Direct Callback Flush Dispatch (`HandleFlush__<Action>__Reducer`)**:
    Upon flush, `MessageReducer__LayoutUpdate_PtyProxy` invokes direct callback functions wired to `*WorkspaceController_` methods:
    * `HandleFlush__ResizeOnly__Reducer(batch__LayoutUpdate_PtyProxy)`: Invokes `SubmitBatch_Visible__Order_PtyResizer` to resize visible PTYs.
    * `HandleFlush__SpawnPty__Reducer(order_SpawnPty, batch__LayoutUpdate_PtyProxy)`: Dispatches `_SpawnPty__WorkspaceOrder_LifecycleCoordinator_` to `LifecycleCoordinator_WorkspacePty`, then resizes visible sibling PTYs via `SubmitBatch_Visible__Order_PtyResizer`.
    * `HandleFlush__Batch_RemovePty__Reducer(order_Batch_RemovePty, batch__LayoutUpdate_PtyProxy)`: Dispatches `_RemovePty__WorkspaceOrder_LifecycleCoordinator_` to `LifecycleCoordinator_WorkspacePty`, then resizes remaining visible sibling PTYs via `SubmitBatch_Visible__Order_PtyResizer`.
    * `HandleFlush__Batch_SyncPty__Reducer(order_Batch_SyncPty)`: Captures target PTYs and pre-sync visibility under mutex, dispatches `_SyncVisibility__WorkspaceOrder_LifecycleCoordinator_` to `LifecycleCoordinator_WorkspacePty`, then resizes visible PTYs and triggers snapshot emission for newly visible PTYs.

### 3. Lock Scoping Invariant & PtyResizer Submission in Layout Reducer Flushing
When executing `HandleFlush__Batch_SyncPty__Reducer`:
```go
func (This *_WorkspaceController_) HandleFlush__Batch_SyncPty__Reducer(
	order_Batch_SyncPty _Batch_SyncPty___MessageOrder__LayoutUpdate_PtyProxy_,
) {
	batch__LayoutUpdate_PtyProxy__SyncPty__state := make(map[uint32]*_LayoutUpdate_PtyProxy_, len(order_Batch_SyncPty.Message__Batch_SyncPty.OrderBatch_LayoutUpdate))
	batch__Submission_PtyResizer__state := make(map[*_PtyResizer_]_Order_PtyResizer_, len(order_Batch_SyncPty.Message__Batch_SyncPty.OrderBatch_LayoutUpdate))
	This.Mutex.Lock()
	for id_WorkspacePty__target_some, order_LayoutUpdate_some := range order_Batch_SyncPty.Message__Batch_SyncPty.OrderBatch_LayoutUpdate {
		batch__LayoutUpdate_PtyProxy__SyncPty__state[id_WorkspacePty__target_some] = &_LayoutUpdate_PtyProxy_{
			ColumnCount_PtyTerminal: order_LayoutUpdate_some.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:    order_LayoutUpdate_some.RowCount_PtyTerminal,
		}
		WorkspacePty_target := This.PtyPool[id_WorkspacePty__target_some]
		if WorkspacePty_target != nil &&
			VISIBLE___Visibility__Client_connected == WorkspacePty_target.Visibility__Client_connected__state {
			batch__Submission_PtyResizer__state[WorkspacePty_target.PtyResizer] = &_ResizePty_Just__Order_PtyResizer_{
				ColumnCount_PtyTerminal: order_LayoutUpdate_some.ColumnCount_PtyTerminal,
				RowCount_PtyTerminal:    order_LayoutUpdate_some.RowCount_PtyTerminal,
			}
		} else if WorkspacePty_target != nil &&
			(NOT_VISIBLE___Visibility__Client_connected == WorkspacePty_target.Visibility__Client_connected__state ||
				DISCONNECTED_UNKNOWN___Visibility__Client_connected == WorkspacePty_target.Visibility__Client_connected__state) {
			batch__Submission_PtyResizer__state[WorkspacePty_target.PtyResizer] = &_ResizePty_EmitSnapshot__Order_PtyResizer_{
				ColumnCount_PtyTerminal:         order_LayoutUpdate_some.ColumnCount_PtyTerminal,
				RowCount_PtyTerminal:            order_LayoutUpdate_some.RowCount_PtyTerminal,
				Id_WorkspacePty:                 id_WorkspacePty__target_some,
				Id_WebsocketConnection_expected: order_Batch_SyncPty.Id_WebsocketConnection_expected,
				QueueChannel_WorkspaceOrder:     This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder,
			}
		} else if nil == WorkspacePty_target {
		} else {
			panic("invalid path: HandleFlush__Batch_SyncPty__Reducer")
		}
	}
	This.Mutex.Unlock()
	select {
	case <-This.MessageReducer__LayoutUpdate_PtyProxy.WorkerContext.Done():
		return
	case This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _SyncVisibility__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: order_Batch_SyncPty.Id_WebsocketConnection_expected,
		Batch__LayoutUpdate_PtyProxy:    batch__LayoutUpdate_PtyProxy__SyncPty__state,
	}:
	}
	for PtyResizer__target_some, order_PtyResizer_some := range batch__Submission_PtyResizer__state {
		PtyResizer__target_some.Submit__Order_PtyResizer(order_PtyResizer_some)
	}
}
```
* `order_Batch_SyncPty.Message__Batch_SyncPty.OrderBatch_LayoutUpdate` represents the authoritative whitelist of visible terminals declared by the client in `BATCH__SYNC_PTY`. Ranging directly over this map guarantees that non-visible terminals are omitted without requiring secondary checks or nested branches.
* `This.Mutex` is acquired **once** upfront to capture target pointers and pre-sync visibility states from `PtyPool` before enqueuing `_SyncVisibility` to `LifecycleCoordinator`. This eliminates race conditions where the coordinator could mutate visibility state to `VISIBLE` before the reducer determines whether a snapshot is required.
* `This.Mutex` is acquired **only** to fetch the target pointer and visibility state from `PtyPool`, and released immediately.
* Layout resize orders are submitted to the decoupled `PtyResizer` worker queue using non-blocking submission (`Submit__Order_PtyResizer`), preventing any stall on the reducer thread.
* Visible PTYs receive `_ResizePty_Just__Order_PtyResizer_`. Hidden/unknown PTYs transitioning to visible via sync receive `_ResizePty_EmitSnapshot__Order_PtyResizer_` to resize and enqueue an authoritative snapshot order behind `_SyncVisibility_` in the coordinator's FIFO queue.
* This ensures zero lock contention between layout updates and concurrent lifecycle or egress operations.

---

## Decoupled Asynchronous Stdin Writing (`PtyWriter`)

### 1. Linux Kernel PTY Pipe Backpressure Hazards
User keypresses sent from the browser client arrive via `WRITE_INPUT__PTY` (0x0007).
* The Linux kernel pseudo-terminal driver allocates a finite in-memory input ring buffer (typically 64 KB).
* If the child process is paused (e.g., job control `Ctrl-Z`), blocked on disk I/O, or unresponsive, the kernel TTY input buffer saturates.
* As detailed in `PtyProxy.md` ("Analysis of Master PTY Write Blocking Scenarios"), once free space drops below the high watermark (~4 KB), any subsequent `write()` system call on `MasterFileDescriptor_PtyDevice` **blocks the calling thread**.

### 2. Double Non-Blocking Ingress Handoff
If `WorkspaceController` executed direct kernel writes on the WebSocket ingress thread, a stuck child process would freeze the entire WebSocket server connection.

To prevent this, `_WriteInput_Pty__PtyMessage_Ingress_.Execute` offloads writes to `PtyWriter` using a double non-blocking `select`:
```go
WorkspaceController_forwarded.Mutex.Lock()
WorkspacePty_target := WorkspaceController_forwarded.PtyPool[this.Id_PtyProxy]
WorkspaceController_forwarded.Mutex.Unlock()

if WorkspacePty_target != nil {
    select {
    case <-WorkspacePty_target.PtyProxy.PtyWriter.WorkerContext.Done():
    default:
        select {
        case WorkspacePty_target.PtyProxy.PtyWriter.QueueChannel_InputOrder <- this.InputOrder_PtyWriter:
        default:
            // Channel saturated: input dropped to prevent server freeze
        }
    }
}
```
 1. **Worker Liveness Guard**: The first `select` verifies that `PtyWriter.WorkerContext` is not closed.
 2. **Non-Blocking Channel Enqueue**: The inner `select` attempts to push `InputOrder_PtyWriter` into `QueueChannel_InputOrder` (capacity 1024).
 3. **Drop-on-Saturation Fallback**: If the queue fills (because the child process stopped consuming stdin and `PtyWriter` blocked in `write()`), the order is immediately dropped via `default:`.

**Architectural Trade-Off**: Dropping stdin bytes during severe child process lockups is an intentional defense mechanism. Preserving server availability, WebSocket keepalives, and UI control for other terminal sessions is prioritized over guaranteeing delivery to an unresponsive process.

---

## Process Termination, Signal Propagation & Administrative Retention

### 1. Process Group Termination (`Terminate_PtyProcess`)
When a client closes a terminal tab or issues an administrative kill command, the browser sends `TERMINATE_PTY` (0x0004) carrying a POSIX signal integer (e.g., `SIGTERM`, `SIGKILL`, `SIGHUP`).

`WorkspaceController` delegates this to `PtyProxy.Terminate_PtyProcess`:
```go
syscallSignal_PtyProcess := _SYSCALL.Signal(terminalSignal_PtyProcess)
error_kill__PtyProcess__maybe := _SYSCALL.Kill(
    -This.PtyCommand.Process.Pid,
    syscallSignal_PtyProcess,
)
if error_kill__PtyProcess__maybe != nil {
    _ = This.PtyCommand.Process.Signal(syscallSignal_PtyProcess)
}
```
* **Process Group Broadcast (`-Pid`)**: In POSIX systems, passing a negative PID to `kill()` broadcasts the signal to every process in the target process group (`pgrp`).
* **Preventing Orphaned Leaks**: When an interactive shell spawns child commands (`python`, `npm`, `make`), those children share the shell's process group. Signaling only the parent shell PID would terminate the shell while leaving orphaned child processes alive, holding open the slave PTY device and leaking OS resources. Signaling `-Pid` cleanses the entire process tree.
* **Direct Signal Fallback**: If process group signaling fails (e.g., if the process group was disestablished), the system falls back to signaling the primary process directly.

### 2. Process Exit Detection & Exit Outcome Propagation
When a child process terminates:
 1. The Linux kernel breaks the slave end of the PTY, causing subsequent reads on the master descriptor in `PtyReader` to return `syscall.EIO` or `os.ErrClosed`.
 2. `PtyProxy` executes `__executeExitedTeardown`: closes the master descriptor, waits on `PtyCommand.Wait()` to reap the kernel zombie, inspects `syscall.WaitStatus`, and dispatches the corresponding leaf callback.
 3. `WorkspaceController` catches the callback and maps it to a strongly typed `_ExitOutcome_PtyProxy_`:
    * `HandleExited_Eio_Success__Pty`: Exit status 0 -> `_Success__ExitOutcome_PtyProxy_{}`
    * `HandleExited_Eio_Failure__Pty`: Non-zero status -> `_Failure__ExitOutcome_PtyProxy_{ExitCode}`
    * `HandleExited_Eio_Killed__Pty`: Terminated by signal -> `_Killed__ExitOutcome_PtyProxy_{ExitSignal}`
    * `HandleExited_Closed__Pty`: Master descriptor closed -> `_Closed__ExitOutcome_PtyProxy_{}`
    * `HandleExited_SystemError__Pty`: OS read error -> `_SystemError__ExitOutcome_PtyProxy_{SystemError}`
 4. `WorkspaceController` pushes `_ExitPty__WorkspaceOrder_LifecycleCoordinator_` into the lifecycle coordinator.
 5. Inside `HandleExit_PtyProxy__Coordinator`:
    * Assigns `ExitOutcome_PtyProxy_maybe` on `_WorkspacePty_` under lock.
    * Invokes `PtyProxy.TransitionMode_ToExited()`.
    * Cancels the stdin writer worker (`PtyWriter.WorkerCancel()`).
    * Transmits `PTY_EXIT` (0x0005) to the client, providing authoritative exit metadata.

### 3. Administrative Retention Invariant (`BATCH__REMOVE_PTY`)
**Invariant: Dead sessions are NEVER automatically deleted from `PtyPool`.**
* **User Experience & Telemetry Requirement**: When a command finishes or crashes, the user still needs to read the terminal output, review error traces, and see the exit code. If the controller immediately removed the session upon process exit, the client tab would vanish or throw errors.
* **Reconnect Continuity**: If a network disconnect occurs while a long compile job finishes, a reconnecting client must still be informed that the job completed and be able to inspect its final scrollback snapshot.
* **Client-Driven Deletion Boundary (`BATCH__REMOVE_PTY` 0x0006)**:
  Sessions are removed from memory if and only if the client explicitly requests removal AND the process has already transitioned to `_State_Exited__WorkspacePty_`. This deletion is strictly executed on `LifecycleCoordinator_WorkspacePty` via `HandleRemovePty__Coordinator`:
  ```go
  This.Mutex.Lock()
  for _, order_RemovePty__target_current := range orderBatch_RemovePty {
      if WorkspacePty_target := This.PtyPool[order_RemovePty__target_current.Id_WorkspacePty]; WorkspacePty_target != nil {
          State__WorkspacePty__target_exited_maybe, _ := WorkspacePty_target.State_state.(*_State_Exited__WorkspacePty_)
          if State__WorkspacePty__target_exited_maybe != nil {
              WorkspacePty_target.PtyResizer.WorkerCancel()
              delete(This.PtyPool, order_RemovePty__target_current.Id_WorkspacePty)
          }
      }
  }
  This.Mutex.Unlock()
  ```
  Attempting to remove an active, non-exited session is rejected. A session must be terminated and transitioned to exited state before it can be purged.

---

## Disconnect, Reconnect & Generational Egress Alignment

### 1. Generational Connection Protection (`id_WebsocketConnection_expected`)
Every outbound message emitted by `WorkspaceController` routes through:
```go
func Emit__PtyMessage_Egress__WebsocketController_Pty(
    WebsocketController_Pty *_WebsocketController_,
    id_WebsocketConnection_expected uint64,
    egressMessage _PtyMessage_Egress_,
)
```
* As established in `WebsocketController.md`, each newly accepted WebSocket connection increments a monotonic uint64 generation ID (`Id_WebsocketConnection_current`).
* Passing `id_WebsocketConnection_expected` guarantees that if a connection was terminated or superseded by a takeover while an egress message was in flight, `WebsocketController` drops the write.
* This eliminates cross-session data leakage and prevents sending stale frames to new sessions.

### 2. Disconnection Lifecycle (`HandleDisconnect_PtyWebsocket__Coordinator`)
When a client disconnects:
 1. The coordinator invokes `HandleDisconnect_PtyWebsocket__Coordinator`.
 2. `PtyPool` is cloned under lock (`_MAPS.Clone(This.PtyPool)`), releasing `This.Mutex` immediately.
 3. For every active session, if its mode is `LIVE_RUNNING` or `POST_SNAPSHOT_RUNNING`, the controller calls `TransitionMode_LiveToPreSnapshot()` or `TransitionMode_PostSnapshotToPreSnapshot()`.
  4. **Operational Result**: All live stdout streaming across WebSocket is silenced. Child processes continue running uninterrupted in the background. `PtyFlusher` continues updating `*xterm.Terminal` emulator grids in memory via `PtyProxy.HandleBlockingFlush`. The workspace is safely suspended in a headless, zero-egress state.

### 3. Reconnection & Workspace Manifest (`HandleConnect_PtyWebsocket__Coordinator`)
When a client connects or reconnects:
 1. The coordinator invokes `HandleConnect_PtyWebsocket__Coordinator(id_WebsocketConnection_expected)`.
 2. Under `This.Mutex`, it constructs a `_WorkspaceManifest__PtyMessage_Egress_` (0x0009) containing a `_Bulletin_WorkspacePty_` for every session in `PtyPool`:
    * `Id_PtyProxy`: Session identifier.
    * `ExitOutcome_PtyProxy_maybe`: Process exit status (or nil if still running).
    * `Visibility_Client_current`: Last known visibility.
 3. Transmits the manifest to the client over WebSocket.
 4. **Manifest-First Protocol Flow**: The client receives the manifest, mounts its local UI tab models, identifies which sessions are alive or dead, and responds with a targeted `BATCH__SYNC_PTY` (0x000a) to request snapshots for its visible tabs.

---

## Multi-Threaded Concurrency Map & Synchronization Hierarchy

`WorkspaceController` operates across eight distinct execution contexts:

```
[Goroutine 1: Network Ingress] -----------> __decodeWebsocketPayload_binaryMessage
                                                   |
      +--------------------+-----------------------+-----------------------+
      | (Async Spawn)      | (Sync/Connect/Exit)   | (Resize)              | (Stdin)
      v                    v                       v                       v
[Goroutines 2: Spawn]  [Goroutine 3: Coord]    [Goroutine 4: Debounce] [Goroutines 5: PtyWriter]
  Spawn_PtyProxy         QueueChannel_Order      QueueChannel            QueueChannel_Input
  _PTY.Start (fork/exec) Reconcile & Execute     50ms Trailing Timer     write(fd) to master
                         Sync / Manifest         PtyProxy.Resize
                               |
                               +-----------------------------+
                                                             v
[Goroutines 6: PtyReader] ---> QueueChannel__Order_PtyFlusher ---> [Goroutines 7: PtyFlusher]
  read(fd) from master         (24 orders / 96 KB)                  16ms / 60 FPS Event Loop
  100% unlocked                                                     HandleBlockingFlush (Mutex)
                                                                    OnOutput_Live (Clone Egress)
```

### Execution Contexts
 1. **Network Ingress Loop (Goroutine 1)**: Executes `conn.ReadMessage()` inside `WebsocketController`. Dispatches binary payloads to `HandlePayload_BinaryMessage__PtyWebsocket`. Must remain strictly non-blocking.
 2. **PTY Spawning Workers (Goroutine Set 2)**: Ephemeral goroutines spawned by `_SpawnPty_.Execute`. Executes `_PTY.Start`, file descriptor setup, and `HandleSpawned_Pty`.
 3. **Lifecycle Coordinator Worker (Goroutine 3)**: Single persistent goroutine running `LifecycleCoordinator_WorkspacePty.RunWorker`. Drains, reconciles, and executes all session orders sequentially.
 4. **Layout Reducer Worker (Goroutine 4)**: Single persistent goroutine running `MessageReducer__LayoutUpdate_PtyProxy.RunWorker`. Coalesces rapid geometry updates on a 50ms trailing edge, while immediately flushing discrete lifecycle transitions (`Spawn`, `Remove`, `Sync`) with 0ms delay in strict FIFO order.
 5. **PTY Stdin Writers (Goroutine Set 5)**: Dedicated background workers (`PtyWriter.RunWorker`, one per PTY) executing blocking `write()` calls against the Linux kernel PTY master descriptor.
 6. **PTY Stdout Readers (Goroutine Set 6)**: Dedicated background workers (`PtyReader.RunWorker`, one per PTY) executing blocking `read()` calls against the Linux kernel PTY master descriptor 100% unlocked and streaming orders into `QueueChannel__Order_PtyFlusher`.
 7. **PTY Egress Flushers (Goroutine Set 7)**: Dedicated background workers (`PtyFlusher.RunWorker`, one per PTY) managing the 16 ms / 60 FPS pacing loop, batch coalescing, grid updating under `PtyProxy.Mutex`, and egress dispatch.
 8. **Main Server Lifecycle Thread (Goroutine 8)**: Executes `main()`, handling OS signals (`SIGINT`, `SIGTERM`) and triggering graceful shutdown via `Shutdown`.

### Lock Ordering & AB-BA Deadlock Prevention
To ensure circular-wait deadlocks remain mathematically impossible:
 * **`WorkspaceController.Mutex` Boundary**: Protects `PtyPool`, `Id_PtyProxy_next`, and `Visibility_Client_current`. It is held **exclusively** for fast in-memory map/field operations (microseconds).
 * **Zero Lock Nesting**: `WorkspaceController.Mutex` is **NEVER** held while acquiring `PtyProxy.Mutex`, calling OS system calls (`pty.Setsize`, `syscall.Kill`), performing network writes, or pushing to channels.
 * **Defensive Map Cloning**: Whenever iterating over `PtyPool` (e.g., in `HandleDisconnect` or `HandleSync`), the pool is cloned under lock via `_MAPS.Clone(This.PtyPool)` and the lock is immediately released. Iteration and downstream proxy method calls proceed 100% unlocked.

---

## Inherent System Realities & Architectural Trade-Offs

### 1. Non-Blocking Stdin Drop vs. Unresponsive Process Freeze
* **System Reality**: When a child process stops draining stdin, the kernel PTY input ring buffer (~64 KB) fills completely.
* **Trade-Off**: `WorkspaceController` opts to drop overflow stdin keypresses via non-blocking select once `PtyWriter.QueueChannel_InputOrder` (capacity 1024) saturates. Guaranteeing 100% input delivery would require blocking the ingress handler, which would freeze WebSocket transport, keepalives, and other sessions.

### 2. Trailing-Edge Resize Latency vs. Terminal Redraw Thrashing
* **System Reality**: Browser window resizing generates dozens of events per second.
* **Trade-Off**: Interposing a 50ms debounce window introduces a microscopic 50ms visual delay before the child process re-renders to the final window geometry. This latency is accepted to eliminate devastating `SIGWINCH` redraw avalanches and network buffer exhaustion.

### 3. In-Memory Headless VTE Overhead vs. Instant Tab Switching
* **System Reality**: Maintaining a full `*xterm.Terminal` emulator instance per open PTY in RAM consumes memory (backing grid buffers, scrollback history up to 10,000 lines).
* **Trade-Off**: The server retains headless VTE state for all background tabs so that switching tabs or reconnecting after a network drop can immediately produce an authoritative ANSI snapshot (`xterm.NewSerializeAddon`). Re-running or reflowing terminal history without server VTE state would cause visual corruption.

---

## Domain Invariants

 1. **Non-Blocking Ingress Callback Contract**: `HandlePayload_BinaryMessage__PtyWebsocket` and all ingress message `Execute` methods MUST remain strictly non-blocking; any latency-inducing operation must be delegated to background workers or queues.
 2. **Monotonic Sequential ID Allocation**: `Id_PtyProxy_next` is strictly incremented under `WorkspaceController.Mutex` prior to launching asynchronous process spawning goroutines.
 3. **Spawning Handshake Precedence**: `Spawn_PtyProxy` synchronously invokes `HandleSpawned_Pty` to register the session in `PtyPool` and emit `STATUS__SPAWN_PTY` (0x0002) BEFORE launching background reader and writer goroutines.
 4. **Elevated Exit Order Reconciliation**: The lifecycle coordinator always reconciles and executes process exit orders (`_ExitPty_`) BEFORE session-level orders (`Connect`, `Disconnect`, `Sync`) within any micro-batch.
 5. **Disconnect Session Purging**: A `_Disconnect_` order atomically purges any preceding pending session orders in the coordinator's micro-batch.
 6. **Quiescent Background Tab Silence**: Sessions marked `NOT_VISIBLE__Visibility_Client` MUST have live stdout egress suppressed (`PRE_SNAPSHOT__RUNNING___Mode_PtyProxy`), transmitting zero network frames while maintaining headless VTE state in memory.
 7. **Bracketed Snapshot Synchronization**: Snapshots delivered during `Not Visible -> Visible` transitions MUST be strictly encapsulated within `START_TASK__SYNC_PTY` (0x000b) and `COMPLETE_TASK__SYNC_PTY` (0x000c) control frames.
 8. **Administrative Process Retention**: Exited sessions are NEVER automatically removed from `PtyPool`; removal requires an explicit client `BATCH__REMOVE_PTY` order AND an established `ExitOutcome_PtyProxy_maybe != nil`.
 9. **Process Group Signal Propagation**: Process termination broadcasts signals to the negative process PID (`-Pid`) to terminate the entire process group and prevent orphaned daemon leaks.
 10. **Zero Cross-Layer Lock Nesting**: `WorkspaceController.Mutex` must never be held across calls to `PtyProxy.Mutex`, OS system calls, channel operations, or network writes.
 11. **Generational Egress Verification**: All outbound frames transmitted via `Emit__PtyMessage_Egress__WebsocketController_Pty` MUST assert the expected connection generation ID to prevent cross-session leakage.
