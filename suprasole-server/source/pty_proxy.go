package source

import (
	"bytes"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"github.com/gitpod-io/xterm-go"
)

type PtyProxyMode int

const (
	PtyProxyMode_Spawning PtyProxyMode = iota
	PtyProxyMode_Running_Live
	PtyProxyMode_Running_PreSnapshot
	PtyProxyMode_Running_PostSnapshot
	PtyProxyMode_Exited
)

/*
===============================================================================
PtyProxy: Pseudo-Terminal Proxy Engine
===============================================================================

System Context & Operational Mode Architecture:
PtyProxy is the core thread-safe Pseudo-Terminal Proxy engine that bridges OS PTY master file descriptors,
headless VTE state maintenance (*xterm.Terminal), zero-contention kernel stream draining (*PtyReader),
and asynchronous web client baseline snapshot serialization & gap-byte replay across client mode transitions.

Operating Modes (PtyProxyMode State Machine):
  - PtyProxyMode_Spawning: Initializing child process and configuring callbacks.
  - PtyProxyMode_Running_Live: Normal streaming mode; output bytes update VTE state and stream live via Proxy_OnOutput_Live.
  - PtyProxyMode_Running_PreSnapshot: Baseline snapshot requested; preparing gap-byte staging buffer.
  - PtyProxyMode_Running_PostSnapshot: Serializing ANSI baseline state under lock; incoming stdout bytes update VTE state and accumulate in Proxy_PostSnapshotBuffer.
  - PtyProxyMode_Exited: Child process or descriptor closed; teardown callbacks dispatched.

Baseline Snapshot Serialization & Gap-Byte Replay Protocol:
During client connects, PtyProxy captures full terminal state via xterm.NewSerializeAddon. To prevent dropped output during serialization,
PtyProxy transitions through Running_PreSnapshot -> Running_PostSnapshot -> Running_Live. Staging gap bytes in Proxy_PostSnapshotBuffer while serializing guarantees zero stdout data loss.

Flush Handling Callback Complexity & Lock Scoping Strategy:
The primary architectural complexity in PtyProxy centers on its flush handling callbacks (Proxy_HandleTryFlush and Proxy_HandleBlockingFlush) and their underlying helper __flushReaderStagingBufferSliceIfLockAcquired. This pipeline bridges PtyReader's lock-free background read loop with PtyProxy's thread-safe state machine across four critical boundaries:
 1. Downstream Coupling to PtyReader Staging Cushion:
    PtyReader drains the OS PTY descriptor into its pre-allocated staging cushion unlocked. It calls Proxy_HandleTryFlush via TryLock(). If Proxy_Mutex is contended, TryLock() returns false immediately, allowing PtyReader to continue absorbing PTY stdout without stalling kernel pipe draining. Only when the staging cushion saturates does PtyReader call Proxy_HandleBlockingFlush to wait on Proxy_Mutex.Lock().
 2. Microsecond Lock Scoping & Deadlock Avoidance:
    __flushReaderStagingBufferSliceIfLockAcquired updates the VTE grid (*xterm.Terminal) and evaluates Proxy_Mode strictly under Proxy_Mutex. However, it MUST unlock Proxy_Mutex BEFORE calling Proxy_OnOutput_Live. Dispatching egress callbacks outside Proxy_Mutex guarantees microsecond lock hold times and prevents deadlocks if a client socket blocks.
 3. Thread-Safe Memory Hand-Off (bytes.Clone):
    Because Proxy_OnOutput_Live executes outside Proxy_Mutex, PtyReader could immediately overwrite its reusable staging buffer on the next read iteration. __flushReaderStagingBufferSliceIfLockAcquired passes bytes.Clone(unflushedStagingBufferSlice) to ensure complete memory safety.
 4. Dynamic Dual-Egress Routing:
    Under lock, __flushReaderStagingBufferSliceIfLockAcquired evaluates Proxy_Mode:
      - Running_Live: VTE grid updated under lock; stdout bytes cloned and dispatched live outside lock.
      - Running_PostSnapshot: VTE grid updated AND bytes copied to Proxy_PostSnapshotBuffer under lock (staging gap bytes while baseline snapshot is delivered).
      - Running_PreSnapshot / Exited: VTE grid updated under lock; live egress suppressed.

Multi-Threaded Synchronization Architecture & Proxy_Mutex Rationale:
PtyProxy operates across two concurrent execution contexts that intersect at shared instance memory:

 1. Background Reader Execution Context (Goroutine 1: PtyReader.Reader_Run):
    Executes continuously on a single background goroutine, reading PTY stdout bytes from the OS master file descriptor unlocked into PtyReader.Reader_StagingBuffer. PTY output and process teardown propagate through nine distinct unblended pseudo callstacks.
    Note on Mutual Exclusivity: All nine paths executing on Goroutine 1 are strictly mutually exclusive and can never overlap. Because PtyReader.Reader_Run operates on a single background goroutine and Proxy_Mode evaluates atomically under Proxy_Mutex, exactly one path executes per flush or exit event. Furthermore, once a teardown path (Paths 7-9) is triggered upon read loop termination, no further flushes can ever occur.

    - Path 1: Optimistic Live Streaming Call Tree (Hot-Path TryFlush in Live Mode)
      PtyReader.Reader_Run
        -> Reader_OnTryFlush
          -> Proxy_HandleTryFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Proxy_Mutex.TryLock
              -> Proxy_TerminalState.Write
              -> Proxy_Mutex.Unlock
              -> Proxy_OnOutput_Live (bytes.Clone)

    - Path 2: Saturation Fallback Live Streaming Call Tree (BlockingFlush in Live Mode)
      PtyReader.Reader_Run
        -> Reader_OnBlockingFlush
          -> Proxy_HandleBlockingFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Proxy_LockAndReturnTrue
                -> Proxy_Mutex.Lock
              -> Proxy_TerminalState.Write
              -> Proxy_Mutex.Unlock
              -> Proxy_OnOutput_Live (bytes.Clone)

    - Path 3: Optimistic Post-Snapshot Gap Staging Call Tree (TryFlush in PostSnapshot Mode)
      PtyReader.Reader_Run
        -> Reader_OnTryFlush
          -> Proxy_HandleTryFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Proxy_Mutex.TryLock
              -> Proxy_TerminalState.Write
              -> Proxy_PostSnapshotBuffer.Write
              -> Proxy_Mutex.Unlock

    - Path 4: Saturation Fallback Post-Snapshot Gap Staging Call Tree (BlockingFlush in PostSnapshot Mode)
      PtyReader.Reader_Run
        -> Reader_OnBlockingFlush
          -> Proxy_HandleBlockingFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Proxy_LockAndReturnTrue
                -> Proxy_Mutex.Lock
              -> Proxy_TerminalState.Write
              -> Proxy_PostSnapshotBuffer.Write
              -> Proxy_Mutex.Unlock

    - Path 5: Optimistic Pre-Snapshot VTE Update Call Tree (Hot-Path TryFlush in PreSnapshot Mode)
      PtyReader.Reader_Run
        -> Reader_OnTryFlush
          -> Proxy_HandleTryFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Proxy_Mutex.TryLock
              -> Proxy_TerminalState.Write
              -> Proxy_Mutex.Unlock

    - Path 6: Saturation Fallback Pre-Snapshot VTE Update Call Tree (BlockingFlush in PreSnapshot Mode)
      PtyReader.Reader_Run
        -> Reader_OnBlockingFlush
          -> Proxy_HandleBlockingFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Proxy_LockAndReturnTrue
                -> Proxy_Mutex.Lock
              -> Proxy_TerminalState.Write
              -> Proxy_Mutex.Unlock

    - Path 7: Descriptor Closure Process Teardown Call Tree
      PtyReader.Reader_Run
        -> Reader_OnExited_Closed
          -> Proxy_HandleExited_Closed
            -> __executeExitedTeardownPipeline
              -> Proxy_Mutex.Lock (Proxy_Mode = Exited)
              -> Proxy_Mutex.Unlock
              -> Proxy_TerminalCommand.Wait
              -> Proxy_HandleDispatchExitedHandler_Closed
                -> Proxy_OnExited_Closed

    - Path 8: Child Process EOF Exit Status Teardown Call Tree
      PtyReader.Reader_Run
        -> Reader_OnExited_Eio
          -> Proxy_HandleExited_Eio
            -> __executeExitedTeardownPipeline
              -> Proxy_Mutex.Lock (Proxy_Mode = Exited)
              -> Proxy_Mutex.Unlock
              -> Proxy_TerminalCommand.Wait
              -> Proxy_HandleDispatchExitedHandler_Eio
                -> [Mutually Exclusive Leaf Callback - Exactly 1 Dispatched]:
                  - Proxy_OnExited_Eio_Success (if ExitStatus == 0)
                  - Proxy_OnExited_Eio_Failure (if ExitStatus != 0)
                  - Proxy_OnExited_Eio_Killed (if Signaled)

    - Path 9: Kernel System Error Teardown Call Tree
      PtyReader.Reader_Run
        -> Reader_OnExited_SystemError
          -> Proxy_HandleExited_SystemError
            -> __executeExitedTeardownPipeline
              -> Proxy_Mutex.Lock (Proxy_Mode = Exited)
              -> Proxy_Mutex.Unlock
              -> Proxy_TerminalCommand.Wait
              -> Proxy_HandleDispatchExitedHandler_SystemError
                -> Proxy_OnExited_SystemError

 2. Main / Workspace Execution Context:
    Executes concurrently on main or workspace goroutines whenever clients connect, disconnect, resize windows, or request state resynchronization:

    - Path 10: Pre-Snapshot Mode Transition Call Tree
      Proxy_TransitionMode_LiveToPreSnapshot
        -> Proxy_Mutex.Lock (Proxy_Mode = Running_PreSnapshot)
        -> Proxy_Mutex.Unlock

    - Path 11: Baseline Snapshot Serialization Call Tree
      Proxy_TransitionMode_PreToPostSnapshot
        -> Proxy_Mutex.Lock (Proxy_Mode = Running_PostSnapshot)
        -> Proxy_PostSnapshotBuffer.Reset
        -> xterm.NewSerializeAddon
        -> serializeAddon.Serialize
        -> Proxy_Mutex.Unlock
        -> Proxy_OnOutput_Snapshot

    - Path 12: Post-Snapshot Gap Replay Call Tree
      Proxy_TransitionMode_PostSnapshotToLive
        -> Proxy_Mutex.Lock (Proxy_Mode = Running_Live)
        -> bytes.Clone(Proxy_PostSnapshotBuffer)
        -> Proxy_Mutex.Unlock
        -> Proxy_OnOutput_PostSnapshotBuffer

    - Path 13: Terminal Window Resizing Call Tree
      Proxy_Resize
        -> pty.Setsize
        -> Proxy_Mutex.Lock
        -> Proxy_TerminalState.Resize
        -> Proxy_Mutex.Unlock

    - Path 14: Terminal User Input / Stdin Writing Call Tree
      Pty_MasterFileDescriptor.Write

    - Path 15: Master Descriptor Administrative Closure Call Tree
      Pty_MasterFileDescriptor.Close

 Isolated Execution Spheres (Where Mutex IS NOT Required):
  - Internal PtyReader Loop: Draining kernel PTY file descriptor into Reader_StagingBuffer, tracking Reader_UnflushedStagingBufferSliceSize, and resetting write head are 100% single-threaded within PtyReader. No mutex is required inside PtyReader itself.
  - Egress Callback Dispatch: Egress callbacks (Proxy_OnOutput_Live, Proxy_OnOutput_Snapshot, Proxy_OnOutput_PostSnapshotBuffer, Proxy_OnExited_*) execute 100% unlocked outside Proxy_Mutex using cloned byte allocations.

 Core Intersecting Conflict Conditions (Why Mutex IS Mandatory):
  - Condition 1 (VTE Grid Read/Write Data Race): If Reader Goroutine executes Proxy_TerminalState.Write() while Web Goroutine scans grid pointers via xterm.NewSerializeAddon without synchronization, Go runtime triggers a fatal data race and memory corruption panic.
  - Condition 2 (Mode Evaluation TOCTOU Race): If Reader Goroutine evaluates Proxy_Mode outside lock to decide live dispatch vs. gap staging while Web Goroutine mutates Proxy_Mode, a Time-Of-Check-To-Time-Of-Use race occurs, resulting in dropped stdout bytes during client reconnect.
  - Condition 3 (Grid Resizing Pointer Race): If Reader Goroutine writes bytes while Web Goroutine resizes VTE dimensions via Proxy_TerminalState.Resize(), concurrent array reallocations cause index out-of-bounds panics.

Struct Property Contracts & Semantics:
  - Pty_MasterFileDescriptor (*os.File) [REQUIRED]
    - Purpose: Master OS PTY file descriptor returned via pty.Start(Proxy_TerminalCommand).
    - Contract: Must be open, valid, and non-nil after successful NewPtyProxy spawn.
    - Semantics: Shared with Proxy_PtyReader for background reading. Closed upon process teardown or explicit administrative close.
  - Proxy_Id (uint32) [REQUIRED]
    - Purpose: Immutable 32-bit unique identifier assigned at creation.
    - Contract: Assigned during NewPtyProxy initialization; must be unique across active proxies.
    - Semantics: Identifies this PTY session across server routes and logging subsystems.
  - Proxy_Mutex (sync.Mutex) [REQUIRED]
    - Purpose: Microsecond instance lock protecting in-memory state.
    - Contract: Protects Proxy_TerminalState, Proxy_PostSnapshotBuffer, and Proxy_Mode. Must never be held across external blocking I/O calls.
    - Semantics: Ensures atomic state transitions and thread-safe VTE updates between reader and web request goroutines.
  - Proxy_Mode (PtyProxyMode) [REQUIRED]
    - Purpose: Unified operational and lifecycle state machine.
    - Contract: Mutated under Proxy_Mutex; transitions follow Spawning -> Running_Live <-> Running_PreSnapshot <-> Running_PostSnapshot -> Exited.
    - Semantics: Controls stdout egress routing and gap-byte buffer accumulation.
  - Proxy_TerminalCommand (*exec.Cmd) [REQUIRED]
    - Purpose: OS child process handle created via exec.Command(...) and started with pty.Start.
    - Contract: Started synchronously during NewPtyProxy. Must be reaped post-loop on exit callbacks via cmd.Wait() outside Proxy_Mutex.
    - Semantics: Manages child process lifecycle, signal handling, and exit status extraction.
  - Proxy_TerminalState (*xterm.Terminal) [REQUIRED]
    - Purpose: Headless VTE state engine maintaining grid lines, cursor, modes, and scrollback.
    - Contract: Updated under Proxy_Mutex whenever stdout bytes arrive from PtyReader.
    - Semantics: Provides complete ANSI state serialization for web clients on demand.
  - Proxy_PtyReader (*PtyReader) [REQUIRED]
    - Purpose: Dedicated stream reader sub-component running background goroutine for zero-contention kernel PTY draining.
    - Contract: Owned exclusively by PtyProxy. Callback fields bound to PtyProxy handler methods.
    - Semantics: Drains kernel descriptor into pre-allocated staging buffer unlocked, flushing to PtyProxy via TryLock.
  - Proxy_PostSnapshotBuffer (*bytes.Buffer) [REQUIRED]
    - Purpose: Pre-allocated RAM buffer staging stdout during asynchronous baseline snapshot serialization.
    - Contract: Access synchronized under Proxy_Mutex. Reset during PreToPostSnapshot mode transition.
    - Semantics: Accumulates gap bytes during PostSnapshot mode for replay upon transition to Live mode.
  - Proxy_OnOutput_Live (func(ptyProxy *PtyProxy, ptyOutputData []byte)) [REQUIRED]
    - Purpose: Egress callback streaming stdout during Live mode.
    - Trigger Condition: Invoked during PtyProxyMode_Running_Live when stdout bytes arrive from PtyReader.
    - Consumer Contract: Must be non-blocking. Invoked 100% outside Proxy_Mutex with a cloned byte slice.
    - Semantics: Delivers real-time stdout streams to active client connections.
  - Proxy_OnOutput_Snapshot (func(ptyProxy *PtyProxy, ptyOutputData []byte)) [REQUIRED]
    - Purpose: Egress callback delivering baseline ANSI snapshot string.
    - Trigger Condition: Invoked during TransitionMode_PreToPostSnapshot after serializing Proxy_TerminalState.
    - Consumer Contract: Must be non-blocking. Invoked 100% outside Proxy_Mutex.
    - Semantics: Delivers initial terminal screen state to connecting web clients.
  - Proxy_OnOutput_PostSnapshotBuffer (func(ptyProxy *PtyProxy, ptyOutputData []byte)) [REQUIRED]
    - Purpose: Egress callback streaming gap bytes accumulated during snapshot serialization.
    - Trigger Condition: Invoked during TransitionMode_PostSnapshotToLive if Proxy_PostSnapshotBuffer contains staged bytes.
    - Consumer Contract: Must be non-blocking. Invoked 100% outside Proxy_Mutex with a cloned byte slice.
    - Semantics: Flushes staged gap bytes to resynchronize client stream after baseline snapshot.
  - Proxy_OnExited_Eio_Success (func(ptyProxy *PtyProxy)) [REQUIRED]
    - Purpose: Teardown callback for clean child process exit (exit code 0).
    - Trigger Condition: Invoked on Reader_OnExited_Eio after cmd.Wait() returns exit code 0.
    - Semantics: Signals successful child process completion.
  - Proxy_OnExited_Eio_Failure (func(ptyProxy *PtyProxy)) [REQUIRED]
    - Purpose: Teardown callback for non-zero child process exit code.
    - Trigger Condition: Invoked on Reader_OnExited_Eio after cmd.Wait() returns non-zero exit status.
    - Semantics: Signals child process execution failure.
  - Proxy_OnExited_Eio_Killed (func(ptyProxy *PtyProxy)) [REQUIRED]
    - Purpose: Teardown callback for signal-terminated child process.
    - Trigger Condition: Invoked on Reader_OnExited_Eio after cmd.Wait() indicates process was killed by OS signal.
    - Semantics: Signals OS signal termination (SIGKILL, SIGTERM).
  - Proxy_OnExited_Closed (func(ptyProxy *PtyProxy)) [REQUIRED]
    - Purpose: Teardown callback for administrative master descriptor closure.
    - Trigger Condition: Invoked on Reader_OnExited_Closed when master descriptor is closed by server.
    - Semantics: Signals explicit session teardown by server.
  - Proxy_OnExited_SystemError (func(ptyProxy *PtyProxy, readerTerminalSignal error)) [REQUIRED]
    - Purpose: Exception callback for unexpected OS read failures.
    - Trigger Condition: Invoked on Reader_OnExited_SystemError when PtyReader encounters an abnormal kernel read error.
    - Semantics: Catches unexpected OS system call failures.

Threading & Memory Ownership Model:
 1. PtyReader.Reader_StagingBuffer (Single-Threaded Ownership):
    Owned 100% single-threaded by background PtyReader goroutine. Bytes are cloned outside Proxy_Mutex before live egress dispatch.
 2. PtyProxy.Proxy_PostSnapshotBuffer (Multi-Threaded Shared State):
    Shared between PtyReader goroutine and web request goroutines. Bytes are cloned under Proxy_Mutex before lock release.

Domain Invariants:
  - 1. Unidirectional Parent Ownership: PtyProxy owns PtyReader with zero back-pointers to PtyProxy.
  - 2. Microsecond Lock Hold Times: Proxy_Mutex protects in-memory RAM mutations only; egress callbacks are invoked 100% outside lock.
  - 3. Zero-Loss Gap-Byte Staging: Stdout bytes arriving during snapshot serialization accumulate in Proxy_PostSnapshotBuffer for replay upon live mode return.
  - 4. Synchronous vs. Asynchronous Failure Separation: Inline pty.Start spawn errors return synchronously; post-spawn errors route through Proxy_OnExited_* callbacks.

===============================================================================
*/
type PtyProxy struct {
	Pty_MasterFileDescriptor          *os.File
	Proxy_Id                          uint32
	Proxy_Mutex                       sync.Mutex
	Proxy_Mode                        PtyProxyMode
	Proxy_TerminalCommand             *exec.Cmd
	Proxy_TerminalState               *xterm.Terminal
	Proxy_PtyReader                   *PtyReader
	Proxy_PostSnapshotBuffer          *bytes.Buffer
	Proxy_OnOutput_Live               func(ptyProxy *PtyProxy, ptyOutputData []byte)
	Proxy_OnOutput_Snapshot           func(ptyProxy *PtyProxy, ptyOutputData []byte)
	Proxy_OnOutput_PostSnapshotBuffer func(ptyProxy *PtyProxy, ptyOutputData []byte)
	Proxy_OnExited_Eio_Success        func(ptyProxy *PtyProxy)
	Proxy_OnExited_Eio_Failure        func(ptyProxy *PtyProxy)
	Proxy_OnExited_Eio_Killed         func(ptyProxy *PtyProxy)
	Proxy_OnExited_Closed             func(ptyProxy *PtyProxy)
	Proxy_OnExited_SystemError        func(ptyProxy *PtyProxy, readerTerminalSignal error)
}

