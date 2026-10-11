# WorkspaceController System Specification

## Macro Architectural Role & System Context

`WorkspaceController` is the root coordinator of `suprasole-server`. It coordinates four primary subsystems: `WorkspaceNetwork`, `PtyPool`, `MessageReducer__LayoutUpdate_PtyProxy`, and `LifecycleCoordinator_WorkspacePty`. It translates asynchronous events from the operating system, network transport, and client interface into sequential state transitions.

* **Root Aggregator Role**:
  * Owns the pseudo-terminal process pool (`PtyPool`).
  * Owns the network interface bridge (`WorkspaceNetwork`).
  * Owns the layout update reducer (`MessageReducer__LayoutUpdate_PtyProxy`).
  * Owns the serialized lifecycle coordinator (`LifecycleCoordinator_WorkspacePty`).
  * Generates sequential identifiers for pseudo-terminal processes (`Id_PtyProxy_next`).

* **Passive Mediation Hub Pattern**:
  * Possesses no dedicated internal run loop or background worker goroutine of its own.
  * Operates as a passive structural hub that stores shared state (`PtyPool`) and wires channels between active worker subsystems.
  * Dispatches callbacks and delegates operations without executing long-running thread loops.

---

## Macro Subsystem Topology & Boundaries

The server isolates functional responsibilities into dedicated subsystems. `WorkspaceController` mediates all cross-subsystem interactions.

* **`WorkspaceNetwork` (Owning `WebsocketController_Pty`)**:
  * Manages HTTP listeners and WebSocket upgrades on the `/pty` path.
  * Owns `WebsocketController_Pty`, an instance of `WebsocketController` dedicated to terminal client sessions.
  * Runs a dedicated single-threaded read loop (`conn.ReadMessage()`).
  * Enforces a single-writer lock (`EgressMutex`) for outbound frames.
  * Dispatches raw binary payloads to `WorkspaceController` through `OnPayload_BinaryMessage__PtyWebsocket__`.

* **`PtyPool` (`map[uint32]*_WorkspacePty_`)**:
  * Stores all active and terminated pseudo-terminal instances in server memory.
  * Holds references to `_PtyProxy_`, visibility state, and process exit outcomes.
  * Protected against concurrent access by `WorkspaceController.Mutex`.

* **`MessageReducer__LayoutUpdate_PtyProxy`**:
  * Runs an independent background worker goroutine.
  * Collects layout update messages (`Spawn`, `Remove`, `Resize`, `Sync`) from the network read thread.
  * Coalesces repetitive dimensions using a key-value map and accumulates an order envelope.
  * Flushes accumulated layout orders, removes, and spawns on a 50ms trailing edge.

* **`LifecycleCoordinator_WorkspacePty`**:
  * Runs an independent background worker goroutine.
  * Receives lifecycle orders through a bounded channel (`QueueChannel_WorkspaceOrder`).
  * Reconciles batches of connection, disconnection, synchronization, and exit orders.
  * Executes state changes sequentially to eliminate race conditions.

---

## Plane Separation Architecture (Control Plane vs. Data Plane)

`WorkspaceController` structurally partitions control operations from terminal data streams to prevent data-flow interference.

* **Control Plane (Serialized & Ordered)**:
  * Manages connection events, disconnection events, manifest broadcasts, visibility synchronization, and process exit handling.
  * Routes operations strictly through `LifecycleCoordinator_WorkspacePty` or synchronized `PtyPool` updates.
  * Enforces deterministic state order across asynchronous network and process events.

* **Data Plane (Direct & Decoupled)**:
  * Manages PTY input streams and process standard output byte streams.
  * Directs PTY input writes into isolated `PtyWriter` channels per terminal.
  * Directs output reads through decoupled `PtyFlusher` pacing loops to WebSocket egress.
  * Bypasses the lifecycle coordinator completely to prevent head-of-line blocking between bulk data streams and control operations.

---

## Concurrent Execution Contexts

`WorkspaceController` has no internal event loop. It operates as a passive target executed concurrently across six external execution contexts.

### Context 1: PTY Network Ingress Worker (`WebsocketController_Pty.RunWorker__Submission_GetWebsocketConnection`)

