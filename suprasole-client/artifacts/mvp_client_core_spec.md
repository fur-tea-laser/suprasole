# Architectural Specification: suprasole-client MVP Core (Macro Design)

This document specifies the macro-level architecture, state machine boundaries, network interactions, and API contracts for the `suprasole-client` core. The core is a headless, framework-agnostic engine designed to manage connection lifecycle, terminal state synchronization, binary protocol coordination, and priority scheduling.

---

## 1. Architectural Overview, Context & Decoupling

The `suprasole-client` is designed around a strict separation of concerns between stateful core business logic and presentation rendering. Rather than mixing networking, byte stream coordination, and state management directly inside UI components (such as React or Solid.js views), all operational capabilities are isolated into a headless **Client Core**.

```mermaid
graph TD
    subgraph UI_Layer [UI Presentation Layer (e.g., React View Shell)]
        A[Terminal UI Tab Grid] -->|1. Awaits core action| B(store.api Methods)
        C[store.subscribe / useSelector] -->|4. Renders state changes| D[(Redux Store State)]
    end

    subgraph Core_Engine [Client Core Engine (Redux + Redux-Saga)]
        B -->|Invokes Saga| E{Redux-Saga Coordinator}
        E -->|2. Dispatches Actions| F[Redux Action Pipeline]
        F -->|3. Mutates State| D
        E -->|5. Packs Outgoing Frame| G[Outbound Queue Channel]
        H[Inbound Event Channel] -->|6. Dispatches parsed data| E
    end

    G -->|WebSocket Binary Frames| I[suprasole-server]
    I -->|WebSocket Binary Frames| H
```

### The UI vs. Core Boundaries

To maintain clean boundaries and preserve testability, the UI and the Core operate as decoupled subsystems:

* **The UI Layer is Passive:** The UI does not open network sockets, serialize binary protocol frames, manage keep-alive pings, or orchestrate reconnection backoffs. It interacts with the outside world purely by subscribing to the Redux store (drawing text when scrollbacks change or updating terminal tabs when terminals spawn or exit) and invoking methods on `store.api`.
* **The Core Layer is View-Agnostic:** The Core has no knowledge of xterm.js instances, browser DOM nodes, CSS layouts, keymaps, or view components. It operates strictly in terms of abstract terminal IDs, columns/rows, binary packet frames, and text stream buffers.
* **The Inter-Module Bridge:**
  * **Core to UI (Data):** Unidirectional data flow via the Redux store. When the Core receives updates (like output or process terminations), it dispatches Redux actions. Reducers mutate the store, and the UI automatically re-renders based on store subscriptions.
  * **UI to Core (Control):** Unidirectional command flow via the Promise-based `store.api` object. The UI triggers operations by invoking these methods and awaiting their completion.

---

## 2. Redux-Saga in the Macro Architecture

Redux-Saga acts as the "engine room" of the Client Core. Sagas are generator functions that coordinate asynchronous operations, bridging the gaps between standard synchronous Redux flows and the real-time, event-driven network socket.

### Why Redux-Saga was Chosen
* **Flattens Async Complexity:** Managing WebSockets, reconnection timers, priority sync events, and backpressure manually inside component hooks or Redux Thunks leads to scattered, stateful side effects. Sagas allow us to express these multi-step asynchronous processes linearly and readably.
* **Insulates Core State:** Sagas catch side-effects from the outside world (WebSocket binary streams, user input events, network drops) and convert them into clean, predictable Redux actions that are dispatched to the store. This keeps reducers pure and easy to test.
* **Sequential Write Coordination:** Sagas excel at managing concurrent write requests, allowing the client to channel all outgoing data through a single sequential queue (using Redux-Saga `actionChannel` or `channel` structures), preventing packet interleaving.

### Saga Design Philosophy & Anti-Pattern Prevention

Sagas are extremely flexible, which makes them susceptible to over-engineering. To keep the codebase basic and linear, we enforce the following usage rules:

1. **Avoid Unnecessary Indirection:** Keep saga loops simple and linear by default. Branching, task nesting, and background forks should only be introduced when there is a clear operational requirement (such as running concurrent inbound reading and outbound writing loops). Do not chain-react sagas unless absolutely necessary.
2. **The Void Return Invariant (`store.api` Semantics):** All API sagas exposed to the outside world must return `void`.
   * Sagas must never return data structures, exit codes, or connection objects to the UI.
    * The Promise returned when calling an API method (e.g. `await store.api.spawnPTY__server_command(...)`) resolves purely as a signal that the saga task has successfully completed.
    * If the UI needs data resulting from an action, it must query that data from the Redux store state.
3. **Saga as a Boundary Guard:** Reducers remain the sole authority on state transitions. Sagas must never mutate state directly. Furthermore, sagas must dispatch actions *exclusively* using the Redux-Saga `put` effect (or other standard Redux-Saga effects) rather than bypassing the saga flow by invoking `store.dispatch` directly. This keeps the entire action pipeline unified within the Saga middleware, ensuring all events can be monitored and coordinated.

---

## 3. End-to-End Control & Data Flow Gist

To illustrate how these systems interact, the following scenarios trace the macro-level flow of control and data:

#### Scenario A: Spawning a New Terminal
1. **Trigger:** The user clicks a button in the UI to spawn terminal ID `5` with dimensions `80x24` running `bash`.
2. **Command:** The UI calls `store.api.spawnPTY__server_command(5, 80, 24, "bash", [])`.
3. **Execution:** The spawn saga dispatches a `SPAWN_TERMINAL_REQUEST` action. The reducer catches this and sets the terminal status for ID `5` to `spawning`.
4. **Serialization:** The saga serializes the command parameters (columns, rows, command path, arguments count, and arguments list) into a binary `0x0001` (`SpawnPTY`) frame and pushes it to the Outgoing Queue.
5. **Network:** The Outgoing loop writes the binary frame to the WebSocket.
6. **Response:** The server spawns the terminal and replies with a binary `0x0002` (`SpawnPTYStatus`) frame indicating success (`0x00`).
7. **Ingress:** The client's Inbound loop reads the binary frame, decodes the success status for terminal `5`, and dispatches `SPAWN_TERMINAL_SUCCESS`.
8. **Render:** The Redux store updates the terminal's status to `active`. The UI detects this change, instantiates an xterm.js renderer, and binds it to terminal `5`.

#### Scenario B: High-Throughput Output Streaming
1. **Server Push:** A command running inside terminal `5` (e.g., `top`) writes data to stdout. The server packages this into a binary `0x0008` (`OutputPTY`) frame and writes it to the WebSocket.
2. **Read Loop:** The client Inbound loop reads the frame, decodes the `Terminal ID` (`5`), converts the binary payload into a UTF-8 string, and dispatches `TERMINAL_OUTPUT_RECEIVED`.
3. **Buffer Management:** The reducer appends the text stream to the scrollback buffer for terminal `5` in the Redux state, dropping old lines if the buffer size exceeds limits.
4. **UI Draw:** The UI terminal view component, subscribed to the Redux scrollback state, detects the update and writes the new text stream directly to the local xterm.js instance.

### Scenario C: Priority Syncing (Tab Switching)
1. **Trigger:** The user clicks a tab in the UI to switch focus from Tab `1` (wrapping terminal `1`) to Tab `2` (wrapping terminal `2`).
2. **Command:** The UI calls `store.api.selectTab__workspace({ tabId: 'tab-2' })`.
3. **State Change:** The Redux store updates `activeTabId` to `'tab-2'` and sets focus to terminal `2`.
4. **Sync Trigger:** The background Priority Sync Saga intercepts the layout change. It queries the Redux store state to collect all active terminals. It marks terminal `2` (inside the active tab) as High priority (`0x01`) and terminal `1` (inside the background tab) as Low priority (`0x00`).
5. **Write:** The saga invokes `store.api.syncPTYPriorities__server_command(compiledPriorities)`, which posts the binary priority sync frame (`0x0009`) to the WebSocket Outgoing Queue.
6. **Server Action:** The server receives the frame and updates its internal scheduling queues, pacing background output for terminal `1` to eliminate Head-of-Line blocking and prioritize terminal `2` data streams.

---

## 4. Redux Store State Schema (Conceptual Model)

The client store state maintains the connection status, terminal state descriptors, output streams, session tab/pane layouts, focus context, and loaded extensions:

* **Connection Status Object:**
  * **Network Status:** Indicates the state of the socket (e.g. `disconnected`, `connecting`, `connected`, or `error`).
  * **Link Coordinates:** The active host URL and matching session token.
  * **Diagnostic Text:** A descriptive error message populated on network drops or connection failures.
* **Terminal Registry Map:**
  * Maps each active numeric terminal ID to its metadata descriptor:
    * **State Status:** Indicates whether the terminal shell is `spawning`, `active`, or `terminated`.
    * **Exit Status Code:** A 1-byte code indicating the process exit code, populated when terminated.
    * **Geometry:** Configured rows and columns layout metrics.
    * **Priority:** Active pacing priority, designated as low/hidden (`0x00`) or high/visible (`0x01`).
* **Scrollback Buffer Collection:**
  * Maps terminal IDs to their accumulated text output streams, truncated to the terminal's capacity to protect memory boundaries.
* **Session Layout & Focus State:**
  * **Active Tab ID:** String ID of the tab currently in view (focused tab). Only one tab is active at a time.
  * **Active Pane ID:** String ID of the pane currently capturing user keyboard input. Only one pane within the active tab is active/focused at a time.
  * **Tabs Dictionary:** Maps `tabId` to Tab descriptors:
    * **ID & Title:** Unique identifiers and label.
    * **Layout Tree:** A hierarchical representation of pane nodes within the tab, describing vertical or horizontal split/stack relationships.
  * **Panes Dictionary:** Maps `paneId` to Pane descriptors:
    * **ID:** Unique identifier.
    * **Tab ID:** The parent tab the pane resides in.
    * **Terminal ID:** The numeric PTY terminal ID associated 1-to-1 with this pane.
* **Plugin Registry Map:**
  * Maps unique `pluginId` string keys to metadata descriptors for active extensions:
    * **ID:** Unique string identifier.
    * **Status:** Current operational state (`loading`, `active`, or `error`).
    * **Transform Action:** Callback function `(input: string) => string` that executes string manipulations on matched inputs.
* **Universal Trigger Patterns:**
  * A list of global string or RegExp patterns maintained in the core state. If raw terminal input matches any pattern in this universal list, the core executes all active plugin transform actions sequentially.
