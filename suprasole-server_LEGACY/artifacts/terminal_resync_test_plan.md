# Comprehensive Test Specification & Strategy Plan: Terminal Resync & Healing Protocol

This document defines the complete, unified test specification and execution plan for the **Lightweight Terminal Resync & Self-Healing Protocol** in `suprasole-server`.

Testing is structured across three distinct tiers (Unit, Integration, and End-to-End) and categorized by action type (**New Tests**, **Refactored/Updated Tests**, and **Retired/Deleted Tests**).

---

## 1. Testing Architecture & Execution Hierarchy

```
                       +-----------------------------------+
                       |      Tier 3: E2E Tests            |  (Real WebSocket / PTY Process Takeover)
                       +-----------------------------------+
                       |    Tier 2: Integration Tests      |  (Gating, Pending Queues, Orphan Cleanup)
                       +-----------------------------------+
                       |      Tier 1: Go Unit Tests        |  (6 Handlers, activeLog, Carryover, 3J)
                       +-----------------------------------+
```

1. **Tier 1 (Go Unit Tests — `source/pty_internal_test.go`)**: Fast-path in-memory verification (< 5ms execution) of stream parsers, carryover buffer protection, sequence deduplication, newline sanitization, and state reset semantics.
2. **Tier 2 (Go Integration Tests — `source/network_internal_test.go` & `source/pty_internal_test.go`)**: Verification of protocol command gating, in-flight hydration pending queues, upfront workspace PTY read-pause OS backpressure, idempotent pause/resume operations, single-writer FIFO egress serialization, 5-second socket write deadlines, extended hydration boundary flushes, priority-sequenced hydration scheduling, fast-forward spawning state transitions, non-existent ID failure isolation, and automatic orphan PTY garbage collection.
3. **Tier 3 (End-to-End Tests — `source/e2e_test.go`)**: Complete end-to-end network socket verification over the parameterless `/ws` endpoint, priority-sequenced hydration, process group `SIGWINCH` self-healing repaints, global terminal exit ordering (`0x0005` follows all stdout), 512-byte carryover split chunks, high-throughput SIMD stream floods (100MB), resync preemption, socket deadlines, and event-driven client UI tab overlay unlocking.

---

## 2. Tier 1: Go Unit Tests (`source/pty_internal_test.go`)

Unit tests execute in memory without spawning external OS processes or opening network sockets.

### A. New Unit Tests to Create

* **`TestParserHandler1_ModeBitsets`**:
  * Verify parsing of DEC Private and ANSI Standard mode enable/disable sequences (`CSI [?] N h/l`) across all 8 mode types:
    1. Alternate Screen Buffer (`\x1b[?1049h / l`)
    2. Bracketed Paste Mode (`\x1b[?2004h / l`)
    3. Focus Reporting (`\x1b[?1004h / l`)
    4. Auto-Wrap Mode (`\x1b[?7h / l`)
    5. Cursor Visibility (`\x1b[?25h / l`)
    6. Application Cursor Keys (`\x1b[?1h / l`)
    7. Mouse Tracking Modes (`\x1b[?1000h / 1002h / 1003h / 1006h / l`)
    8. Insert Mode (`\x1b[4h / l`)
  * Verify in-place updating of mode entries in `activeLog []Sequence`.
  * Verify disable sequences (e.g. `\x1b[?1049l`) cleanly remove corresponding entries from `activeLog`.
* **`TestParserHandler2_GenericOSC`**:
  * Verify parsing of string parameters by command number $N$: Window Titles (`OSC 0/2`), Current Working Directory (`OSC 7`), Theme Colors (`OSC 10/11/12`), and Shell Integration (`OSC 133`).
  * Verify support for BOTH string terminators: `BEL` (`\x07`) and `ST` (`\x1b\\`).
  * Verify in-place value updates when titles or CWD change.
* **`TestParserHandler3_OSCPalette`**:
  * Verify parsing of 256-color palette overrides (`OSC 4;Idx;Val ST`) in both `#rrggbb` and `rgb:r/g/b` formats.
  * Verify query responses containing `?` are discarded without mutating `activeLog`.
* **`TestParserHandler4_ISO2022`**:
  * Verify parsing of G0–G3 character set designations (`Esc ( X`) and Shift-Out (`0x0f`) / Shift-In (`0x0e`) states for `ncurses` box-drawing support.
* **`TestParserHandler5_StructuralParameters`**:
  * Verify parsing of multi-argument grid parameters: Scroll Margins (`DECSTBM` — `r`), Cursor Style (`DECSCUSR` — `q`), and Keypad Mode (`DECKPAM` — `=`).
  * Verify empty parameter scroll margin resets (`\x1b[r`) cleanly clear scroll region restrictions in `activeLog`.
* **`TestParserHandler6_ResetSemantics`**:
  * **Soft Reset (`DECSTR` — `\x1b[!p`)**: Verify `CSI` mode and structural sequences are pruned from `activeLog`, default-active modes (**Auto-Wrap Mode 7** and **Cursor Visibility Mode 25**) are restored to enabled, and **all `OSC` titles, CWDs, and color themes are strictly preserved**.
  * **Full Reset (`RIS` — `ESC c` / `\x1bc`)**: Verify `activeLog` is completely emptied (`activeLog = activeLog[:0]`) AND `ringBuffer.Clear()` is invoked.
  * **Erase Saved Lines (`ED 3` — `\x1b[3J`)**: Verify `ringBuffer.Clear()` is invoked to truncate scrollback history while `activeLog` sequence entries remain 100% intact.
  * **Erase Display (`ED 2` — `\x1b[2J`)**: Verify `ringBuffer` is **NOT** cleared when `\x1b[2J` is detected.
  * **`DA` Query Isolation (`CSI c`)**: Verify Device Attribute queries (`CSI c`) are strictly distinguished from `ESC c` (`RIS`) and do **NOT** clear `activeLog` or history.
  * **Tab Stop Restoration**: Verify Soft/Full reset restores standard 8-column tab stops.
* **`TestCarryoverBuffer_SplitChunkProtection`**:
  * Verify that long escape sequences (e.g. `OSC 7` file URLs or `OSC 2` window titles) split across chunk boundaries are stitched correctly using `carryover [512]byte` without sequence loss.
  * Verify that if an unparsed fragment at chunk end exceeds 512 bytes, `carryoverLen` resets to 0 with zero memory corruption or buffer overflow.
* **`TestHeadOfBuffer_NewlineSanitization`**:
  * Verify that when compiling history from a wrapped 256KB ring buffer, the server forward-scans to the first newline (`\n`) byte and trims all preceding partial ANSI or split UTF-8 character bytes.
* **`TestSIMDScanner_FastPath`**:
  * Verify that stdout chunks containing no escape byte (`0x1b`) bypass parsing instantly via assembly `bytes.IndexByte` with zero heap allocation.