/*
===============================================================================
NewPtyProxy: Pseudo-Terminal Proxy Factory & Spawn Initializer
===============================================================================

System Context & Factory Responsibilities:
NewPtyProxy is the synchronous constructor and process spawner for PtyProxy.
It allocates the headless VTE grid, pre-allocates RAM staging buffers, spawns the OS PTY
child process, wires sub-component PtyReader callbacks, and launches the background reader loop.

Synchronous vs. Asynchronous Lifecycle Failure Separation:
 1. Synchronous Phase (NewPtyProxy Call):
    Any inline spawn failure during pty.Start (e.g. non-existent Proxy_DirectoryPath, missing shell binary,
    or OS process/descriptor exhaustion) returns (nil, ptyStartError) immediately. Zero memory, goroutines, or
    zombie processes leak, and NO Proxy_OnExited_* callbacks are ever fired.
 2. Asynchronous Phase (Post-Return Execution):
    Once NewPtyProxy returns (newPtyProxyResult, nil) successfully, all subsequent stream terminations or failures
    (child process exit, server descriptor closure, or host OS system errors) flow exclusively
    through the asynchronous Proxy_OnExited_* callback suite.

Struct Property Contracts & Semantics (NewPtyProxyApi):
  - Proxy_Id (uint32) [REQUIRED]
    - Purpose: Unique 32-bit identifier assigned to the new PtyProxy instance.
    - Contract: Must be non-zero and unique across active PTY proxy sessions.
    - Semantics: Transferred directly to newPtyProxyResult.Proxy_Id.
  - Proxy_ColumnCount (int) [REQUIRED]
    - Purpose: Initial terminal grid column count (e.g. 80).
    - Contract: Must be a positive integer greater than zero.
    - Semantics: Configures initial width of xterm.Terminal VTE grid.
  - Proxy_RowCount (int) [REQUIRED]
    - Purpose: Initial terminal grid row count (e.g. 24).
    - Contract: Must be a positive integer greater than zero.
    - Semantics: Configures initial height of xterm.Terminal VTE grid.
  - Command_ShellBinaryPath (string) [REQUIRED]
    - Purpose: Absolute file path to shell executable (e.g. "/bin/bash").
    - Contract: Must point to an existing, executable binary on the host OS.
    - Semantics: Passed to exec.Command() and spawned via pty.Start().
  - Proxy_EnvironmentVariables ([]string) [OPTIONAL]
    - Purpose: Environment variable key-value pairs (KEY=VAL) passed to the spawned process.
    - Contract: May be nil or empty; elements must follow standard OS environment format.
    - Semantics: Assigned to __Proxy_TerminalCommand.Env before process start.
  - Proxy_DirectoryPath (string) [OPTIONAL]
    - Purpose: Initial working directory for the spawned child process.
    - Contract: May be empty string; if non-empty, must be a valid directory path.
    - Semantics: Assigned to __Proxy_TerminalCommand.Dir before process start.
  - TerminalState_ScrollbackLineCount (int) [REQUIRED]
    - Purpose: VTE grid scrollback history line limit.
    - Contract: Must be a non-negative integer (e.g. 1000).
    - Semantics: Configures scrollback buffer capacity in xterm.Terminal.
  - Reader_StagingBufferSize (int) [REQUIRED]
    - Purpose: Pre-allocated byte capacity for PtyReader.Reader_StagingBuffer (e.g. 512 KB).
    - Contract: Must be a positive integer (recommended 512 KB = 524288).
    - Semantics: Determines zero-allocation headroom for background kernel PTY reads.
  - Proxy_PostSnapshotBufferSize (int) [REQUIRED]
    - Purpose: Pre-allocated byte capacity for Proxy_PostSnapshotBuffer (e.g. 64 KB).
    - Contract: Must be a positive integer (recommended 64 KB = 65536).
    - Semantics: Determines RAM buffer capacity for staging gap bytes during PostSnapshot mode.
  - Proxy_OnPtySpawned (func(ptyProxy *PtyProxy)) [REQUIRED]
    - Purpose: Initialization callback invoked synchronously right after pty.Start succeeds.
    - Trigger Condition: Invoked inline inside NewPtyProxy right after pty.Start returns success.
    - Consumer Contract: Must configure initial Proxy_Mode (e.g. PtyProxyMode_Running_Live) and return promptly.
    - Semantics: Allows caller to register or configure initial proxy operational mode before reader loop starts.
  - Proxy_OnOutput_Live (func(ptyProxy *PtyProxy, ptyOutputData []byte)) [REQUIRED]
    - Purpose: Egress callback streaming stdout during PtyProxyMode_Running_Live.
    - Trigger Condition: Invoked when stdout bytes arrive during PtyProxyMode_Running_Live.
    - Consumer Contract: Must be non-blocking.
    - Semantics: Transferred directly to newPtyProxyResult.Proxy_OnOutput_Live.
  - Proxy_OnOutput_Snapshot (func(ptyProxy *PtyProxy, ptyOutputData []byte)) [REQUIRED]
    - Purpose: Egress callback streaming baseline snapshot during PreToPostSnapshot mode transition.
    - Trigger Condition: Invoked when baseline snapshot string is serialized under lock.
    - Consumer Contract: Must be non-blocking.
    - Semantics: Transferred directly to newPtyProxyResult.Proxy_OnOutput_Snapshot.
  - Proxy_OnOutput_PostSnapshotBuffer (func(ptyProxy *PtyProxy, ptyOutputData []byte)) [REQUIRED]
    - Purpose: Egress callback streaming gap bytes during PostSnapshotToLive mode transition.
    - Trigger Condition: Invoked when transitioning from PostSnapshot back to Live mode if gap bytes exist.
    - Consumer Contract: Must be non-blocking.
    - Semantics: Transferred directly to newPtyProxyResult.Proxy_OnOutput_PostSnapshotBuffer.
  - Proxy_OnExited_Eio_Success (func(ptyProxy *PtyProxy)) [REQUIRED]
    - Purpose: Callback invoked when child process exits with exit code 0.
    - Trigger Condition: Invoked on Reader_OnExited_Eio when cmd.Wait() returns exit code 0.
    - Semantics: Transferred directly to newPtyProxyResult.Proxy_OnExited_Eio_Success.
  - Proxy_OnExited_Eio_Failure (func(ptyProxy *PtyProxy)) [REQUIRED]
    - Purpose: Callback invoked when child process exits with non-zero exit code.
    - Trigger Condition: Invoked on Reader_OnExited_Eio when cmd.Wait() returns non-zero exit code.
    - Semantics: Transferred directly to newPtyProxyResult.Proxy_OnExited_Eio_Failure.
  - Proxy_OnExited_Killed (func(ptyProxy *PtyProxy)) [REQUIRED]
    - Purpose: Callback invoked when child process is terminated by OS signal.
    - Trigger Condition: Invoked on Reader_OnExited_Eio when cmd.Wait() indicates process signal termination.
    - Semantics: Transferred directly to newPtyProxyResult.Proxy_OnExited_Eio_Killed.
  - Proxy_OnExited_Closed (func(ptyProxy *PtyProxy)) [REQUIRED]
    - Purpose: Callback invoked when master PTY descriptor is closed by server.
    - Trigger Condition: Invoked on Reader_OnExited_Closed when master descriptor is closed.
    - Semantics: Transferred directly to newPtyProxyResult.Proxy_OnExited_Closed.
  - Proxy_OnExited_SystemError (func(ptyProxy *PtyProxy, readerTerminalSignal error)) [REQUIRED]
    - Purpose: Callback invoked on unexpected OS read system call errors.
    - Trigger Condition: Invoked on Reader_OnExited_SystemError when unexpected OS error occurs.
    - Semantics: Transferred directly to newPtyProxyResult.Proxy_OnExited_SystemError.

===============================================================================
*/
type NewPtyProxyApi struct {
	Proxy_Id                          uint32
	Proxy_ColumnCount                 int
	Proxy_RowCount                    int
	Command_ShellBinaryPath           string
	Proxy_EnvironmentVariables        []string
	Proxy_DirectoryPath               string
	TerminalState_ScrollbackLineCount int
	Reader_StagingBufferSize          int
	Proxy_PostSnapshotBufferSize      int
	Proxy_OnPtySpawned                func(ptyProxy *PtyProxy)
	Proxy_OnOutput_Live               func(ptyProxy *PtyProxy, ptyOutputData []byte)
	Proxy_OnOutput_Snapshot           func(ptyProxy *PtyProxy, ptyOutputData []byte)
	Proxy_OnOutput_PostSnapshotBuffer func(ptyProxy *PtyProxy, ptyOutputData []byte)
	Proxy_OnExited_Eio_Success        func(ptyProxy *PtyProxy)
	Proxy_OnExited_Eio_Failure        func(ptyProxy *PtyProxy)
	Proxy_OnExited_Eio_Killed         func(ptyProxy *PtyProxy)
	Proxy_OnExited_Closed             func(ptyProxy *PtyProxy)
	Proxy_OnExited_SystemError        func(ptyProxy *PtyProxy, readerTerminalSignal error)
}

