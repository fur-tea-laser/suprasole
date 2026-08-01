# Specification: Resync Workspace Macro Lifecycle & Stream Staging (`resync_workspace_spec.md`)

This specification defines the exact semantics, temporal boundaries, state machine transitions, control-plane routing policies, and wire delivery invariants for the `ResyncWorkspaceCommand` (`0x000e`) macro lifecycle in `suprasole-server`.

---

## 1. Executive Summary & Purpose

When a client establishes or re-establishes a WebSocket session (`/ws`), the terminal screen state across all active developer terminals must be recovered quickly and accurately. 

The **Workspace Resync Macro Lifecycle** provides an event-driven, non-blocking synchronization protocol that guarantees:
* **Zero Output Loss**: Terminal stdout generated during reconnection or snapshot generation is never dropped.
* **Zero Data Duplication**: Baseline screen snapshots and live stdout streams transition seamlessly without duplicate lines or overlapping screen draws.
* **Non-Blocking PTY Execution**: The background kernel PTY reader loop (`master_fd`) is never paused or throttled during resync.
* **Atomic Screen Substitution**: The browser client receives explicit boundary events (`ResyncPtyStartEvent` and `ResyncPtyCompleteEvent`) to reset local `xterm.js` state and prevent race conditions with user keystrokes.

---

## 2. High-Level Macro Execution Flow & 6-Step Pipeline

State synchronization begins on connection drop (which immediately places PTY processes into `ModePreSnapshot`) and completes when the client transmits a `ResyncWorkspaceCommand` (`0x000e`) frame containing the active terminal list and grid dimensions.

The Workspace Controller processes each terminal in priority order using a **strict 6-step sequential pipeline** on its hydration worker thread:

1. **Hydration Phase Initiation**: The Workspace Controller receives `ResyncWorkspaceCommand` and verifies the target terminal is in `PtyMode_PreSnapshot`.
2. **Boundary Event Emission**: The controller enqueues `ResyncPtyStartEvent` (`0x000c`) to the egress queue, instructing the browser client to reset its local `xterm.js` screen buffer and lock user keyboard/UI input.
3. **Snapshot Pointer Freeze ($t_{\text{snap}}$)**: The controller calls `TransitionMode_PreToPostSnapshot()`. Under `PtyProcess.mu` lock ($< 50\text{ \mu s}$), the process executes an $O(1)$ memory pointer clone of the `xterm-go` state engine (`p.stateClone = p.xtermState.Clone()`), transitions to `PtyMode_PostSnapshot`, and releases `PtyProcess.mu`.
4. **Lock-Free Async Serialization & Chunking**: On the controller thread (without holding `PtyProcess.mu`), the controller calls `Serialize()` on `p.stateClone` to generate the ANSI snapshot stream $H_T$. It breaks $H_T$ into $32\text{ KB}$ or $64\text{ KB}$ chunks and dispatches all chunks via `onOutput_Snapshot(id, chunk)` to egress.
5. **Completion Marker Emission**: The controller enqueues `ResyncPtyCompleteEvent` (`0x000d`) to egress, instructing the browser client to unlock user input.
6. **Post-Snapshot Flush & Live Streaming Handoff ($t_{\text{end}}$)**: **Only after Steps 1–5 are complete**, the controller calls `TransitionMode_PostSnapshotToLive()`. Under a single microsecond lock acquisition ($< 5\text{ \mu s}$), the process pushes all stdout bytes accumulated in `postSnapshotBuffer` during snapshot serialization directly into `onOutput_PostSnapshotBuffer(id, bytes)` and transitions to `PtyMode_Live` for direct stdout streaming via `onOutput_Live(id, chunk)`.

---

## 3. PTY Execution State Machine & Thread Isolation Guarantees

State recovery for each terminal progresses across three microsecond-bounded temporal markers: initiation ($w_0$), snapshot capture ($t_{\text{snap}}$), and handoff completion ($t_{\text{end}}$).

### A. Initiation Phase ($w_0 \to t_{\text{snap}}$): Pre-Snapshot Subsumption (`PtyMode_PreSnapshot`)