* **Priority Mapping Invariant:**
  * All terminal instances (panes) inside the active tab (`activeTabId`) are **visible** and mapped to high priority (`0x01`) on the server.
  * All terminal instances (panes) inside background/inactive tabs are **hidden** and mapped to low priority (`0x00`) on the server.
  * Focus shifts in tabs or panes automatically trigger priority recalculation and synchronization.

---

## 5. Core Action Definitions

| Action Type | Origin | Payload Details | Purpose |
| --- | --- | --- | --- |
| `WS_CONNECT` | API Call | Host URL string, Token string | Triggers WebSocket connection process |
| `WS_CONNECTED` | Connection Saga | None | Marks connection as established |
| `WS_DISCONNECTED` | Connection Saga | None | Marks connection as offline |
| `WS_CONNECT_ERROR` | Connection Saga | Error message string | Captures WebSocket errors |
| `SPAWN_TERMINAL_REQUEST` | API Call / UI | Terminal ID, columns, rows, command string, args array | Triggers spawn frame generation |
| `SPAWN_TERMINAL_SENT` | Core Saga | Terminal ID | Marks terminal state as 'spawning' |
| `SPAWN_TERMINAL_SUCCESS` | Socket Reader | Terminal ID | Marks terminal state as 'active' |
| `SPAWN_TERMINAL_FAILURE` | Socket Reader | Terminal ID, error message string | Marks terminal state as failed to spawn |
| `RESIZE_TERMINAL_REQUEST` | API Call / UI | Terminal ID, columns, rows | Triggers resize frame generation |
| `KILL_TERMINAL_REQUEST` | API Call / UI | Terminal ID | Triggers kill/SIGKILL frame generation |
| `TERMINAL_OUTPUT_RECEIVED` | Socket Reader | Terminal ID, raw string data chunk | Appends stdout/stderr stream data to scrollback |
| `TERMINAL_TERMINATED` | Socket Reader | Terminal ID, exit status code byte | Marks terminal state as 'terminated' |
| `SERVER_WORKSPACE_RESET` | Socket Reader | None | Triggers clean slate wipe of client layout states and terminal pane tree |
| `RESET_WORKSPACE_REQUEST` | API Call / UI | None | Initiates client-side and server-side workspace reset |
| `RESET_WORKSPACE` | Core Saga | None | Clears all local layout tab and pane tree states |
| `CREATE_TAB` | API Call / UI | Tab ID, Title | Spawns a new tab and default pane |
| `CLOSE_TAB` | API Call / UI | Tab ID | Closes a tab and kills all associated PTYs |
| `SPLIT_PANE` | API Call / UI | Parent Pane ID, new Pane ID, direction (vertical/horizontal), new Terminal ID | Splits a pane and allocates a new PTY |
| `CLOSE_PANE` | API Call / UI | Pane ID | Closes a pane and kills its PTY |
| `SET_ACTIVE_TAB` | API Call / UI | Tab ID | Changes focus to the tab and syncs priorities |
| `SET_ACTIVE_PANE` | API Call / UI | Pane ID | Changes focus to the pane and syncs priorities |
| `SYNC_PRIORITIES` | Priority Saga | None | Gathers active terminal priorities and sends to server |
| `LOAD_PLUGIN` | API Call / UI | Plugin ID, entrypoint URL, config object | Triggers dynamic plugin loading |
| `LOAD_PLUGIN_SUCCESS` | Core Saga | Plugin ID | Registers plugin status as active |
| `LOAD_PLUGIN_FAILURE` | Core Saga | Plugin ID, error message string | Marks plugin status as error |
| `REMOVE_PLUGIN` | API Call / UI | Plugin ID | Unregisters and unloads plugin middleware/handlers |
| `SUBMIT_PROXY_INPUT` | API Call / UI | Terminal ID, raw string/byte data | Triggers input proxy pipeline |
| `TRANSFORM_PROXY_INPUT` | Core Saga | Input string | Initiates sequential plugin transformations |
| `TRANSFORM_PROXY_INPUT_SUCCESS` | Core Saga | Transformed string | Commits transformed input to Redux registry |

---

## 6. Binary Wire Protocol layout

The communication wire uses binary framing. All multi-byte integers are serialized in **Big-Endian** byte order.

### Framing Layout
Every binary frame consists of a **4-byte header** followed by an optional payload:
* **Bytes `0 - 1` (uint16):** Action ID
* **Bytes `2 - 3` (uint16):** Terminal ID
* **Bytes `4 - N` (bytes):** Payload data

### Action Protocols

1. **SpawnPTY (`0x0001` - Client → Server):**
   * *Terminal ID:* The target ID to allocate.
   * *Payload:* `[Cols: 2B (uint16)]` + `[Rows: 2B (uint16)]` + `[Command Length: 2B (uint16)]` + `[Command: Command Length Bytes]` + `[Args Count: 1B (uint8)]` + Followed by `Args Count` instances of: `[Arg Length: 2B (uint16)]` + `[Arg: Arg Length Bytes]`.
2. **SpawnPTYStatus (`0x0002` - Server → Client):**
   * *Terminal ID:* The matching ID requested.
   * *Payload:* `[Status: 1B]` where:
     * `0x00` = Success (Background process started).
     * `0x01` = Failure (Startup error). A matching `PTYTerminalExit` (`0x0005`) frame with exit code `255` is enqueued in the control queue.
     * `0x02` = Spawning (Validation passed, background initialization beginning).
     * `0x03` = Canceled (Spawning cancelled via `KillPTY` or `RemovePTY` mid-launch).