func NewPtyProxy(api NewPtyProxyApi) (*PtyProxy, error) {
	newPtyProxyResult := &PtyProxy{
		Pty_MasterFileDescriptor:          nil,
		Proxy_Id:                          api.Proxy_Id,
		Proxy_Mode:                        PtyProxyMode_Spawning,
		Proxy_TerminalCommand:             nil,
		Proxy_TerminalState:               nil,
		Proxy_PostSnapshotBuffer:          nil,
		Proxy_OnOutput_Live:               api.Proxy_OnOutput_Live,
		Proxy_OnOutput_Snapshot:           api.Proxy_OnOutput_Snapshot,
		Proxy_OnOutput_PostSnapshotBuffer: api.Proxy_OnOutput_PostSnapshotBuffer,
		Proxy_OnExited_Eio_Success:        api.Proxy_OnExited_Eio_Success,
		Proxy_OnExited_Eio_Failure:        api.Proxy_OnExited_Eio_Failure,
		Proxy_OnExited_Eio_Killed:         api.Proxy_OnExited_Eio_Killed,
		Proxy_OnExited_Closed:             api.Proxy_OnExited_Closed,
		Proxy_OnExited_SystemError:        api.Proxy_OnExited_SystemError,
	}
	newPtyProxyResult.Proxy_PtyReader = &PtyReader{
		Pty_MasterFileDescriptor:               nil,
		Reader_StagingBuffer:                   nil,
		Reader_UnflushedStagingBufferSliceSize: 0,
		Reader_OnTryFlush:                      newPtyProxyResult.Proxy_HandleTryFlush,
		Reader_OnBlockingFlush:                 newPtyProxyResult.Proxy_HandleBlockingFlush,
		Reader_OnExited_Closed:                 newPtyProxyResult.Proxy_HandleExited_Closed,
		Reader_OnExited_Eio:                    newPtyProxyResult.Proxy_HandleExited_Eio,
		Reader_OnExited_SystemError:            newPtyProxyResult.Proxy_HandleExited_SystemError,
	}
	newPtyProxyResult.Proxy_PtyReader.Reader_StagingBuffer = make(
		[]byte,
		api.Reader_StagingBufferSize,
	)
	newPtyProxyResult.Proxy_PostSnapshotBuffer = bytes.NewBuffer(
		make(
			[]byte,
			0,
			api.Proxy_PostSnapshotBufferSize,
		),
	)
	newPtyProxyResult.Proxy_TerminalState = xterm.New(
		xterm.WithCols(api.Proxy_ColumnCount),
		xterm.WithRows(api.Proxy_RowCount),
		xterm.WithScrollback(api.TerminalState_ScrollbackLineCount),
	)
	__Proxy_TerminalCommand := exec.Command(api.Command_ShellBinaryPath)
	__Proxy_TerminalCommand.Env = api.Proxy_EnvironmentVariables
	__Proxy_TerminalCommand.Dir = api.Proxy_DirectoryPath
	__Pty_MasterFileDescriptor, ptyStartError := pty.Start(__Proxy_TerminalCommand)
	if ptyStartError != nil {
		return nil, ptyStartError
	}
	newPtyProxyResult.Proxy_TerminalCommand = __Proxy_TerminalCommand
	newPtyProxyResult.Pty_MasterFileDescriptor = __Pty_MasterFileDescriptor
	newPtyProxyResult.Proxy_PtyReader.Pty_MasterFileDescriptor = __Pty_MasterFileDescriptor
	api.Proxy_OnPtySpawned(newPtyProxyResult)
	go newPtyProxyResult.Proxy_PtyReader.Reader_Run()
	return newPtyProxyResult, nil
}

