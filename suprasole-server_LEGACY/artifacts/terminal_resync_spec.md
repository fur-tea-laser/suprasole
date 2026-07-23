# Architectural Specification: Lightweight Terminal Resync & Healing Protocol

This specification defines the lightweight, deterministic, and high-performance terminal resync and self-healing protocol for pseudo-terminals (PTYs) in `suprasole-server`. 

This design coordinates state recovery across connection drops and takeovers without the complexity of a server-side terminal emulator. It relies on **SIMD-accelerated 6-handler stream parsing**, **a single unified ordered active sequence array**, **an event-driven workspace resync macro protocol (`0x000e`)**, **granular per-PTY lifecycle status frames (`0x000c`/`0x000d`/`0x000f`)**, **upfront workspace-wide idempotent kernel PTY read-pause OS backpressure for zero-loss resync**, **the global terminal exit ordering invariant (`ActionTerminalExit` 0x0005 always follows all stdout bytes)**, **live spawn status ordering (`ActionSpawnStatus` 0x0002 [0x00] always precedes stdout bytes)**, **single-writer FIFO egress serialization**, **priority-weighted hydration scheduling**, **pre-resync command gating**, **in-flight hydration queueing**, **extended hydration boundary flush**, **automatic orphan PTY garbage collection**, **fast-forward spawning state recovery**, **a 512-byte carryover buffer**, **head-of-buffer newline sanitization**, and **unified asynchronous process-group signaling (`SIGWINCH`)**.

---

## 1. The Core Resync Challenge

A rolling raw-byte history buffer is lossy. When the buffer wraps around and evicts old data, essential state initialization ANSI escape sequences are lost (e.g. alternate screen toggles, cursor visibility, bracketed paste mode, focus reporting, scroll regions, window titles, current working directories, color themes, and mouse tracking modes). 

If a client reconnects or takes over a session:
* The client's terminal engine may remain in the primary screen while the active server process runs in the alternate screen, polluting the scrollback log.
* Pasting code into the shell after a reconnect could execute multi-line commands unexpectedly if Bracketed Paste Mode is lost.
* Editors like `neovim`, `helix`, or `lazygit` may stop receiving tab-blur auto-save events if Focus Reporting is lost.
* Custom color themes (Solarized, Monokai), working directory tracking (`OSC 7`), or window tab titles may revert to default blank states if Operating System Control (`OSC`) codes are lost.
* Sub-panel scrolling in `tmux` or `vim` may destroy top/bottom status bars if Scroll Margins (DECSTBM) are lost or misordered.
* Legacy `ncurses` box borders may render as corrupted ASCII characters if ISO-2022 character set slots (G0–G3) or Shift-In/Shift-Out states are lost.
* Reconnecting from a device with different screen dimensions could cause disruptive double-repaints and screen flickering.
* The active text user interface (TUI) layout may appear corrupted or static.

---

## 2. Server-Side State Tracking & Architecture

The server does not parse or store character grid cells. Instead, during the active streaming phase, the server's PTY reader loop scans outgoing stdout bytes using **segregated parsing handlers** that feed a **single unified ordered active sequence array**.

### A. Shared Common Mechanism (Stream Reader & SIMD Fast-Path)

All incoming PTY stdout streams pass through a shared, zero-allocation scanner pipeline before being queued for network egress:

1. **SIMD Performance Fast-Path (`bytes.IndexByte`)**:
   * To prevent pattern matching from becoming a CPU bottleneck under high-volume log floods (100MB/sec), the reader loop uses Go's assembly-optimized `bytes.IndexByte(chunk, 0x1b)` SIMD scanner.
   * If no `ESC` (`0x1b`) byte is present in the current chunk, the scanner bypasses sequence parsing instantly ($O(1)$ assembly check).
   * Parser logic is executed strictly when an `ESC` byte is detected.

2. **Split-Buffer Fragment Protection (512-Byte Carryover Buffer)**:
   * To prevent long escape sequences (e.g. `OSC 7` working directory file paths or `OSC 2` window titles) from breaking when split across arbitrary OS `read()` chunk boundaries:
   * Each PTY session maintains a 512-byte `carryover` buffer (`carryover [512]byte`).
   * Before scanning a new read chunk, any unparsed trailing bytes from the previous chunk are prepended.
   * If a chunk ends mid-sequence (e.g. `\x1b]7;file://hostname/path`), the unparsed trailing bytes are retained in `carryover` for the next read cycle.
   * **Overflow Protection Valve**: If an unparsed fragment at the end of a chunk exceeds 512 bytes without a terminating `BEL` (`\x07`) or `ST` (`\x1b\\`) byte, the reader logs a debug warning, resets `carryoverLen = 0`, and resumes SIMD scanning to prevent buffer overflow or memory corruption.

---

### B. Segregated Handlers & The Single Unified Ordered Array Model

To avoid the memory and fragmentation overhead of separate map and struct collections, the stream parsing pipeline routes incoming stdout bytes through three distinct stages:

