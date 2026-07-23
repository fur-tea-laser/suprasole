# Proposal & Outline Specification: Refactored PTY Spawning Semantics

This document outlines the preliminary architectural proposal for improving and refactoring **PTY Spawning Semantics** in `suprasole-server`. 

It adapts the lessons, invariants, and event-driven patterns surfaced during the **Terminal Resync Protocol** refactor—establishing a clean, robust, and idempotent spawning lifecycle that integrates seamlessly with workspace priority scheduling and reconnect state recovery.

---

## 1. Architectural Motivation & Core Invariants

During the development of the terminal resync specification, several critical nuances regarding PTY creation were surfaced:

1. **Client-Only Spawning Invariant**: Spawning PTYs is strictly client-initiated (`ActionSpawnPTY` / `0x0003`). The server never spawns PTYs autonomously without client request.
2. **Upfront Geometry & Priority Assignment**: `ActionSpawnPTY` (`0x0003`) must supply target dimensions (`columns`, `rows`) and initial priority weight (`priority`) in its initial payload, ensuring the Linux kernel PTY master descriptor is configured *before* launching the process.
3. **Granular Lifecycle Events**: PTY creation follows an explicit four-stage status lifecycle (`ActionSpawnPTY` $\rightarrow$ `ActionPTYSpawning` $\rightarrow$ `ActionPTYSpawned` or `ActionPTYFailed`).
4. **Category Separation**: `ActionPTYSpawned` (`0x0004`) is strictly a **Live Creation Event**. It is emitted **ONCE** when a PTY is created live. It is never re-emitted during reconnect resync hydration (where `ActionSyncStart` $\rightarrow$ Hydration Stream $\rightarrow$ `ActionSyncComplete` is used instead), preventing duplicate tab DOM creation.
5. **Dropped Spawn Frame Self-Healing**: If `ActionSpawnPTY` (`0x0003`) is dropped in transit due to a network disconnect, the client's subsequent `ActionResyncWorkspace` (`0x000e`) request detects that the terminal ID was never created on the server and emits `ActionSyncFailed` (`0x000f`), allowing the client UI to resolve the pending tab cleanly without hanging.

---

## 2. Refactored Spawning Protocol Flow & Status Lifecycle

### A. The Inbound Spawn Request (`ActionSpawnPTY` — `0x0003`)

The client sends a single, structured `ActionSpawnPTY` (`0x0003`) control frame:

```json
{
  "terminalID": 1,
  "command": "bash",
  "args": ["-l"],
  "cwd": "/home/coder/project",
  "columns": 140,
  "rows": 50,
  "priority": 1
}
```

* **`terminalID`**: Client-assigned unique terminal identifier (`uint16`).
* **`columns` & `rows`**: Initial target UI dimensions (`uint16`).
* **`priority`**: Initial tab priority weight (`uint8` — `1` for active focused tab, `2` for background tab).

---

### B. The 4-Stage Spawning Lifecycle

```
  1. Client sends ActionSpawnPTY (0x0003)
                |
                v
  2. [Server] Registers PTY, applies initial geometry (columns, rows) & priority
                |
                v
  3. [Server] Emits ActionPTYSpawning (0x0001) Status Event
                |  (Client UI displays spawning/loading spinner)
                v
  4. [Server] Fork/Exec Process Group (syscall.SysProcAttr { Setsid: true })
                |
        +-------+-------+
        |               |
     (Success)       (Failure)
        |               |
        v               v
  5a. Emits         5b. Emits
      ActionPTYSpawned  ActionPTYFailed (0x000f)
      (0x0004)          (Client UI renders error state)
        |
        v
  6. Live stdout streaming begins
```

---

## 3. Detailed Spawning Status Opcodes & Payloads

| Opcode | Name | Direction | Payload & Behavioral Rule |
| :--- | :--- | :--- | :--- |
| **`0x0003`** | **`ActionSpawnPTY`** | Client $\rightarrow$ Server | Inbound JSON request: `{"terminalID":1, "command":"bash", "args":[], "cwd":"...", "columns":140, "rows":50, "priority":1}`. |
| **`0x0001`** | **`ActionPTYSpawning`** | Server $\rightarrow$ Client | Outbound status event (`TerminalID uint16`, 0 bytes payload). Emitted immediately when server starts `fork/exec`. Client UI shows spawning spinner. |
| **`0x0004`** | **`ActionPTYSpawned`** | Server $\rightarrow$ Client | Outbound creation event (`TerminalID uint16`, JSON metadata payload). Emitted ONCE when process `exec` succeeds. Client UI mounts live tab. |
| **`0x000f`** | **`ActionPTYFailed`** | Server $\rightarrow$ Client | Outbound failure event (`TerminalID uint16`, JSON error payload). Emitted if `exec` fails (e.g. `binary not found`). Client UI renders error badge. |

---

## 4. Integration with Resync & Network Edge Cases

### A. Wi-Fi Drop During Spawn (`ActionSpawnPTY` Dropped in Transit)
* If `ActionSpawnPTY` (`0x0003`) is dropped on the network before reaching the server, the server has no record of the `TerminalID`.
* When the client reconnects and sends `ActionResyncWorkspace` (`0x000e`) containing the pending `TerminalID`, the server checks its workspace map, finds no matching PTY, and emits `ActionSyncStart` $\rightarrow$ **`ActionPTYFailed` (`0x000f`)**.
* The client UI receives `0x000f` and marks the pending tab as failed, allowing the user to retry cleanly.

### B. Disconnect Right After `ActionPTYSpawned` (`0x0004`)
* If the server successfully spawns the PTY and emits `ActionPTYSpawned` (`0x0004`), but Wi-Fi drops before the client receives `0x0004`:
* The PTY is now in `TerminalStateRunning` on the server.
* On reconnect, `ActionResyncWorkspace` (`0x000e`) detects `TerminalStateRunning`, suppresses `0x0004`, and executes the standard resync sequence: `ActionSyncStart` $\rightarrow$ Hydration Stream $\rightarrow$ `ActionSyncComplete`.
* Receiving `ActionSyncStart` signals the client UI that the terminal is alive and hydrating, transitioning the tab from spawning to active seamlessly!

---

## 5. Summary of Proposed Changes to `suprasole-server`

1. **`source/core.go`**: Update `SpawnPTY` method signature to accept initial `columns`, `rows`, and `priority` upfront, applying `setSize()` before process launch.
2. **`source/network.go`**: Update `ActionSpawnPTY` (`0x0003`) JSON parser to extract `columns`, `rows`, and `priority`, forwarding them directly to `workspace.SpawnPTY()`.
3. **`source/pty_internal_test.go`**: Add unit tests for `ActionPTYSpawning` $\rightarrow$ `ActionPTYSpawned` and `ActionPTYFailed` lifecycle transitions.