/*
===============================================================================
Proxy_LockAndReturnTrue: Mutex Lock Adapter Method
===============================================================================

Proxy_LockAndReturnTrue locks thisPtyProxy.Proxy_Mutex and returns true.
It serves as a raw method value callback adapter (signature func() bool) matching sync.Mutex.TryLock
for deterministic blocking lock acquisition inside __flushReaderStagingBufferSliceIfLockAcquired.

===============================================================================
*/
func (thisPtyProxy *PtyProxy) Proxy_LockAndReturnTrue() bool {
	thisPtyProxy.Proxy_Mutex.Lock()
	return true
}

/*
===============================================================================
__flushReaderStagingBufferSliceIfLockAcquired: Higher-Order Lock-Scoped Output Flush Pipeline
===============================================================================

__flushReaderStagingBufferSliceIfLockAcquired encapsulates the lock-scoped VTE grid update, mode check, and lock release.
It invokes maybeAcquirePtyProxyMutexLock and updates Proxy_TerminalState under lock, accumulating in Proxy_PostSnapshotBuffer if in PostSnapshot mode, releasing the lock, and dispatching live output outside the mutex if in Live mode.

===============================================================================
*/
func __flushReaderStagingBufferSliceIfLockAcquired(
	maybeAcquirePtyProxyMutexLock func() bool,
	ptyProxy *PtyProxy,
	unflushedStagingBufferSlice []byte,
) bool {
	if maybeAcquirePtyProxyMutexLock() {
		var isWasPtyProxyModeRunningLive bool
		ptyProxy.Proxy_TerminalState.Write(unflushedStagingBufferSlice)
		if ptyProxy.Proxy_Mode == PtyProxyMode_Running_Live {
			isWasPtyProxyModeRunningLive = true
		} else if ptyProxy.Proxy_Mode == PtyProxyMode_Running_PostSnapshot {
			ptyProxy.Proxy_PostSnapshotBuffer.Write(unflushedStagingBufferSlice)
		}
		ptyProxy.Proxy_Mutex.Unlock()
		if isWasPtyProxyModeRunningLive {
			ptyProxy.Proxy_OnOutput_Live(
				ptyProxy,
				bytes.Clone(unflushedStagingBufferSlice),
			)
		}
		return true
	}
	return false
}