1. **The Fast-Path SIMD Scanner**: Outgoing PTY stdout chunks are first scanned for escape characters (`0x1b`). Non-escape chunks bypass parsing instantly.
2. **The 6 Segregated Parsing Handlers**: When an escape character is detected, the sequence is dispatched to one of six specialized grammar handlers:
   * **Handler 1 (Mode Bitsets — `CSI [?] N h / l`)**: Parses DEC Private and ANSI Standard mode toggles (Alt-Screen, Bracketed Paste, Focus Reporting, Auto-Wrap, Cursor Visibility, App Cursor, Mouse modes, Insert mode).
   * **Handler 2 (Generic OSC Parameters — `OSC N;Value ST`)**: Parses string parameters by command number $N$ (Window Titles `0/2`, CWD `7`, Colors `10/11/12`, Shell Integration `133`).
   * **Handler 3 (Generic OSC 256-Color Palette — `OSC 4;Index;Value ST`)**: Parses 256-color palette overrides (`#rrggbb` or `rgb:r/g/b`), discarding query responses (`?`).
   * **Handler 4 (ISO-2022 Character Sets — `Esc ( X`, `SO`, `SI`)**: Parses G0–G3 character set designations and Shift-Out (`0x0f`) / Shift-In (`0x0e`) states for `ncurses` box-drawing support.
   * **Handler 5 (Structural Parameter Commands — `r`, `q`, `=`)**: Parses multi-argument grid parameters (Scroll Margins `DECSTBM`, Cursor Style `DECSCUSR`, Keypad Mode `DECKPAM`).
   * **Handler 6 (Reset & Sanitization Controller — `RIS`, `DECSTR`, `3J`)**: Handles terminal state resets, device attribute queries, and scrollback purges.
3. **The Single Unified Ordered Array (`activeLog`)**: Rather than storing state across separate maps or structs, all handlers route their parsed updates into a single, linear array (`activeLog []Sequence`) that preserves exact chronological insertion order.

---

### C. Unified Array Mutation & Reset Semantics

All six handlers mutate or filter the single unified ordered array (`activeLog`) and history buffer:

1. **In-Place Value Updates (Handlers 1–5)**: When Handlers 1–5 parse an enable or configuration sequence, they check `activeLog` for an existing entry matching the sequence key. If present, the sequence string is updated in-place; if not present, the sequence is appended to the end of the array.
2. **Disable Removal (Handler 1)**: When Handler 1 parses a disable sequence (e.g. `\x1b[?1049l`), it removes the corresponding sequence entry from `activeLog`.
3. **Soft Reset Discrimination (`DECSTR` — `\x1b[!p` — Handler 6)**:
   * When Handler 6 detects a Soft Reset sequence (emitted by `reset` or `tput reset`), it filters `activeLog` in-place, removing `CSI` mode and structural sequences, and restoring default-active modes (**Auto-Wrap Mode 7** and **Cursor Visibility Mode 25** to enabled).
   * **State Preservation Invariant**: Soft Reset **must strictly preserve** all `OSC` string parameters in `activeLog` (Window Titles, Current Working Directories, and background/foreground color themes).
4. **Full Reset Execution (`RIS` — `ESC c` / `0x1b 0x63` — Handler 6)**:
   * When Handler 6 detects a Full Reset sequence, it empties `activeLog` completely (`activeLog = activeLog[:0]`), triggers `ringBuffer.Clear()`, and restores default-active modes (Modes 7 and 25 enabled).
   * **Parser Isolation**: Handler 6 strictly distinguishes `ESC c` (`RIS` / Full Reset) from `CSI c` (`DA` / Device Attributes Query — `0x1b 0x5b 0x63`). Device Attribute queries (`CSI c`) **must never clear the array or history**.
5. **Display Clear (`\x1b[2J`) vs Scrollback History Purge (`\x1b[3J` — Handler 6)**:
   * **Erase Display (`\x1b[2J`)**: Clears the visible viewport text. Handler 6 **must NOT** truncate the 256KB raw history buffer when `\x1b[2J` is detected.
   * **Erase Saved Lines (`\x1b[3J`)**: Upon detecting `\x1b[3J`, Handler 6 immediately truncates and clears the 256KB raw history ring buffer up to that offset.
   * **Purge Isolation**: Executing a scrollback purge (`\x1b[3J`) clears raw history text *only*. It **must NOT** alter or clear the `activeLog` sequence array.
6. **Tab Stop Restoration (Handler 6)**:
   * Upon receiving a Soft Reset (`DECSTR`) or Full Reset (`RIS`), Handler 6 clears custom tab stops (`HTS`) and restores standard **8-column tab stops** (columns 9, 17, 25, 33, 41, 49, 57, 65...).

---

## 3. Head-of-Buffer Newline Sanitization

When the 256KB history buffer wraps around and evicts old bytes, the byte at the head (start) of the history buffer might begin in the middle of a multi-byte ANSI escape sequence or a split UTF-8 character.

To eliminate leading garbage text or fragment numbers at the top line of the replayed scrollback:
* When compiling the history payload for a reconnecting client, the server scans forward from the head of the 256KB buffer to the **first newline (`\n`) byte**.
* The server trims all bytes preceding that first newline.
* This guarantees that the replayed history stream **always begins cleanly at the start of a fresh line**, with zero partial sequence artifacts.