#### Associated Handlers & Callback Mapping
* `HandleConnected_PtyWebsocket` -> `OnConnected_PtyWebsocket__`
* `HandleConnected_Takeover__PtyWebsocket` -> `OnConnected_Takeover__PtyWebsocket__`
* `HandleDisconnected_PtyWebsocket` -> `OnDisconnected_PtyWebsocket__`
* `HandleDisconnected_Takeover__PtyWebsocket` -> `OnDisconnected_Takeover__PtyWebsocket__`
* `HandlePayload_BinaryMessage__PtyWebsocket` -> `OnPayload_BinaryMessage__PtyWebsocket__`

#### Interplay: WorkspaceController <- Thread
* **Synchronous Network Ingress**: The `WebsocketController_Pty` thread invokes handlers directly upon socket events and binary frame arrival.
* **Zero-Latency Non-Blocking Invariant**: Never blocks this thread on slow disk I/O, heavy computation, or lock contention to preserve WebSocket keepalive responsiveness.
* **Asynchronous Task Offloading**: Offloads operations immediately off the `WebsocketController_Pty` thread to protect transport liveness:
  * **Process Spawning**: Delegates process creation to ephemeral background goroutines.
  * **Window Resizing**: Enqueues geometry update batches into the layout reducer queue.
  * **PTY Input**: Routes input chunks directly into isolated per-process writer queues.
  * **Session Synchronization**: Enqueues declarative visibility sync batches into the coordinator order queue.
  * **Process Termination**: Forwards termination signals directly to target child processes.
* **Lossless Control-Plane Dispatch**: Routes connection, takeover, disconnection, and session synchronization transitions as blocking lifecycle orders to the bounded coordinator channel, enforcing total event serialization across network and process lifecycles while propagating upstream backpressure.
* **Generational Connection Tagging**: Tags coordinator orders with the active connection generation, ensuring egress messages are blocked if the client disconnected or was superseded by a takeover.

### Context 2: Lifecycle Coordinator Worker (`LifecycleCoordinator_WorkspacePty.RunWorker`)

#### Associated Handlers & Callback Mapping
* `HandleConnect_PtyWebsocket__Coordinator` -> `OnConnect_PtyWebsocket__`
* `HandleDisconnect_PtyWebsocket__Coordinator` -> `OnDisconnect_PtyWebsocket__`
* `HandleSyncVisibility__Coordinator` -> `OnSyncVisibility__`
* `HandleEmitSnapshot_PtyProxy__Coordinator` -> `OnEmitSnapshot_PtyProxy__`
* `HandleExit_PtyProxy__Coordinator` -> `OnExit_PtyProxy__`
* `HandleSpawnPty__Coordinator` -> `OnSpawnPty__`
* `HandleRemovePty__Coordinator` -> `OnRemovePty__`
* `HandleTerminatePty__Coordinator` -> `OnTerminatePty__`
* `HandleStatus_SpawnPty__Success__Coordinator` -> `OnStatus_SpawnPty__Success__`
* `HandleStatus_SpawnPty__Failure__Coordinator` -> `OnStatus_SpawnPty__Failure__`

#### Interplay: WorkspaceController <-> Thread
* **Bidirectional Pipeline Flow**: Enqueues lifecycle orders into the coordinator channel; the `LifecycleCoordinator_WorkspacePty` thread drains and reconciles orders before executing sequential callbacks.
* **Serialized Callback Execution**: The `LifecycleCoordinator_WorkspacePty` thread executes callbacks sequentially, eliminating concurrent mutation races across `PtyPool` entries.
* **Isolated `WorkspacePty` Mutation**: Confines mutations of existing `WorkspacePty` instances strictly to the `LifecycleCoordinator_WorkspacePty` thread, serializing client visibility and process exit updates.
* **Lock-Decoupled Egress**: Emits WebSocket messages and snapshots outside of lock, preventing network write latency and socket backpressure from stalling concurrent `PtyPool` operations.
* **Serialized `PtyProxy` Mode Transitions**: Serializes all `PtyProxy` mode transitions driven by network connectivity, client tab visibility, and child process exits.
  * **Snapshot Egress Dispatch**: The `LifecycleCoordinator_WorkspacePty` thread streams terminal snapshots and post-snapshot output directly to WebSocket egress during session synchronization.

### Context 3: Layout Reducer Worker (`MessageReducer__LayoutUpdate_PtyProxy.RunWorker`)