/*
===============================================================================
Proxy_HandleTryFlush: Optimistic Non-Blocking Output Flush Handler
===============================================================================

Proxy_HandleTryFlush is the primary non-blocking flush callback assigned to PtyReader.Reader_OnTryFlush.
It attempts opportunistic lock acquisition via Proxy_Mutex.TryLock, updating VTE state and streaming live output if free, or returning false immediately without blocking if busy.

===============================================================================
*/
func (thisPtyProxy *PtyProxy) Proxy_HandleTryFlush(unflushedStagingBufferSlice []byte) bool {
	return __flushReaderStagingBufferSliceIfLockAcquired(
		thisPtyProxy.Proxy_Mutex.TryLock,
		thisPtyProxy,
		unflushedStagingBufferSlice,
	)
}

/*
===============================================================================
Proxy_HandleBlockingFlush: Deterministic Blocking Output Flush Handler
===============================================================================

Proxy_HandleBlockingFlush is the fallback blocking flush callback assigned to PtyReader.Reader_OnBlockingFlush.
It is invoked under 100% staging buffer saturation when TryLock fails, blocking on Proxy_Mutex.Lock() to flush accumulated bytes and apply controlled backpressure.

===============================================================================
*/
func (thisPtyProxy *PtyProxy) Proxy_HandleBlockingFlush(unflushedStagingBufferSlice []byte) {
	__flushReaderStagingBufferSliceIfLockAcquired(
		thisPtyProxy.Proxy_LockAndReturnTrue,
		thisPtyProxy,
		unflushedStagingBufferSlice,
	)
}