---

## 4. The Event-Driven Resync Macro Protocol & Status Lifecycle

Rather than automatically dumping history on raw WebSocket connection upgrade, resync is driven by an explicit, event-driven macro request from the client (`ActionResyncWorkspace` `0x000e`) and governed by a granular per-PTY status lifecycle (`0x000c`/`0x000d`/`0x000f`).

### A. The Resync Macro Request (`ActionResyncWorkspace` — `0x000e`)

When a client connects or reconnects over WebSocket, the client sends a single macro control frame (`ActionResyncWorkspace` / `0x000e`) specifying all active PTY instances and their current UI target dimensions and priority weights:

```json
{
  "terminals": [
    { "terminalID": 1, "columns": 140, "rows": 50, "priority": 1 },
    { "terminalID": 2, "columns": 80,  "rows": 24, "priority": 2 }
  ]
}
```

* **Client-Only Spawning Invariant**: Spawning PTYs is strictly client-initiated (`ActionSpawn` / `0x0001`). The server never spawns PTYs autonomously without client request.
* **Asynchronous Concurrent Spawning Invariant**: PTY creation is **100% asynchronous and concurrent** in Go memory. When `ActionSpawn` (`0x0001`) arrives, the server executes `workspace.SpawnPTY()` immediately in its own background Goroutine without blocking or waiting for resync passes to finish. As soon as `fork/exec` completes, `ActionSpawnStatus` (`0x0002`) is enqueued into the single-writer FIFO egress queue (`wsConnection.WriteFrame`) and transmitted over the WebSocket.
* **Upfront Workspace-Wide PTY Read-Pause Invariant**: Upon receiving `ActionResyncWorkspace` (`0x000e`), the server immediately pauses `read(masterFd)` for **ALL requested PTYs upfront** in one atomic step before hydration compilation begins. This freezes stdout state across the entire workspace at the exact moment `0x000e` is received, guaranteeing zero history eviction while background PTYs wait in the priority queue. Each PTY's `read(masterFd)` is un-paused and resumed immediately as its individual `ActionSyncComplete` (`0x000d`) frame is written to the socket.
* **Idempotent PTY Read Pause / Resume Invariant**: PTY pause (`PauseRead()`) and resume (`ResumeRead()`) operations are **100% IDEMPOTENT**. Calling `PauseRead()` on an already paused PTY or `ResumeRead()` on an already un-paused PTY is a safe no-op.
* **Atomic Resync Preemption / Restart Invariant**: `ActionResyncWorkspace` is **100% Idempotent and Preemptible**. If a shaky network connection sends a new `ActionResyncWorkspace` (`0x000e`) while a previous resync pass is mid-compilation, the server executes an **atomic restart**: it cancels the in-flight resync compiler loop, applies the new pass's upfront read-pause (idempotently), updates target dimensions and priority weights, and restarts clean priority-sequenced hydration from scratch with zero conditional logic.
* **Empty Terminal Array Orphan Eviction Invariant (`terminals: []`)**: If `ActionResyncWorkspace` carries an empty terminal array `[]` (e.g. user closed all tabs offline), the server compares `[]` against `workspace.terminals`. All active PTYs in Go memory are recognized as orphaned and destroyed via `workspace.RemovePTY()`, leaving a clean empty workspace without emitting per-PTY streams.
* **Single-Pass Pre-Resizing**: Upon receiving `ActionResyncWorkspace`, the server immediately resizes each master PTY descriptor to its target `columns` and `rows` *before* compiling replay history, guaranteeing single-pass layout rendering with zero double-repaints.
* **Priority-Weighted Scheduling**: The server workspace scheduler immediately applies the requested `priority` weights (`SyncPTYPriorities`), ensuring high-priority focused tabs (`priority: 1`) receive 100% bandwidth priority so the active user viewport unlocks instantly.
* **Priority-Sequenced Concurrency Invariant**: Resyncing PTYs follows a **priority-sequenced concurrency model**. High-priority PTY hydration streams (the focused tab) are compiled and flushed over the WebSocket **FIRST**, guaranteeing zero socket contention while the active user viewport unlocks. Once the high-priority hydration burst completes, low-priority background PTY hydration streams execute in priority sequence. Each PTY emits its own `ActionSyncStart` (`0x000c`) and `ActionSyncComplete` (`0x000d`) boundary markers independently as its hydration payload finishes.
* **Foreground Process Group Repaint Invariant**: During history compilation, the server queries `ioctl(masterFd, TIOCGPGRP, &pgid)` to resolve the active foreground TUI application (e.g. `vim`, `htop`, `tmux`) and dispatches `SIGWINCH` directly to its PGID. If `TIOCGPGRP` fails, the server catches the error silently and completes hydration without failing the resync sequence.
* **Automatic Orphan PTY Garbage Collection**: If an `ActionRemove` (`0x0006`) frame was lost due to a Wi-Fi drop while closing a tab, the server compares the requested PTY list in `ActionResyncWorkspace` against its active workspace map. Any active PTY in Go memory omitted from the client's open tab list is recognized as an orphaned tab and automatically destroyed via `workspace.RemovePTY()`.

