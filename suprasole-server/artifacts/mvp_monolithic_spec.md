# File: mvp_spec.md

# Architectural Specification: Stateful Multiplexed Remote Terminal Server (MVP)

This specification defines the black-box functional requirements, wire protocol, state machine, and traffic coordination strategies for a centralized, session-persistent PTY proxy server.

---

## 1. Wire Protocol Specification (Polymorphic Framing)

The application communicates exclusively via binary frames over a single, persistent WebSocket connection. Every network packet leads with a fixed **2-byte Action ID** (`uint16`, Big-Endian). The internal structure of the remaining payload changes dynamically based on its semantic group.

### Group A: PTY Lifecycle Actions

These actions manage the physical allocation, structural layout sizing, and operational existence of a pseudo-terminal instance on the host operating system.

| Action ID | Name | Direction | Scope | Payload Binary Layout |
| --- | --- | --- | --- | --- |
| **`0x0001`** | **PTY Spawn Request** | Client → Server | Global | `[Terminal ID: 2B]` + `[Cols: 2B]` + `[Rows: 2B]` |
| **`0x0002`** | **PTY Spawn Response** | Server → Client | Terminal | `[Terminal ID: 2B]` + `[Status Byte: 1B]` <br>

<br> *(`0x00` = Success, `0x01` = Failure)* |
| **`0x0003`** | **PTY Resize** | Client → Server | Terminal | `[Terminal ID: 2B]` + `[Cols: 2B]` + `[Rows: 2B]` |
| **`0x0004`** | **PTY Lifecycle Termination** | Bidirectional | Terminal | `[Terminal ID: 2B]` + `[Exit Code Byte: 1B]` |

### Group B: PTY Data Actions

This group facilitates high-frequency, asynchronous streaming of input and output characters through the channel.

| Action ID | Name | Direction | Scope | Payload Binary Layout |
| --- | --- | --- | --- | --- |
| **`0x0005`** | **PTY Stream I/O** | Bidirectional | Terminal | `[Terminal ID: 2B]` + `[Raw ASCII/UTF-8 Text Stream: Variable Length]` |

### Group C: PTY Workspace Layout Actions

These actions act as administrative hints to optimize server resources and traffic streams based on how the client is visually interacting with the interface.

| Action ID | Name | Direction | Scope | Payload Binary Layout |
| --- | --- | --- | --- | --- |
| **`0x0006`** | **PTY Visibility Shift** | Client → Server | Global | `[Terminal ID: 2B]` + `[Visibility State Byte: 1B]` <br>

<br> *(`0x02` = Focused, `0x01` = Visible, `0x00` = Hidden)* |

> ### 📋 Transactional Invariant
> 
> 
> Only `0x0001 (PTY Spawn Request)` triggers an explicit, blocking transaction loop that expects a `0x0002 (PTY Spawn Response)` counterpart. Actions `0x0003`, `0x0005`, and `0x0006` operate strictly as fire-and-forget streams, relying on the underlying TCP transport layer to guarantee structural ordering and delivery without application-level overhead. Action `0x0004` acts as a terminal notification event that tears down its associated channel routing when caught by either side.

---

## 2. State Machine & Lifecycle Management

The server acts as a stateful coordinator tracking multiple isolated **Workspaces**, each containing $N$ independent **PTY Instances**.

### A. Workspace Isolation

* **Initialization:** Connections must present a unique session identifier (e.g., an authentication token or workspace UUID) via URL query parameters.
* **Multi-Tenancy Bound:** PTY processes belong strictly to their parent workspace. Cross-workspace visibility or control is explicitly prohibited.
* **Orphan State:** If the network connection drops, the workspace transitions to an *Orphaned* state. Underlying PTY processes must continue executing uninterrupted.

### B. PTY Invocation Invariants

When a `0x0001 (PTY Spawn Request)` is parsed, the server must execute the underlying target binary (e.g., `/bin/bash`) under the following strict configuration constraints:

1. **Environment Seeding:** The execution context must explicitly inject `TERM=xterm-256color` and `LANG=en_US.UTF-8` into the process environment strings to ensure correct ANSI sequence evaluation by underlying software.
2. **Geometry Seeding:** The OS pseudo-terminal must be structurally configured with the client's transmitted `Cols` and `Rows` dimensions *before* the shell process starts execution.
3. **Unique Tracking:** The server must map the client-provided `Terminal ID` directly to the resulting PTY Master file descriptor and Process ID (PID).

### C. Termination & Child Process Reaping

When a child shell process exits (e.g., the user runs `exit`, or a shell error triggers a hangup):

1. The reader thread for that specific file descriptor will intercept an `EOF` or an I/O system error.
2. The server must immediately broadcast a `0x0004 (PTY Lifecycle Termination)` packet to the client.
3. The file descriptor must be explicitly closed, and the terminal session instance purged from the workspace memory map.
4. **Zombie Prevention:** The server must invoke appropriate wait routines (`syscall.Wait4` or equivalent process tracking handles) to cleanly release the dead PID back to the host operating system.

---

## 3. Session Persistence & Reconnection Mechanics

The system must handle abrupt network disconnections (laptop lid closures, IP changes, cell tower handoffs) without losing terminal state or historical shell outputs.

```
                  ┌──────────────────────────────┐
                  │      Network Connected       │
                  └──────────────┬───────────────┘
                                 │
                     [WebSocket Connection Drops]
                                 │
                                 ▼
                  ┌──────────────────────────────┐
                  │      Workspace Orphaned      │◄────────┐ [PTY Continues Execution]
                  └──────────────┬───────────────┘         │ [Output Appends to Scrollback]
                                 │                         └───────┘
                      [Grace Period Expires?]
                                 │
                    ┌────────────┴────────────┐
                    │YES                      │NO
                    ▼                         ▼
        ┌───────────────────────┐ ┌───────────────────────┐
        │  Teardown Workspace   │ │  Client Reconnects   │
        │   [Kill All PTYs]     │ │  [Replay Backlog]   │
        └───────────────────────┘ └───────────────────────┘

```

### A. The Scrollback Memory Buffer (Ring Buffer)

* Each PTY instance must maintain an isolated in-memory FIFO ring buffer dedicated to caching `stdout`/`stderr` text output.
* The cache capacity must be bounded by a hard limit (e.g., 256KB per terminal) to guarantee absolute protection against server memory exhaustion. Older bytes must be instantly evicted as new data arrives.
* **Always-On Ingestion:** The server must continuously drain output from every active PTY file descriptor into its local ring buffer, regardless of whether a client WebSocket is currently connected or offline.
* **Truncation Demarcation:** If an eviction event occurs (the text stream wraps past the cache ceiling), the server must set a persistent `IsTruncated` boolean flag on that PTY instance.

### B. The Reconnection Handshake Sequence

When an orphaned workspace receives a fresh WebSocket connection matching a known valid session token:

1. **Socket Hijacking:** The server must immediately bind the fresh socket reference to the workspace, securely severing and closing any dangling, dead network socket handles from the prior session.
2. **State Replay:** Before opening the channel to interactive live traffic, the server must iterate through every active PTY instance within that workspace and transmit its entire stored scrollback memory buffer down the pipe using Action `0x0005`.
3. **Continuity Injection:** If a PTY instance has its `IsTruncated` flag set to `true`, the server must prepend a standardized, visible warning line to the very beginning of the replayed binary text stream payload:
```text
\r\n\x1b[33m[... Output truncated due to buffer overflow ...]\x1b[0m\r\n\r\n

```


This explicit ANSI-escaped boundary preserves temporal continuity, ensuring the user immediately notes the structural break in historical output.
4. **Visual Synchronization:** This sequence ensures that the client terminal views instantly repaint their exact history state, appearing seamless and uninterrupted to the end-user.

### C. The Cleanup Sweeper

* Upon transitioning to an *Orphaned* state, a configurable countdown timer (e.g., 5 minutes) must bind to the workspace.
* If the client successfully reconnects before the timer expires, the countdown is canceled.
* If the timer reaches zero, the server must loop through all terminal assets in that workspace, issue an explicit termination signal (`SIGKILL`) to their process groups, close all open file descriptors, and remove the workspace from global memory tracking.

---

## 4. 3-Tier Priority Scheduler & Traffic Coordination

To eliminate the risk of Head-of-Line (HoL) blocking—where a hidden background tab introduces severe input and rendering latency to the user's active view—the server must manage output using a **Strict Three-Tier Priority Scheduler**.

### A. The In-Memory Layout Priority Matrix

Every PTY instance within a workspace is assigned one of three operational visibility classifications, updated dynamically via client visibility messages (Action `0x0006`):

* **Priority 1: Focused (`0x02`):** The exact terminal tab the user is actively viewing and typing inside. (Max 1 instance per client viewport window).
* **Priority 2: Visible (`0x01`):** Terminal windows rendered alongside the focused window (e.g., split panes or tiled dashboard arrangements) that are visible but not currently capturing keyboard focus.
* **Priority 3: Hidden (`0x00`):** Terminals assigned to closed, inactive tabs or background views that are entirely hidden from the user's view.

### B. The Multiplexed Output Scheduler

Instead of asynchronous background threads writing output directly into the WebSocket connection, data must pass through structured priority lanes managed by a centralized scheduler thread:

1. **Draining Rule:** The scheduler must run a continuous verification loop. It is permitted to read and transmit a data packet from the Priority 2 (`Visible`) queue **only if** the Priority 1 (`Focused`) queue is completely empty.
2. **Background Starvation:** The scheduler is permitted to read and transmit a data packet from the Priority 3 (`Hidden`) queue **only if** both the Priority 1 and Priority 2 queues are completely empty.
3. **Atomic Write Invariant:** The scheduler must complete writing an entire frame packet to the underlying network socket before fetching the next message. Interleaving bytes from distinct terminal streams within a single frame packet is strictly prohibited.

---

## 5. Crucial Defensiveness & Edge-Case Constraints

### A. Backpressure Strategy (The Log-Bomb Safeguard)

If a background process executes a command that dumps an infinite loop of text (e.g., `yes` or `cat /dev/urandom`), it can overwhelm the scheduler queues.

* **Bounded Queues:** The internal channels or memory pipelines feeding into the Priority Scheduler must be strictly bounded.
* **Drop-Oldest Eviction:** If a background PTY queue (Priority 3) hits its ceiling, the server must discard the oldest chunk of text from that queue to make room for the latest process output. It must *never* block the PTY execution loop or allow memory allocations to grow unconstrained.

### B. Network Heartbeats (Idle Proxy Eviction Safeguard)

Many production cloud load balancers forcefully terminate stateful WebSocket channels if no data travels across the pipe for 60 seconds.

* **Keep-Alives:** The server must maintain a background heartbeat thread that issues standard, low-level WebSocket `Ping` frames to the client every 30 seconds.
* **Liveness Detection:** If the client fails to reply with a corresponding native network `Pong` within a 10-second window, the server must treat the connection as broken, terminate the socket immediately, and shift the workspace into the stateful *Orphaned* mode.

### C. Atomic Resize Invariant

Window resizes are asynchronous and frequent as users manipulate browser boundaries.

* When a `0x0003 (PTY Resize)` packet is handled, the server must block concurrent read/write operations on that specific PTY instance momentarily while executing the platform-level system call (`ioctl` with `TIOCSWINSZ`). This prevents structural race conditions where applications try to draw text characters based on old column boundaries right as the window geometry changes.


# File: 01_core_architecture_lifecycle.md

# MVP Sub-Spec 01: Core Architecture & PTY Lifecycle

This sub-specification defines the foundation of the Stateful Multiplexed Remote Terminal Server. It covers the core data models, thread safety, process isolation, execution environments, and process reaping.