3. **ResizePTY (`0x0003` - Client → Server):**
   * *Terminal ID:* The active ID to resize.
   * *Payload:* `[Cols: 2B (uint16)]` + `[Rows: 2B (uint16)]`
4. **KillPTY (`0x0004` - Client → Server):**
   * *Terminal ID:* Target terminal ID.
   * *Payload:* Empty.
5. **PTYTerminalExit (`0x0005` - Server → Client):**
   * *Terminal ID:* Target terminal ID.
   * *Payload:* `[Exit Code: 1B]`
6. **RemovePTY (`0x0006` - Client → Server):**
   * *Terminal ID:* Target terminal ID.
   * *Payload:* Empty.
7. **InputPTY (`0x0007` - Client → Server):**
   * *Terminal ID:* Target terminal ID.
   * *Payload:* Raw bytes representing keystrokes/data input.
8. **OutputPTY (`0x0008` - Server → Client):**
   * *Terminal ID:* Target terminal ID.
   * *Payload:* Raw stdout/stderr terminal output bytes.
9. **SyncPTYPriorities (`0x0009` - Client → Server):**
   * *Terminal ID:* Set to `0` (ignored by server).
   * *Payload:* Contiguous list of 3-byte blocks. Each block is: `[Terminal ID: 2B (uint16)]` + `[Priority State: 1B]` (where `0x00` = Low/Hidden, `0x01` = High/Focused).
10. **ResetWorkspace (`0x000a` - Bidirectional):**
    * *Terminal ID:* Set to `0` (ignored by recipient).
    * *Payload:* Empty.
    * *Behavior:*
      * **Client → Server:** Requests a clean slate wipe of all process tree registries, active PTY buffers, and active spawning context cancellations.
      * **Server → Client:** Broadcasts workspace reset status, notifying the client core to purge all layout structures, active tab mappings, and terminal pane states.

---

## 7. Saga Orchestration Flows

The Redux-Saga layer operates as the network and side-effect supervisor. It coordinates four concurrent loops:

### A. Connection Lifecycle Loop
* Watches for `WS_CONNECT` actions.
* Establishes a WebSocket connection and wraps it in a saga event channel to convert socket callback events (open, message, error, close) into Redux actions.
* Monitors for socket termination, triggering `WS_DISCONNECTED` and cleaning up background loops.

### B. Inbound Reader Loop
* Continuously drains incoming frames from the Event Channel.
* Parses headers and decodes payloads based on Action ID:
  * For spawn status (`0x0002`), dispatches success or failure.
  * For text stream output (`0x0008`), decodes binary data into UTF-8 text and dispatches it to append to scrollbacks.
  * For exit notification (`0x0005`), updates the terminal status and registers its exit code.

### C. Outgoing Queue Coordinator
* Listens to a central outgoing action channel.
* Ensures sequential transmission of binary payloads over the active socket to prevent race conditions or payload corruption.

### D. Priority Sync Loop
* Triggered on terminal focus changes, spawns, and terminations.
* Scans the Redux store to identify which terminals are active and which one has user focus.
* Serializes a priority synchronization list containing all terminals and posts it to the outgoing write queue.

---

## 8. Layered Saga API Architecture

To ensure separation of concerns, the Saga API is structured into three distinct abstraction layers, exposed via a unified, flat method interface on the `store.api` object. Rather than forcing methods into nested objects, we partition the flat surface using naming suffixes to designate their level of abstraction and boundary responsibilities:

1. **Presentation Layer (UI View Shell):**
   * Acts as a passive view. Subscribes directly to the Redux store state to render terminals, tabs, and layout splits, and invokes functions on `store.api` in response to user actions.
2. **Composed Core Sagas (Internal Coordinators):**
   * Reactive background loops (such as the *Priority Synchronization Bridge*) that watch the local Redux store and orchestrate side effects. For example, when a layout shift occurs in the `__workspace` state, the saga reacts by compiling visible terminal listings and calling the `__server_command` priority sync API.
3. **Flat API Surface (`store.api`):**
   * **Workspace Layout partition (Suffix `__workspace`):** Handles local state transitions (e.g. creating tabs, focusing panes, splitting views). These are client-side operations that modify the local Redux layout tree.
   * **Proxy partition (Suffix `__proxy`):** Intercepts user typing and interactive inputs (e.g. `submitInput__proxy`) to pass them through the plugin transformation pipeline before submission.
   * **PTY Input partition (Suffix `__pty`):** Tailored, nuanced input utilities (e.g. keypress passthrough, input pacing, normalized clipboard pasting). These are client-side input coordinators.
   * **Plugin partition (Suffix `__plugin`):** Dynamic extension management and evaluation controls (e.g. `loadPlugin__plugin`, `removePlugin__plugin`, `transformProxyInput__plugin`).
   * **Server Socket partition (Suffix `__server`):** Low-level socket lifecycle (connecting and disconnecting WebSockets) and raw frame transmission primitives.
   * **Server Command partition (Suffix `__server_command`):** Outbound serialized wire action commands sent over the active connection.

---

## 9. Server & Network API Layer (Suffixes `__server` & `__server_command`)

The Server & Network API Layer acts as a stateless reflection of WebSocket connections and server wire capabilities. To isolate these remote side-effects, socket control methods are suffixed with `__server`, and protocol wire commands are suffixed with `__server_command` on the flat `store.api` surface.