---

### B. Kernel PTY Read-Pause, OS Backpressure & Command Gating

To guarantee zero data loss, zero history eviction, and zero stream corruption during resync, the server enforces kernel PTY read-pause and command gating rules:

1. **Kernel PTY Read-Pause & OS Backpressure Invariant**:
   * During the history compilation window for a PTY (between `ActionSyncStart` `0x000c` and `ActionSyncComplete` `0x000d`), stdout reading on the master PTY file descriptor (`read(masterFd)`) is paused under the upfront workspace read-pause rule.
   * **OS Kernel Backpressure**: If the child process emits stdout during this hydration window, output accumulates in the Linux OS kernel PTY buffer. If the OS kernel buffer fills, Linux naturally pauses the child process's `write()` syscall.
   * **Zero History Eviction**: Pausing `read(masterFd)` guarantees that zero new bytes enter the 256KB ring buffer during history compilation, preventing history eviction or stream corruption.
   * **Immediate Resumption**: Immediately after `ActionSyncComplete` (`0x000d`) is written to the WebSocket socket, `read(masterFd)` resumes, and live streaming takes over seamlessly.
2. **Pre-Resync Command Gating Policy**: Upon WebSocket connection upgrade, the connection enters `Unsynced` state. If a client attempts to send operational commands (`ActionSpawn` `0x0001`, `ActionResize` `0x0003`, `ActionKill` `0x0004`, `ActionRemove` `0x0006`, `ActionInput` `0x0007`, `ActionPrioritySync` `0x0009`) **BEFORE** sending `ActionResyncWorkspace` (`0x000e`), the server **rejects/drops** those frames, enforcing proper resync handshake sequencing.
3. **In-Flight Hydration Queueing Policy**: When `ActionResyncWorkspace` is received and a PTY is in `Hydrating` state (`ActionSyncStart` `0x000c` emitted, `ActionSyncComplete` `0x000d` pending), any operational frames (`ActionInput`, `ActionResize`) received for that PTY are **enqueued in a per-PTY pending buffer**. They are NOT written to the master PTY or executed during history compilation.
4. **Extended Hydration Boundary Invariant (Mid-Hydration Resizes)**: If `ActionResize` (`0x0003`) arrives mid-hydration for a PTY, the server updates the PTY's pending resize slot (`pendingCols`, `pendingRows`). At the end of history compilation:
   * **BEFORE emitting `ActionSyncComplete` (`0x000d`)**, the server checks for pending resizes.
   * If a pending resize exists, the server executes `setSize(pendingCols, pendingRows)`, dispatches `SIGWINCH`, and flushes the resulting repaint bytes over the WebSocket socket.
   * **THEN AND ONLY THEN does the server emit `ActionSyncComplete` (`0x000d`)!** This guarantees that when the client receives `0x000d` and unlocks the tab, the tab is 100% hydrated AND fully repainted to the latest target dimensions with ZERO post-unlock layout shifts!
5. **Control Frame Socket Write Deadline Invariant**: Outbound control frame writes (`ActionSyncStart`, `ActionSyncComplete`, `ActionSyncFailed`) enforce a standard 5-second socket write deadline (`SetWriteDeadline`) to prevent stalled or un-drained TCP sockets from hanging server takeover Goroutines.
6. **Post-Hydration Pending Queue Flush**: Immediately after `ActionSyncComplete` (`0x000d`) is emitted for a PTY, the server flushes any pending stdin keystrokes (`ActionInput` `0x0007`) to the master PTY descriptor and unlocks live streaming for that PTY.

---

### C. State Recovery & Protocol Category Invariants

1. **Architectural Category Separation Invariant**: The protocol strictly separates **Live Creation Lifecycle Events** from **Resync Hydration Protocol Events**:
   * **Live Creation Lifecycle Events (`0x0001`, `0x0002`, `0x0006`)**: Handshake frames emitted ONCE during live streaming when a PTY instance is created or destroyed.
   * **Resync Hydration Protocol Events (`0x000e`, `0x000c`, `0x000d`, `0x000f`)**: Event-driven frames executed on reconnect to rebuild baseline state, active setup codes, and scrollback history.
2. **Global Terminal Exit Ordering Invariant (`ActionTerminalExit` `0x0005`)**: 
   * **Established Server Behavior**: In the existing `suprasole-server` implementation (`source/core.go` L648–653), `ActionTerminalExit` (`0x0005`) is strictly guaranteed to be emitted AFTER all PTY stdout bytes are read from `masterFd` (`<-terminal.readDone`) AND all queued `ActionOutput` frames are flushed to the TCP socket (`pendingCount == 0`).
   * **Mandatory Preservation Rule**: This established behavior is **authoritative and MUST BE STRICTLY PRESERVED AND MAINTAINED** through all future refactors. Across the entire server (both live streaming and resync hydration), **`ActionTerminalExit` (`0x0005`) MUST NEVER be emitted until ALL remaining stdout bytes from the master PTY file descriptor and ring buffer have been 100% read, parsed, and flushed to the network socket**.
   * *Live Streaming*: On process exit (`EOF`/`EIO`), the server reads all remaining stdout bytes from the master descriptor, flushes them to the socket, and THEN emits `ActionTerminalExit` (`0x0005`).
   * *Resync Hydration (`TerminalStateExited`)*: Emits `ActionSyncStart` (`0x000c`) $\rightarrow$ Flushes ALL final scrollback/stdout bytes $\rightarrow$ `ActionTerminalExit` (`0x0005`, exit code payload) $\rightarrow$ `ActionSyncComplete` (`0x000d`).