/*
===============================================================================
__executeExitedTeardownPipeline: Higher-Order Process Teardown & Reaping Pipeline
===============================================================================

__executeExitedTeardownPipeline updates Proxy_Mode to PtyProxyMode_Exited under lock, unlocks Proxy_Mutex, waits for child process termination via cmd.Wait() outside the lock, and dispatches onDispatchExitedHandler with readerTerminalSignal.

===============================================================================
*/
func __executeExitedTeardownPipeline(
	onDispatchExitedHandler func(readerTerminalSignal error),
	ptyProxy *PtyProxy,
	readerTerminalSignal error,
) {
	ptyProxy.Proxy_Mutex.Lock()
	ptyProxy.Proxy_Mode = PtyProxyMode_Exited
	ptyProxy.Proxy_Mutex.Unlock()
	ptyProxy.Proxy_TerminalCommand.Wait()
	onDispatchExitedHandler(readerTerminalSignal)
}

/*
===============================================================================
Proxy_HandleExited_Closed: Server-Initiated Descriptor Closure Handler
===============================================================================

Proxy_HandleExited_Closed is the teardown callback assigned to PtyReader.Reader_OnExited_Closed.
It updates Proxy_Mode to PtyProxyMode_Exited under lock, waits for child process termination via cmd.Wait() outside the lock, and dispatches Proxy_OnExited_Closed.

===============================================================================
*/
func (thisPtyProxy *PtyProxy) Proxy_HandleExited_Closed(readerTerminalSignal error) {
	__executeExitedTeardownPipeline(
		thisPtyProxy.Proxy_HandleDispatchExitedHandler_Closed,
		thisPtyProxy,
		readerTerminalSignal,
	)
}