### Connection & Socket Commands (Suffix `__server`)
* **`store.api.connectWebsocket__server({ host: string, token: string })`**
  * *Behavior:* Sets the network status to `connecting`, establishes the raw WebSocket connection, and registers the inbound event channel loops.
* **`store.api.disconnectWebsocket__server()`**
  * *Behavior:* Closes the raw WebSocket, dispatches `WS_DISCONNECTED`, and tears down background connection channels.
* **`store.api.sendBinaryFrame__server(action: number, terminalId: number, payload?: Uint8Array)`**
  * *Behavior:* Packages the action and terminal ID into a Big-Endian header, appends the payload bytes, and writes the frame directly to the outbound socket queue.

### Outbound Server Wire Commands (Suffix `__server_command`)
* **`store.api.spawnPTY__server_command(terminalId: number, cols: number, rows: number, command: string, args: string[])`**
  * *Wire Action:* Sends a `SpawnPTY` (`0x0001`) frame using `sendBinaryFrame__server`. The payload layout consists of:
    * `columns`: 2 bytes (Big-Endian uint16)
    * `rows`: 2 bytes (Big-Endian uint16)
    * `commandLength`: 2 bytes (Big-Endian uint16)
    * `command`: `commandLength` bytes (UTF-8 string)
    * `argCount`: 1 byte (uint8)
    * Followed by `argCount` iterations of:
      * `argLength`: 2 bytes (Big-Endian uint16)
      * `arg`: `argLength` bytes (UTF-8 string)
* **`store.api.resizePTY__server_command(terminalId: number, cols: number, rows: number)`**
  * *Wire Action:* Sends a `ResizePTY` (`0x0003`) frame using `sendBinaryFrame__server`.
* **`store.api.killPTY__server_command(terminalId: number)`**
  * *Wire Action:* Sends a `KillPTY` (`0x0004`) frame using `sendBinaryFrame__server`.
* **`store.api.removePTY__server_command(terminalId: number)`**
  * *Wire Action:* Sends a `RemovePTY` (`0x0006`) frame using `sendBinaryFrame__server`.
* **`store.api.resetWorkspace__server_command()`**
  * *Wire Action:* Sends a `ResetWorkspace` (`0x000a`) frame using `sendBinaryFrame__server`.
* **`store.api.inputPTY__server_command(terminalId: number, data: Uint8Array)`**
  * *Wire Action:* Sends an `InputPTY` (`0x0007`) frame containing the raw input bytes using `sendBinaryFrame__server`.
* **`store.api.syncPTYPriorities__server_command(priorities: Array<{ id: number, priority: number }>)`**
  * *Wire Action:* Sends a `SyncPTYPriorities` (`0x0009`) frame containing terminal priority states using `sendBinaryFrame__server`.

### Inbound Server Events
Translated directly by the socket reader loop into unopinionated dispatches:
* **SpawnPTYStatus (`0x0002`):** Dispatches `SERVER_PTY_SPAWN_STATUS` (payload: `[statusByte: 1B]` where `0x00` = success, `0x01` = failure, `0x02` = spawning, `0x03` = canceled).
* **PTYTerminalExit (`0x0005`):** Dispatches `SERVER_PTY_TERMINATED` (payload: `[exitStatusByte: 1B]` representing exit status code).
* **OutputPTY (`0x0008`):** Dispatches `SERVER_PTY_STREAM_IO` (payload: raw stdout/stderr bytes, converted to string chunk).
* **ResetWorkspace (`0x000a`):** Dispatches `SERVER_WORKSPACE_RESET` (payload: empty).

---

## 10. Workspace Layout Layer (Suffix `__workspace`)

The Workspace Layout Layer manages local, client-side session states (tabs, split panes, and layout geometries). These methods do not execute network calls directly. They are suffixed with `__workspace` on the flat `store.api` surface.

* **`store.api.createTab__workspace({ tabId: string, title: string, cols: number, rows: number, terminalId: number })`**
  * *Behavior:* Dispatches a `CREATE_TAB` action. The reducer allocates a new tab entry, default layout node, parent pane, and maps the pane to `terminalId`.
* **`store.api.closeTab__workspace({ tabId: string })`**
  * *Behavior:* Dispatches a `CLOSE_TAB` action. Purges the tab layout registry and its child panes from the Redux store.
* **`store.api.splitPane__workspace({ parentPaneId: string, newPaneId: string, orientation: 'horizontal' | 'vertical', cols: number, rows: number, newTerminalId: number })`**
  * *Behavior:* Dispatches a `SPLIT_PANE` action. Updates the tab layout node and maps the new split pane to `newTerminalId`.
* **`store.api.closePane__workspace({ paneId: string })`**
  * *Behavior:* Dispatches a `CLOSE_PANE` action. Removes the pane and updates the remaining sibling node sizes.
* **`store.api.selectTab__workspace({ tabId: string })`**
  * *Behavior:* Dispatches a `SET_ACTIVE_TAB` action, switching the visible tab view and setting focus to the tab's active pane.
* **`store.api.selectPane__workspace({ paneId: string })`**
  * *Behavior:* Dispatches a `SET_ACTIVE_PANE` action, targeting user keyboard input focus to the selected pane.
* **`store.api.resetWorkspace__workspace()`**
  * *Behavior:* Dispatches a `RESET_WORKSPACE` action, wiping all local tab definitions, split geometries, active focus selections, and active terminal mappings back to a clean state.