3. **Live Spawn Status Ordering Invariant (`ActionSpawnStatus` `0x0002 [0x00]`)**:
   * **Mandatory Precedence Rule**: On live PTY creation (`ActionSpawn` `0x0001`), `ActionSpawnStatus` (`0x0002`, `Payload: [0x00]`) **MUST ALWAYS be written to the network egress queue BEFORE launching `startReadLoop()` or emitting any `ActionOutput` (`0x0008`) stdout bytes for that PTY**.
   * **Eliminates Race Conditions**: This eliminates the potential race condition in legacy server code where `startReadLoop()` could read stdout bytes and enqueue `ActionOutput` before `ActionSpawnStatus [0x00]` reached the socket, ensuring the client UI mounts the tab and initializes `xterm.js` *before* receiving stdout bytes.
4. **Single-Writer Egress FIFO Serialization Invariant**:
   * **Refactoring Target vs Legacy Dual-Channel Egress**: The legacy server implementation utilized two separate un-synchronized channels (`EnqueueControlFrame` for control frames vs `EnqueueFrame` for data frames), allowing high-priority control frames to potentially overtake data frames out of order on the TCP wire.
   * **Single-Writer FIFO Queue Requirement**: All outbound frames for a WebSocket connection (stdout data `ActionOutput`, status events `0x0002`, `0x0005`, resync markers `0x000c`/`0x000d`/`0x000f`) **MUST pass through a single, unified FIFO egress queue (`wsConnection.WriteFrame`) governed by a single-writer lock**.
   * **Deterministic Egress Guarantee**: Frame order written to the TCP wire is guaranteed to match the server's logical state transitions 100% deterministically with zero race conditions.
5. **`ActionSpawnStatus` (`0x0002`) Redundant Frame Suppression Rule**: For an actively running PTY (`TerminalStateRunning`), redundant `ActionSpawnStatus` frames (`Payload: [0x00]`) are **suppressed and NOT emitted during resync**. Re-emitting `0x00` on an already running tab is omitted because resync fast-forwards directly to `TerminalStateRunning` via `ActionSyncStart` (`0x000c`) $\rightarrow$ Hydration Stream $\rightarrow$ `ActionSyncComplete` (`0x000d`).
6. **Universal Status Frame Idempotency Rule**: Status lifecycle frames (`ActionSpawnStatus` `0x0002`, `ActionTerminalExit` `0x0005`, `ActionSyncFailed` `0x000f`) are **100% IDEMPOTENT** in the client UI state machine. Re-emitting a status frame during resync (e.g. re-emitting `ActionSpawnStatus [0x02]` on a PTY that is still spawning, or re-emitting `ActionTerminalExit [0x0005]` on an exited tab) produces zero visual flicker and ensures the client UI converges deterministically to the server's authoritative state.
7. **Errored Spawn Stream Suppression Rule (`TerminalStateError`)**: When resyncing a PTY that failed during `exec` (`TerminalStateError`), **the hydration history stream is skipped entirely**.
   * *Sequence*: `ActionSyncStart` (`0x000c`) $\rightarrow$ `ActionSyncFailed` (`0x000f`) (containing error payload). `ActionSyncComplete` (`0x000d`) is **SKIPPED** when `ActionSyncFailed` (`0x000f`) is emitted.
8. **Active Spawn Non-Blocking Resync Rule (`TerminalStateSpawning`)**: When resyncing a PTY currently in the middle of creation (`TerminalStateSpawning`), the server emits `ActionSyncStart` (`0x000c`) $\rightarrow$ `ActionSpawnStatus` (`0x0002`, `Payload: [0x02]`) $\rightarrow$ `ActionSyncComplete` (`0x000d`) instantly with 0 history bytes. When `fork/exec` completes live later on the server, it emits `ActionSpawnStatus` (`0x0002`, `Payload: [0x00]`) on success or `ActionSpawnStatus` (`0x0002`, `Payload: [0x01]`) on failure and begins live stdout streaming naturally.
9. **Non-Existent Terminal Failure Isolation Rule (Dropped Spawn Frames)**: If a client requests resync via `ActionResyncWorkspace` (`0x000e`) for a `TerminalID` that was dropped in transit pre-disconnect and never created on the server, the server immediately emits `ActionSyncStart` (`0x000c`) $\rightarrow$ `ActionSyncFailed` (`0x000f`) for that non-existent `TerminalID`. `ActionSyncComplete` (`0x000d`) is **SKIPPED** for that PTY, while hydration for all remaining valid PTYs in the request continues immediately without blocking or delay.
10. **Uniform Deferred `RemovePTY` Queueing Rule**: Client batch PTY removal (`ActionRemove` / `0x0006`) is one-directional, non-blocking, and treated as a uniform Class 2 Deferred Command. If `ActionRemove` arrives mid-hydration for a PTY, it is enqueued in that PTY's `pendingQueue` alongside stdin bytes, and executed via `workspace.RemovePTY()` immediately **AFTER `ActionSyncComplete` (`0x000d`)** is emitted.