func (thisPtyProxy *PtyProxy) Proxy_HandleDispatchExitedHandler_Closed(_ error) {
	thisPtyProxy.Proxy_OnExited_Closed(thisPtyProxy)
}

/*
===============================================================================
Proxy_HandleExited_Eio: Child Process Termination & EOF Handler
===============================================================================

Proxy_HandleExited_Eio is the teardown callback assigned to PtyReader.Reader_OnExited_Eio.
It updates Proxy_Mode to PtyProxyMode_Exited under lock, reaps process status via cmd.Wait() outside the lock, and routes to Proxy_OnExited_Eio_Success, Proxy_OnExited_Eio_Failure, or Proxy_OnExited_Eio_Killed based on exit status.

===============================================================================
*/
func (thisPtyProxy *PtyProxy) Proxy_HandleExited_Eio(readerTerminalSignal error) {
	__executeExitedTeardownPipeline(
		thisPtyProxy.Proxy_HandleDispatchExitedHandler_Eio,
		thisPtyProxy,
		readerTerminalSignal,
	)
}

func (thisPtyProxy *PtyProxy) Proxy_HandleDispatchExitedHandler_Eio(_ error) {
	processState := thisPtyProxy.Proxy_TerminalCommand.ProcessState
	processWaitStatus := processState.Sys().(syscall.WaitStatus)
	if processWaitStatus.Signaled() {
		thisPtyProxy.Proxy_OnExited_Eio_Killed(thisPtyProxy)
	} else if processState.Success() {
		thisPtyProxy.Proxy_OnExited_Eio_Success(thisPtyProxy)
	} else {
		thisPtyProxy.Proxy_OnExited_Eio_Failure(thisPtyProxy)
	}
}