---

## 11. Proxy API Layer (Suffix `__proxy`)

The Proxy API Layer intercepts user-interactive inputs (like typing or manual pastes) to pass them through the plugin transformation pipeline before submitting them to the terminal. All methods in this partition are suffixed with `__proxy` on the flat `store.api` surface.

* **`store.api.submitInput__proxy({ terminalId: number, data: string | Uint8Array })`**
  * *Behavior:*
    1. Intercepts the user input. Converts the input payload to a string if needed.
    2. Invokes `store.api.transformProxyInput__plugin({ input: string })` to execute the sequential plugin pipeline.
    3. Retrieves the finalized transformed string from the Redux store state.
    4. Converts the transformed string to UTF-8 bytes.
    5. Submits the bytes to the PTY input layer (calling `passthroughInput__pty` for short keystrokes or `paceInput__pty` for large pastes).

---

## 12. PTY Input API Layer (Suffix `__pty`)

The PTY Input API Layer provides raw, flexible, and un-proxied methods for transmitting keyboard keystrokes, pasted texts, and stream inputs directly to individual PTY instances. These methods bypass the plugin transformation pipeline. All methods in this partition are suffixed with `__pty` on the flat `store.api` surface.

### Nuanced PTY Input Commands
* **`store.api.passthroughInput__pty({ terminalId: number, data: Uint8Array })`**
  * *Behavior:* Raw keyboard passthrough. Writes raw keystroke bytes directly to `store.api.sendPTYInput__server_command` without buffering, parsing, or pacing. Ideal for low-latency interactive keystrokes.
* **`store.api.paceInput__pty({ terminalId: number, data: Uint8Array, chunkSize?: number, intervalMs?: number })`**
  * *Behavior:* Scheduled paced feeder. Splits the byte array into small packets and schedules their transmission to the server at metered intervals to prevent overflowing PTY process input buffers.
* **`store.api.pasteInput__pty({ terminalId: number, text: string })`**
  * *Behavior:* Normalizes line endings (CRLF to CR), encodes the string to UTF-8 bytes, and invokes `paceInput__pty` with safe chunk pacing to guarantee all characters are absorbed reliably.

### Independence Invariant
None of the `__pty` input methods query or modify layout structures like `activeTabId` or `activePaneId`. They operate strictly on the target PTY `terminalId`, allowing automated command scripts, background copy-pastes, or background plugins to run concurrently on hidden terminals without altering the user's active UI view selection.

---

## 13. Plugin API Layer (Suffix `__plugin`)

The Plugin API Layer provides mechanisms to dynamically load, configure, and unload client-side extensions, and execute transformation evaluations. All methods in this partition are suffixed with `__plugin` on the flat `store.api` surface.

* **`store.api.loadPlugin__plugin({ pluginId: string, entrypoint: string, config?: object })`**
  * *Behavior:* Dispatches a `LOAD_PLUGIN` action. The core fetches and evaluates the plugin script bundle from the specified `entrypoint` URL. The plugin registers its transformer callback function globally into the core's active transformer pipeline list, and transitions its status to `active` (`LOAD_PLUGIN_SUCCESS`) or `error` (`LOAD_PLUGIN_FAILURE`).
* **`store.api.removePlugin__plugin({ pluginId: string })`**
  * *Behavior:* Dispatches a `REMOVE_PLUGIN` action. Gracefully purges the plugin's registered transform callback function from the active transformer pipeline list, ensuring no residual callbacks execute.
* **`store.api.transformProxyInput__plugin({ input: string })`**
  * *Behavior:*
    1. Dispatches `TRANSFORM_PROXY_INPUT`.
    2. Evaluates the raw input string against the global **Universal Trigger Patterns**.
    3. If a pattern matches, it passes the input sequentially through all registered active plugin callbacks:
       `finalInput = PluginC.transform(PluginB.transform(PluginA.transform(input)))`
    4. Dispatches `TRANSFORM_PROXY_INPUT_SUCCESS` with the finalized string, committing it to the Redux store state. If no pattern matches, dispatches success with the unmodified raw input.

---

## 14. Reactive Core Coordinator Sagas

Sagas in this layer are internal background processes that coordinate and bridge states reactively in response to Redux actions.

### Priority Synchronization Bridge (Saga)
A reactive background Saga bridges layout states to server priorities:
1. **Trigger:** Listens to layout mutation actions (`SET_ACTIVE_TAB`, `SET_ACTIVE_PANE`, `CREATE_TAB`, `CLOSE_TAB`, `SPLIT_PANE`, `CLOSE_PANE`).
2. **Analysis:** Queries the Redux store state. Collects all terminal PTY IDs belonging to panes in the active tab (visible, priority `0x01`) and all terminal PTY IDs in background tabs (hidden, priority `0x00`).
3. **Synchronization:** Invokes `store.api.syncPTYPriorities__server_command(compiledPriorities)` to transmit updates to the server registry.

### PTY Lifecycle Synchronization Bridge (Saga)
A reactive background Saga coordinates the allocation and cleanup of remote server PTY resources in response to local UI layout mutations:
1. **Resource Allocation (Spawning):**
   * When a new tab/pane is created mapping to a terminal ID, the saga intercepts `SPAWN_TERMINAL_REQUEST` (triggered by the UI calling `spawnPTY__server_command`), sets the local state status to `spawning`, and streams the binary spawn frame to the network.