#### Distinction Between Targeted PTY Commands vs New PTY Spawning During Resync:

It is vital to distinguish commands targeting an existing resyncing PTY from commands spawning a brand new PTY:

1. **Targeted Commands on Existing PTYs (`0x0003`, `0x0004`, `0x0006`, `0x0007`, `0x0009`)**:
   * *Target*: An existing `TerminalID` currently in `Hydrating` state (`ActionSyncStart` `0x000c` emitted, `ActionSyncComplete` `0x000d` pending).
   * *Semantics*: Commands are gated and enqueued in that PTY's `pendingQueue`. Class 1 (`ActionResize`, `ActionPrioritySync`) update geometry/priority in-flight; Class 2 (`ActionInput`, `ActionKill`, `ActionRemove`) flush to `masterFd` immediately **AFTER `ActionSyncComplete` (`0x000d`)** is emitted for that PTY.
2. **Spawning a Brand New PTY (`ActionSpawn` / `0x0001`)**:
   * *Target*: Creates a brand new PTY instance (`terminalID: N+1`) with zero historical data or descriptor binding to existing PTYs.
   * *Semantics*: `workspace.SpawnPTY()` executes `fork/exec` **asynchronously and concurrently in Go memory** in its own background Goroutine. It does NOT touch, wait for, or interfere with active resync passes. As soon as `fork/exec` completes (~5ms), `ActionSpawnStatus` (`0x0002`) is enqueued into `wsConnection.WriteFrame` and transmitted over the WebSocket.

#### Resync Status Frame Emission Summary Table:

| PTY Current State | `ActionSyncStart` (`0x000c`) | Hydration Stream | Status Frame Emitted | `ActionSyncComplete` (`0x000d`) | Resync Behavior |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **`TerminalStateRunning`** | Emitted | Emitted | **None** (No `0x0002 [0x00]`) | Emitted | Stream caught up to live |
| **`TerminalStateExited`** | Emitted | Emitted | **`ActionTerminalExit` (`0x0005`)** *after* history | Emitted | Renders final text + exit status |
| **`TerminalStateError`** | Emitted | **Skipped** | **`ActionSyncFailed` (`0x000f`)** | **Skipped** | Communicates error state |
| **`TerminalStateSpawning`**| Emitted | 0 bytes (instant) | **`ActionSpawnStatus` (`0x0002`, `[0x02]`)** | Emitted instantly | Non-blocking; live spawn finishes later |
| **`Non-Existent / Dropped`**| Emitted | **Skipped** | **`ActionSyncFailed` (`0x000f`)** | **Skipped** | Communicates failed/unborn state |

#### Targeted PTY Command Resync Classification Matrix:

All targeted per-PTY protocol commands fall into two distinct architectural classes during a resync hydration sequence:

| Opcode | Command Name | Resync Class | Processing & Flushing Semantics |
| :--- | :--- | :--- | :--- |
| **`0x000e`** | **`ActionResyncWorkspace`** | **Macro Trigger** | Inbound macro trigger starting resync. Cleans orphaned PTYs, sets initial target dimensions, applies priority weights, and initiates priority-sequenced hydration. |
| **`0x0003`** | **`ActionResize`** | **Class 1: Integrated** | Targeted PTY resize. Mid-hydration resizes update `pendingResize` slot. At end of history compilation, server executes `setSize()`, dispatches `SIGWINCH`, and flushes repaint bytes **BEFORE emitting `ActionSyncComplete` (`0x000d`)**. |
| **`0x0009`** | **`ActionPrioritySync`** | **Class 1: Integrated** | Priority weight updates. Mid-hydration priority updates adjust priority queues immediately, updating socket frame interleaving for in-flight streams. |
| **`0x0007`** | **`ActionInput`** (Stdin) | **Class 2: Deferred** | Targeted PTY stdin keystrokes. Enqueued in `pendingQueue`. Flushed to master PTY descriptor `masterFd.Write()` immediately **AFTER `ActionSyncComplete` (`0x000d`)**. |
| **`0x0004`** | **`ActionKill`** | **Class 2: Deferred** | Targeted process signal. Enqueued in `pendingQueue`. Dispatches `SIGKILL`/`SIGTERM` to process group immediately **AFTER `ActionSyncComplete` (`0x000d`)**. |
| **`0x0006`** | **`ActionRemove`** | **Class 2: Deferred** | Targeted PTY removal. Enqueued in `pendingQueue`. Destroys PTY instance and closes descriptors immediately **AFTER `ActionSyncComplete` (`0x000d`)**. |
| **`0x0001`** | **`ActionSpawn`** | **Async Concurrent** | Targeted PTY spawn request. Executed immediately in background Go Goroutine (`fork/exec`). As soon as spawn finishes, `ActionSpawnStatus` is enqueued into the single-writer FIFO egress queue and sent over the socket. |