#### Associated Handlers & Callback Mapping
* `HandleFlush__ResizeOnly__Reducer` -> `OnFlush__ResizeOnly__`
* `HandleFlush__SpawnPty__Reducer` -> `OnFlush__SpawnPty__`
* `HandleFlush__Batch_RemovePty__Reducer` -> `OnFlush__Batch_RemovePty__`
* `HandleFlush__Batch_SyncPty__Reducer` -> `OnFlush__Batch_SyncPty__`

#### Interplay: WorkspaceController <-> Thread
* **Serialized Layout & Lifecycle Order Reduction**: Offloads layout-altering orders (`Spawn`, `Remove`, `Resize`, `Sync`) into the reducer channel; pure layout orders are debounced on a 50ms trailing edge while lifecycle operations flush immediately.
* **Ingress Latency Decoupling**: Confines geometry updates and lifecycle sequencing to the `MessageReducer__LayoutUpdate_PtyProxy` thread, shielding the `WebsocketController_Pty` thread from high-frequency browser resize bursts.
* **Invalid Layout Order Handling**: Absorbs and discards invalid layout orders without raising errors:
  * **Missing or Purged Sessions**: Silently drops orders targeting unknown identifiers or sessions removed from `PtyPool` during the debounce window.
  * **Terminated Process Descriptors**: Suppresses Linux kernel `ioctl` errors if target processes have exited and closed their master descriptors.

### Context 4: PTY Stdout Readers & Egress Flushers (Concurrent, 2 per active PTY: `PtyReader.RunWorker`, `PtyFlusher.RunWorker`)

#### Associated Handlers & Callback Mapping
* `HandleOutput_Pty` -> `OnOutput_Live__PtyProxy__`
* `HandleExited_Eio_Success__Pty` -> `OnExited_Eio_Success__PtyProxy__`
* `HandleExited_Eio_Failure__Pty` -> `OnExited_Eio_Failure__PtyProxy__`
* `HandleExited_Eio_Killed__Pty` -> `OnExited_Eio_Killed__PtyProxy__`
* `HandleExited_Closed__Pty` -> `OnExited_Closed__PtyProxy__`
* `HandleExited_SystemError__Pty` -> `OnExited_SystemError__PtyProxy__`

#### Interplay: WorkspaceController <- Thread
* **Direct Egress Streaming**: `PtyFlusher` threads stream live terminal output directly to WebSocket egress outside of lock on a 16 ms / 60 FPS pacing cadence (with 0 µs leading-edge prompt flush).
* **Control-Plane Exit Serialization**: `PtyFlusher` threads trigger exit callbacks upon process termination (after draining `QueueChannel__Order_PtyFlusher`); packages outcomes into blocking `LifecycleCoordinator_WorkspacePty` orders, serializing process teardown against connection and sync events. Blocking is safe because handlers execute outside of lock on terminating `PtyFlusher` threads with no remaining lifecycle tasks.

### Context 5: Process Spawning Workers (Ephemeral `go func()`)

#### Associated Handlers & Callback Mapping
* `HandleSpawned_Pty` -> `OnSpawned_PtyProxy__`
* `HandleSpawnFailed_Pty` -> Error handler for `Spawn_PtyProxy` failures

#### Interplay: WorkspaceController <-> Thread
* **Syscall Latency Isolation**: Confines blocking operating system calls (`fork/exec`, PTY allocation) to this ephemeral thread, shielding the `WebsocketController_Pty` thread from kernel scheduling delays.
* **All-or-Nothing Registry Publication**: The ephemeral thread absorbs the fallible process creation window in isolation. Registers the session in `PtyPool` under lock only after process viability is proven, ensuring concurrent reader threads never observe intermediate or unready states.
* **Zero-Rollback Failure Containment**: Confines spawn failures entirely to the ephemeral worker. Because unconfirmed processes are never inserted into `PtyPool`, failures require zero compensatory rollback, tombstoning, or cleanup in controller state.
* **Coordinator Pipeline Decoupling**: Bypasses `LifecycleCoordinator_WorkspacePty` completely during both registration and failure reporting, preventing OS process creation latency from introducing head-of-line blocking on the control-plane order queue.

### Context 6: Main Server Thread (`main()`)

#### Associated Methods & Lifecycle Mapping
* `Start` -> Direct invocation by `main` (server bootstrap)
* `Shutdown` -> Direct invocation by `main` (shutdown signal receipt)