### B. Refactored / Updated Unit Tests

* **`TestPTYInstance_StateInitialization`**: Update `ptyInstance` struct allocation tests to verify initialization of `activeLog []Sequence`, `carryover [512]byte`, and `activeLogMutex sync.RWMutex`.
* **`TestPTYInstance_ThreadSafety`**: Update concurrency tests to verify the lock acquisition hierarchy (`workspace.mutex` acquired BEFORE `ptyInstance.activeLogMutex`).

---

## 3. Tier 2: Go Integration Tests (`source/network_internal_test.go` & `source/pty_internal_test.go`)

Integration tests verify multi-component interactions, protocol command gating, priority queues, and lifecycle state transitions.

### A. New Integration Tests to Create

* **`TestGlobalTerminalExitOrdering_AllStdoutFlushedFirst`**:
  * **Preserves Established Server Behavior (`core.go` L648–653)**: Verify that `ActionTerminalExit` (`0x0005`) is strictly guaranteed to be emitted AFTER 100% of stdout bytes have been read from `masterFd` (`<-terminal.readDone`) AND flushed to the network socket (`pendingCount == 0`), both during live streaming and resync hydration.
* **`TestLiveSpawnStatusOrdering_SpawnStatusPrecedesStdout`**:
  * Verify that on live PTY creation (`ActionSpawn` `0x0001`), `ActionSpawnStatus` (`0x0002`, `Payload: [0x00]`) is written to the network egress queue **BEFORE** `startReadLoop()` is launched or any `ActionOutput` (`0x0008`) stdout bytes are emitted for that PTY.
* **`TestSingleWriterFifoEgressSerialization_DeterministicFrameOrder`**:
  * Verify that all outbound frames (stdout data `ActionOutput`, status events `0x0002`, `0x0005`, resync markers `0x000c`/`0x000d`/`0x000f`) pass through a single, unified FIFO egress queue (`wsConnection.WriteFrame`) under a single-writer lock.
  * Verify zero out-of-order frame overtakes on the TCP wire.
* **`TestUpfrontWorkspacePTYReadPause_OSBackpressure`**:
  * Verify that receiving `ActionResyncWorkspace` (`0x000e`) pauses `read(masterFd)` for ALL requested PTYs upfront.
  * Verify zero new bytes enter or evict 256KB ring buffers during workspace-wide hydration compilation.
  * Verify `read(masterFd)` resumes immediately for each PTY post-`0x000d` and live streaming takes over cleanly.
  * Verify that `PauseRead()` and `ResumeRead()` are **100% idempotent** (calling `PauseRead()` or `ResumeRead()` repeatedly produces no race conditions or deadlocks).
* **`TestControlFrame_WriteDeadline`**:
  * Simulate a completely un-drained, stalled TCP socket during resync hydration.
  * Verify that control frame socket writes (`0x000c`, `0x000d`, `0x000f`) enforce a 5-second write deadline, closing the stalled socket gracefully without hanging Go takeover Goroutines.
* **`TestPrioritySequencedHydration_FocusedTabFirst`**:
  * Send `ActionResyncWorkspace` (`0x000e`) specifying PTY 1 (`priority: 1` focused tab) and PTY 2 (`priority: 2` background tab).
  * Verify the server takeover compiler compiles and flushes PTY 1 hydration frames (`0x000c` $\rightarrow$ history $\rightarrow$ `0x000d`) **BEFORE** starting PTY 2 hydration.
* **`TestCommandGating_PreResyncRejection`**:
  * Connect over `/ws` and attempt to send operational commands (`ActionInput` `0x0007`, `ActionResize` `0x0003`, `ActionSpawn` `0x0001`, `ActionKill` `0x0004`, `ActionRemove` `0x0006`, `ActionPrioritySync` `0x0009`) **BEFORE** sending `ActionResyncWorkspace` (`0x000e`).
  * Verify the server rejects/drops ALL pre-resync operational frames.
* **`TestInFlightHydrationQueueing`**:
  * Send `ActionResyncWorkspace` (`0x000e`) for a PTY. While the PTY is in `Hydrating` state (`ActionSyncStart` `0x000c` emitted, `ActionSyncComplete` `0x000d` pending), send operational commands (`ActionInput`, `ActionKill`, `ActionRemove`).
  * Verify operational frames are enqueued in `pendingQueue` and **NOT** written to the master PTY descriptor during history compilation.
  * Verify that immediately after `ActionSyncComplete` (`0x000d`) is emitted, the server flushes `pendingQueue` and delivers the frames in strict FIFO order.
* **`TestExtendedHydrationBoundary_MidHydrationResize`**:
  * Send `ActionResyncWorkspace` (`0x000e`) specifying target geometry `120x40`.
  * Send a mid-hydration `ActionResize` (`0x0003`) specifying `160x60` while history compilation is in-flight.
  * Verify `ActionResize` updates `pendingResize` slot.
  * Verify that at the end of history compilation, the server executes `setSize(160, 60)`, dispatches `SIGWINCH`, and flushes 160x60 repaint bytes **BEFORE emitting `ActionSyncComplete` (`0x000d`)**.
* **`TestAutomaticOrphanGarbageCollection`**:
  * Spawn PTY 1 and PTY 2.
  * Send `ActionResyncWorkspace` (`0x000e`) containing only `[PTY 1]`, omitting PTY 2.
  * Verify the server recognizes PTY 2 as an orphaned tab and automatically invokes `workspace.RemovePTY(2)`, closing its master file descriptor and deleting it from memory.
  * Verify that sending `ActionResyncWorkspace` (`0x000e`) carrying `terminals: []` destroys all active PTYs in Go memory.
* **`TestNonExistentTerminal_SyncFailedIsolation`**:
  * Send `ActionResyncWorkspace` (`0x000e`) containing valid PTY 1 and non-existent PTY 999.
  * Verify server emits `ActionSyncStart` (`0x000c`) $\rightarrow$ `ActionSyncFailed` (`0x000f`) carrying error payload for PTY 999.
  * Verify hydration for PTY 1 continues cleanly without blocking or failing the macro resync pass.
* **`TestResyncPreemption_RapidReRequest`**:
  * Send `ActionResyncWorkspace` (`0x000e`). While history compilation is in-flight, immediately send a second `ActionResyncWorkspace` (`0x000e`).
  * Verify the server preempts/cancels the in-flight hydration loop, applies Pass B's upfront read-pause (idempotently), and restarts clean priority-sequenced hydration without leaking un-paused states.