*(Note: Workspace-wide macro commands such as `ActionReset` `0x000a` operate at the global workspace level and are scheduled for retirement via the `remove_ptys_refactor_proposal.md` refactor).*

#### Protocol Action Opcodes & Category Registry:

| Opcode | Protocol Action Name | Category | Direction | Payload & Purpose |
| :--- | :--- | :--- | :--- | :--- |
| **`0x0001`** | **`ActionSpawn`** | Live Creation | Client $\rightarrow$ Server | Inbound spawn request containing command & arguments. |
| **`0x0002`** | **`ActionSpawnStatus`** | Live Creation & Status | Server $\rightarrow$ Client | Outbound status event: `0x00` Success/Spawned, `0x01` Failure, `0x02` Spawning, `0x03` Canceled. |
| **`0x0005`** | **`ActionTerminalExit`** | Process Lifecycle | Server $\rightarrow$ Client | Outbound process exit event carrying exit code payload. |
| **`0x0006`** | **`ActionRemove`** | Live Destruction | Client $\rightarrow$ Server | Fire-and-forget inbound batch removal frame `[terminalID1, terminalID2]`. |
| **`0x000e`** | **`ActionResyncWorkspace`** | Resync Hydration | Client $\rightarrow$ Server | Inbound macro request `[{ terminalID, columns, rows, priority }]`. |
| **`0x000c`** | **`ActionSyncStart`** | Resync Hydration | Server $\rightarrow$ Client | Outbound status event. Hydration stream starting for terminal ID. |
| **`0x000d`** | **`ActionSyncComplete`** | Resync Hydration | Server $\rightarrow$ Client | Outbound status event. Hydration stream complete for terminal ID. |
| **`0x000f`** | **`ActionSyncFailed`** | Resync Hydration | Server $\rightarrow$ Client | Outbound status event. Resync failed for terminal ID. |

---

## 5. System Change & Impact Propagation Analysis

Implementing this specification introduces structural changes across the server codebase (`suprasole-server/source`) that propagate through PTY stream readers, memory management, lock hierarchies, transport handlers, priority queues, and Linux kernel syscall boundaries.

### A. PTY State Container & Concurrency Safety (`ptyInstance` & `activeLogMutex`)
* **Nature & Purpose**: Expands the core `ptyInstance` struct to hold the 512-byte carryover buffer (`carryover [512]byte`), unparsed fragment length (`carryoverLen int`), the active sequence array (`activeLog []Sequence`), a dedicated read/write mutex (`activeLogMutex sync.RWMutex`), and a per-PTY hydration pending queue (`pendingQueue []ActionFrame`, `pendingResize *ResizePayload`).
* **System Propagation**: Establishes thread-safety between background PTY stdout reader Goroutines (`startReadLoop`) and WebSocket takeover Goroutines (`compileReplayFramesLocked`). To prevent deadlocks, the system enforces a strict lock acquisition hierarchy: `workspace.mutex` **must always be acquired BEFORE** `ptyInstance.activeLogMutex`. On PTY process exit or termination (`handleProcessExit()`, `TerminatePTY()`, `RemovePTY()`, `teardown()`), unreferencing `ptyInstance` propagates memory release automatically to the Go garbage collector.

### B. Stream Reader, SIMD Fast-Path & 6-Handler Parser Pipeline (`startReadLoop`)
* **Nature & Purpose**: Refactors the PTY stdout read loop to scan outgoing bytes in real time with zero CPU overhead.
* **System Propagation**: Outgoing PTY stdout chunks pass through assembly-optimized SIMD `bytes.IndexByte(chunk, 0x1b)`. Chunks containing no escape bytes bypass parsing instantly ($O(1)$). When `0x1b` is detected, carryover bytes are prepended, and the sequence is dispatched to one of six segregated handlers. Handlers mutate `activeLog` under write-lock in real time, keeping active state setup codes updated 24/7 in memory. 100% of stdout bytes flow live to the client and history buffer without modification or delay; handlers observe bytes as a passive side-effect. If an unparsed fragment at chunk end exceeds 512 bytes, `carryoverLen` resets to 0 with zero memory allocation.

### C. In-Place Scrollback & State Purging (`ringBuffer.Clear()`)
* **Nature & Purpose**: Equips the circular 256KB history buffer with a thread-safe `Clear()` method to support history and full terminal reset purges.
* **System Propagation**: When Handler 6 parses an Erase Saved Lines sequence (`\x1b[3J`), it calls `ringBuffer.Clear()`, truncating historical scrollback in-place while preserving active `activeLog` theme and mode configurations. When Handler 6 parses a Full Reset sequence (`RIS` — `\x1bc`), it empties `activeLog` (`activeLog = activeLog[:0]`) **AND** invokes `ringBuffer.Clear()`.