#### Interplay: WorkspaceController <- Thread
* **Top-Down Lifecycle Invocation**: The `main` thread controls macro application state by calling `Start` and `Shutdown`.
* **Dependency-Ordered Activation**: `Start` starts `MessageReducer__LayoutUpdate_PtyProxy` and `LifecycleCoordinator_WorkspacePty` worker routines before activating `WorkspaceNetwork`, ensuring queues are operational before accepting client traffic.
* **Cascaded Worker Teardown**: `Shutdown` cancels worker contexts for `MessageReducer__LayoutUpdate_PtyProxy` and `LifecycleCoordinator_WorkspacePty` before delegating `WorkspaceNetwork` listener closure within the bounded deadline context.

---

## WorkspacePty Registry Lifecycle (Registration, Access, Mutation & Removal)

`WorkspaceController` coordinates pseudo-terminal sessions in `PtyPool`. It enforces distinct lock scopes, access rules, and thread boundaries across each lifecycle phase:

* **Session Registration (Insertion)**:
  * **Executing Context**: Ephemeral `Spawn_PtyProxy` goroutines (Context 5) through `HandleSpawned_Pty`.
  * **Lock Scope & Isolation**: Acquires lock to insert the new `&_WorkspacePty_` pointer into `PtyPool`. Bypasses `LifecycleCoordinator_WorkspacePty` completely.
  * **Registration Sequencing**: Commits the session to `PtyPool` under lock before emitting wire status `SUCCESS__Status_SpawnPty` outside of lock.

* **Session Access (Lookups & Dispatch)**:
  * **Executing Contexts**:
    * `WebsocketController_Pty` thread (Context 1) looks up sessions for PTY input chunks and termination signals.
    * `MessageReducer__LayoutUpdate_PtyProxy` thread (Context 3) looks up sessions to apply layout updates.
    * `LifecycleCoordinator_WorkspacePty` thread (Context 2) clones `PtyPool` to generate session manifests and synchronize visibility.
  * **Lock Scope**: Performs pointer lookups or clones the registry map under lock.
  * **Idempotency Invariant**: Rechecks session pointers (`if WorkspacePty_target != nil`). Discards operations on missing or purged identifiers without raising errors or creating invalid state.

* **Session Mutation (State Updates)**:
  * **Executing Context**: Strictly confined to the `LifecycleCoordinator_WorkspacePty` thread (Context 2).
  * **Lock Scope**: Acquires lock to update existing `_WorkspacePty_` fields (`Visibility_Client_current` in `HandleSync_PtyPool__Coordinator`, and `ExitOutcome_PtyProxy_maybe` in `HandleExit_PtyProxy__Coordinator`).
  * **Concurrency Boundary**: No other execution context is permitted to mutate existing `_WorkspacePty_` struct fields. This strict boundary eliminates concurrent write races.

* **Session Removal (Deletion & Cleanup)**:
  * **Executing Context**: `WebsocketController_Pty` thread (Context 1) through `_Batch_RemovePty_.Execute`.
  * **Lock Scope & Isolation**: Acquires lock to delete entries from `PtyPool`. Bypasses `LifecycleCoordinator_WorkspacePty` completely.
  * **Purge Precondition**: Asserts that `ExitOutcome_PtyProxy_maybe != nil`. Active, running sessions are immune to removal orders and remain protected in `PtyPool`.
  * **Teardown Independence**: Deletes session pointers only after the target process has already exited, its `PtyWriter` worker has been cancelled, and its `PtyReader` routine has stopped.

---

## Ingress Dispatch Policies & Backpressure Routing

`WorkspaceController` operates as an ingress shock absorber. It classifies incoming events and enforces distinct channel dispatch policies to protect server liveness.

* **Lossless Blocking Dispatch (Lifecycle Control Plane)**:
  * **Target Channel**: `LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder` (Capacity 16).
  * **Dispatch Semantic**: Synchronous blocking send (`QueueChannel_WorkspaceOrder <- order`).
  * **Operational Rationale**: Connection, disconnection, takeover, synchronization, and exit events represent structural state invariants. Prohibits dropping or coalescing lifecycle transitions under load. Linearizes cross-domain network and OS process events into a single FIFO queue, while bounded channel saturation propagates backpressure directly to upstream TCP ingress.