* **`TestFastForwardSpawningState_Matrix`**:
  * **Running PTY (`TerminalStateRunning`)**: Verify sequence: `ActionSyncStart` (`0x000c`) $\rightarrow$ Hydration Stream $\rightarrow$ `ActionSyncComplete` (`0x000d`), verifying redundant `ActionSpawnStatus` (`0x0002`, `[0x00]`) is suppressed.
  * **Exited PTY (`TerminalStateExited`)**: Verify sequence: `ActionSyncStart` (`0x000c`) $\rightarrow$ Final Scrollback History $\rightarrow$ `ActionTerminalExit` (`0x0005`) $\rightarrow$ `ActionSyncComplete` (`0x000d`), verifying `0x0005` strictly follows stdout bytes.
  * **Errored PTY (`TerminalStateError`)**: Verify sequence: `ActionSyncStart` (`0x000c`) $\rightarrow$ `ActionSyncFailed` (`0x000f`), verifying 0 history bytes are compiled or written.
  * **Spawning PTY (`TerminalStateSpawning`)**: Verify sequence: `ActionSyncStart` (`0x000c`) $\rightarrow$ `ActionSpawnStatus` (`0x0002`, `[0x02]`) $\rightarrow$ `ActionSyncComplete` (`0x000d`) instantly with 0 history bytes.
  * **Non-Existent PTY**: Verify sequence: `ActionSyncStart` (`0x000c`) $\rightarrow$ `ActionSyncFailed` (`0x000f`), verifying hydration continues immediately for remaining valid PTYs in request.
* **`TestUniformDeferredRemovePTY`**:
  * Send batch `ActionRemove` (`0x0006`) payload `[terminalID1, terminalID2]` mid-hydration.
  * Verify batch `ActionRemove` is enqueued in `pendingQueue` and executed via `workspace.RemovePTY()` for both PTYs immediately after `ActionSyncComplete` (`0x000d`) is emitted.

### B. Refactored / Updated Integration Tests

* **`TestPerformReplayTakeover_AtomicBinding`**: Update `PerformReplayTakeover` tests in `network_internal_test.go` and `pty_internal_test.go` to use the `ActionResyncWorkspace` (`0x000e`) macro handler rather than legacy automatic replay triggers.

### C. Retired / Deleted Integration Tests

* **Delete Legacy URL Query Geometry Tests**: Remove tests checking `?cols=X&rows=Y` URL query parameters on WebSocket upgrade.
* **Delete Legacy Token Query Tests**: Remove tests checking mandatory `?token=...` URL query parameters.

---

## 4. Tier 3: Comprehensive End-to-End (E2E) Test Specifications

All Tier 3 E2E test scenarios validate complete network socket verification over the parameterless `/ws` endpoint, priority-sequenced hydration, process group `SIGWINCH` self-healing repaints, and event-driven client UI tab overlay unlocking.

### A. State Tracking & Prepend Verification Scenarios

* **Scenario 3.1: Alternate Screen Buffer Restoration & Eviction Recovery**:
  * **Setup**: Spawn an active terminal session. Execute an interactive application (`vim` or `htop`) that emits the alternate screen enable sequence (`\x1b[?1049h`). Flood the output stream with text bytes until the initial `1049h` sequence is evicted from the 256KB ring buffer.
  * **Execution**: Disconnect the WebSocket client and reconnect over parameterless `/ws`. Send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. The server's `activeLog` array maintains the active `\x1b[?1049h` sequence entry despite ring buffer eviction.
    2. The prepended hydration stream delivered to the client starts with `\x1b[!p` followed by `\x1b[?1049h`.
    3. The client terminal engine transitions to the alternate screen buffer before processing replayed history bytes.