### D. Pre-Resync Command Gating & Resync Macro Handler (`ActionResyncWorkspace` $\rightarrow$ `PerformReplayTakeover`)
* **Nature & Purpose**: Handles the inbound `ActionResyncWorkspace` (`0x000e`) macro control frame, enforcing pre-resync command gating, atomic preemption of in-flight resyncs, applying priority weights, pre-resizing target PTYs, and garbage-collecting orphaned PTYs before compiling history.
* **System Propagation**: `network.go` enforces pre-resync command gating, rejecting operational frames prior to `0x000e`. Upon receiving `0x000e`, `network.go` routes to `PerformReplayTakeover()`. If `0x000e` contains `terminals: []`, all active PTYs in Go memory are destroyed cleanly via `workspace.RemovePTY()`. If an in-flight resync loop is active, `0x000e` preempts and restarts clean hydration atomically. The server pauses `read(masterFd)` for all requested PTYs upfront (idempotently). The server compares requested PTY IDs against `workspace.terminals`, invoking `workspace.RemovePTY()` on omitted PTYs to self-heal lost `ActionRemove` (`0x0006`) frames. For active PTYs, the server applies priority weights (`SyncPTYPriorities`) and resizes master PTY descriptors via `setSize()` *before* compiling replay frames. If `ActionSpawn` (`0x0001`) arrives, the server executes `workspace.SpawnPTY()` asynchronously in Go memory without waiting for resync passes to finish.

### E. Extended Single-Pass Hydration Replay Compiler (`compileReplayFramesLocked`)
* **Nature & Purpose**: Compiles baseline resets, active state sequences, sanitized history bytes, mid-hydration pending resizes, and status lifecycle markers into deterministic hydration payloads per PTY.
* **System Propagation**: Fast-forwards to current PTY state. Generates `ActionSyncStart` (`0x000c`). For running/exited PTYs, read-locks `activeLogMutex`, prepends `\x1b[!p` Soft Reset baseline + default-active modes (Auto-Wrap Mode 7 `\x1b[?7h` and Cursor Visibility `\x1b[?25h`) + `activeLog` sequence bytes in natural insertion order, trims the 256KB history buffer from its head to the first newline (`\n`), and executes process group signaling (`TIOCGPGRP`). At the end of history compilation, if a mid-hydration `ActionResize` arrived in `pendingResize`, it applies `setSize()`, fires `SIGWINCH`, and flushes repaint bytes **BEFORE** emitting `ActionSyncComplete` (`0x000d`). For errored or non-existent PTYs, it emits `ActionSyncFailed` (`0x000f`) in place of `ActionSyncComplete` (`0x000d`), skipping history compilation entirely. After `0x000d` (or `0x000f`) is emitted, it resumes `read(masterFd)` for that PTY, flushes any pending `ActionInput` or `ActionRemove` frames to the master PTY, and unlocks live streaming.

### F. Process Group Repaint Signaling & Syscall Safety (`TIOCGPGRP` & `SIGWINCH`)
* **Nature & Purpose**: Triggers active foreground TUI applications (`vim`, `htop`, `tmux`) to self-healingly repaint their viewports upon client connection.
* **System Propagation**: `compileReplayFramesLocked` queries the PTY master file descriptor for the active foreground process group ID via `ioctl(masterFd, TIOCGPGRP, &pgid)` and dispatches `syscall.Kill(-pgid, SIGWINCH)`. If `ioctl(masterFd, TIOCGPGRP, &pgid)` fails (e.g. process group uninitialized or slave closing), the server catches the error silently, skips `SIGWINCH`, and continues history compilation without failing the resync sequence.

### G. Priority Queue Control Frame Routing & Single-Writer Egress Serialization (`wsConnection.WriteFrame`)
* **Nature & Purpose**: Refactors WebSocket egress from legacy un-synchronized dual-channel queues into a single-writer FIFO serialization queue (`wsConnection.WriteFrame`) with 5-second socket write deadlines.
* **System Propagation**: Registers opcodes `ActionResyncWorkspace = 0x000e`, `ActionSyncStart = 0x000c`, `ActionSyncComplete = 0x000d`, `ActionSyncFailed = 0x000f`, `ActionSpawnStatus = 0x0002`, and `ActionTerminalExit = 0x0005` in the protocol action registry. Outbound frames pass through a single unified FIFO queue, ensuring control frames (`ActionSpawnStatus`, `ActionTerminalExit`, `0x000c`, `0x000d`) never overtake data frames out of order on the TCP wire. Control frame writes enforce a 5-second socket write deadline to prevent stalled TCP sockets from blocking server takeover Goroutines.

---

## 6. Miscellaneous Transport Refactor: Parameterless WebSocket Endpoint (`/ws`)

As a local single-tenant server for a single developer, `suprasole-server` simplifies its HTTP transport by exposing a clean, parameterless WebSocket endpoint (`/ws`). 

* **Purged URL Parameters**: All legacy `?token=...` or `?workspaceId=...` URL query parameters are purged from `source/network.go`.
* **Single-Tenant Workspace Binding**: Connecting to `/ws` upgrades directly to the single active developer workspace instance in Go memory (`GetOrCreateWorkspace()`).
* **Clean Surface Area**: Simplifies the client-side network transport layer by eliminating dummy token generation.