* **Decoupled Layout Reduction (Geometry & Lifecycle Control Plane)**:
  * **Target Channel**: `MessageReducer__LayoutUpdate_PtyProxy.QueueChannel` (Capacity 512).
  * **Dispatch Semantic**: Direct queue handoff upon ingress message decoding.
  * **Operational Rationale**: Moves high-frequency browser layout changes and layout-altering lifecycle messages immediately off the `WebsocketController_Pty` thread into an asynchronous coalescing/reduction tier.

* **Lossy Non-Blocking Dispatch (Terminal Data Plane)**:
  * **Target Channel**: `PtyWriter.QueueChannel_InputOrder` (Capacity 1024 per process).
  * **Dispatch Semantic**: Double non-blocking `select` with immediate `default:` discard.
  * **Operational Rationale**: Protects the `WebsocketController_Pty` thread from Linux kernel TTY input ring buffer backpressure. Drops PTY input during child process pauses to prevent freezing the active WebSocket connection.

* **Buffer Sizing Authority**:
  * Owns global default options (`OptionConfig_PtyProxy__default__`).
  * Defines the per-process input queue quota (`Size_QueueBuffer__InputOrder_PtyWriter: 1024`).
  * Applies default queue quotas to all newly spawned pseudo-terminal instances.

---

## Subsystem Lifecycle Cascading (Startup & Teardown Sequencing)

`WorkspaceController` coordinates deterministic subsystem startup and shutdown sequences to protect against race conditions.

* **Startup Cascade (`Start`)**:
  * **Queue Subsystem Pre-Activation**: Spawns layout reducer and lifecycle coordinator worker routines before opening the network listener, ensuring that internal state channels are actively drained before client ingress arrives.
  * **Listener Ignition**: Binds and starts `WorkspaceNetwork` HTTP/WebSocket listener only after dependent background consumers achieve operational readiness.

* **Teardown Cascade (`Shutdown`)**:
  * **Worker Pipeline Cancellation**: Signals cancellation contexts for layout reducer and coordinator pipelines through `WorkerCancel()`, draining pending orders and preventing subsequent state mutation.
  * **Network Teardown Delegation**: Delegates listener shutdown to `WorkspaceNetwork.Shutdown` with the bounded timeout context (`context_shutdownDeadline__HttpServer`).

---

## Session Registry Integrity & Failure Handling

`WorkspaceController` enforces defensive validation rules to preserve `PtyPool` consistency and isolate failure states across asynchronous workflows.

* **Partial Session State Immunity (Spawn Failure Containment)**:
  * **Failure Event**: `_PTY.Start` fails during process creation (missing executable, permission denied, process limits exceeded).
  * **Controller Defense**: Registration in `PtyPool` occurs strictly inside `HandleSpawned_Pty`. If process spawning fails, `PtyPool` remains unmutated, the allocated identifier is discarded, and the controller emits `STATUS__SPAWN_PTY` with `FAILURE__Status_SpawnPty`.

* **Missing Target Idempotency (Stale Reference Toleration)**:
  * **Failure Event**: Client transmits input, resize, termination, or removal messages targeting an unknown or previously deleted pseudo-terminal identifier.
  * **Controller Defense**: Controller performs `PtyPool` lookups under lock and validates pointer existence (`if WorkspacePty_target != nil`). Operations on missing identifiers complete as silent no-ops without panicking or creating corrupt state.

* **Running Session Purge Protection**:
  * **Failure Event**: Client transmits `BATCH__REMOVE_PTY` targeting an active, running pseudo-terminal process.
  * **Controller Defense**: `HandleRemovePty__Coordinator` asserts that the process has reached `_State_Exited__WorkspacePty_`. Running sessions are immune to removal orders and remain protected in `PtyPool`.

* **Malformed Ingress Frame Isolation**:
  * **Failure Event**: Client transmits payloads shorter than two bytes, unrecognized message opcodes, or corrupted field schemas.
  * **Controller Defense**: `__decodeWebsocketPayload_binaryMessage` logs the protocol error and terminates execution immediately. The malformed frame is discarded without mutating session state, modifying queues, or interrupting the `WebsocketController_Pty` thread.

* **Generational Stale Egress Discarding**:
  * **Failure Event**: Asynchronous background readers or coordinators trigger outbound writes after a network disconnect or session takeover.
  * **Controller Defense**: `Emit__PtyMessage_Egress__WebsocketController_Pty` passes the expected connection identifier to `WebsocketController_Pty`. If connection identifiers misalign, the underlying `WebsocketController` drops the write safely without leaking data to new sessions.