* **Scenario 3.2: Bracketed Paste Mode Security Preservation**:
  * **Setup**: Spawn a terminal session. Emit Bracketed Paste enable (`\x1b[?2004h`). Evict the sequence by filling the 256KB buffer.
  * **Execution**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`). Simulate a multi-line clipboard paste action.
  * **Validation Assertions**:
    1. The server prepends `\x1b[?2004h` to the reconnection stream from `activeLog`.
    2. The client terminal ingests the prepended sequence and wraps the pasted multi-line text inside `\x1b[200~` and `\x1b[201~` markers.
    3. The shell does not execute intermediate lines automatically.
* **Scenario 3.3: Terminal Soft Reset State Sanitization (DECSTR / RIS)**:
  * **Setup**: Spawn a terminal session. Enable alternate screen (`1049h`), hide cursor (`25l`), and set mouse tracking (`1000h`). Emit custom Window Title (`OSC 2`) and CWD (`OSC 7`). Immediately follow with a Soft Reset sequence (`\x1b[!p`).
  * **Execution**: Inspect the server's `activeLog` array for the terminal and reconnect client over `/ws`.
  * **Validation Assertions**:
    1. `activeLog` prunes `CSI` mode entries (alt-screen, mouse, focus) while preserving default-active modes (**Auto-Wrap Mode 7** and **Cursor Visibility Mode 25**).
    2. `activeLog` **STRICTLY PRESERVES** `OSC 2` Window Title and `OSC 7` Current Working Directory sequences intact.
    3. Reconnecting a client delivers a clean hydration stream restoring title and CWD setup codes with zero stale mode prepends.
* **Scenario 3.4: OSC Window Title & Custom Palette Theme Restoration**:
  * **Setup**: Spawn a terminal session. Emit custom Window Title (`\x1b]2;My Custom Tab Title\x07`) and background theme color (`\x1b]11;#1e1e2e\x07`). Evict the sequences from the 256KB buffer.
  * **Execution**: Reconnect a client session over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. `activeLog` maintains `\x1b]2;My Custom Tab Title\x07` and `\x1b]11;#1e1e2e\x07`.
    2. The prepended hydration stream contains both `OSC` sequences in natural insertion order.
    3. The client tab header and background theme update immediately upon connecting.

### B. Head-of-Buffer Newline Sanitization Scenarios

* **Scenario 3.5: Leading Truncated ANSI Sequence Trimming**:
  * **Setup**: Fill a terminal's ring buffer past 256KB such that the eviction boundary cuts off the first byte of a multi-byte color sequence (e.g. `\x1b[38;2;255;` is evicted, leaving `128;64mHello\n` at the head of the buffer).
  * **Execution**: Send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. The server's head-sanitization routine scans forward to the first newline (`\n`).
    2. The trailing fragment `128;64mHello` is trimmed from the replayed history payload.
    3. The replayed stream begins cleanly at the start of a fresh line with zero partial sequence text artifacts.

### C. Signal Routing & Self-Healing Redraw Scenarios

* **Scenario 3.6: Foreground Process Group PGID Resolution (`TIOCGPGRP`)**:
  * **Setup**: Spawn a terminal session running a shell (`bash`). From the shell, execute an interactive TUI (`vim`).
  * **Execution**: Send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. The server queries the master PTY descriptor via `ioctl(masterFd, TIOCGPGRP, &pgid)` and resolves `vim`'s process group ID (PGID), not `bash`'s PID.
    2. The server delivers `SIGWINCH` directly to `vim`'s PGID.
    3. `vim` catches the signal and emits a full-screen viewport repaint stream over the WebSocket.
    4. If `ioctl(masterFd, TIOCGPGRP, &pgid)` fails, the server catches the error silently, skips `SIGWINCH`, and continues history compilation without failing the resync sequence.
* **Scenario 3.7: Macro Handshake Pre-Resizing (Zero Double-Repaint)**:
  * **Setup**: Establish a terminal running at 80x24. Reconnect a client and send `ActionResyncWorkspace` (`0x000e`) specifying target geometry `columns=120, rows=40`.
  * **Execution**: Monitor PTY master `ioctl` calls and outgoing stdout frames during the connection handshake.
  * **Validation Assertions**:
    1. The server resizes the PTY master to 120x40 *before* writing history or issuing `SIGWINCH`.
    2. The foreground process receives a single `SIGWINCH` at 120x40.
    3. Exactly one full-screen repaint is delivered to the client, verifying zero intermediate 80x24 layout jumps.
* **Scenario 3.8: Priority-Sequenced Asynchronous Repaint Stream Verification**:
  * **Setup**: Establish a workspace with 2 terminals: Terminal 1 (focused tab, `priority: 1`, high-latency SSH) and Terminal 2 (background tab, `priority: 2`).
  * **Execution**: Send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. Terminal 1 receives hydration bytes, `SIGWINCH`, and `ActionSyncComplete` (`0x000d`) first.
    2. The client parses `0x000d` for Terminal 1 and unlocks the focused tab overlay immediately upon ingesting the history buffer, without hanging for network RTT.
    3. When remote SSH repaint bytes arrive 100ms later over the WebSocket, `xterm.js` ingests them as standard live stream frames, confirming the unified asynchronous repaint model.

### D. Multi-Terminal Boundary Completion & Self-Healing Scenarios

* **Scenario 3.9: Granular Per-PTY `ActionSyncComplete` (`0x000d`) Framing**:
  * **Setup**: Establish a workspace with 3 active terminals: Terminal 1 (`bash`), Terminal 2 (`htop`), and Terminal 3 (`spawning`).
  * **Execution**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. Server writes history, fires `SIGWINCH`, and appends `ActionSyncComplete` (`0x000d`) carrying `TerminalID = 1`.
    2. Server writes history, fires `SIGWINCH`, and appends `ActionSyncComplete` (`0x000d`) carrying `TerminalID = 2`.
    3. Server emits `ActionSpawnStatus` (`0x0002`, `[0x02]`) and appends `ActionSyncComplete` (`0x000d`) carrying `TerminalID = 3` instantly.
    4. Client core unlocks each terminal tab independently as its specific `0x000d` frame is parsed, verifying progressive hydration.
* **Scenario 3.10: Historical Toggle Sequence Convergence**:
  * **Setup**: Within a single 256KB buffer window, execute `vim` (emits `1049h`), exit `vim` (emits `1049l`), execute `htop` (emits `1049h`), exit `htop` (emits `1049l`), and finally execute `neovim` (emits `1049h`).
  * **Execution**: Reconnect the client and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. The history stream replays all historical open/close sequences in exact chronological order.
    2. The final sequence parsed by the client is `neovim`'s active `1049h`.
    3. The terminal viewport converges to `neovim`'s active alternate screen without layout leaks or frozen frames.

### E. Protocol Command Gating, In-Flight Queueing & Failure Recovery Scenarios

* **Scenario 3.11: Pre-Resync Command Gating E2E Enforcement**:
  * **Setup**: Upgrade WebSocket connection over `/ws`. Do NOT send `ActionResyncWorkspace` (`0x000e`).
  * **Execution**: Immediately send `ActionInput` (`0x0007`) or `ActionResize` (`0x0003`).
  * **Validation Assertions**:
    1. Server drops/rejects the pre-resync operational frames.
    2. Server connection remains in `Unsynced` state until `ActionResyncWorkspace` (`0x000e`) is received.
* **Scenario 3.12: Extended Hydration Boundary Mid-Sync Resize Verification**:
  * **Setup**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`) specifying target geometry `120x40`.
  * **Execution**: While Terminal 1 history is actively streaming over TCP, send `ActionResize` (`0x0003`) specifying `160x60`.
  * **Validation Assertions**:
    1. Server updates `pendingResize` slot without interrupting the 120x40 history stream compilation.
    2. At the end of history compilation, server executes `setSize(160, 60)`, dispatches `SIGWINCH`, and flushes 160x60 repaint bytes **BEFORE emitting `ActionSyncComplete` (`0x000d`)**.
    3. Client parses `0x000d` and unlocks the tab overlay with the active TUI application (`vim`) ALREADY repainted to 160x60 with zero post-unlock layout shifts.
* **Scenario 3.13: Mid-Hydration Stdin & Removal Pending Queue Verification**:
  * **Setup**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Execution**: While Terminal 1 is hydrating, send `ActionInput` (`0x0007`) containing `"ls -la\n"` followed by `"clear\n"`. While Terminal 2 is hydrating, send batch `ActionRemove` (`0x0006`) payload `[2]`.
  * **Validation Assertions**:
    1. Keystrokes `"ls -la\n"` and `"clear\n"` are enqueued in Terminal 1's `pendingQueue` in **STRICT CHRONOLOGICAL FIFO ORDER** and NOT delivered to the master PTY during history compilation.
    2. Immediately after `ActionSyncComplete` (`0x000d`) is emitted for Terminal 1, server flushes `"ls -la\n"` then `"clear\n"` to `masterFd.Write()`, and live output streams cleanly.
    3. `ActionRemove` payload `[2]` is enqueued in Terminal 2's `pendingQueue` and executed via `workspace.RemovePTY(2)` immediately after Terminal 2's `ActionSyncComplete` (`0x000d`) is emitted, destroying Terminal 2 cleanly.
* **Scenario 3.14: Automatic Orphan PTY Garbage Collection E2E Recovery**:
  * **Setup**: Workspace contains Terminal 1 and Terminal 2. Client closes Terminal 2 offline.
  * **Execution**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`) carrying `[Terminal 1]`, omitting Terminal 2.
  * **Validation Assertions**:
    1. Server compares requested PTY list against `workspace.terminals`.
    2. Server automatically invokes `workspace.RemovePTY(2)`, destroying Terminal 2 in Go memory, closing its master file descriptor (`masterFd`), and terminating its reader Goroutine.
    3. Server hydrates Terminal 1 cleanly. Client UI mounts Terminal 1 with zero ghost tabs.
* **Scenario 3.15: Fast-Forward Spawning State Matrix & Live Handover E2E Verification**:
  * **Setup**: Workspace contains 5 PTYs in different states: Terminal 1 (`Running`), Terminal 2 (`Exited`), Terminal 3 (`Error`), Terminal 4 (`Spawning`), and Terminal 5 (non-existent/dropped spawn).
  * **Execution**: Send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. Terminal 1: Emits `ActionSyncStart` $\rightarrow$ Hydration Stream $\rightarrow$ `ActionSyncComplete` (suppresses redundant `ActionSpawnStatus` `0x0002 [0x00]`).
    2. Terminal 2: Emits `ActionSyncStart` $\rightarrow$ Final History $\rightarrow$ `ActionTerminalExit` (`0x0005`) $\rightarrow$ `ActionSyncComplete`, verifying `0x0005` strictly follows stdout bytes (preserving established `core.go` L648-653 behavior).
    3. Terminal 3: Emits `ActionSyncStart` $\rightarrow$ `ActionSyncFailed` (`0x000f`) (skips history; 0 history bytes compiled or written).
    4. Terminal 4: Emits `ActionSyncStart` $\rightarrow$ `ActionSpawnStatus` (`0x0002`, `[0x02]`) $\rightarrow$ `ActionSyncComplete` instantly with 0 history bytes. When `fork/exec` completes 50ms later on the server, `ActionSpawnStatus` (`0x0002`, `[0x00]`) is emitted live over the socket BEFORE launching `startReadLoop()` or emitting stdout bytes.
    5. Terminal 5: Emits `ActionSyncStart` $\rightarrow$ `ActionSyncFailed` (`0x000f`) for non-existent ID, continuing hydration for Terminals 1–4 cleanly.
* **Scenario 3.16: Full Reset (`RIS` `\x1bc`) Dual-Storage Purge Verification**:
  * **Setup**: Spawn terminal session. Execute `RIS` (`\x1bc`).
  * **Execution**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. Server emptied `activeLog = activeLog[:0]` AND invoked `ringBuffer.Clear()`.
    2. Client receives clean baseline hydration stream with 0 history bytes.
    3. Baseline setup codes explicitly re-enable default-active modes (**Auto-Wrap Mode 7** and **Cursor Visibility Mode 25**).
* **Scenario 3.17: Scrollback Purge (`ED 3` `\x1b[3J`) History Truncation Verification**:
  * **Setup**: Spawn terminal session with custom window title and background theme. Execute `clear` (`\x1b[3J`).
  * **Execution**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. Server invoked `ringBuffer.Clear()`, truncating historical scrollback bytes.
    2. `activeLog` maintained custom title and theme color sequences intact.
    3. Reconnecting client receives custom title and theme color setup codes, followed by 0 history bytes.

### F. Grammar Handler & Command Class Exhaustive Scenarios

* **Scenario 3.18: Split-Chunk Sequence Protection E2E (512-Byte Carryover Buffer)**:
  * **Setup**: Stream a long $200+$ byte escape sequence (e.g. `OSC 7` working directory path `\x1b]7;file://hostname/home/coder/project/deeply/nested/directory/structure\x07`) split right across two OS `read()` chunk boundaries during high-volume stdout floods.
  * **Execution**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. Server 512-byte carryover buffer stitches the long sequence fragments across read boundaries seamlessly without path truncation.
    2. `activeLog` stores `OSC 7` working directory path intact.
    3. Reconnecting client receives `OSC 7` in its baseline setup stream and restores working directory tab indicators.
* **Scenario 3.19: OSC 7 Working Directory & OSC 133 Shell Integration Reconnect Recovery**:
  * **Setup**: Execute shell commands emitting `OSC 7` (`\x1b]7;file://hostname/home/coder/project\x07`) and `OSC 133` shell prompt integration markers. Evict sequences from ring buffer.
  * **Execution**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. `activeLog` maintains `OSC 7` working directory and `OSC 133` prompt markers.
    2. Hydration stream delivers `OSC 7` and `OSC 133` in baseline setup.
    3. Client UI tab breadcrumbs and shell prompt indicators update immediately on reconnect.
* **Scenario 3.20: OSC 4 256-Color Palette Override & Query Response Filter E2E**:
  * **Setup**: Emit `OSC 4` 256-color palette overrides (`\x1b]4;1;rgb:ff/00/00\x07`) alongside query responses (`\x1b]4;1;?\x07`). Evict from history.
  * **Execution**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. `activeLog` stores color palette override (`1 -> #ff0000`).
    2. Query responses containing `?` are discarded without polluting `activeLog`.
    3. Reconnecting client receives `OSC 4` palette overrides in baseline setup.
* **Scenario 3.21: ISO-2022 Character Set & Line-Drawing Shift State Restoration E2E**:
  * **Setup**: Execute `ncurses` application designating slot G0 to Special Graphics (`\x1b(0`) and Shift-Out (`0x0f`). Evict from ring buffer.
  * **Execution**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. `activeLog` maintains G0 designation (`\x1b(0`) and Shift-Out state (`0x0f`).
    2. Hydration stream delivers `\x1b(0` and `0x0f` in baseline setup.
    3. Client terminal engine renders box-drawing borders cleanly without ASCII character corruption.
* **Scenario 3.22: Structural Parameters Restoration E2E (Scroll Margins, Cursor Style, Keypad)**:
  * **Setup**: Emit Scroll Margins (`DECSTBM` `10;20r`), Cursor Style (`DECSCUSR` `2q`), and Keypad Application Mode (`DECKPAM` `=`). Evict from history.
  * **Execution**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. `activeLog` maintains scroll region `10;20r`, cursor shape `2q`, and keypad mode `=`.
    2. Hydration stream restores all structural parameters in baseline setup.
    3. Client terminal engine renders correct cursor shape and scroll boundary margins.
* **Scenario 3.23: Device Attributes Query (`CSI c`) Non-Reset Isolation E2E**:
  * **Setup**: Emit Primary Device Attributes Query (`CSI c` / `\x1b[c`).
  * **Execution**: Inspect `activeLog` and reconnect client over `/ws`.
  * **Validation Assertions**:
    1. Server Handler 6 strictly distinguishes `CSI c` from `ESC c` (`RIS` / Full Reset).
    2. `CSI c` does **NOT** clear `activeLog` or truncate history buffer.
    3. Reconnecting client receives full history and active state codes intact.
* **Scenario 3.24: Custom Tab Stop Reset & 8-Column Default Restoration E2E**:
  * **Setup**: Set custom tab stops (`HTS` `\x1bH`). Emit Soft Reset (`DECSTR` `\x1b[!p`).
  * **Execution**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. Server Handler 6 clears custom tab stops and restores standard 8-column tab stops.
    2. Hydration stream restores 8-column tab stop grid.
    3. Reconnecting client aligns tab stops to columns 9, 17, 25, 33...
    4. Tab characters (`\t`) expand cleanly to 8-column boundaries in `xterm.js`.
* **Scenario 3.25: Mid-Hydration Priority Update Interleaving E2E (`ActionPrioritySync` `0x0009`)**:
  * **Setup**: Send `ActionResyncWorkspace` (`0x000e`).
  * **Execution**: While Terminal 2 history is streaming, send `ActionPrioritySync` (`0x0009`) raising Terminal 2 priority from 2 to 1.
  * **Validation Assertions**:
    1. Server priority scheduler immediately updates Terminal 2's priority weight in-flight.
    2. Socket frame interleaving adjusts bandwidth allocation to Terminal 2 mid-hydration.
* **Scenario 3.26: Mid-Hydration Process Kill Signal Pending Queue E2E (`ActionKill` `0x0004`)**:
  * **Setup**: Send `ActionResyncWorkspace` (`0x000e`).
  * **Execution**: While Terminal 1 history is streaming, send `ActionKill` (`0x0004`) for Terminal 1.
  * **Validation Assertions**:
    1. `ActionKill` is enqueued in Terminal 1's `pendingQueue` during history compilation.
    2. Immediately **AFTER `ActionSyncComplete` (`0x000d`)** is emitted for Terminal 1, server dispatches `SIGKILL`/`SIGTERM` to process group, terminating Terminal 1 cleanly post-hydration.

### G. Macro Invariants & Session Disambiguation Scenarios

* **Scenario 3.27: Macro Multi-Tab Priority Bandwidth Re-balancing & 3-Tab Interleaving E2E**:
  * **Setup**: Send `ActionResyncWorkspace` (`0x000e`) with Tab 1 (`priority: 1`), Tab 2 (`priority: 2`), and Tab 3 (`priority: 3`).
  * **Execution**: While all 3 tabs stream high-volume stdout data simultaneously, send a macro priority update shifting Tab 3 to `priority: 1`, Tab 1 to `priority: 2`, Tab 2 to `priority: 3`.
  * **Validation Assertions**:
    1. Server workspace scheduler dynamically adjusts socket frame interleaving across all 3 PTYs in real time.
    2. Bandwidth allocation shifts immediately to Tab 3 without socket starvation or stream corruption on background tabs.
* **Scenario 3.28: Macro Server Non-Autonomous Spawning Invariant E2E**:
  * **Setup**: Connect over `/ws` and send `ActionResyncWorkspace` (`0x000e`) with an empty terminal array `[]`.
  * **Execution**: Monitor workspace state and outgoing frames.
  * **Validation Assertions**:
    1. Server verifies that PTY creation is strictly client-initiated (`ActionSpawn` / `0x0001`).
    2. Server never spawns new PTYs autonomously or allocates default terminals without an explicit client request.
* **Scenario 3.29: Macro Status Frame Idempotency & Re-entry Safety E2E**:
  * **Setup**: Reconnect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Execution**: Inject duplicate status frames (`ActionSpawnStatus` `0x0002 [0x02]`, `ActionTerminalExit` `0x0005`, `ActionSyncFailed` `0x000f`) to a client terminal session during resync.
  * **Validation Assertions**:
    1. Client UI state machine ingests duplicate status frames idempotently with zero visual flicker or state corruption.
    2. Terminal state converges deterministically to the server's authoritative state.
* **Scenario 3.30: Initial Session Connection vs Reconnect Takeover Disambiguation & Graceful Close E2E**:
  * **Setup**: Connect Client 1 over `/ws` and spawn Tab 1 (`ActionSpawn` `0x0001`). Connect Client 2 over `/ws` (takeover).
  * **Execution**: Inspect server event emission across both connection sockets.
  * **Validation Assertions**:
    1. Server cleanly disambiguates initial session creation from reconnect takeover (where `ActionSpawnStatus [0x00]` is suppressed and `ActionResyncWorkspace` `0x000e` drives hydration).
    2. Client 1 connection receives WebSocket Close Code `1000` (Normal Closure) and is evicted gracefully via takeover commit, while Client 2 hydrates state cleanly.
* **Scenario 3.31: Upfront Macro PTY Read-Pause & OS Kernel Backpressure E2E Validation**:
  * **Setup**: Workspace contains 2 PTYs running high-throughput processes (`yes` or `cat /dev/urandom`).
  * **Execution**: Connect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. Server pauses `read(masterFd)` for both PTYs upfront before compiling history.
    2. OS kernel backpressure pauses child process `write()` syscalls during the hydration window.
    3. Zero new bytes enter or evict 256KB history ring buffers during history compilation.
    4. Each PTY's `read(masterFd)` is un-paused and resumed immediately as its `ActionSyncComplete` (`0x000d`) is emitted.
* **Scenario 3.32: Global Terminal Exit Ordering (`0x0005` Always Follows Stdout) E2E Validation**:
  * **Setup**: Spawn a process that writes 10KB of text and calls `exit(0)`.
  * **Execution**: Disconnect and reconnect over `/ws`. Send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. Server flushes all 10KB of stdout bytes over the WebSocket socket FIRST (preserving established `core.go` L648-653 behavior).
    2. Server emits `ActionTerminalExit` (`0x0005`) carrying the exit code strictly AFTER all 10KB stdout bytes.
    3. `xterm.js` renders all stdout text *before* the UI state machine renders the "Exited" badge.
* **Scenario 3.33: Rapid Re-Request Resync Preemption / Atomic Restart E2E Validation**:
  * **Setup**: Connect client over `/ws`.
  * **Execution**: Send `ActionResyncWorkspace` frame A. 5 milliseconds later (mid-hydration), send `ActionResyncWorkspace` frame B.
  * **Validation Assertions**:
    1. Server preempts/cancels Pass A mid-flight.
    2. Server applies Pass B's upfront read-pause idempotently, updates target dimensions/priorities, and completes Pass B hydration.
    3. Zero duplicate control frames or memory leaks occur on the WebSocket connection.
* **Scenario 3.34: Stalled TCP Client Socket 5-Second Control Frame Write Deadline E2E Validation**:
  * **Setup**: Connect a custom TCP test socket over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Execution**: Pause reading on the client TCP socket to simulate network congestion/stall.
  * **Validation Assertions**:
    1. Server control frame egress (`0x000c`/`0x000d`) hits the 5-second socket write deadline.
    2. Server gracefully closes the stalled WebSocket connection, preventing server Goroutine leaks.
* **Scenario 3.35: Empty Terminal Array (`terminals: []`) Complete Workspace Eviction E2E Validation**:
  * **Setup**: Workspace contains active PTY 1 and PTY 2.
  * **Execution**: Connect client over `/ws` and send `ActionResyncWorkspace` carrying `terminals: []`.
  * **Validation Assertions**:
    1. Server compares `terminals: []` against `workspace.terminals`.
    2. Server recognizes all active PTYs as orphaned and destroys PTY 1 and PTY 2 via `workspace.RemovePTY()`, closing master file descriptors (`masterFd`).
    3. Server leaves a clean empty workspace in Go memory without emitting per-PTY sync streams.
* **Scenario 3.36: High-Throughput SIMD Scanner Escaped Stream Flooding (100MB Egress Stress) E2E Validation**:
  * **Setup**: Spawn a terminal running a command emitting 100MB of high-volume ANSI escape sequence floods (`cargo build` / `cat /dev/urandom`).
  * **Execution**: Connect client over `/ws` and send `ActionResyncWorkspace` (`0x000e`).
  * **Validation Assertions**:
    1. Server SIMD fast-path scanner (`bytes.IndexByte`) parses escape streams at full assembly throughput ($> 100\text{MB/sec}$).
    2. `activeLog` updates sequence states under write-lock without deadlocks, CPU starvation, or Goroutine blocking.
    3. Reconnection hydration stream completes in $< 10\text{ms}$ with zero dropped frames or WebSocket disconnects.
* **Scenario 3.37: Single-Writer Egress FIFO Serialization E2E Validation**:
  * **Setup**: Spawn live terminal session. Trigger rapid interleaved stream events (`ActionSpawnStatus` `0x0002 [0x00]`, high-throughput `ActionOutput`, `ActionTerminalExit` `0x0005`).
  * **Execution**: Connect client over `/ws` and capture raw WebSocket frame egress order.
  * **Validation Assertions**:
    1. Single-writer FIFO egress queue (`wsConnection.WriteFrame`) serializes all frames in strict chronological order.
    2. `ActionSpawnStatus [0x00]` is verified on the socket BEFORE any `ActionOutput` bytes.
    3. `ActionTerminalExit [0x0005]` is verified on the socket AFTER all `ActionOutput` bytes.

---

## 5. Summary Matrix of Complete Test Suite

| Test Tier | Test Target | Action Type | Test Name / Scope | Purpose |
| :--- | :--- | :--- | :--- | :--- |
| **Tier 1** | `pty_internal_test.go` | **NEW** | `TestParserHandler1_ModeBitsets` | Verify DEC/ANSI mode bitset parsing & disable removal across all 8 mode types. |
| **Tier 1** | `pty_internal_test.go` | **NEW** | `TestParserHandler2_GenericOSC` | Verify OSC titles, CWD, theme, and shell prompt parsing with both `BEL` (`\x07`) and `ST` (`\x1b\\`) terminators. |
| **Tier 1** | `pty_internal_test.go` | **NEW** | `TestParserHandler3_OSCPalette` | Verify OSC 256-color palette parsing and query filtering. |
| **Tier 1** | `pty_internal_test.go` | **NEW** | `TestParserHandler4_ISO2022` | Verify ISO-2022 G0-G3 slots and SI/SO state switches. |
| **Tier 1** | `pty_internal_test.go` | **NEW** | `TestParserHandler5_StructuralParameters` | Verify scroll margins (including `\x1b[r` empty reset), cursor style, and keypad mode. |
| **Tier 1** | `pty_internal_test.go` | **NEW** | `TestParserHandler6_ResetSemantics` | Verify Soft Reset (`DECSTR`), Full Reset (`RIS`), Erase Saved Lines (`3J`), Erase Display (`2J`), and DA query isolation. |
| **Tier 1** | `pty_internal_test.go` | **NEW** | `TestCarryoverBuffer_SplitChunkProtection` | Verify 512-byte carryover split-sequence protection for long OSC 7/OSC 2 paths. |
| **Tier 1** | `pty_internal_test.go` | **NEW** | `TestHeadOfBuffer_NewlineSanitization` | Verify forward scanning to first `\n` on wrapped buffer. |
| **Tier 1** | `pty_internal_test.go` | **NEW** | `TestSIMDScanner_FastPath` | Verify SIMD `bytes.IndexByte` fast-path zero-allocation scanner. |
| **Tier 1** | `pty_internal_test.go` | **UPDATED** | `TestPTYInstance_StateInitialization` | Update for `activeLog`, `carryover [512]byte`, and `activeLogMutex`. |
| **Tier 2** | `pty_internal_test.go` | **NEW** | `TestGlobalTerminalExitOrdering_AllStdoutFlushedFirst` | Verify established `core.go` behavior: `ActionTerminalExit (0x0005)` strictly follows stdout bytes. |
| **Tier 2** | `pty_internal_test.go` | **NEW** | `TestLiveSpawnStatusOrdering_SpawnStatusPrecedesStdout` | Verify `ActionSpawnStatus [0x00]` strictly precedes stdout bytes. |
| **Tier 2** | `network_internal_test.go` | **NEW** | `TestSingleWriterFifoEgressSerialization_DeterministicFrameOrder` | Verify single-writer FIFO egress queue prevents frame overtakes on TCP wire. |
| **Tier 2** | `network_internal_test.go` | **NEW** | `TestUpfrontWorkspacePTYReadPause_OSBackpressure` | Verify pausing `read(masterFd)` for all PTYs upfront during resync & idempotent pause/resume. |
| **Tier 2** | `network_internal_test.go` | **NEW** | `TestControlFrame_WriteDeadline` | Verify 5-second socket write deadline on control frame egress under socket stall. |
| **Tier 2** | `network_internal_test.go` | **NEW** | `TestPrioritySequencedHydration_FocusedTabFirst` | Verify focused tab (`priority: 1`) hydrates BEFORE background tab (`priority: 2`). |
| **Tier 2** | `network_internal_test.go` | **NEW** | `TestCommandGating_PreResyncRejection` | Verify rejection of ALL operational opcodes (`0x0007`, `0x0003`, `0x0001`, `0x0004`, `0x0006`, `0x0009`) before `0x000e`. |
| **Tier 2** | `pty_internal_test.go` | **NEW** | `TestInFlightHydrationQueueing` | Verify operational frames during hydration queue in `pendingQueue` in strict FIFO order. |
| **Tier 2** | `pty_internal_test.go` | **NEW** | `TestExtendedHydrationBoundary_MidHydrationResize` | Verify mid-hydration resize applies `setSize()` + `SIGWINCH` before `0x000d`. |
| **Tier 2** | `network_internal_test.go` | **NEW** | `TestAutomaticOrphanGarbageCollection` | Verify omitted PTY IDs in `0x000e` trigger `RemovePTY()` and close master descriptors. |
| **Tier 2** | `network_internal_test.go` | **NEW** | `TestNonExistentTerminal_SyncFailedIsolation` | Verify `ActionSyncFailed` (`0x000f`) payload contents and non-blocking batch isolation. |
| **Tier 2** | `network_internal_test.go` | **NEW** | `TestResyncPreemption_RapidReRequest` | Verify rapid re-requests preempt in-flight resync loops cleanly. |
| **Tier 2** | `pty_internal_test.go` | **NEW** | `TestFastForwardSpawningState_Matrix` | Verify status frame sequences for Running, Exited, Errored, Spawning, and Non-Existent PTYs. |
| **Tier 2** | `pty_internal_test.go` | **NEW** | `TestUniformDeferredRemovePTY` | Verify batch `ActionRemove` payload `[id1, id2]` enqueues in `pendingQueue` and executes post-`0x000d`. |
| **Tier 2** | `network_internal_test.go` | **UPDATED** | `TestPerformReplayTakeover_AtomicBinding` | Update takeover tests for `0x000e` macro protocol. |
| **Tier 2** | `network_internal_test.go` | **DELETED** | *Legacy Query Param Tests* | Delete `?cols=X&rows=Y` and mandatory `?token=...` URL tests. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.1: Alt-Screen Restoration` | E2E verification of `activeLog` alt-screen restoration over `/ws`. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.2: Bracketed Paste Security` | E2E verification of bracketed paste mode security wrapper. |
| **Tier 3** | `source/e2e_test.go` | **UPDATED** | `Scenario 3.3: Soft Reset State Sanitization` | E2E verification of DECSTR/RIS mode pruning while strictly preserving OSC 2 titles and OSC 7 CWDs. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.4: OSC Titles & Themes` | E2E verification of OSC titles and 256-color palette themes. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.5: Head Newline Trimming` | E2E verification of leading partial ANSI text trimming. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.6: PGID SIGWINCH Redraw` | E2E verification of `TIOCGPGRP` foreground TUI self-healing redraws. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.7: Zero Double-Repaint Pre-Resizing` | E2E verification of single-pass pre-resizing layout rendering. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.8: Priority-Sequenced Hydration` | E2E verification of priority-sequenced hydration and async repaints. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.9: Granular 0x000d Framing` | E2E verification of per-PTY `ActionSyncComplete` boundary framing. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.10: Toggle Convergence` | E2E verification of chronological toggle sequence convergence. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.11: Pre-Resync Command Gating` | E2E verification of pre-resync command rejection. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.12: Extended Hydration Mid-Sync Resize` | E2E verification of mid-hydration resizes repainting BEFORE `0x000d`. |
| **Tier 3** | `source/e2e_test.go` | **UPDATED** | `Scenario 3.13: Mid-Sync Stdin & Removal Queueing` | E2E verification of pending stdin and removal queues flushing in strict FIFO order post-`0x000d`. |
| **Tier 3** | `source/e2e_test.go` | **UPDATED** | `Scenario 3.14: Automatic Orphan Garbage Collection` | E2E verification of automatic orphan PTY destruction, descriptor closure, and Goroutine release. |
| **Tier 3** | `source/e2e_test.go` | **UPDATED** | `Scenario 3.15: Fast-Forward Spawning State & Live Handover` | E2E verification of Running, Exited, Errored (0 history), Spawning, and Non-Existent PTY states + live `0x0002 [0x00]` handover preceding stdout bytes. |
| **Tier 3** | `source/e2e_test.go` | **UPDATED** | `Scenario 3.16: Full Reset RIS Dual-Storage Purge` | E2E verification of `\x1bc` clearing `activeLog` and `ringBuffer` + re-enabling Modes 7 & 25. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.17: Scrollback Purge 3J Truncation` | E2E verification of `\x1b[3J` clearing scrollback while preserving `activeLog`. |
| **Tier 3** | `source/e2e_test.go` | **UPDATED** | `Scenario 3.18: Split-Chunk Sequence Protection` | E2E verification of 512-byte carryover split-sequence protection for long $200+$ byte OSC 7/OSC 2 paths under log floods. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.19: OSC 7 CWD & OSC 133 Shell Recovery` | E2E verification of OSC 7 working directory and OSC 133 prompt markers. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.20: OSC 4 Palette & Query Filtering` | E2E verification of OSC 4 256-color palette overrides & query filtering. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.21: ISO-2022 Line-Drawing Shift State` | E2E verification of ISO-2022 G0-G3 slots and SI/SO shift states. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.22: Structural Parameters Restoration` | E2E verification of scroll margins (DECSTBM), cursor style, and keypad mode. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.23: DA Query CSI c Non-Reset Isolation` | E2E verification of CSI c query non-reset isolation. |
| **Tier 3** | `source/e2e_test.go` | **UPDATED** | `Scenario 3.24: Custom Tab Stop Reset Restoration` | E2E verification of custom tab stop reset & 8-column default restoration in `xterm.js`. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.25: Mid-Sync ActionPrioritySync` | E2E verification of mid-hydration `ActionPrioritySync` (`0x0009`) interleaving. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.26: Mid-Sync ActionKill Pending Queue` | E2E verification of mid-hydration `ActionKill` (`0x0004`) queueing. |
| **Tier 3** | `source/e2e_test.go` | **UPDATED** | `Scenario 3.27: Macro Multi-Tab Priority Bandwidth Re-balancing` | E2E verification of dynamic multi-tab priority bandwidth re-balancing & 3-tab interleaving. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.28: Macro Server Non-Autonomous Spawning` | E2E verification of server non-autonomous spawning invariant. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.29: Macro Status Frame Idempotency` | E2E verification of universal status frame idempotency & re-entry safety. |
| **Tier 3** | `source/e2e_test.go` | **UPDATED** | `Scenario 3.30: Session Disambiguation & Graceful Close` | E2E verification of initial session connection vs reconnect takeover + Close Code 1000 eviction. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.31: Upfront Workspace PTY Read-Pause & OS Backpressure` | E2E validation of upfront PTY read pause and OS kernel backpressure during high-throughput log floods. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.32: Global Terminal Exit Ordering (0x0005 Follows Stdout)` | E2E validation of `ActionTerminalExit` (`0x0005`) strictly following all stdout bytes (preserving established `core.go` L648-653 behavior). |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.33: Rapid Re-Request Resync Preemption / Atomic Restart` | E2E validation of rapid `0x000e` re-requests preempting in-flight resync passes atomically. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.34: Stalled TCP Client Socket 5-Second Write Deadline` | E2E validation of 5-second socket write deadlines on control frame egress under TCP socket stalls. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.35: Empty Terminal Array (terminals: []) Eviction` | E2E validation of `terminals: []` destroying all active PTY instances and closing master descriptors. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.36: High-Throughput SIMD Scanner Flooding (100MB Stress)` | E2E validation of SIMD fast-path scanner handling 100MB escape floods without CPU starvation or drops. |
| **Tier 3** | `source/e2e_test.go` | **NEW** | `Scenario 3.37: Single-Writer Egress FIFO Serialization` | E2E validation of single-writer FIFO egress queue (`wsConnection.WriteFrame`) serializing all frames in strict chronological order. |