The process enters `PtyMode_PreSnapshot` automatically on socket disconnect or when `TransitionMode_LiveToPreSnapshot()` is invoked at microsecond $w_0$.

* **State Transition**: Under `PtyProcess.mu` ($< 5\text{ \mu s}$), the process sets its mode to `PtyMode_PreSnapshot` and unbinds network output callbacks.
* **Reader Loop Behavior**: The background reader loop continues reading from `master_fd` without interruption. Bytes read from the kernel are fed directly into `xterm-go.Write()` under lock ($< 5\text{ \mu s}$), updating the terminal grid in memory. Outbound network transmission is suppressed.
* **Subsumption Invariant**: Stdout produced between $w_0$ and $t_{\text{snap}}$ does not need to be buffered separately. Because it updates `xterm-go` in memory in real time, it is automatically captured when the baseline snapshot is cloned at $t_{\text{snap}}$.

### B. Snapshot Capture Phase ($t_{\text{snap}} \to t_{\text{end}}$): Lock-Free Serialization (`PtyMode_PostSnapshot`)

When the controller is ready to generate the baseline snapshot for a terminal at microsecond $t_{\text{snap}}$, it invokes `TransitionMode_PreToPostSnapshot()`.

* **Pointer Cloning**: Under `PtyProcess.mu`, the process executes an $O(1)$ memory pointer clone (`p.stateClone = p.xtermState.Clone()`, $< 50\text{ \mu s}$).
* **State Transition & Localized Reset**: Within the exact same atomic lock acquisition, the process sets its mode to `PtyMode_PostSnapshot` and executes a single localized buffer reset (`postSnapshotBuffer.Reset()`).
* **Thread Isolation & "Lock-Free" Definition**: `PtyProcess.mu` is released immediately ($< 50\text{ \mu s}$). "Lock-free serialization" means running **without holding `PtyProcess.mu`**, allowing the background PTY reader loop on Thread 2 to continue reading `master_fd` non-stop. However, `p.stateClone.Serialize()` runs **synchronously on the Workspace Controller thread** (Thread 1), guaranteeing that Step 6 (`TransitionMode_PostSnapshotToLive()`) cannot execute until Step 4 (snapshot chunks) and Step 5 (`ResyncPtyCompleteEvent`) have been fully generated and enqueued.
* **Reader Loop Behavior**: While Step 4 is serializing on Thread 1, the background reader loop on Thread 2 reads continuously from `master_fd`. Bytes read are written to `xterm-go` **and** appended to `postSnapshotBuffer` under lock ($< 5\text{ \mu s}$). Outbound network transmission remains suppressed. Younger stdout bytes are held in `postSnapshotBuffer` and **cannot race ahead** of the baseline snapshot.

### C. Handoff Completion Phase ($t_{\text{end}}$): Atomic Handoff (`PtyMode_Live`)

When all baseline snapshot chunks have been submitted to network egress at microsecond $t_{\text{end}}$, the controller emits `ResyncPtyCompleteEvent` (`0x000d`) and calls `TransitionMode_PostSnapshotToLive()`.

* **Atomic Flush & State Mutation**: Under a single acquisition of `PtyProcess.mu` ($< 5\text{ \mu s}$), the process pushes all accumulated bytes in `postSnapshotBuffer` directly into `onOutput_PostSnapshotBuffer(id, bytes)` and sets its mode to `PtyMode_Live`.
* **Zero-Gap & Strict Order Guarantee**: Because flushing `postSnapshotBuffer` into `onOutput_PostSnapshotBuffer` and switching to `PtyMode_Live` occur inside the same single microsecond lock acquisition, the background reader loop on Thread 2 cannot dispatch live output to `onOutput_Live` until `postSnapshotBuffer` is queued. Live stdout is guaranteed to follow post-snapshot gap bytes without data loss or reordering.

---

## 4. Chronological Wire Framing Rules

All outbound frames pass through the single-writer FIFO egress queue (`wsConnection.WriteFrame`). Wire delivery for each terminal follows a strict chronological order:

1. **`ResyncPtyStartEvent` (`0x000c`)**: Contains target grid dimensions (`cols`, `rows`). The client clears its local `xterm.js` instance and locks user keyboard/UI input.
2. **`PtyOutputEvent` (`0x0008`) Chunks**: Contains $32\text{ KB}$ or $64\text{ KB}$ baseline snapshot data streams. The client writes these chunks into `xterm.js` to restore grid layout and scrollback.
3. **`ResyncPtyCompleteEvent` (`0x000d`)**: Empty payload marking snapshot stream end. The client unlocks user keyboard/UI input.
4. **`PtyOutputEvent` (`0x0008`) Gap Bytes**: Contains trailing stdout accumulated in `postSnapshotBuffer` during snapshot generation. The client applies these bytes seamlessly to the restored screen.
5. **`PtyOutputEvent` (`0x0008`) Live Streams**: Standard live stdout streaming in `ModeLive`.

---

## 5. Control-Plane Routing Policy During Active Resync

When client commands arrive while a resync macro is in progress, the server handles them according to specific routing policies:

* **New `ResyncWorkspaceCommand` (`0x000e`)**: Immediately cancels the active resync pass via Go `context.Context`, resets all PTY modes to `ModeLive`, clears `postSnapshotBuffer`, and starts a fresh resync pass.
* **`RemovePtyCommand` (`0x0006`)**: Sets an atomic `isDestroyed` flag, cancels hydration context, terminates the child process, and frees memory. Emits `ResyncPtyFailedEvent` (`0x000f`) if `ResyncPtyStartEvent` was already sent.
* **`TerminatePtyCommand` (`0x0004`)**: Dispatches the OS signal (`syscall.Kill`) to the process group immediately. The reader loop drains remaining stdout into `xterm-go` and `postSnapshotBuffer`, and defers emitting `PtyExitEvent` (`0x0005`) until after `TransitionMode_PostSnapshotToLive()`.
* **`ResizePtyCommand` (`0x0003`)**: Executes the `pty.Setsize()` kernel syscall on `master_fd` 100% unlocked outside `PtyProcess.mu`, and then resizes `xterm-go` under `PtyProcess.mu` ($< 5\text{ \mu s}$). If in `ModePreSnapshot`, the baseline snapshot is serialized at the new dimensions. If in `ModePostSnapshot`, redraw bytes accumulate in `postSnapshotBuffer`.
* **`WritePtyInputCommand` (`0x0007`)**: Writes stdin bytes directly to `master_fd` without blocking. Output generated in response is handled according to the active mode (`ModePreSnapshot`, `ModePostSnapshot`, or `ModeLive`).
* **`SetPtyPrioritiesCommand` (`0x0009`)**: Re-sorts the remaining unhydrated terminal queue in workspace memory and updates egress scheduler weights.

---

## 6. Resilience & Edge Cases

* **Unrecognized or Missing Terminals**: If the client requests resync for a terminal ID missing from memory, the server emits `ResyncPtyStartEvent` (`0x000c`) followed immediately by `ResyncPtyFailedEvent` (`0x000f`) with JSON error details, then continues hydrating the remaining terminals.
* **Spawning Terminals During Resync**: If a targeted terminal process is currently spawning, the server emits `ResyncPtyStartEvent` (`0x000c`) and `SpawnPtyStatusEvent` (`0x0002`, status Spawning), withholding `ResyncPtyCompleteEvent` (`0x000d`). When process creation finishes, the server emits `SpawnPtyStatusEvent` (status Success) followed by `ResyncPtyCompleteEvent` (`0x000d`).
* **Child Process Exit Mid-Resync**: If a process exits during resync, the reader loop drains remaining stdout into `xterm-go` and `postSnapshotBuffer`, marks the process as exited, and defers `PtyExitEvent` (`0x0005`) until after `postSnapshotBuffer` is flushed.
* **High-Volume Output Floods**: High-volume log floods continue updating the `xterm-go` grid live in memory. `postSnapshotBuffer` enforces a maximum RAM safety ceiling ($2\text{ MB}$). If exceeded during snapshot generation, excess bytes are truncated from `postSnapshotBuffer` to prevent memory exhaustion while maintaining $100\%$ grid state accuracy.