2. **Resource Cleanup (Deletions):**
   * When a tab (`CLOSE_TAB`) or pane (`CLOSE_PANE`) is closed, this Saga extracts the associated numeric `terminalId`(s) that were removed from the Redux layout tree.
   * For each removed terminal ID, the Saga invokes `store.api.removePTY__server_command(terminalId)`, transmitting a `RemovePTY` (`0x0006`) frame to free matching process descriptors, FDs, and ring buffers on the server.
3. **Offline Reconciler (Reconnection):**
   * Upon WebSocket reconnection (`WS_CONNECTED`), this Saga halts new input submissions and awaits the server's handshake state replays.
   * Once replays are consumed, it compares the local layout's mapped terminal IDs against the server's reported active terminals. If a mapped terminal was reaped or lost on the server during the offline window, the Saga flags the corresponding pane as inactive/dead to prevent orphaned writes.

---

## 15. Flow Control & Performance Invariants

* **Write Serialization:** Sagas must never write to the raw WebSocket directly from arbitrary tasks. All traffic must be enqueued through the Outgoing Queue Coordinator to maintain strict packet sequencing.
* **Scrollback Buffer Management:** Scrollbacks stored in Redux are capped at a maximum character or line limit. When buffer limits are reached, the core drops the oldest entries to prevent memory leaks during long-running sessions.
* **Batching Output Updates:** In high-throughput scenarios (e.g., `cat` on a large file or reconnection state replay), the core must throttle or batch output updates before pushing them to UI subscribers, preventing main-thread lockups.

---

## 16. Client-Server Interplay & Protocol Adaptation

To maintain a resilient terminal session across network instabilities and client restarts, the client core must adapt to the stateless protocols and resource limitations of the server. This section documents the natural-language coordination rules, timing behaviors, and recovery strategies governing client-server interplay.

### A. Connection Endpoint Queries
When establishing a connection, the client core must target the server's dedicated WebSocket handler. The server expects the WebSocket upgrade request on the `/ws` path, with the authentication token passed as a query string parameter. The client core must format the connection string exactly as:
`ws://<host>/ws?token=<token>` (or `wss://` for secure connections).

### B. Connection Handshake & Automated State Replay
The server operates under an automated state-replay model. The moment a client upgrades its WebSocket connection, the server immediately pushes two sets of frames for all active processes in that workspace without waiting for client requests:
1. An initial output stream frame (`0x0008`) containing the accumulated PTY ring buffer history (scrollback).
2. A status frame (`0x0002` with status `0x00`) confirming the terminal is active.

The client's ingress listener must be prepared to consume these replayed frames as soon as the connection succeeds.

### C. Client-Side Byte-Matching Reconstitution (Overlaps & Evictions)
Because the server replays the active PTY scrollbacks on every connection, a reconnecting client risks duplicating its visible history. However, because the server PTY ring buffer is finite (e.g., 256KB), simply clearing the client state to avoid duplication would wipe out older history that the client already loaded but the server has since evicted. 

To prevent both duplication and history loss, the client core implements a self-healing merge protocol when receiving the server's replayed scrollback buffer ($R$) during a reconnect:
* **Scenario 1: Overlapping Sequence Match:** The client scans backward from the tail of its existing scrollback ($C$) to find a matching byte sequence corresponding to the beginning of the replayed buffer ($R$). If a match is found at byte index $M$, the client discards the overlapping tail of its history ($C[M:]$) and appends the replayed buffer ($R$) in its entirety. This creates a seamless splice without breaking multi-byte UTF-8 characters or splitting ANSI escape sequences.
* **Scenario 2: Evicted History Gap:** If the client was disconnected too long and the server recycled its ring buffer, the beginning of the replayed buffer ($R$) will not match any part of the client's history. In this case, the client preserves its entire history ($C$), appends a visual warning marker to the screen indicating that a gap in history has occurred, and then appends the server's replayed buffer ($R$).

### D. Server-Retained Terminated PTY Registry
Rather than immediately reaping and discarding process metadata upon shell exit, the server maintains PTY instances in its registry in a `terminated` state (preserving their exit status codes and final scrollback buffers) until the client explicitly requests their removal.
* **Handshake Replay:** When a client reconnects, the server replays the status of both active PTYs and non-removed terminated PTYs (sending the exact process exit frame `0x0005` during the connection handshake). This allows the client to receive the exact process exit code without running verifying timers or inventing dummy exit statuses.
* **Client-Driven Cleanup:** The client is responsible for sending explicit terminal deletion commands (e.g. via `removePTY__server_command`) when the user closes a tab or pane in the UI layout. Upon receiving this command, the server purges the PTY instance and its buffers from memory.