Reference: [mvp_spec.md](file:///home/coder/project/suprasole-server/meta/mvp_spec.md#L49-L76)

---

## 1. Core Data Models & Workspace Isolation

The server acts as a stateful coordinator tracking multiple isolated **Workspaces**, each containing $N$ independent **PTY Instances**.

* **Workspace Registry:** A thread-safe global registry (e.g., a synchronized map with a read/write mutex) must track active workspace structures keyed by session identifiers (authentication token or workspace UUID).
* **Workspace Lifecycle State:** A workspace transitions between three state phases:
  * `Active`: Client WebSocket is connected.
  * `Orphaned`: Client connection lost; countdown timer active.
  * `TornDown`: Swapper triggered; workspace is deleted.
* **PTY Registry Map:** Each workspace must maintain a thread-safe registry map of `Terminal ID` (uint16) to the PTY instance metadata (file descriptor, process handle/PID, state).
* **Lock Hierarchy:** To prevent deadlocks across concurrent threads (WebSocket reader, PTY readers, scheduler, reaper), the following strict lock acquisition order must be maintained:
  1. Global Registry Lock
  2. Workspace State Lock
  3. PTY Instance Lock

---

## 2. PTY Invocation Invariants & Unix System Nuances

When a `0x0001 (PTY Spawn Request)` is handled, the server must spawn a target executable directly under the PTY using system-level PTY allocation calls (`posix_openpt`, `ptsname`, `unlockpt`, `grantpt` or native OS wrappers).

### Direct Executable Execution (No Shell Wrapper)
* **Direct Execution:** The target executable is spawned directly as the PTY child process. The server **must not** wrap the executable in a shell (e.g., running `/bin/sh -c "<binary>"`), as this adds unnecessary process overhead and introduces shell-escape parsing hazards.
* **Target Resolution:** Since the `0x0001` wire protocol payload does not transmit a path or command string, the target executable to launch is determined by the server's local configuration (e.g., defaulting to `/bin/bash`, `/bin/sh`, or a specific configured utility like `/usr/bin/htop`).
* **Strict Invariants:** Regardless of whether the target is an interactive shell or a direct tool (like `vim` or `htop`), the execution must respect the following system invariants:

### A. Controlling Terminal (`TIOCSCTTY`) & Session ID (`setsid`)
* **Invariant:** The spawned child process must execute `setsid()` to initiate a new session group leader, and then call `ioctl(slave_fd, TIOCSCTTY, 0)` on the PTY Slave descriptor to establish it as its controlling terminal.
* **Reasoning:** Without a controlling terminal, applications inside the terminal (such as `sudo`, `ssh`, or Unix job control like `Ctrl+Z` / `fg`) will fail to open `/dev/tty` and terminate or behave incorrectly.

### B. Parent-Side Slave Descriptor Closure (The EOF Bug Safeguard)
* **Invariant:** Immediately after successfully forks/spawning the child process, the parent server process **must close** its file descriptor handle to the PTY Slave.
* **Reasoning:** If the parent process keeps the PTY Slave descriptor open, the PTY Master descriptor will *never* receive an `EOF` (End of File) read error when the child process terminates. The kernel keeps the channel open as long as a reference to the slave exists.

### C. Environment & Path Seeding
* **Invariant:** The child process execution context must explicitly inject `TERM=xterm-256color` and `LANG=en_US.UTF-8` into the process environment strings.
* **Seeding Safeguard:** The server must ensure a valid `PATH` environment variable (e.g., `/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin`) is seeded. If the environment is completely cleared except for `TERM` and `LANG`, the child process will fail to locate basic command binaries.

### D. Geometry Seeding
* **Invariant:** The OS pseudo-terminal must be structurally configured with the client's transmitted `Cols` and `Rows` dimensions (via `ioctl(slave_fd, TIOCSWINSZ, ...)` or equivalent) *before* the child process starts execution.

### E. Handles Mapping
* **Invariant:** The server must map the client-provided `Terminal ID` directly to the resulting PTY Master file descriptor and Process ID (PID).

---

## 3. Termination & Child Process Reaping

When a child process exits:

### A. EOF / Error Interception
* The reader thread/task running non-blocking or asynchronous I/O on the PTY Master file descriptor will read an `EOF` or an I/O error (e.g., `EIO` on Linux).
* This event must trigger the immediate cleanup sequence.

### B. Purging & Notification
* The server must immediately broadcast a `0x0004 (PTY Lifecycle Termination)` packet containing the `Terminal ID` and the calculated exit status byte to the client.
* The PTY Master file descriptor must be explicitly closed.
* The PTY Instance must be safely removed from the Workspace's registry map under the workspace lock.

### C. Zombie Prevention (Wait Handles)
* **Invariant:** The server must invoke appropriate wait routines (`syscall.Wait4` or platform equivalents like `waitpid`) to reap the dead PID.
* **Exit Status Encoding:** The exit status byte in the `0x0004` frame must be calculated as follows:
  * If the process exited normally: Use the exit status code (e.g., `status.ExitStatus()`).
  * If the process was terminated by a signal: Use `128 + signal_number` (conforming to Unix conventions).

---

## 4. Required Logical Interface Contracts (Decoupling Boundary)

To prevent implementation regression and ensure decoupled layers, the Core module must expose logical programmatic contracts to the Network Layer (`02_network_protocol`) and future modules. These contracts define operations and events conceptually without binding to a specific programming language.

### A. Global Workspace Registry Contract

The registry is a thread-safe coordinator managing tenant workspaces:

* **Retrieve/Initialize Workspace:**
  * *Inputs:* `Workspace ID` (String)
  * *Outputs:* Reference/Handle to the `Workspace`
  * *Behavior:* Look up the workspace. If it does not exist, initialize its resources and memory state.
* **Teardown Workspace:**
  * *Inputs:* `Workspace ID` (String)
  * *Behavior:* Discards the workspace state and releases its tracked resources.

### B. Workspace Operations Contract

A `Workspace` encapsulates a collection of running terminal processes:

* **Spawn Pseudo-Terminal:**
  * *Inputs:* `Terminal ID` (uint16), Initial `Cols` (uint16), Initial `Rows` (uint16)
  * *Behavior:* Spawns the default target executable directly under a PTY master/slave pair, configuring the initial dimensions and environment variables.
* **Resize Pseudo-Terminal:**
  * *Inputs:* `Terminal ID` (uint16), New `Cols` (uint16), New `Rows` (uint16)
  * *Behavior:* Invokes the platform-level window resize system call (`ioctl` with `TIOCSWINSZ`) to update the terminal geometry.
* **Write Terminal Input:**
  * *Inputs:* `Terminal ID` (uint16), Raw Input `Data` (Binary/Bytes)
  * *Behavior:* Writes the input bytes directly to the PTY stdin pipe.
* **Terminate Process:**
  * *Inputs:* `Terminal ID` (uint16)
  * *Behavior:* Issues an explicit termination signal (`SIGKILL` or equivalent) to the PTY process group.
* **Retrieve Scrollback Buffer:**
  * *Inputs:* `Terminal ID` (uint16)
  * *Outputs:* Raw Scrollback `Data` (Binary/Bytes), `IsTruncated` (Boolean)
  * *Behavior:* Returns the in-memory buffered history of output characters for that terminal and a flag indicating if older data was evicted (see Sub-Spec 03).
* **List Active Terminal IDs:**
  * *Outputs:* List of `Terminal IDs` (List of uint16)
  * *Behavior:* Returns the identifiers of all currently running PTYs in the workspace.
* **Set Terminal Visibility:**
  * *Inputs:* `Terminal ID` (uint16), `Visibility State` (Byte: Focused/Visible/Hidden)
  * *Behavior:* Updates the visual priority status of the terminal (used for scheduling output, see Sub-Spec 04).

### C. Workspace Event Contracts (Callbacks / Message Pipelines)

To notify external consumers of asynchronous process state changes, the Core module must publish the following events:

* **On Terminal Output:**
  * *Payload:* `Terminal ID` (uint16), Output `Data` (Binary/Bytes)
  * *Triggers when:* The Core's background reader loop drains new output characters from a PTY master file descriptor.
* **On Process Termination:**
  * *Payload:* `Terminal ID` (uint16), `Exit Status Byte` (Byte)
  * *Triggers when:* A PTY process exits, is killed, or encounters a read error, after the Core has completed PID reaping and closed descriptor resources.

### D. Encapsulation Rules (Decoupling Invariant)

1. **Network Independence:** The Core module must have **no** dependency on WebSockets, HTTP, or socket connections. It functions purely as a local process coordinator.
2. **File Descriptor Isolation:** The Network Layer and feature layers must **never** read from or write directly to raw PTY file descriptors. The Core maintains exclusive ownership of file reads/writes.
3. **Reaping Isolation:** Asynchronous process exit monitoring and PID harvesting are fully encapsulated within the Core module.


# File: 01_core_integration_test_spec.md

# Black-Box Integration Test Specification: Core PTY Architecture

This document defines the black-box integration test cases and assertions to validate the Stateful Multiplexed Remote Terminal Server Core (`Sub-Spec 01`). 

These tests evaluate process supervision, workspace tenant sandboxing, child process reaping, signal propagation, and Unix PTY behaviors extrinsically. They specify exact execution timelines and success thresholds while remaining highly realistic and pragmatic, avoiding paranoid testing setups that promote over-engineering.

---

## Test Case 1: Workspace Tenant Isolation & Sandbox Verification

### 1. Objective
Confirm that workspaces are fully isolated, sandboxed administrative units. A request or query from one workspace must never access, view, modify, or terminate terminal instances residing in another workspace.

### 2. Precise Input Parameters
* **Workspace A Identifier:** `WS-A-TOKEN-998`
* **Workspace B Identifier:** `WS-B-TOKEN-112`
* **Terminal ID A:** `100` (Spawned in Workspace A)
* **Initial Geometry:** `Cols = 80`, `Rows = 24`
* **Target Command:** `/bin/sh`

### 3. Step-by-Step Execution Sequence
1. Initialize both workspaces: `WS-A-TOKEN-998` and `WS-B-TOKEN-112` in the global registry.
2. Under `WS-A-TOKEN-998`, invoke `SpawnPTY` with `Terminal ID = 100`, `Cols = 80`, and `Rows = 24` running `/bin/sh`.
3. Register an output listener on `WS-A-TOKEN-998`.
4. Write input bytes `echo 'from-Workspace-A'\n` to `Terminal ID = 100` via `WS-A-TOKEN-998`. 
5. Wait for the output listener to capture and verify the echo response `from-Workspace-A`.
6. From the context of `WS-B-TOKEN-112`, perform the following actions sequentially:
   * **Action 1:** Invoke `WritePTYInput` to `Terminal ID = 100` with payload `echo 'from-Workspace-B'\n`.
   * **Action 2:** Invoke `ResizePTY` to `Terminal ID = 100` with dimensions `Cols = 120`, `Rows = 40`.
   * **Action 3:** Invoke `GetScrollbackBuffer` for `Terminal ID = 100`.
   * **Action 4:** Invoke `GetActiveTerminalIDs`.
   * **Action 5:** Invoke `TerminatePTY` for `Terminal ID = 100`.
7. Under `WS-A-TOKEN-998`, write input bytes `stty size\n` to `Terminal ID = 100` and read the output stream.

### 4. Assertions & Expected Predicates
* **Error Trigger Predicate:** Actions 1, 2, 3, and 5 executed via `WS-B-TOKEN-112` must fail immediately, returning a standard "Terminal ID 100 not found" error.
* **Invisible Registry Predicate:** Action 4 executed via `WS-B-TOKEN-112` must return an empty list `[]` (or must not include `100`).
* **Input Isolation Predicate:** The output stream captured by `WS-A-TOKEN-998` must **not** contain the substring `from-Workspace-B`.
* **Geometry Preservation Predicate:** The output of `stty size` in Step 7 must return exactly `24 80` (Rows, Columns). The dimensions must not have changed to `40 120`.
* **Process Liveness Predicate:** The process in `WS-A-TOKEN-998` must remain active and functional throughout the test.

---

## Test Case 2: Environment & Geometry Seeding Invariants

### 1. Objective
Ensure that PTY child processes are spawned with necessary variables and that physical geometry dimensions are fully applied to the PTY *before* the child executable begins drawing.

### 2. Precise Input Parameters
* **Workspace Identifier:** `WS-A`
* **Terminal ID:** `200`
* **Spawn Geometry:** `Cols = 132`, `Rows = 43`
* **Target Command:** `/bin/sh -c "env && stty size && echo 'SEEDING_COMPLETE'"`

### 3. Step-by-Step Execution Sequence
1. Under `WS-A`, invoke `SpawnPTY` with `Terminal ID = 200`, `Cols = 132`, and `Rows = 43` executing the target command.
2. Capture the complete, raw byte stream output from the `On Terminal Output` event handler.
3. Wait for the `On Process Termination` event to fire.

### 4. Assertions & Expected Predicates
* **Terminal Variable Predicate:** The captured output stream must contain the substring `TERM=xterm-256color\n` (or delimited by carriage returns).
* **Locale Variable Predicate:** The captured output stream must contain the substring `LANG=en_US.UTF-8\n` (or delimited by carriage returns).
* **Binaries Resolution Path Predicate:** The captured output stream must contain a non-empty `PATH` environment variable (e.g. `PATH=/usr/bin:...`), verifying the child did not inherit a completely blank env.
* **Geometry Invariant Predicate:** The output text generated by `stty size` must contain the exact string `43 132` (Rows, Columns) before the `SEEDING_COMPLETE` marker. 
* **Ordering Success:** If `stty size` returns default dimensions (like `24 80`), the test fails, proving geometry was set asynchronously after shell launch instead of before.

---

## Test Case 3: Controlling Terminal (`TIOCSCTTY`) Verification

### 1. Objective
Validate that the child process is spawned inside a new session group leader (`setsid`) and has its PTY Slave correctly bound as the controlling terminal (`TIOCSCTTY`), enabling full interactive job control.

### 2. Precise Input Parameters
* **Workspace Identifier:** `WS-A`
* **Terminal ID:** `300`
* **Spawn Geometry:** `Cols = 80`, `Rows = 24`
* **Target Command:** `/bin/sh`

### 3. Step-by-Step Execution Sequence
1. Under `WS-A`, spawn `Terminal ID = 300` with the target command.
2. Write raw input bytes to the PTY: `tty && echo "TTY_STATUS_$?"\n`.
3. Write raw input bytes to verify openability of the controlling terminal device: `cat < /dev/tty && echo "DEV_TTY_OK"\n` but feed it an immediate EOF (e.g. by passing `exit\n` or sending `Ctrl+D`).
4. Read the captured output stream.

### 4. Assertions & Expected Predicates
* **Controlling TTY Route Predicate:** The output stream must contain a valid Unix pseudo-terminal file path (matching pattern `/dev/pts/[0-9]+`) and the success code string `TTY_STATUS_0`.
* **Dev Tty Access Predicate:** The output stream must contain the string `DEV_TTY_OK`. It must **not** contain any error message containing `No such device or address` or `Permission denied` (which indicates `/dev/tty` failed to open due to missing controlling terminal attributes).

---

## Test Case 4: Parent-Side Slave Close & Hanging EOF Prevention

### 1. Objective
Verify that the parent process closes its copy of the PTY Slave descriptor immediately upon spawn. This ensures the master descriptor intercepts process exit (`EOF`/`EIO` read error) without hanging.

### 2. Precise Input Parameters
* **Workspace Identifier:** `WS-A`
* **Terminal ID:** `400`
* **Target Command:** `/bin/sh -c "echo 'quick_exit' && exit 0"`
* **Watchdog Timeout:** 2000 milliseconds

### 3. Step-by-Step Execution Sequence
1. Register output and termination listeners for `WS-A`.
2. Record starting timestamp $T_{spawn}$.
3. Invoke `SpawnPTY` with `Terminal ID = 400` running the target command.
4. Record the timestamp $T_{output}$ when the output listener receives the string `quick_exit`.
5. Start a watchdog timer set to 2000ms.
6. Record the timestamp $T_{termination}$ when the `On Process Termination` event fires.

### 4. Assertions & Expected Predicates
* **Immediate Teardown Predicate:** The difference between termination and output timestamps must be near-instantaneous:
  $$T_{termination} - T_{output} < 100\text{ ms}$$
* **Hanging Loop Prevention:** The watchdog timer must not fire. If $T_{termination}$ is not reached within 2000ms, the test fails, proving the master reader loop hung because the parent process kept its PTY Slave descriptor open (preventing the OS kernel from sending EOF).

---

## Test Case 5: Clean PID Reaping & Exit Code Translation

### 1. Objective
Validate that process exit codes (including signal exits and violent crashes) are mapped to a 1-byte representation and that child PIDs are fully reaped from the host OS.

### 2. Precise Input Parameters
* **Workspace Identifier:** `WS-A`
* **Terminal ID 1:** `501` (Normal exit test)
* **Target Command 1:** `/bin/sh -c "exit 77"`
* **Terminal ID 2:** `502` (Signal exit via SIGKILL)
* **Target Command 2:** `/bin/sh`
* **Terminal ID 3:** `503` (Violent crash exit via SIGSEGV)
* **Target Command 3:** `/bin/sh -c "kill -11 $$"`

### 3. Step-by-Step Execution Sequence
1. Register the termination listener on `WS-A`.
2. Spawn `Terminal ID = 501`, `502`, and `503` running their respective commands. Capture their Process IDs ($PID_{501}$, $PID_{502}$, $PID_{503}$).
3. Wait for the termination events of `501` and `503` to fire.
4. Invoke `TerminatePTY(502)` to force kill process `502` (sending `SIGKILL`).
5. Wait for the termination event of `502` to fire.
6. Verify that the PIDs have been cleanly reaped from the OS process table by sending signal 0 (e.g. `kill -0 PID` returning `ESRCH` error).

### 4. Assertions & Expected Predicates
* **Normal Exit Code Translation:** The exit status returned in the termination event for `501` must be exactly `77`.
* **Signal Code Translation:** The exit status returned in the termination event for `502` must be exactly `137` (calculated as `128 + 9` for `SIGKILL`).
* **Crash Code Translation:** The exit status returned in the termination event for `503` must be exactly `139` (calculated as `128 + 11` for `SIGSEGV`).
* **Zombie Prevention Invariant:**
  * The system call check `kill -0 PID` for $PID_{501}$, $PID_{502}$, and $PID_{503}$ must return `ESRCH` (No such process) within 100ms of their termination events firing. If any PID shows status `Z` (Zombie) or still responds to signal 0, the test fails.

---

## Test Case 6: Concurrent Load & Lock Invariants (Deadlock Check)

### 1. Objective
Validate that the internal mutexes and synchronization mechanics prevent race conditions, memory corruption, and deadlocks under high concurrent operations.

### 2. Precise Input Parameters
* **Workspace Identifier:** `WS-A`
* **PTY Count:** 20 instances (`Terminal IDs = 601` to `620`)
* **Thread Pool Size:** 10 concurrent executor threads
* **Test Duration:** 2000 milliseconds
* **Target Command:** `/bin/sh -c "yes > /dev/null"` (Generates continuous infinite background load)

### 3. Step-by-Step Execution Sequence
1. Under `WS-A`, spawn all 20 PTY instances (`601` to `620`) concurrently.
2. Initialize 10 concurrent tester threads executing operations randomly:
   * **Threads 1-3:** Select a random active Terminal ID and execute `WritePTYInput` with a random 10-byte payload. Repeat with 0ms sleep.
   * **Threads 4-6:** Select a random active Terminal ID and execute `ResizePTY` with randomized dimensions. Repeat with 10ms sleep.
   * **Threads 7-8:** Continually query `GetActiveTerminalIDs` and call `GetScrollbackBuffer` on active IDs.
   * **Threads 9-10:** Repeatedly call `GetOrCreateWorkspace` and query global metrics.
3. Run the concurrent operations for 2000ms.
4. From the main thread, concurrently call `TerminatePTY(id)` for all 20 Terminal IDs.
5. Wait for all threads to terminate.

### 4. Assertions & Expected Predicates
* **Liveness Invariant:** The test suite must complete within 3000ms (no thread blocks or deadlock locks).
* **Safe Error Handling:** All concurrent operations executed during teardown must either succeed or return a clean "Terminal not found" error. The system must not crash or panic.
* **Complete Cleanup:** After step 4, the active terminal ID list for `WS-A` must be empty, and all 20 process IDs must be reaped from the host operating system.

---

## Test Case 7: Detached Daemon Process Reaping (Orphaned Children)

### 1. Objective
Ensure that if a child process spawns background children (orphaned daemons or detached process groups) and then exits, the Core immediately detects the main child's exit, fires the termination event, and reaps the main PID without blocking on the active background children.

### 2. Precise Input Parameters
* **Workspace Identifier:** `WS-A`
* **Terminal ID:** `700`
* **Target Command:** `/bin/sh -c "(sleep 100 &) && exit 0"` (Spawns a background worker child process that outlives the parent shell, then exits immediately)

### 3. Step-by-Step Execution Sequence
1. Register output and termination listeners on `WS-A`.
2. Record $T_{spawn}$.
3. Invoke `SpawnPTY` for `Terminal ID = 700` running the target command. Capture the child Process ID ($PID_{700}$).
4. Start a watchdog timer set to 3000ms.
5. Wait for the `On Process Termination` event to fire for `Terminal ID = 700`.
6. Measure $T_{termination}$.
7. Verify that $PID_{700}$ has been cleanly reaped from the OS process table.

### 4. Assertions & Expected Predicates
* **Non-Blocking Reaping Invariant:** The `On Process Termination` event must fire within 2000ms of spawn ($T_{termination} - T_{spawn} < 2000\text{ ms}$). The reaper task must **not** block or wait on the background child (`sleep 100`) process.
* **Clean Reaper Exit:** The main process PID ($PID_{700}$) must be fully reaped (checking `kill -0 PID` must return `ESRCH`).
* **Resource Leak Prevention:** The PTY Master file descriptor must be successfully closed, even if the background daemon still holds the slave PTY file descriptor open.

---

## Test Case 8: Rapid Lifecycle Command Race Conditions

### 1. Objective
Confirm that the Core safely handles rapid-fire, interleaved lifecycle requests (such as spawning, immediately resizing, and immediately terminating a PTY within a 1ms window) without race conditions or memory state corruption.

### 2. Precise Input Parameters
* **Workspace Identifier:** `WS-A`
* **Terminal ID:** `800`

### 3. Step-by-Step Execution Sequence
1. Under `WS-A`, construct and fire the following three operations in rapid, back-to-back sequence (aiming for sub-millisecond execution gaps, without waiting for async responses):
   * **Call 1:** `SpawnPTY(Terminal ID = 800, Cols = 80, Rows = 24)`
   * **Call 2:** `ResizePTY(Terminal ID = 800, Cols = 120, Rows = 40)`
   * **Call 3:** `TerminatePTY(Terminal ID = 800)`
2. Monitor system events and process registry table.

### 4. Assertions & Expected Predicates
* **No Race Crashes:** The server must not crash or trigger null-pointer/nil-reference panics.
* **Clean Terminal State Resolution:**
  * If `Call 3` executes after the process started: The process must be terminated and reaped.
  * If `Call 3` executes before the process fully initialized: The initialization routine must catch the termination flag, abort the launch, release allocated file descriptors, and cleanly remove the terminal from the registry map.
* **Registry Cleanliness:** After all three async calls complete, the active terminal ID list for `WS-A` must be empty, and no dangling file descriptors or PIDs must remain active.

---

## Test Case 9: Binary & Non-UTF8 Character Handshake Integrity

### 1. Objective
Assert that the Core remains a transparent binary proxy. It must pass all raw non-ASCII, non-printable, and invalid UTF-8 bytes to and from the process without alteration, escaping, or unicode character conversions.

### 2. Precise Input Parameters
* **Workspace Identifier:** `WS-A`
* **Terminal ID:** `900`
* **Binary Payload:** A sequence of non-UTF-8 binary bytes: `[0x00, 0xFF, 0xFE, 0x01, 0x1B, 0x5B, 0x48, 0x02, 0x0A]` (contains null byte, invalid UTF-8 sequences, and escape codes).

### 3. Step-by-Step Execution Sequence
1. Under `WS-A`, spawn `Terminal ID = 900` executing `/bin/sh`.
2. Register the output listener.
3. Write the exact binary payload to the PTY via `WritePTYInput` (configured to echo input, e.g. using a command that returns raw input like `cat`).
4. Capture the returned bytes from the output listener.

### 4. Assertions & Expected Predicates
* **Binary Transparency Invariant:** The captured output stream must contain the exact, unmodified sequence of bytes: `0x00, 0xFF, 0xFE, 0x01, 0x1B, 0x5B, 0x48, 0x02, 0x0A`.
* **No Byte Transformation:** No byte replacement (such as UTF-8 replacement characters `0xEF 0xBF 0xBD`), stripping of null bytes, or conversion of carriage return/newline characters must occur.

---

## Test Case 10: Write Error & SIGPIPE Immunity

### 1. Objective
Ensure that the server handles writes to a terminating or closed process gracefully. The write operation must return a clean, handled error rather than triggering a server crash from uncaught `SIGPIPE` or `EPIPE` exceptions.

### 2. Precise Input Parameters
* **Workspace Identifier:** `WS-A`
* **Terminal ID:** `1000`

### 3. Step-by-Step Execution Sequence
1. Spawn `Terminal ID = 1000` executing `/bin/sh`.
2. Terminate the child shell (e.g. call `TerminatePTY(1000)` and wait for the process to exit).
3. Immediately write input bytes (e.g., `echo "data"\n`) using `WritePTYInput(1000, ...)`.
4. Verify the status of the server.

### 4. Assertions & Expected Predicates
* **Graceful Failure Invariant:** The write attempt to the closed terminal must return a handled error (e.g., "Terminal not found" or `EPIPE` write error) within 50ms.
* **Server Liveness Invariant:** The server must not crash or exit. This proves that standard Unix `SIGPIPE` signals (which default to terminating the host process when writing to a closed pipe) are successfully handled, caught, or ignored by the server runtime.

---

## Test Case 11: Global Workspace Teardown & Process Sweep

### 1. Objective
Verify that when a workspace is programmatically removed from the global registry, the Core immediately triggers a sweeping teardown: terminating all active terminal processes, closing open file descriptors, and cleanly reaping all process IDs in that workspace to prevent memory leaks and orphaned process accumulations.

### 2. Precise Input Parameters
* **Workspace Identifier:** `WS-A`
* **Terminal ID 1:** `1101` (running `/bin/sh`)
* **Terminal ID 2:** `1102` (running `/bin/sh`)
* **Terminal ID 3:** `1103` (running `/bin/sh`)

### 3. Step-by-Step Execution Sequence
1. Under `WS-A`, invoke `SpawnPTY` for all three terminals (`1101`, `1102`, and `1103`).
2. Capture and record the process IDs of all three children: $PID_{1101}$, $PID_{1102}$, and $PID_{1103}$.
3. Invoke the global registry's `RemoveWorkspace("WS-A")` command.
4. Start a watchdog timer set to 2000ms.
5. Extrinsically check the OS process table for $PID_{1101}$, $PID_{1102}$, and $PID_{1103}$ (e.g., by executing `kill -0 PID`).

### 4. Assertions & Expected Predicates
* **Registry Clearance Predicate:** A subsequent lookup for `WS-A` in the global registry must return a new, blank workspace state, confirming the prior workspace memory and assets were fully purged.
* **Process Sweep Predicate:** The Core must issue termination signals (`SIGKILL` or equivalent process group kills) to all PTY process groups in the workspace.
* **Complete Reaping Predicate:** All three process PIDs ($PID_{1101}$, $PID_{1102}$, and $PID_{1103}$) must be fully reaped from the host operating system. The system call checks `kill -0 PID` must return `ESRCH` (No such process) for all three PIDs within 200ms of invoking `RemoveWorkspace`.
* **Resource Release Invariant:** The PTY master file descriptors for all three instances must be closed, returning their handles back to the operating system pool.


# File: 02_network_protocol.md

# MVP Sub-Spec 02: Network Layer & Wire Protocol

This sub-specification wraps the core architecture with a stateful communication network, defining connection handling, binary frame parsing, action routing, and raw I/O streaming.

References:
* [mvp_spec.md (Wire Protocol)](file:///home/coder/project/suprasole-server/meta/mvp_spec.md#L7-L47)
* [mvp_spec.md (Workspace Isolation)](file:///home/coder/project/suprasole-server/meta/mvp_spec.md#L53-L58)

---

## 1. Connection Establishment, Upgrade & Hijacking

* **WebSocket Upgrade Endpoint:**
  * The server upgrades incoming HTTP requests to WebSocket connections on the path `/ws`.
  * The client must provide a session token via the URL query parameters: `/ws?token=<session_token>`.
* **Session Token Extraction & Authentication:**
  * If the `token` parameter is missing or empty, the server rejects the upgrade with an `HTTP 400 Bad Request` or `HTTP 401 Unauthorized` status.
  * For local developer usage, any non-empty `token` is accepted as valid.
* **Workspace Binding:**
  * If the token matches a currently active workspace in memory, the connection binds to that workspace.
  * If the token does not match any active workspace, the server initializes a new stateful Workspace in memory.
* **Single-Client Hijacking Rule:**
  * Since this is a single-client development tool, a workspace can have at most one active WebSocket connection at any time.
  * If a new WebSocket connection upgrades with a token matching an already active workspace, the server must **immediately hijack** the session: it will close the previous socket handle and bind the new one.

## 2. Polymorphic Frame Parsing & Endianness

The application communicates exclusively via binary frames over a single, persistent WebSocket connection.
* **Byte Order:** All multi-byte integer fields (including `Action ID`, `Terminal ID`, `Cols`, and `Rows`) must be encoded in **Big-Endian (Network Byte Order)**.
* **Header:** Every packet begins with a **2-byte Action ID** (`uint16`). The remaining layout varies by action:

### Group A: PTY Lifecycle Actions

| Action ID | Name | Direction | Payload Binary Layout |
| --- | --- | --- | --- |
| **`0x0001`** | **PTY Spawn Request** | Client → Server | `[Terminal ID: 2B]` + `[Cols: 2B]` + `[Rows: 2B]` <br> *(Total frame size: 8 bytes)* |
| **`0x0002`** | **PTY Spawn Response** | Server → Client | `[Terminal ID: 2B]` + `[Status Byte: 1B]` <br> *(Total frame size: 5 bytes; Status: `0x00` = Success, `0x01` = Failure)* |
| **`0x0003`** | **PTY Resize** | Client → Server | `[Terminal ID: 2B]` + `[Cols: 2B]` + `[Rows: 2B]` <br> *(Total frame size: 8 bytes)* |
| **`0x0004`** | **PTY Lifecycle Termination** | Bidirectional | **Asymmetric Payload:** <br>• **Client → Server (Kill Request):** `[Terminal ID: 2B]` <br> *(Total frame size: 4 bytes)* <br>• **Server → Client (Exit Notification):** `[Terminal ID: 2B]` + `[Exit Code Byte: 1B]` <br> *(Total frame size: 5 bytes)* |

### Group B: PTY Data Actions

| Action ID | Name | Direction | Payload Binary Layout |
| --- | --- | --- | --- |
| **`0x0005`** | **PTY Stream I/O** | Bidirectional | `[Terminal ID: 2B]` + `[Raw UTF-8 Data Stream: Variable Length]` <br> *(Total frame size: 4 bytes + $N$ bytes)* |

### Group C: PTY Workspace Layout Actions

| Action ID | Name | Direction | Payload Binary Layout |
| --- | --- | --- | --- |
| **`0x0006`** | **PTY Visibility Shift** | Client → Server | `[Terminal ID: 2B]` + `[Visibility State Byte: 1B]` <br> *(Total frame size: 5 bytes; State: `0x02` = Focused, `0x01` = Visible, `0x00` = Hidden)* |

---

## 3. Protocol Validation & Error Handling Invariants

* **Malformed Frames:**
  * If the server receives a frame with an unrecognized `Action ID`, or if the frame's length does not match the exact expected size for its `Action ID` (for variable-length `0x0005` frames, the frame must be at least 4 bytes), the server must **immediately terminate** the WebSocket connection with WebSocket close code **`1002 (Protocol Error)`**.
* **Target Non-Existence:**
  * If a client executes an operation (`0x0003 Resize`, `0x0005 Stream I/O`, or `0x0006 Visibility Shift`) targeting a `Terminal ID` that is not registered in the active workspace, the server must silently ignore the request to prevent client-induced panic states.

---

## 4. Basic I/O Routing & Core API Integration

* **Client WebSocket to PTY Input:**
  * When a `0x0005 (PTY Stream I/O)` frame is received, the server parses the `Terminal ID` and payload, then invokes `Workspace.WritePTYInput(terminalID, payload)`.
* **PTY Output to Client WebSocket:**
  * The Network Layer registers an output listener via `Workspace.RegisterOutputListener(...)`.
  * When output is received, the server wraps the raw bytes in a `0x0005` binary frame (prepending the `Terminal ID` as a Big-Endian `uint16`) and transmits it over the active WebSocket.
* **Process Exit Notification:**
  * The Network Layer registers a termination listener via `Workspace.RegisterTerminationListener(...)`.
  * When a shell terminates, the server builds a `0x0004` frame containing `[Terminal ID: 2B]` + `[Exit Code Byte: 1B]` and transmits it to the client.

---

> ### 🔗 Downstream Scheduler Dependency Note
> * **Initial Concurrency Strategy:** During the implementation of this Network Layer (Sub-Spec 02), outbound writes to the WebSocket connection must be serialized using a simple mutex wrapper or a dedicated channel to prevent concurrent write collisions from PTY reader threads.
> * **Downstream Integration (`04_priority_scheduler.md`):** Once the Priority Scheduler is introduced, direct PTY-to-WebSocket routing will be decommissioned. All outbound traffic will instead queue into priority lanes, and a single, centralized scheduler runtime loop will handle socket writes, satisfying the serialization constraint natively.


# File: 02_network_integration_test_spec.md

# MVP Sub-Spec 02: Network Layer & Wire Protocol Integration Test Specification

This document defines the consolidated integration test suite for validating the WebSocket network layer, upgrade routing, binary polymorphic frame parsing, client hijacking, and protocol validation rules.

---

## 1. Test Harness Setup Invariants

* **Port Allocation:** Tests must spin up an ephemeral HTTP server (using `net/http/httptest` binding to `127.0.0.1:0`) hosting the WebSocket upgrade handler on path `/ws`.
* **Clean Teardown (LIFO Cleanup Invariant):** Every test must register cleanup routines to run in LIFO order (last in, first out):
  1. Close all active client WebSocket connections.
  2. Call `WorkspaceRegistry.RemoveWorkspace(workspaceID)` which invokes `workspace.teardown()`. This call **must block** (`wg.Wait()`) until all PTY process monitor threads have exited and all subprocesses are reaped.
  3. Close the HTTP test server to free TCP port allocations.
* **Socket Read/Write Deadlines:** To prevent slow or broken sockets from causing tests to hang, the server and client connections must enforce short write/read deadlines (e.g., maximum 5 seconds).
* **Bounded Test Assertions (Anti-Hang Invariant):** All test frame assertions (such as waiting for `0x0002 Spawn Responses` or `0x0005 Stream I/O` output) must be wrapped in a non-blocking `select` structure with a strict timeout (e.g., maximum 3 seconds). If the expected frame does not arrive, the test must call `t.Fatalf` immediately rather than hanging the test runner.
* **Atomic Process Reaping Invariant:** All PTY process allocations must be cleaned up via `killDescendants` procfs-traversal (reaping grandchildren, sub-shells, and background daemons) to ensure the system process table is 100% clean and free of orphaned children upon test completion.

---

## 2. Consolidated Test Suite Plan

### Test Case 1: TestWebSocketUpgradeAndAuthentication
* **Objective:** Validate upgrade endpoint routing and session token validation.
* **Assertions:**
  1. Connection requests to `/ws` without a `token` query parameter must be rejected with HTTP `400 Bad Request` or `401 Unauthorized`.
  2. Connection requests with an empty `token` query parameter must be rejected.
  3. Connection requests with a non-empty `token` must succeed, returning a valid WebSocket connection.

### Test Case 2: TestPTYLifecycleSpawn
* **Objective:** Validate terminal allocation and duplicate registration failures.
* **Assertions:**
  1. Sending `0x0001 (Spawn Request)` with a unique `Terminal ID` (Big-Endian) must trigger a `0x0002` response with `Status: 0x00` (Success).
  2. Sending a duplicate `0x0001` with the same `Terminal ID` must fail, returning a `0x0002` response with `Status: 0x01` (Failure).
  3. The active shell from the first spawn must remain active and unaffected by the duplicate request.
  4. Sending spawn requests to a non-existent workspace must initialize the workspace and succeed.

### Test Case 3: TestPTYStreamIO
* **Objective:** Validate bidirectional stream routing and raw binary transparency (non-UTF-8 bytes).
* **Assertions:**
  1. Sending input via `0x0005` must write to PTY stdin.
  2. Server output must be broadcast back to the client as `0x0005` frames.
  3. Binary Transparency: Sending invalid UTF-8 bytes (`[]byte{0xFF, 0xFE, 0xFD, 0xFC}`) must reach the shell stdin unmodified. Shell stdout returning those same bytes must be received by the client as the exact unmodified byte array (`0xFF, 0xFE, 0xFD, 0xFC`), verifying zero string-decoding corruption.

### Test Case 4: TestPTYResize
* **Objective:** Validate window dimension resizes.
* **Assertions:**
  1. Sending a `0x0003` Resize frame with new `Cols` and `Rows` must update the OS pseudo-terminal size.
  2. The update must be verified by executing the `stty size` command in the shell and parsing the echoed output, asserting it matches the requested dimensions.

### Test Case 5: TestPTYLifecycleTermination
* **Objective:** Validate terminal closure paths and ensure 100% PID reaping.
* **Assertions:**
  1. **Client-Initiated Close:** Sending a client-to-server `0x0004` frame containing `[Terminal ID: 2B]` (exactly 4 bytes total) must kill the PTY process and return a server-to-client `0x0004` frame with `Exit Status: 137` (SIGKILL status).
  2. **Process-Initiated Close:** Running the `exit <code\>` command in the PTY must trigger a server-to-client `0x0004` frame with the correct `Exit Status` byte (matching the exit code).
  3. In both cases, the OS process must be completely reaped (no zombie processes remaining).

### Test Case 6: TestWebSocketSessionHijackLifecycle
* **Objective:** Validate session takeover lifecycle, including Close code propagation, output listener unregistration, and pending spawn routing.
* **Assertions:**
  1. Connecting client `wsB` with the same token as active client `wsA` must trigger a session hijack: `wsA` is closed immediately, and `wsB` is bound to the workspace.
  2. `wsA` must be closed with WebSocket close code `4000 (Session Taken Over)` or `1008 (Policy Violation)`.
  3. Continuous high-volume output (e.g., `yes`) on a PTY must seamlessly transition to `wsB` without duplicated frames, and `wsA` must receive no further data after closure.
  4. If `wsA` initiates a `0x0001 (Spawn)` request and a hijack occurs *before* the spawn completes, the resulting `0x0002` response must route dynamically to the new active socket `wsB`.

### Test Case 7: TestProtocolViolationRejections
* **Objective:** Validate robust rejection and close codes for all malformed frames and invalid data inputs (Table-Driven).
* **Table of Test Scenarios (All must trigger immediate close with code `1002` or `1003`):**
  * **Text Frame:** Client sends a standard WebSocket text message instead of a binary frame (close code `1003`).
  * **Unrecognized Action ID:** Client sends an unknown action code (e.g. `0x9999`).
  * **Truncated Layouts:** Client sends frames with lengths smaller than the mandatory schema (e.g. `0x0001` spawn frame < 8 bytes; `0x0003` resize < 8 bytes; `0x0005` data < 4 bytes).
  * **Oversized Message:** Client sends a `0x0005` frame with a payload size exceeding the defensive limit of 64KB (e.g. 65,537 bytes).
  * **Zero Dimensions:** Client sends `0x0001 (Spawn)` or `0x0003 (Resize)` with `Cols = 0` or `Rows = 0`.
  * **Invalid Visibility State:** Client sends `0x0006` containing a visibility state byte that is not `0x00`, `0x01`, or `0x02` (e.g. `0x05`).

### Test Case 8: TestWebSocketLivenessAndClosure
* **Objective:** Validate socket keep-alives, write timeouts, standard closure handshakes, and target non-existence robustness.
* **Assertions:**
  1. **Write Deadline:** Simulating a slow client (saturating TCP buffer without reading) during continuous PTY output must trigger the server write deadline (e.g., 5 seconds), causing the server to close the socket and orphan the workspace.
  2. **Read Deadline & Pong:** Receiving low-level `Pong` frames in response to server `Ping` frames must successfully extend the server's read deadline, keeping the connection alive.
  3. **Standard Close Handshake:** If the client initiates a clean WebSocket Close frame (`1000` or `1001`), the server must respond with a corresponding Close frame before shutting down the socket (satisfying RFC 6455).
  4. **Target Non-Existence:** Operations (`Resize`, `Stream I/O`, `Visibility`) targeting a non-existent `Terminal ID` must be silently ignored, and the WebSocket connection must remain open and functional.


# File: 03_session_recovery.md

# MVP Sub-Spec 03: Session Reconnection & Scrollback Buffers

This sub-specification defines the persistence features, including handling abrupt network drops, managing stateful ring buffers, and replaying history upon client reconnection.

Reference: [mvp_spec.md](file:///home/coder/project/suprasole-server/meta/mvp_spec.md#L78-L135)

---

## 1. The Orphaned State & Cleanup Sweeper

* **Orphan Transition:** If the network connection drops or is abruptly severed, the workspace transitions to an *Orphaned* state.
* **PTY Continuity:** All underlying shell/PTY processes must continue executing uninterrupted. The server must not send termination signals or close process pipes upon connection failure.
* **Countdown Initiation:** Upon transitioning to an *Orphaned* state (when the active WebSocket connection is severed and no active client remains), a countdown timer must bind to the workspace.
  * **Default Duration:** The default countdown duration must be **5 minutes**.
  * **Configurability:** The duration must be exposed as a configurable setting (e.g. via a package-level variable or configuration option) so it can be overridden to a short duration (e.g., `200ms`) during test execution to guarantee fast, non-blocking runs.
* **Cancellation:** If the client successfully reconnects before the timer expires, the countdown must be immediately stopped and canceled.
* **Teardown:** If the timer reaches zero without a client reconnecting, the server must loop through all terminal assets in that workspace, issue an explicit termination signal (`SIGKILL`) to their process groups, close all open file descriptors, and remove the workspace from global memory tracking (via `WorkspaceRegistry.RemoveWorkspace`).
  * **Non-Blocking Sweeper Teardown Invariant:** The connection registry lock **must be released** before invoking the blocking `WorkspaceRegistry.RemoveWorkspace` call. Core process teardowns must execute asynchronously and outside of any connection registry critical sections to prevent blocking connections to other workspaces.

## 2. Scrollback Buffering & Reconnection Handshake

* **Isolated Ring Buffer:** Each PTY instance must maintain an isolated in-memory FIFO ring buffer dedicated to buffering `stdout`/`stderr` text output.
* **Bounded Capacity:** The buffer capacity must be bounded by a hard limit (e.g., 256KB per terminal) to guarantee absolute protection against server memory exhaustion. Older bytes must be instantly evicted as new data arrives.
* **Always-On Ingestion:** The server must continuously drain output from every active PTY file descriptor into its local ring buffer, regardless of whether a client WebSocket is currently connected or offline.
* **Truncation Demarcation:** If an eviction event occurs (the text stream wraps past the buffer capacity ceiling), the server must set a persistent `IsTruncated` boolean flag on that PTY instance.
* **Reconnection Handshake Sequence:** When an orphaned workspace receives a fresh WebSocket connection matching a known valid session token:
  1. **Socket Hijacking:** The server must immediately bind the fresh socket reference to the workspace, securely severing and closing any dangling, dead network socket handles from the prior session.
  2. **State Replay:** Before opening the channel to interactive live traffic, the Network Layer calls `Workspace.GetActiveTerminalIDs()` to list active terminals. For each terminal, it retrieves the buffer and truncation status using `Workspace.GetScrollbackBuffer(terminalID)`. The entire buffer is wrapped in a **single Action `0x0005` Stream I/O frame** per active terminal and transmitted to the client.
  3. **Continuity Injection:** If `isTruncated` is returned as `true` for a PTY, the server must prepend a standardized, visible warning line to the very beginning of the replayed binary text stream payload:
     ```text
     \r\n\x1b[33m[... Output truncated due to buffer overflow ...]\x1b[0m\r\n\r\n
     ```
     This explicit ANSI-escaped boundary preserves temporal continuity, ensuring the user immediately notes the structural break in historical output.
  4. **Visual Synchronization:** This sequence ensures that the client terminal views instantly repaint their exact history state, appearing seamless and uninterrupted to the end-user.

## 3. Concurrency, Synchronization & Sibling Integration

* **Atomic Session Transitions:** To prevent race conditions where a workspace is torn down while a reconnecting client is completing its upgrade handshake, all connection registrations, takeovers, and sweeper timer operations (creation, cancellation, and execution) **must be atomic and synchronized under the connection registry mutex.**
* **Thread-Safe Buffer Copies:** Copying the scrollback bytes during State Replay must occur under the PTY instance's lock to prevent concurrent shell reads from modifying or corrupting the buffer during the copy operation.
* **Mutex-Synchronized Playback Invariant (Zero Data Loss):** To prevent data loss of output printed *during* playback, the output/termination listeners must be registered *before* the state replay begins, but the write mutex on the WebSocket connection (`wsConn.mu`) must be held locked for the entire duration of the playback. Any live output generated concurrently will block on the write lock and automatically stream cleanly in chronological order once playback concludes and the lock is released.
* **Concurrent Input Invariant:** Client-to-server frames (such as Stream I/O, Resize, or Spawn requests) received during the State Replay phase must be processed concurrently. Any resulting shell output will be stored in the PTY's scrollback buffer and streamed once the live output listener is activated.
* **Sub-Spec 04 (Priority Scheduler) Bypass:** The State Replay sequence writes historical scrollback and exit frames directly to the WebSocket connection, bypassing the 3-Tier Priority Scheduler queues. The Priority Scheduler is activated only for subsequent live traffic after the State Replay phase has fully concluded.
* **Sub-Spec 05 (Defensive Heartbeat) Sweeper Integration:** Any connection termination triggered by the heartbeat liveness detection mechanism (defined in Sub-Spec 05) must route through the standard connection unregistration pipeline, transitioning the workspace to the *Orphaned* state and initiating the Cleanup Sweeper countdown.


# File: 03_session_recovery_integration_test_spec.md

# Integration Test Specification: Session Reconnection & Scrollback

This document specifies the integration testing requirements for validating Sub-Spec 03 (Session Reconnection & Scrollback Buffers) of the Suprasole Remote Terminal Server.

---

## 1. Test Environment Setup & Resources

* **Host Context:** Tests run on a standard Linux environment.
* **Workspace Setup:** Every test case must instantiate an isolated workspace instance and clean up all spawned processes and file descriptors using LIFO pipelines.
* **Sweeper Customization:** Tests verifying the Cleanup Sweeper must override the global countdown duration to a short window (e.g. `200ms`) to prevent test suites from blocking on production timeouts (5 minutes).

---

## 2. Integration Test Cases

### Test Case 1: PTY Continuity & Always-On Ingestion
* **Objective:** Verify PTY processes continue executing uninterrupted when the client disconnects, and output is continuously ingested into the scrollback buffer while offline.
* **Identifier:** `TestWorkspaceOrphanStateAndPTYContinuity`
* **Flow:**
  1. Upgrade a client WebSocket connection `ws` using a valid token `token-101`.
  2. Spawn PTY `1` (dimensions 80x24).
  3. Write input `sleep 0.3 && echo 'OFFLINE_OUTPUT'\n` to PTY `1` stdin.
  4. Forcefully close the WebSocket connection `ws` immediately (before the shell sleep concludes).
  5. Wait `600ms` (allowing the process to conclude executing and printing offline).
  6. Establish a fresh WebSocket connection `wsNew` using the same token `token-101`.
  7. Read the replayed scrollback data frame.
* **Assertions:**
  * The process must not be terminated when the socket drops.
  * The replayed scrollback stream received on `wsNew` must contain the string `'OFFLINE_OUTPUT'`.

### Test Case 2: WebSocket Session Hijack Takeover
* **Objective:** Verify atomic session takeover (hijacking) closes the old socket and routes all output to the new socket reference.
* **Identifier:** `TestWebSocketSessionHijack`
* **Flow:**
  1. Connect Client `A` using token `token-201` and spawn PTY `1`.
  2. Connect Client `B` using the same token `token-201`.
  3. Wait for connection unregistration.
  4. Write `echo 'HELLO_HIJACK'\n` to PTY `1`.
  5. Collect output frames from Client `B` and read Client `A`'s close code.
* **Assertions:**
  * Client `A` must be forcefully closed with Close Code `4000 Session Taken Over`.
  * Client `B` must receive the output frame containing `'HELLO_HIJACK'`.
  * Client `A` must receive no output after Client `B` registers.

### Test Case 3: Ring Buffer Eviction & Warning Injection
* **Objective:** Verify bounded buffer capacity limits (256KB) and eviction warning injection on reconnection.
* **Identifier:** `TestRingBufferEvictionAndTruncationWarning`
* **Flow:**
  1. Connect client using token `token-301` and spawn PTY `1`.
  2. Write input command to PTY `1` that dumps more than 256KB of data (e.g. `printf 'A%.0s' {1..300000}`).
  3. Wait `500ms` to guarantee the buffer wraps and evicts older bytes.
  4. Disconnect the client.
  5. Connect a new client WebSocket using `token-301`.
  6. Read the replayed scrollback stream frame.
* **Assertions:**
  * The replayed payload must start with the exact ANSI truncation warning line:
    `\r\n\x1b[33m[... Output truncated due to buffer overflow ...]\x1b[0m\r\n\r\n`
  * The size of the replayed payload (excluding the warning line) must be bounded exactly to the 256KB limit.
  * *Negative Assertion:* Replayed scrollback streams for non-truncated terminals (such as PTY `1` in `TestWorkspaceOrphanStateAndPTYContinuity`) must **not** contain the ANSI truncation warning prefix.

### Test Case 4: Cleanup Sweeper Lifecycle
* **Objective:** Verify sweeper timer countdown, cancellation on reconnection, and complete workspace teardown on expiration.
* **Identifier:** `TestCleanupSweeperLifecycle`
* **Flow:**
  * **Scenario A: Expiration, Teardown & Non-Blocking Registry**
    1. Override the global sweeper duration to `200ms` (`network.DefaultSweeperDuration = 200 * time.Millisecond`).
    2. Connect Client `A1` using `token-401a` and spawn PTY `1`.
    3. Spawn a background daemon process inside PTY `1` (e.g. `sleep 100 &`).
    4. Retrieve the PIDs of **both** the parent shell process and the background daemon process.
    5. Disconnect Client `A1` (initiating the sweeper countdown).
    6. Concurrently, while Client `A1`'s workspace is in the countdown/teardown phase, connect Client `B1` using a different token `token-401b` and spawn PTY `2`. Verify Client `B1` connects and spawns instantly without blocking.
    7. Wait `400ms` (exceeding the sweeper duration for `token-401a`).
    8. Query the workspace registry and host process table.
    * **Assertions:**
      * Client `B1`'s connection and spawn must not be blocked (verifying the Non-Blocking Sweeper Teardown Invariant).
      * Client `A1`'s workspace must be removed from the registry.
      * The parent shell PID and the background daemon PID must be completely reaped from the host OS process table, ensuring zero orphaned process leaks.

  
  * **Scenario B: Cancellation on Reconnection**
    1. Connect client using `token-402` and spawn PTY `2`.
    2. Retrieve the shell process PID.
    3. Disconnect the client (initiating the sweeper countdown).
    4. Wait `50ms` (well before the `200ms` countdown concludes).
    5. Connect a new client WebSocket using `token-402` (triggering sweeper cancellation).
    6. Wait `300ms` (exceeding the original sweeper deadline).
    7. Query the workspace registry and host process table.
    * **Assertions:**
      * The new WebSocket connection must remain open.
      * The workspace must remain registered and active.
      * The shell process PID must remain alive and healthy.

### Test Case 5: Mutex-Synchronized Playback & Concurrent Inputs
* **Objective:** Verify that live output printed *during* playback is not lost (Mutex-Synchronized Playback Invariant) and client inputs (Stream I/O and Resizes) received *during* playback are handled concurrently.
* **Identifier:** `TestMutexSynchronizedPlaybackAndConcurrentInput`
* **Flow:**
  1. Connect client, spawn PTY `1` (which has some scrollback).
  2. Disconnect the client.
  3. Reconnect the client.
  4. While the server is executing the State Replay write to the socket (holding `wsConn.mu`), programmatically:
     - Trigger a PTY write from the shell (e.g. write to PTY master).
     - Write a `0x0005` Stream I/O command frame from the client.
     - Write a `0x0003` PTY Resize frame (setting size to 100 cols and 30 rows) from the client.
  5. Wait for playback to finish and read all socket frames.
* **Assertions:**
  * The client must receive the complete historical scrollback first.
  * The live output generated during playback must be received *immediately after* the scrollback stream concludes, without data loss or corruption.
  * The client stream input sent during replay must be received and processed by the shell concurrently.
  * The PTY must be successfully resized to 100x30 concurrently during playback (verified via checking the PTY window size).


---

## 3. Sibling Integration Validation Notes

* **Sub-Spec 04 (Priority Scheduler) Integration:** Once Sub-Spec 04 is implemented, the State Replay test cases must assert that replayed scrollback stream frames bypass all Priority Scheduler lanes and write directly to the WebSocket writer, and that live stream output only begins entering the Priority Scheduler lanes after State Replay completes.
* **Sub-Spec 05 (Defensive Heartbeat) Integration:** Once Sub-Spec 05 is implemented, tests must simulate a heartbeat liveness failure (by blocking client Pong frames) and verify that the resulting socket eviction successfully triggers the transition of the workspace to the Orphaned state, initiating the Sweeper countdown timer.


# File: 04_priority_scheduler.md

# MVP Sub-Spec 04: PTY-Centric 2-Tier Priority Scheduler & Traffic Coordination

This specification defines the 2-tier centralized scheduler used to coordinate outbound traffic. By reducing scheduling classification to a binary matrix (High Priority vs. Low Priority), assigning draining priorities directly to individual frames, and round-robin load balancing outbound writes, the server achieves fair-share delivery for active viewports and strict starvation of background sessions.

Reference: [mvp_spec.md](file:///home/coder/project/suprasole-server/meta/mvp_spec.md#L137-L157)

---

## 1. The In-Memory Layout Priority Matrix & Status API

Every PTY instance within a workspace is assigned one of two operational priority classifications. The client synchronizes the entire workspace priority state by sending a batch **PTY Priority Sync** message (Action `0x0006`):

### Action `0x0006` Binary Wire Layout:
* **Header:** `[Action ID: 2B (0x0006)]` + `[ignored Header TerminalID: 2B (0x0000)]`
* **Payload:** `[PTY Status List: 3N bytes]` consisting of $N$ consecutive 3-byte blocks:
  * `[Terminal ID: 2B]` + `[Priority State Byte: 1B]`
* **Total Frame Size:** $4 + 3N$ bytes.

### Priority State Values:
* **High Priority (`0x01`):** Terminals requiring real-time updates and strict zero-loss output delivery (e.g. active visible viewports or foreground scripts).
* **Low Priority (`0x00`):** Terminals running background or non-critical views where old output can be safely discarded to prevent memory bloat.

### State Sync Behavior:
1. **Batch Update:** The server updates the priority of all specified terminals.
2. **Implicit Demotion:** Any active PTY inside the workspace that is **omitted** from the sync payload list is automatically demoted to Low Priority (`0x00`), allowing the client to synchronize the entire viewport state in a single action.
3. **Invalid Value Rejection:** If any priority byte is not `0x00` or `0x01`, or if the payload length is not a multiple of 3 (malformed layout), the server must immediately terminate the WebSocket with close code `1002 (Protocol Error)`.

---

## 2. PTY-Centric Message Queues & Frame-Level Priorities

Each PTY instance (`ptyInstance` in `core.go`) maintains its own independent, bounded outbound message queue:

1. **Queue Structure:** A slice of prioritized outbound frame packets (`action`, `terminalID`, `payload`, `drainingPriority`) protected by a local PTY-level mutex.
2. **Priority Initialization:** Upon creation, a newly spawned PTY defaults to **High Priority (`0x01`)**. This ensures that shell startup logs, environment greetings, and initial prompts are enqueued with blocking priority and not dropped before the client UI synchronizes its layout state.
3. **Frame-Level Priority Assignment:**
   * **Replay & Status Frames:** All scrollback playback frames (`0x0005`), spawn responses (`0x0002`), and termination notifications (`0x0004`) are enqueued with `drainingPriority = 0x01` (High).
   * **Live Stream Frames:** Live PTY outputs (`0x0005`) are enqueued with `drainingPriority` matching the PTY's current priority classification (`0x01` or `0x00`) at the moment of ingestion.
4. **Zero-Leak Lifecycle:** When a PTY is terminated, it is deleted from the workspace map. The garbage collector automatically sweeps the PTY and its associated queue, purging all pending frames.

---

## 3. 2-Tier Round-Robin Starvation Draining

A single centralized workspace workspace scheduler thread manages socket transmission by round-robining PTY queues based on the priority of the frame at the front of each queue:

1. **Active Socket Binding:** The scheduler loop queries the connection registry for the active connection. If a client is connected, it drains the PTY queues and writes to the socket. If no client is connected, the scheduler sleeps.
2. **Starvation Draining Rules:** The scheduler inspects the front message of each non-empty PTY queue (`pty.queue[0]`) in two distinct priority tiers:
   * **High-Priority Draining (Tier 1):** The scheduler gathers all PTYs whose front message has `drainingPriority == 0x01`. It round-robins across these queues, popping and transmitting one frame, then loops.
   * **Low-Priority Starvation (Tier 2):** Only if no PTY queue has a `0x01` frame at its front, the scheduler gathers all PTYs whose front message has `drainingPriority == 0x00`. It round-robins across these queues, popping and transmitting one frame at a time.
3. **Fairness Execution (Round-Robin):** To prevent low-indexed terminals from starving high-priority sibling panes, the scheduler must rotate PTY checks fairly. In Go, iterating over the PTY map (which has randomized key orders) naturally provides statistical round-robin fairness without requiring persistent index pointers.
4. **Atomic Write Invariant:** The scheduler must complete writing an entire frame packet to the underlying network socket before fetching the next message. Interleaving bytes from distinct terminal streams within a single frame packet is strictly prohibited.
5. **Peek-then-Pop (Transactional Writes):** To ensure zero-loss delivery on connection drops, the scheduler must **peek** at the front frame of the selected queue and write it to the socket. The frame is **popped** and discarded from the queue **only after** the socket write operation returns success.

---

## 4. Bounded Capacities & Differentiated Backpressure

Each PTY queue has a maximum ceiling of `1024` frames. Backpressure and eviction are managed locally on the PTY level based on the PTY's priority classification:

* **Low-Priority PTYs (`0x00`):** If a Low-Priority PTY queue reaches its `1024` ceiling, the next incoming stdout frame triggers a **drop-oldest eviction** (discarding index 0) to prevent background commands (e.g. `yes`) from blocking the shell process or leaking memory.
* **High-Priority PTYs (`0x01`):** If a High-Priority PTY queue reaches its `1024` ceiling, the reader loop **blocks on the enqueue operation**, propagating socket-level backpressure directly back to the active foreground processes to slow them down without losing any visible user output.

---

## 5. Message Classification & Routing

* **Enqueued Outbound Traffic:**
  * Action `0x0005` (Stream I/O): Enqueued in the terminal's PTY queue.
  * Action `0x0004` (PTY Termination): Enqueued in the terminal's PTY queue to guarantee correct chronological termination sequence.
* **Bypassed Traffic:**
  * Action `0x0002` (Spawn Response): Sent during handshake or connection recovery setup bypasses the scheduler and is written directly during the initial connection handshake.

---

## 6. Global Connection Singleton Invariant

To simplify server resource footprints and guarantee single-client isolation, the connection registry must enforce that **at most one active WebSocket connection exists across the entire server at any given time**:

* **Aggressive Eviction:** When `registerOrHijack` is called for any workspace token, the registry must iterate through all tracked connections (regardless of token), stop their active sweeper timers, and forcefully close the old WebSockets (sending Close Code `4000` for takeover of the same token, or a normal close frame for other tokens) before storing the new connection.

---

## 7. Latent Behaviors & Core Invariants

### A. Reconnection Replay Starvation Prevention
Because scrollback playback frames and spawn status frames are enqueued with `drainingPriority = 0x01` (High) during connection setup, they naturally bypass priority starvation, ensuring that historical playback for all split panes is fully transmitted before live output streams are prioritized.

### B. Priority Shift Wakeup
Whenever a PTY's priority classification is updated (promoted or demoted via `SetPTYPriority`), the server must call `Broadcast()` on that PTY's condition variable (`pty.queueCond`). This immediately wakes up any blocked PTY reader loop, allowing it to re-evaluate the queue policy and switch between blocking (High Priority) and drop-oldest (Low Priority) states.

### C. Orphaned Drop-Oldest Fallback
When no active client connection is registered for the workspace token, the server must temporarily treat all PTY queues as having Low Priority (`0x00`), executing the drop-oldest eviction policy instead of blocking. This ensures that user compilation tasks or background scripts continue running to completion while the user is disconnected.

### D. Outbound-Only Scheduling
The priority scheduler governs only outbound traffic (Server $\rightarrow$ Client stdout/termination). Ingestion of keyboard input (Client $\rightarrow$ Server Action `0x0005`) is processed immediately by the read loop and written directly to the PTY stdin descriptor, bypassing all queues to eliminate input latency.

### E. Socket Write Error Recovery & Transactional Hold
If a socket write fails during draining, the scheduler immediately detaches the socket writer but continues its run loop in a blocked state. Because of the **Peek-then-Pop** flow, the failed frame remains at index `0` of the PTY's queue, preserving it for the next reconnecting client.

### F. Mutex Consolidation (Atomic Resizes)
The PTY's local mutex (`pty.queueCond`'s locker) protects priority updates, queue operations, and ring buffer writes. Terminal resizing (`ResizePTY`) must acquire this lock, ensuring that reader loops and scheduler pops are temporarily blocked during the `ioctl` window.

---

## 8. High-Level Architectural Refactors

To support the centralized scheduler, the server transitions away from the baseline network layout in four areas:

1. **Unified Socket Writes:** Baseline callback-based output listeners (which wrote asynchronously and concurrently to the socket) are replaced. PTY reader loops and process monitors route outputs through local PTY queues, leaving the centralized scheduler thread as the sole writer to the WebSocket.
2. **Enqueued Handshake Replays:** Direct, synchronous socket writes during connection upgrades and spawn recovery playbacks are replaced by enqueuing these historical frames directly into PTY queues prior to binding the socket writer.
3. **Batch Priority Synchronization:** The single-terminal visibility shift delta format is replaced by a batch synchronization API (`0x0006`) that maps all viewport priorities in a single transaction and demotes omitted sessions.
4. **Global Singleton Connections:** Workspace-scoped socket takeover is replaced by a registry-wide singleton invariant. Establishing any connection evicts all other active WebSockets across the server to enforce a single-client model and automate orphaned session cleanups.

---

## 9. Cross-Specification Propagation (Downstream Ripple Effects)

The Priority Scheduler transitions trigger a chain reaction of behavior updates across earlier specs:

### A. Core Operations (`01_core_operations.md`)
* **Delayed Terminal Deletion:** Terminals exiting naturally or reaped via teardown are not deleted immediately from the workspace map. The core monitor thread must delay deletion until the local PTY queue has been fully drained by the scheduler (or a timeout is hit), guaranteeing stdout transmission integrity.
* **Visible Startup Invariant:** Spawning PTYs default to High Priority (`0x01`) rather than Low Priority (`0x00`) to protect early shell greetings and prompts.
* **Interface Contract Renaming (Section 4.B):** The core API function `Set Terminal Visibility` is refactored to `Set Terminal Priority`, accepting binary states (`0x01` High / `0x00` Low).
* **Legacy Callback Deprecation (Section 4.C):** The core output and termination callbacks are formally marked as deprecated/bypassed in the active network routing path, replaced by the centralized queue and the `SocketWriter` interface.

### B. Network Layer (`02_network_protocol.md`)
* **Write Mutex Elimination:** Network layer socket write wrappers (`wsConn.mu`) are decommissioned from live output loops, as all writes are now serialized through the workspace's single scheduler thread.
* **Sync Validation Rule:** Malformed `0x0006` frames (payload not divisible by 3) or invalid priorities immediately drop the socket connection with close code `1002` (Protocol Error).

### C. Session Recovery (`03_session_recovery.md`)
* **Registry-Wide Eviction:** Connecting workspace `A` cancels sweeper timers and closes WebSocket sockets for *any* other workspace token tracked globally by the server.
* **Upgrade Lock Elimination:** Scrollback replay frames are enqueued during connection setup, eliminating HTTP-handler-level socket locks during the handshake.
* **Lock-Free Playback Invariant:** The lock-based synchronization that previously locked the connection write mutex (`wsConn.mu`) during the entire replay phase is fully replaced. By enqueuing both replays and concurrent live outputs into the PTY's FIFO queue, chronological order is guaranteed natively by the queue structure, making the handshake process completely lock-free.
* **Scheduler Bypass Removal:** The exception permitting the state replay sequence to bypass the Priority Scheduler queues is fully decommissioned. All handshake replays now flow through the scheduling thread.

### D. Defensive Mechanisms (`05_defensive_mechanisms.md`)
* **Differentiated Eviction Policies:** The log-bomb safeguard is split. PTY queues classified as Low Priority (`0x00`) execute drop-oldest, while PTY queues classified as High Priority (`0x01`) propagate socket congestion back to the shell process by blocking read loops.
* **Orphaned Backpressure Shift:** When a WebSocket heartbeat (keep-alive) failure places a session into Orphaned mode, the server demotes all queues to Low Priority, preventing active shells from blocking when offline.
* **Consolidated Mutex Lock:** Resizes (`ResizePTY`) lock `pty.mu` during `ioctl` operations, blocking both PTY readers and scheduler pop loops to satisfy the atomic window invariant.


# File: 04_priority_scheduler_integration_test_spec.md

# MVP Sub-Spec 04: Priority Scheduler Integration Test Specification

This document defines the integration and unit tests required to verify the PTY-Centric 2-Tier Priority Scheduler, its edge cases, and all downstream refactor propagations.

Reference: [04_priority_scheduler.md](file:///home/coder/project/suprasole-server/meta/04_priority_scheduler.md)

---

## 1. Specification Coverage Matrix

The following matrix maps the 12 integration test cases directly to the requirements defined in the Priority Scheduler specification to guarantee 100% coverage:

| Test Case ID | Target Specification Requirement | Target Layer |
| :--- | :--- | :--- |
| **Test 1** | Strict priority draining & background starvation (Section 3.2) | Scheduler / Network |
| **Test 2** | Circular round-robin fair-share delivery (Section 3.3) | Scheduler / Network |
| **Test 3** | Bounded capacities & differentiated backpressure (Section 4) | Core / PTY Reader |
| **Test 4** | Orphaned state drop-oldest fallback (Section 7.C) | Core / Network |
| **Test 5** | Peek-then-Pop transactional write & write error recovery (Section 3.5 & 7.E) | Scheduler / Network |
| **Test 6** | Implicit terminal demotions on status sync (Section 1.3) | Network / Core |
| **Test 7** | Reconnection replay starvation prevention (Section 7.A) | Scheduler / Network |
| **Test 8** | Visibility shift wakeup via condition variable broadcast (Section 7.B) | Core / PTY Reader |
| **Test 9** | Registry-wide WebSocket singleton eviction (Section 6) | Connection Registry |
| **Test 10** | Zero-leak queue cleanup on PTY termination (Section 2.4) | Scheduler / Core |
| **Test 11** | Chronological PTY termination frame sequence (Section 5) | Scheduler / Core |
| **Test 12** | Priority promotion and backpressure reversion on reconnect (Section 7.C) | Core / PTY Reader |

> [!IMPORTANT]
> **Deterministic Test Guarantees:** 
> To prevent flaky tests, do **not** use arbitrary `time.Sleep()` calls to wait for scheduling operations. All test assertions must use deterministic polling loops (inspecting memory structures) or channel-based synchronization gates.

---

## SECTION 1: New Test Cases

These new tests must be written inside `go-tests/` to validate all scheduler invariants.

### 1. `TestStarvationAndStrictPriorityDraining`
* **Objective:** Verify that High-Priority (`0x01`) queues completely starve Low-Priority (`0x00`) queues.
* **Setup:**
  * Spawn two PTY instances in the same workspace: `T1` and `T2`.
  * Set `T1` to High Priority (`0x01`) and `T2` to Low Priority (`0x00`).
* **Execution:**
  * Flood both PTY stdout processes simultaneously with continuous data.
  * Allow the scheduler to drain messages to the client.
* **Assertions:**
  * Assert that the WebSocket connection receives **only** frames from `T1`.
  * Stop output on `T1` and empty its queue; assert that the scheduler then begins delivering frames from `T2`.
  * Assert that no frames from `T2` were delivered during `T1`'s active transmission window (strict starvation).

### 2. `TestRoundRobinFairShareDraining`
* **Objective:** Verify that multiple High-Priority terminals share socket bandwidth fairly without starvation.
* **Setup:**
  * Spawn three PTY instances: `T1`, `T2`, and `T3`.
  * Set all three to High Priority (`0x01`).
* **Execution:**
  * Write matching volumes of stdout data to all three PTYs concurrently.
* **Assertions:**
  * Assert that the WebSocket frames received by the client show a mixed distribution of payloads from `T1`, `T2`, and `T3` (all three are active in the delivery stream).
  * Assert that no single terminal is starved or blocked from writing while others have pending data.

### 3. `TestDifferentiatedBackpressure`
* **Objective:** Verify that High-Priority queues block when full, and Low-Priority queues execute drop-oldest eviction.
* **Setup:**
  * Attach a mock `SocketWriter` that simulates extreme network congestion (blocking writes).
* **High-Priority Test Block:**
  * Spawn `T1` (High Priority `0x01`).
  * Flood `T1`'s stdout.
  * Assert that when `T1`'s queue reaches the `1024` frame ceiling, the PTY reader loop **blocks**, propagating backpressure and causing the child process write to block.
* **Low-Priority Test Eviction:**
  * Spawn `T2` (Low Priority `0x00`).
  * Flood `T2`'s stdout past 1024 frames.
  * Assert that the PTY reader loop **does not block** and the child process executes to completion.
  * Assert that the scheduler queue has dropped the oldest frames and contains only the latest 1024 frames.

### 4. `TestOrphanedDropOldestFallback`
* **Objective:** Verify that when the WebSocket is disconnected, High-Priority terminals switch to drop-oldest to prevent offline task freezing.
* **Setup:**
  * Spawn `T1` (High Priority `0x01`).
  * Disconnect the client WebSocket, placing the workspace into the stateful `Orphaned` state.
* **Execution:**
  * Flood `T1`'s stdout with 2000 frames while offline.
* **Assertions:**
  * Assert that `T1`'s reader loop does not block and the process runs to completion.
  * Reconnect the client, and assert that the replayed queue contains exactly the last 1024 frames (verifying the low-priority drop-oldest fallback was active during the offline period).

### 5. `TestSocketWriteErrorTransactionalHold`
* **Objective:** Verify the **Peek-then-Pop** invariant keeps the failed write frame at index `0` of the queue.
* **Setup:**
  * Spawn `T1` and queue several stdout frames.
  * Configure the `SocketWriter` to return a network write error on the first frame write.
* **Execution:**
  * Trigger the scheduler. The scheduler attempts to write the first frame, encounters the error, and detaches the writer.
* **Assertions:**
  * Assert that the first frame remains at index `0` of `T1`'s queue.
  * Attach a new `SocketWriter` (simulating reconnection).
  * Assert that the new writer immediately receives the exact frame that previously failed, with zero data loss.

### 6. `TestImplicitDemotionOnPrioritySync`
* **Objective:** Verify that omitted active terminals are implicitly demoted to Low Priority.
* **Setup:**
  * Spawn `T1` and `T2` (both defaulting to High Priority `0x01` upon spawn).
* **Execution:**
  * Send a batch Priority Sync frame (Action `0x0006`) containing only `T1` set to High Priority (`0x01`), completely omitting `T2`.
* **Assertions:**
  * Verify that `T2` has been demoted to Low Priority (`0x00`) by flooding its stdout under mock socket congestion and asserting that `T2`'s reader loop does not block (executes drop-oldest eviction), while `T1` still blocks.

### 7. `TestReconnectionReplayStarvationPrevention`
* **Objective:** Verify that scrollback replay frames bypass priority starvation during reconnection setup.
* **Setup:**
  * Spawn `T1` (High Priority `0x01`) and `T2` (Low Priority `0x00`).
  * Disconnect the WebSocket and write historical output data to both processes.
* **Execution:**
  * Reconnect the client (initiating state replay for both terminals).
  * Simultaneously flood `T1` with live output frames.
* **Assertions:**
  * Assert that the client receives all replayed scrollback frames for `T2` (enqueued with High Priority `0x01` for replays) before any of `T1`'s live outputs are prioritized, confirming frame-level draining.

### 8. `TestPrioritySyncWakeup`
* **Objective:** Verify that demoting a blocked High-Priority PTY immediately wakes it up and unblocks the reader loop.
* **Setup:**
  * Spawn `T1` (High Priority `0x01`) under slow socket congestion.
  * Flood `T1`'s stdout until its queue hits `1024` and the core reader loop blocks on `pty.queueCond.Wait()`.
* **Execution:**
  * Send a batch Priority Sync frame (Action `0x0006`) demoting `T1` to Low Priority (`0x00`).
* **Assertions:**
  * Assert that the blocked reader loop immediately unblocks (wakes up), resumes execution, and transitions to drop-oldest eviction without deadlocking or dropping the shell process.

### 9. `TestGlobalConnectionSingletonEviction`
* **Objective:** Verify that upgrading any connection evicts all other active connections registry-wide.
* **Setup:**
  * Establish WebSocket `A` for workspace token `token-A`.
  * Establish WebSocket `B` for workspace token `token-B`.
* **Assertions:**
  * Assert that WebSocket `A` is forcefully closed by the server (receiving a regular close frame), confirming that at most one active WebSocket exists across the entire server registry at any time.

### 10. `TestQueueCleanupOnPTYTermination`
* **Objective:** Verify that PTY termination is clean and does not trigger scheduler panics or memory leaks.
* **Setup:**
  * Spawn `T1` (High Priority `0x01`) and write several stdout frames to populate its queue.
* **Execution:**
  * Explicitly terminate `T1` via shell exit, which deletes the terminal from the workspace map.
* **Assertions:**
  * Assert that the workspace scheduler thread continues execution gracefully, does not panic on the missing map key, and safely stops referencing `T1`'s queue.

### 11. `TestPTYTerminationChronologicalSequence`
* **Objective:** Verify that termination frames are enqueued and delivered in exact chronological order after stdout data.
* **Setup:**
  * Spawn `T1`.
* **Execution:**
  * Write a specific, identifiable output string to `T1`'s stdout, then trigger shell process exit.
* **Assertions:**
  * Assert that the WebSocket client receives all buffered stdout data frames first, followed exactly by the exit notification frame (`0x0004`) containing the exit status code.
  * Assert that no output frames for `T1` are received after the `0x0004` exit notification frame.

### 12. `TestOrphanedPriorityReversionOnReconnect`
* **Objective:** Verify that reconnecting promotes Low-Priority fallback queues back to High-Priority blocking backpressure.
* **Setup:**
  * Spawn `T1` (High Priority `0x01`).
  * Disconnect the client WebSocket (workspace transitions to Orphaned; `T1` falls back to Low Priority drop-oldest).
* **Execution:**
  * Reconnect the WebSocket client (workspace transitions back to Active).
  * Flood `T1`'s stdout under mock socket congestion.
* **Assertions:**
  * Assert that `T1`'s reader loop now **blocks** when the queue hits `1024`, confirming that `T1` has reverted back to its High-Priority blocking backpressure behavior upon client reconnection.

---

## SECTION 2: Updated Test Cases (Refactor Integration)

These existing tests must be refactored to align with the new contracts and behaviors.

### 1. Core Integration Isolation Tests (`core_integration_test.go`)
* **Stale Behavior:** These tests registered callbacks on `RegisterOutputListener` and `RegisterTerminationListener`.
* **Update Refactor:**
  * Implement a `mockSocketWriter` in the test file that implements `core.SocketWriter` and writes incoming frames to a Go channel.
  * Replace all `ws.RegisterOutputListener(...)` calls with `ws.SetSocketWriter(mockWriter)`.
  * Update assertions to read frames from the channel (checking for correct Action ID, Terminal ID, and Payload).

### 2. Protocol Violation Rejections (`network_integration_test.go`)
* **Stale Behavior:** Asserts on a 5-byte single terminal visibility shift delta frame containing invalid states.
* **Update Refactor:**
  * Refactor `"Visibility Shift with invalid state (0x05)"` sub-test to use the new 7-byte batch priority sync format (`[0x0006] [0x0000] [TermID] [0x05]`).
  * Add a new sub-test verifying that a malformed batch sync payload (e.g. payload length of 8 bytes, which is not divisible by 3) is rejected with WebSocket close code `1002 (Protocol Error)`.

### 3. Mutex-Synchronized Playback (`session_recovery_integration_test.go`)
* **Stale Behavior:** `TestMutexSynchronizedPlaybackAndConcurrentInput` asserts that concurrent live output blocks on the connection write lock (`wsConn.mu`) during scrollback replay.
* **Update Refactor:**
  * Remove assertions relating to `wsConn.mu` lock contention.
  * Update assertions to verify that all historical replay frames (loaded with `drainingPriority = 0x01`) are received first, followed immediately by concurrent live stdout frames, confirming the queue's FIFO integrity.

---

## SECTION 3: Deleted Test Cases

The following tests are rendered completely invalid by the new architecture and must be removed.

### 1. Unit tests for `RegisterOutputListener` / `RegisterTerminationListener`
* **Reason:** These APIs are completely deprecated and removed. Tests verifying their callback registrations are deleted.
* **Target Files:** Any raw mock verification tests inside `core_integration_test.go` checking listener-nulling or duplicate registration rejections.


# File: 05_defensive_mechanisms.md

# MVP Sub-Spec 05: Crucial Defensiveness & Edge-Case Constraints

This sub-specification defines the defensive mechanisms of the server, including backpressure management (log-bomb protection), connection keep-alives (heartbeats), and handling of concurrent operations during terminal resizes.

Reference: [mvp_spec.md](file:///home/coder/project/suprasole-server/meta/mvp_spec.md#L159-L179)

---

## 1. Backpressure Strategy (The Log-Bomb Safeguard)

If a background process executes a command that dumps an infinite loop of text (e.g., `yes` or `cat /dev/urandom`), it can overwhelm the scheduler queues.

* **Bounded Queues:** The internal channels or memory pipelines feeding into the Priority Scheduler must be strictly bounded.
* **Drop-Oldest Eviction:** If a background PTY queue (Priority 3) hits its ceiling, the server must discard the oldest chunk of text from that queue to make room for the latest process output. It must *never* block the PTY execution loop or allow memory allocations to grow unconstrained.

## 2. Network Heartbeats (Idle Proxy Eviction Safeguard)

Many production cloud load balancers forcefully terminate stateful WebSocket channels if no data travels across the pipe for 60 seconds.

* **Keep-Alives:** The server must maintain a background heartbeat thread that issues standard, low-level WebSocket `Ping` frames to the client every 30 seconds.
* **Liveness Detection:** If the client fails to reply with a corresponding native network `Pong` within a 10-second window, the server must treat the connection as broken, terminate the socket immediately, and shift the workspace into the stateful *Orphaned* mode.

## 3. Atomic Resize Invariant

Window resizes are asynchronous and frequent as users manipulate browser boundaries.

* When a `0x0003 (PTY Resize)` packet is handled, the server must block concurrent read/write operations on that specific PTY instance momentarily while executing the platform-level system call (`ioctl` with `TIOCSWINSZ`). This prevents structural race conditions where applications try to draw text characters based on old column boundaries right as the window geometry changes.


# File: 06_polish_and_wrap_up_spec.md

# Sub-Spec 06: Polish, Resource Hygiene, and Edge-Case Finalization Spec

This specification defines the ultimate criteria for verifying the ecological resource management, systemic stability, and holistic correctness of the Suprasole Server. It serves as the final audit manifest to guarantee zero latent bugs, zero discretionary behaviors, and complete conformance to defined remote execution protocols.

---

## 1. Ecological Resource Hygiene (Zero-Leak Mandate)

All server runtimes must conform to a strict zero-leak resource strategy. No file descriptors, CPU threads, or operating system processes may remain unallocated or zombie upon connection upgrades, timeouts, or workspace teardowns.

### File Descriptor Cleanup Verification
* **Conforming Behavior:**
  * The parent process closes its copy of the slave terminal file descriptor immediately after `cmd.Start()`.
  * The master PTY file descriptor is closed exactly once when the reader loop terminates (`handleProcessExit()`) or when the workspace is torn down.
  * The WebSocket socket is forcefully closed (`conn.Close()`) upon read timeouts, write timeouts, takeovers, or graceful disconnects.

### Goroutine Exit Verification
* **Conforming Behavior:**
  * The PTY exit monitor goroutine terminates immediately after `cmd.Wait()` returns.
  * The PTY reader loop goroutine exits immediately when `pty.master.Read()` returns a read error (such as when the master PTY descriptor is closed).
  * The workspace central scheduler loop (`startScheduler()`) terminates immediately when `w.isTornDown` becomes `true`.
  * No background goroutines may accumulate on the Go runtime during rapid socket upgrades or takeovers.

### Process Table Zombie Prevention
* **Conforming Behavior:**
  * All spawned child shells must be reaped via `cmd.Wait()` to clean their entries from the OS process table.
  * Teardown of orphaned background processes must recursively traverse process trees (`killDescendants()`) and issue group-level signals (`-PID` and ioctl `TIOCGPGRP`) to ensure grandchild processes are terminated.

---

## 2. Systemic Behavior & State Consistency

State changes across distinct layers (PTY streams, connection upgrades, priority transitions, and cleanup sweepers) must proceed atomically and predictably.

### Atomic Takeover & Scrollback Replay
* **Conforming Behavior:**
  * Upgrades and takeovers must be processed atomically in `registerOrHijack()`. The evicted socket must be notified and unbound, and the new connection bound.
  * Clear socket writer calls (`ClearSocketWriter()`) must only detach the writer if the active writer identity matches the caller, preventing connection takeover races.
  * Connection replay packets and spawn statuses must be loaded and enqueued atomically via `FlushAndEnqueueReplays()` before the writer is bound.

### Transient Replay Priority
* **Conforming Behavior:**
  * During the connection state replay phase, the PTY `replayActive` flag must be set to `true`.
  * Newly generated live stream frames enqueued during the replay phase must have their `DrainingPriority` overridden to `0x00` (Low priority).
  * Replay frames themselves must be enqueued with `DrainingPriority = 0x01` (High priority) to prevent live traffic of other PTYs from starving the replay.
  * Once the replay queue is fully drained, the `popFrame()` method must reset `pty.replayActive = false`, restoring original priority traffic flow automatically.

### Sweeper Timer Reconnection Safeguard
* **Conforming Behavior:**
  * Connection disconnects trigger workspace sweeper registration. If the client reconnects during the exact millisecond the sweeper callback is scheduled to run, the callback must verify `r.connections[token] != nil` under lock and abort the teardown sequence.

---

## 3. Holistic Protocol Rigidity (No Discretionary Behavior)

The server must reject all out-of-spec or malformed packets with immediate connection termination, avoiding any discretionary fallback assumptions.

### Strict Input Layout Validation
* **Conforming Behavior:**
  * **Frame Size:** Any binary frame under 4 bytes must be rejected (`CloseProtocolError`).
  * **Spawn Request (`0x0001`):** Payload must be exactly 4 bytes; dimensions (columns/rows) must be $> 0$.
  * **Resize Request (`0x0003`):** Payload must be exactly 4 bytes; dimensions must be $> 0$.
  * **Kill Request (`0x0004`):** Payload must be exactly 0 bytes.
  * **Priority Sync (`0x0006`):** Payload length must be a multiple of 3; priority states must be strictly `0x01` (High) or `0x00` (Low). Any other values (such as visibility values `0x02`) must trigger a protocol rejection error.
  * **Unsupported Frame Types:** Any non-binary frame (text mode) must trigger immediate connection close (`CloseUnsupportedData`).

---

## 4. Architectural & Implementation Naming Holism (Minimizing Name Indirection)

The naming of variables, functions, and test cases must consistently reflect the domain models. Remnants of deprecated specifications (such as visibility naming conventions) must be purged to maintain namespace hygiene and clarity.

### Domain Name Alignment
* **Workspace API:** The core workspace methods must only expose priority-based terminology. Legacy wrappers like `SetPTYVisibility()` and `GetPTYVisibility()` must be deleted in favor of `SetPTYPriority()` and `GetPTYPriority()` directly.
* **Network Handler:** Action `0x0006` is a Priority Sync packet and must invoke priority-based core methods (`SetPTYPriority()`).
* **Test Helpers:** Test pack/unpack framing helpers must use priority-based naming (e.g., `packPrioritySync()` instead of `packVisibilityShift()`).
* **Test Cases:** Test case names must align with the Priority Scheduler specification (e.g., `TestValidPrioritySync` instead of `TestValidVisibilityShifts`), and internal variables within tests must use `priority` instead of `vis` or `visibility`.

### Semantic Variable Alignment
* **Priority Variables:** All parameters, structural variables, and indices representing priorities must be named `priority` (or `DrainingPriority`/`effectivePriority`) to reflect their semantic meaning directly, avoiding generic names like `state`, `value`, or `status`.

### Parameter Name Consistency (Preventing Semantic Distortion)
* **Preserving Names Across Stack Boundaries:** When data is passed through a chain of nested, singular, tailored functions, the variable and parameter names representing that data must remain exactly the same. Renaming a parameter at different levels of the call stack (e.g., from `terminalID` to `tid` or `id`) introduces indirection and semantic distortion. The exact naming convention must be maintained across all nested call boundaries.

### Zero Abbreviation Rule
* **Use Full Words for All Names:** To eliminate semantic indirection and cognitive load, abbreviations must be avoided in all function names, struct fields, local variables, and parameter signatures. Names must use full, descriptive, English words. For example:
  * Use `terminalID` instead of `tid`.
  * Use `columns` and `rows` instead of `cols` and `rows`.
  * Use `workspace` instead of `ws`.
  * Use `socketWriter` instead of `sw`.
  * Use `connection` instead of `conn`.
  * Use `mutex` instead of `mu`.
  * Use `error` instead of `err`.

---

## 5. Minimizing Indirection (Code & Structural Hygiene)

Code architecture must minimize structural indirection, ensuring that the semantic purpose of every construct matches its execution path with near-zero cognitive overhead.

### Code Path Flattening
* **No Redundant Wrapper Functions:** Functions that forward arguments to a sub-method without introducing new logic or locks must be flattened. For example, `wsConnection`'s double-forwarding `WriteFrame()` $\rightarrow$ `writeFrame()` $\rightarrow$ `writeFrameLocked()` must be consolidated by removing the intermediate `writeFrame()` and directly implementing the lock-and-write in the interface method `WriteFrame()`.

### Semantic Constants (Zero Magic Value Indirection)
* **Explicit Action IDs:** Magic action bytes must be declared as package-level constants to avoid semantic translation indirection:
  * `ActionSpawn uint16 = 0x0001`
  * `ActionSpawnStatus uint16 = 0x0002`
  * `ActionResize uint16 = 0x0003`
  * `ActionKill uint16 = 0x0004`
  * `ActionStreamIO uint16 = 0x0005`
  * `ActionPrioritySync uint16 = 0x0006`
* **Explicit Priority Levels:** Priority magic values must be defined:
  * `PriorityLow byte = 0x00`
  * `PriorityHigh byte = 0x01`

### Tailored Functions vs. Over-Encapsulation
* **Singular Tailored Functions:** Over-encapsulation and the creation of overly generic, reusable utility functions must be avoided. Generic functions introduce semantic indirection because their signatures must use generic variable names to accommodate different contexts. Instead, prefer singular, tailored functions designed specifically for their single call-site. Extract shared logic into a helper function only when that exact code pathway is genuinely executed across multiple separate execution paths in the codebase.

---

## 6. Concurrency Safety & Lock Contention Minimization

To guarantee architectural holism and high performance under load, mutex locking must be optimized to prevent lock contention and deadlocks.

### Lock Hold Duration Minimization
* **No Blocking Operations Under Lock:** Blocking operations (such as PTY writes, WebSocket transmissions, or file I/O) must not be executed while holding state mutexes. For example, in `WritePTYInput()`, writing to `pty.master` should be done outside `pty.mu` if possible, or the lock must be immediately released after checking file descriptor validity to prevent blocking other goroutines attempting to update priority or read buffers on a congested terminal.
* **Defer vs. Explicit Unlock:** While `defer mu.Unlock()` is safe and clean, it holds the lock until function exit. In high-frequency pathways where the lock is only needed for quick map lookups or state transitions, the lock must be released explicitly as early as possible using `mu.Unlock()`.

---

## 7. Diagnostic & Error Propagation Hygiene

Errors must be propagated transparently through the stack, avoiding swallowed errors or loss of debugging context.

### Explicit Error Wrapping
* All internal subsystem errors must be wrapped using Go's `%w` formatting directive (e.g., `fmt.Errorf("failed to spawn PTY: %w", err)`) to preserve call-stack context and allow callers to inspect root causes via `errors.Is()` or `errors.As()`.

### Swallowed Errors Auditing
* Operations that discard errors (using `_ =`) must be explicitly audited. Discarding is only permitted for operations where failure is structurally inconsequential (such as closing already-terminated file descriptors during teardown). Any potential system call failures (like PTY geometry changes) must return explicit errors to the caller.

---

## 8. Memory Allocation & Buffer Optimization

To minimize Garbage Collection pressure and memory fragmentation under high-frequency stream input/output, buffer allocations must be optimized.

### Loop Allocation Minimization
* **Reusable Buffers:** Reading loops (such as `startReadLoop()`) must allocate their primary read buffers once outside the loop.
* **Slice Copy Overhead:** Duplicating slice arrays for queue buffering is necessary for concurrency safety but must be kept to a minimum size. Avoid over-allocating capacities when copying payloads for queue frames.

---

## 9. Compilation Hygiene & Warning Elimination

Code compilation must be clean, free of warnings, and strictly compliant with Go toolchain diagnostics to ensure code correctness and eliminate dead code.

### Warning-Free Compilation
* **Zero Compiler Warnings:** All code must compile under the Go toolchain with zero warnings.
* **Unused Identifiers:** There must be zero unused imports, unused local variables, or unused constants. Declared identifiers must either be utilized or explicitly removed.
* **Tidy Dependencies:** The Go module state must remain synchronized with code references. Dependency locks must be kept tidy (`go mod tidy` verification) to avoid unused package dependencies.
* **Static Diagnostics:** All files must pass standard `go vet` inspections and static checks without diagnostics, preventing issues like unreachable code, incorrect locking sequences, or malformed formatting arguments.

---

## 10. Verification & Regression Mandate

To preserve the holistic health of the server, the test suite must enforce:
1. **Zero Static Sleeps:** Waiting for network deadlines or timeouts must rely on dynamic socket polling (e.g., ping writes) to maximize test speed and system load resilience.
2. **Deterministic Queueing:** Starvation, backpressure, and round-robin scheduling checks must use programmatic frame enqueuing to assert boundary behaviors cleanly.
3. **Continuous Execution:** Every test case must compile and execute cleanly in `tests/integration/` with zero skipped tests.


# File: 06_polish_integration_test_spec.md

# Sub-Spec 06 Test Verification Spec: Polish & Naming Hygiene

This document defines the integration test updates, naming audits, and regression verification criteria for Sub-Spec 06 (Polish, Resource Hygiene, and Edge-Case Finalization Spec).

---

## 1. Naming & Domain Model Alignment Audits

The integration test suite must be updated to align with priority-based domain models, purging all obsolete references to the legacy visibility specifications.

### Test Case Renames
* **Test Case:** `TestValidVisibilityShifts` in `session_recovery_integration_test.go` must be renamed to `TestValidPrioritySync`.
* **Assertions:** Update all assertions to use `ws.GetPTYPriority()` instead of `ws.GetPTYVisibility()`.
* **Variable Names:** Rename all local variables inside the test (such as `vis1` and `vis2`) to `priority1` and `priority2` to reflect their semantic meaning directly.

### Helper Function Renames
* **Helper:** `packVisibilityShift()` in `network_integration_test.go` must be renamed to `packPrioritySync()`.
* **Logic:** The helper must construct Action `0x0006` binary frames.

---

## 2. Zero Abbreviation Audit

The test suite files (`tests/integration/*_test.go`) must be audited to ensure compliance with the Zero Abbreviation Rule. All identifiers must use full words:

* **Workspace Variables:** Rename `ws` $\rightarrow$ `workspace`.
* **Connection Variables:** Rename `conn` $\rightarrow$ `connection`.
* **Socket Writer Mock Variables:** Rename `sw` $\rightarrow$ `socketWriter`.
* **PTY Identifiers:** Rename `tid` $\rightarrow$ `terminalID`.
* **PTY Dimensions:** Rename `cols` $\rightarrow$ `columns` and `rows` $\rightarrow$ `rows` (remain full word).
* **Mutex Variables:** Rename `mu` $\rightarrow$ `mutex`.
* **Error Variables:** Rename `err` $\rightarrow$ `error` (except where it clashes with Go's `error` built-in interface type).

---

## 3. Semantic Constants Alignment

All test assertions and mock packet builders must use the newly defined package-level constants in `core` instead of magic numbers.

| Magic Value | Replace With |
| :--- | :--- |
| **`0x0001` (Action ID)** | `core.ActionSpawn` |
| **`0x0002` (Action ID)** | `core.ActionSpawnStatus` |
| **`0x0003` (Action ID)** | `core.ActionResize` |
| **`0x0004` (Action ID)** | `core.ActionKill` |
| **`0x0005` (Action ID)** | `core.ActionStreamIO` |
| **`0x0006` (Action ID)** | `core.ActionPrioritySync` |
| **`0x00` (Priority Level)** | `core.PriorityLow` |
| **`0x01` (Priority Level)** | `core.PriorityHigh` |

---

## 4. Regression Verification (Sweeper Timer Callback Race)

Verify that the regression test is implemented to prevent the workspace from being reaped during rapid reconnects.

### Test Case: `TestSweeperTimerCallbackRaceRegression`
* **File:** `session_recovery_integration_test.go`
* **Test Structure:**
  1. Temporarily override `network.DefaultSweeperDuration` to `30ms`.
  2. Connect Client A to establish a workspace session, and spawn an active PTY.
  3. Close Client A to trigger the 30ms sweeper timer.
  4. Sleep exactly `30ms` to allow the timer to expire and Go to schedule the callback goroutine.
  5. Immediately dial Client B to trigger connection hijacking.
  6. Sleep `100ms` to allow the scheduled sweeper callback goroutine to execute.
  7. **Assertions:**
     * Assert that the workspace still exists in the registry (i.e. `registry.GetOrCreateWorkspace("race-token")` succeeds without creating a new workspace).
     * Assert that Client B's socket remains open and functional (does not get closed by the sweeper).

---

## 5. API Signature & Dead Code Purge Verification

We must verify that no test files attempt to compile or execute calls to the deleted legacy APIs:

* **Deleted Methods Verification:** Ensure that any references to `RegisterOutputListener()`, `RegisterTerminationListener()`, `SetPTYVisibility()`, and `GetPTYVisibility()` have been completely removed from all test scripts.
* **Dead Code Check:** Verify that compiling the test suite package does not throw "undefined field/method" compilation errors on the `Workspace` interface, confirming that tests have successfully migrated to `SetPTYPriority()` and `GetPTYPriority()`.

---

## 6. Compilation & Diagnostic Validation Pipeline

The ultimate verification of the polish phase requires executing the following diagnostic command pipeline:

```bash
# 1. Verify zero static compiler warnings, unused imports, or compile errors
go test -c ./tests/integration

# 2. Verify zero static vet diagnostics (unreachable code, incorrect formatting, etc.)
go vet ./...

# 3. Verify zero tidy module warnings
go mod tidy -v

# 4. Verify 100% test pass rate with zero skipped tests
go test -v ./tests/integration
```