/*
===============================================================================
Proxy_HandleExited_SystemError: OS Kernel System Error Handler
===============================================================================

Proxy_HandleExited_SystemError is the exception callback assigned to PtyReader.Reader_OnExited_SystemError.
It updates Proxy_Mode to PtyProxyMode_Exited under lock, reaps child process termination via cmd.Wait() outside the lock, and dispatches Proxy_OnExited_SystemError with the terminal signal.

===============================================================================
*/
func (thisPtyProxy *PtyProxy) Proxy_HandleExited_SystemError(readerTerminalSignal error) {
	__executeExitedTeardownPipeline(
		thisPtyProxy.Proxy_HandleDispatchExitedHandler_SystemError,
		thisPtyProxy,
		readerTerminalSignal,
	)
}

func (thisPtyProxy *PtyProxy) Proxy_HandleDispatchExitedHandler_SystemError(readerTerminalSignal error) {
	thisPtyProxy.Proxy_OnExited_SystemError(
		thisPtyProxy,
		readerTerminalSignal,
	)
}

/*
===============================================================================
Proxy_TransitionMode_LiveToPreSnapshot: Live to Pre-Snapshot Mode Transition
===============================================================================

Proxy_TransitionMode_LiveToPreSnapshot transitions Proxy_Mode from PtyProxyMode_Running_Live to PtyProxyMode_Running_PreSnapshot under lock.
It prepares PtyProxy for baseline snapshot capture when a new web client connects.

===============================================================================
*/
func (thisPtyProxy *PtyProxy) Proxy_TransitionMode_LiveToPreSnapshot() {
	thisPtyProxy.Proxy_Mutex.Lock()
	thisPtyProxy.Proxy_Mode = PtyProxyMode_Running_PreSnapshot
	thisPtyProxy.Proxy_Mutex.Unlock()
}

/*
===============================================================================
Proxy_TransitionMode_PreToPostSnapshot: Pre-Snapshot to Post-Snapshot Mode Transition
===============================================================================

Proxy_TransitionMode_PreToPostSnapshot serializes the current ANSI baseline state from Proxy_TerminalState under lock, resets Proxy_PostSnapshotBuffer to begin staging gap bytes, updates Proxy_Mode to PtyProxyMode_Running_PostSnapshot, and dispatches the snapshot via Proxy_OnOutput_Snapshot outside the lock.

===============================================================================
*/
func (thisPtyProxy *PtyProxy) Proxy_TransitionMode_PreToPostSnapshot() {
	thisPtyProxy.Proxy_Mutex.Lock()
	thisPtyProxy.Proxy_PostSnapshotBuffer.Reset()
	thisPtyProxy.Proxy_Mode = PtyProxyMode_Running_PostSnapshot
	serializeAddon := xterm.NewSerializeAddon(thisPtyProxy.Proxy_TerminalState)
	snapshotBytes := serializeAddon.Serialize(nil)
	thisPtyProxy.Proxy_Mutex.Unlock()
	thisPtyProxy.Proxy_OnOutput_Snapshot(
		thisPtyProxy,
		snapshotBytes,
	)
}

/*
===============================================================================
Proxy_TransitionMode_PostSnapshotToLive: Post-Snapshot to Live Mode Transition
===============================================================================

Proxy_TransitionMode_PostSnapshotToLive extracts and resets staged gap bytes from Proxy_PostSnapshotBuffer under lock, updates Proxy_Mode back to PtyProxyMode_Running_Live, and streams accumulated gap bytes via Proxy_OnOutput_PostSnapshotBuffer outside the lock.

===============================================================================
*/
func (thisPtyProxy *PtyProxy) Proxy_TransitionMode_PostSnapshotToLive() {
	thisPtyProxy.Proxy_Mutex.Lock()
	clonedPostSnapshotBuffer := bytes.Clone(thisPtyProxy.Proxy_PostSnapshotBuffer.Bytes())
	thisPtyProxy.Proxy_Mode = PtyProxyMode_Running_Live
	thisPtyProxy.Proxy_Mutex.Unlock()
	if len(clonedPostSnapshotBuffer) > 0 {
		thisPtyProxy.Proxy_OnOutput_PostSnapshotBuffer(
			thisPtyProxy,
			clonedPostSnapshotBuffer,
		)
	}
}

/*
===============================================================================
Proxy_Resize: PTY Window & VTE Grid Resizer
===============================================================================

Proxy_Resize updates OS kernel PTY window dimensions via pty.Setsize and resizes the in-memory VTE grid (Proxy_TerminalState) under Proxy_Mutex to guarantee thread safety against concurrent stream flushes.

===============================================================================
*/
func (thisPtyProxy *PtyProxy) Proxy_Resize(
	nextColumnCount int,
	nextRowCount int,
) error {
	nextWinsize := &pty.Winsize{
		Rows: uint16(nextRowCount),
		Cols: uint16(nextColumnCount),
	}
	ptySetSizeError := pty.Setsize(
		thisPtyProxy.Pty_MasterFileDescriptor,
		nextWinsize,
	)
	if ptySetSizeError != nil {
		return ptySetSizeError
	}
	thisPtyProxy.Proxy_Mutex.Lock()
	thisPtyProxy.Proxy_TerminalState.Resize(
		nextColumnCount,
		nextRowCount,
	)
	thisPtyProxy.Proxy_Mutex.Unlock()
	return nil
}