### E. Persistent Workspace Lifecycle (Manual Registry Control)
The server does not implement any automatic background sweeper timers or session timeouts. When a client disconnects, all active shell processes, output queues, and terminated PTY descriptors are held in memory indefinitely.
* **Overnight Continuity:** This allows users to disconnect (e.g., closing a laptop at the end of the day) and reconnect the next morning to resume their work with all split panes, processes, and terminal output history exactly as they left them.
* **Manual Cleanup Authority:** The client is the sole authority for managing process lifecycles. Go memory and OS resources are only freed when the client sends explicit `RemovePTY` commands via `removePTY__server_command`.
* **Workspace-Level Reset Coordination:** To ensure client-side states and the server registry reset in a loop-free, convergent manner:
  * **Client-Initiated Reset Flow:**
    1. The user requests a reset in the UI.
    2. The UI invokes the local `store.api.resetWorkspace__workspace()`, which dispatches a `RESET_WORKSPACE_REQUEST` action.
    3. The client saga coordinator intercepts `RESET_WORKSPACE_REQUEST`, immediately dispatching a local `RESET_WORKSPACE` (clearing Redux tab, pane, and active focus state trees) and invoking `store.api.resetWorkspace__server_command()` to transmit the `ResetWorkspace` (`0x000a`) frame to the server.
    4. The server kills all running shell processes, deletes all PTY entries, and broadcasts the `ResetWorkspace` (`0x000a`) status frame back over the active socket.
    5. The client's inbound socket reader intercepts the `0x000a` frame, dispatching `SERVER_WORKSPACE_RESET`.
    6. **Inbound Loop-Prevention Rule:** When the client saga coordinator intercepts `SERVER_WORKSPACE_RESET`, it must perform *only* local state mutation/verification and **must never** emit a secondary outbound `resetWorkspace__server_command()` back to the network.
  * **Remote-Initiated Reset Flow (Multi-Client Convergence):**
    1. If another client session resets the workspace, the server broadcasts `ResetWorkspace` (`0x000a`) to all connected client sockets.
    2. Any receiving client's socket reader dispatches `SERVER_WORKSPACE_RESET`.
    3. The saga coordinator catches `SERVER_WORKSPACE_RESET`, executing a local layout reset to align the UI with the empty server state, without sending any command back to the server.

### F. Protocol Write Limits
The server enforces a strict message read limit of 64KB per binary frame. If the client writes a frame exceeding this size, the server triggers a protocol error and closes the socket. 
To prevent socket drops, the client core's pacing and submission layers must slice large copy-pastes into chunks strictly smaller than 64KB (such as 4KB packets) and feed them sequentially through the pacing interval loops.

### G. Spawning & Terminated State Input Guards
To preserve strict process consistency, the server enforces input and layout safety guards based on the PTY instance's state:
* **Spawning State Discards:** While a terminal is in the background spawning phase (after a `SpawnPTY` request is received but before `SpawnPTYStatus` is transmitted), any client `InputPTY` (`0x0007`) or `ResizePTY` (`0x0003`) frames targeting that terminal ID are immediately discarded by the server.
* **Terminated State Discards:** Once a PTY exits and transitions to the `Terminated` state, the server ignores and discards any subsequent client `InputPTY` (`0x0007`) or `ResizePTY` (`0x0003`) requests for that terminal ID.

### H. Out-of-Band Lifecycle Dispatching
The server groups outbound frames into separate priority scheduling queues:
* Critical process lifecycle frames—namely `SpawnPTYStatus` (`0x0002`) and `PTYTerminalExit` (`0x0005`)—are routed through the dedicated **Control Queue**.
* Data streaming stdout/stderr frames (`0x0008`) are routed through **High** or **Low** priority queues based on layout focus.
Because Control frames bypass the priority pacing scheduler entirely, client ingress loops are guaranteed to receive process status and termination notifications immediately, ensuring they are never starved by heavy data output streams on other active shell sessions.

### I. Connection Hijack/Takeover Race Protection
To handle cases where multiple connection upgrades occur in quick succession under high network latency, the server uses a double-checked registry lock during takeovers:
* Before binding a connection as the workspace's active socket writer, the server locks the global connection registry to verify the connection is still the registered active singleton.
* If a connection was evicted by a newer upgrade event before it can bind, the takeover attempt is ignored, and the socket is closed.
* This ensures that `ClearSocketWriter` cleanup calls from evicted read loops never clear the active socket writer of newer connections.

### J. Optimistic Local State Transitions vs. Asynchronous Network Synchronization
The Client Core balances zero-latency UI responsiveness with network consistency by partition-based design rules:
* **Optimistic Layout State Transitions (UI responsive):**
  * All layout mutations (`createTab__workspace`, `closeTab__workspace`, `splitPane__workspace`, `closePane__workspace`, `selectTab__workspace`, `selectPane__workspace`, and `resetWorkspace__workspace`) are executed **preemptively and optimistically** within local client reducers.
  * Local Redux layout structures are updated immediately when the action is dispatched. This guarantees that UI rendering responds instantly without awaiting a round-trip network acknowledgment.
  * Background coordinator sagas subsequently detect these layout changes and issue the necessary server commands (e.g. `removePTY` or `syncPTYPriorities`) to align the server with the new layout state.
* **Asynchronous Process State Synchronization (Server-driven):**
  * Remote process lifecycle states (such as active PTY spawning and process terminations) are **server-driven and asynchronous**.
  * When a command is sent, the client core moves the local PTY descriptor to an intermediate state (e.g. `spawning`) and updates the UI (e.g. showing a loading spinner).
  * The client waits for the corresponding event frames (`SpawnPTYStatus` or `PTYTerminalExit`) from the server before finalizing local state updates (e.g. transitioning the status to `active` or `terminated` and mounting/unmounting xterm.js renderers).
* **Workspace-Level Reset Convergence:**
  * When the workspace is reset, the client immediately wipes its local layout state optimistically to return to the welcome screen. It then sends `0x000a` to the server. The server's broadcasted response `0x000a` acts as the eventual synchronization checkpoint.


