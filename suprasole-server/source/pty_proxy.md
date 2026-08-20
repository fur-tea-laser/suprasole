# PtyProxy: Pseudo-Terminal Proxy Engine

## System Context & Operational Mode Architecture

PtyProxy is the thread-safe Pseudo-Terminal Proxy engine that manages OS PTY master file descriptors,
headless VTE terminal state maintenance (*xterm.Terminal), background stream draining via PtyReader,
and atomic mode-driven stdout egress routing with zero-loss Proxy_PostSnapshotBuffer staging during baseline snapshot serialization.

## State Machine & Operational Mode Transition Complexity

PtyProxy continuously maintains full headless VTE terminal screen state (Proxy_TerminalState) and uses a formal state machine (PtyProxyMode) to synchronize background stdout stream processing with baseline snapshot serialization and session teardown:

 1. Operational State Enum Definitions:
    - PtyProxyMode_Spawning: Initializing OS child process, PTY descriptors, and callback bindings.
    - PtyProxyMode_Running_Live: Direct live streaming mode; incoming stdout bytes update VTE state and dispatch live via Proxy_OnOutput_Live.
    - PtyProxyMode_Running_PreSnapshot: Pre-snapshot mode; incoming stdout bytes update VTE state while live egress is suppressed in preparation for baseline snapshot serialization.
    - PtyProxyMode_Running_PostSnapshot: Post-snapshot mode; serializing baseline ANSI snapshot under lock while incoming stdout bytes update VTE state and accumulate in Proxy_PostSnapshotBuffer.
    - PtyProxyMode_Exited: Teardown mode; read loop termination triggers reader cleanup, PTY descriptor cleanup, process reaping (Proxy_TerminalCommand.Wait()), and exit callback dispatch.

 2. Live Egress Suppression Rationale (LiveToPreSnapshot Stage):
    Transitioning from Running_Live to Running_PreSnapshot represents a mode transition from online live streaming to offline snapshot preparation. It suppresses live egress callbacks (Proxy_OnOutput_Live) while continuing background VTE grid updates under lock, pausing live output streaming while ensuring that Proxy_TerminalState remains continuously updated prior to baseline snapshot serialization.

 3. Atomic Grid Serialization & Zero-Loss Staging Rationale (PreToPostSnapshot Stage):
    Because xterm.NewSerializeAddon traverses internal grid line pointers and row buffers in Proxy_TerminalState, concurrent VTE state updates (Proxy_TerminalState.Write) driven by background stdout flushes during serialization would trigger fatal Go runtime data races and memory panics. Executing serialization strictly under Proxy_Mutex guarantees snapshot consistency. Concurrently mutating Proxy_Mode to Running_PostSnapshot under the same lock hold ensures that any stdout bytes arriving during snapshot serialization immediately accumulate in Proxy_PostSnapshotBuffer, preventing gap-data loss.

 4. Unlocked Replay & Downstream Decoupling Rationale (PostSnapshotToLive Stage):
    Transitioning back to Running_Live requires extracting and cloning Proxy_PostSnapshotBuffer.Bytes() under lock before releasing Proxy_Mutex. Cloning staged bytes under lock allows Proxy_OnOutput_PostSnapshotBuffer to be dispatched 100% unlocked outside Proxy_Mutex, eliminating downstream lock coupling and circular-wait deadlocks while seamlessly resynchronizing the stdout stream prior to resuming direct live output.

## Flush Handling Callback Complexity & Lock Scoping Strategy

The primary architectural complexity in PtyProxy centers on its flush handling callbacks (Proxy_HandleTryFlush and Proxy_HandleBlockingFlush) and their underlying helper __flushReaderStagingBufferSliceIfLockAcquired. This pipeline bridges PtyReader's lock-free background read loop with PtyProxy's thread-safe state machine across four critical boundaries:
 1. Downstream Coupling to PtyReader Staging Cushion:
    PtyReader drains the OS PTY descriptor into its pre-allocated staging cushion unlocked. It calls Proxy_HandleTryFlush via TryLock(). If Proxy_Mutex is contended, TryLock() returns false immediately, allowing PtyReader to continue absorbing PTY stdout without stalling kernel pipe draining. Only when the staging cushion saturates does PtyReader call Proxy_HandleBlockingFlush to wait on Proxy_Mutex.Lock().
 2. Lock Scoping & Deadlock Avoidance:
    __flushReaderStagingBufferSliceIfLockAcquired updates the VTE grid (*xterm.Terminal) and evaluates Proxy_Mode strictly under Proxy_Mutex. However, it MUST unlock Proxy_Mutex BEFORE calling Proxy_OnOutput_Live. Dispatching egress callbacks outside Proxy_Mutex eliminates downstream lock coupling and minimizes lock contention.
 3. Memory Safety & Allocation Nuances Across Staging Buffer Consumers:
    - Synchronous In-Memory Writers (Proxy_TerminalState.Write & Proxy_PostSnapshotBuffer.Write): Do NOT require byte cloning. Both methods synchronously consume or copy incoming bytes into their own backing memory during the locked execution window, leaving the caller's slice untouched.
    - Asynchronous Egress Callbacks (Proxy_OnOutput_Live): MUST receive bytes.Clone(unflushedStagingBufferSlice). Because egress callbacks execute outside Proxy_Mutex, PtyReader could immediately overwrite its reusable staging buffer on the next read iteration. Cloning guarantees complete memory safety across goroutines.
 4. Dynamic Dual-Egress Routing:
    Routes stdout bytes to dual targets (VTE Grid + Secondary Destination) depending on Proxy_Mode:
      - Running_Live: Dual Target -> VTE Grid + Live Callback (direct stdout streaming).
      - Running_PostSnapshot: Dual Target -> VTE Grid + Proxy_PostSnapshotBuffer (Proxy_PostSnapshotBuffer staging during snapshot delivery).
      - Running_PreSnapshot / Exited: Single Target -> VTE Grid only (live egress suppressed).

## Exit Handling, Process Reaping & Signal Propagation Pipeline

When background PtyReader stream draining terminates, PtyProxy executes a 3-stage teardown pipeline (__executeExitedTeardownPipeline) to safely reap the OS process and dispatch terminal signal notifications:

 1. Atomic Mode Transition & Lock Release Stage:
    PtyProxy acquires Proxy_Mutex, mutates Proxy_Mode = PtyProxyMode_Exited, and immediately releases Proxy_Mutex. Because PtyReader's read loop has already terminated and drained all stdout bytes prior to invoking the exit handler, mutating Proxy_Mode to Exited aligns instance state with mechanical reality, ensuring any concurrent goroutine querying Proxy_Mode observes the terminal Exited state. Unlocking Proxy_Mutex prior to process reaping ensures zero lock hold times during kernel process synchronization.

 2. Lock-Free OS Process Reaping Stage (Proxy_TerminalCommand.Wait()):
    PtyProxy invokes Proxy_TerminalCommand.Wait() strictly UNLOCKED outside Proxy_Mutex. This blocks the background reader thread until the operating system reaps the child process and populates ProcessState, preventing zombie processes without coupling kernel process waits to Proxy_Mutex.

 3. WaitStatus Signal Disambiguation & Exit Callback Dispatch Stage:
    Following Proxy_TerminalCommand.Wait(), PtyProxy evaluates the terminal signal received from PtyReader to dispatch the corresponding exit handler:
      - Closed Signal (Reader_OnExited_Closed): Invokes Proxy_OnExited_Closed, signaling administrative descriptor closure.
      - SystemError Signal (Reader_OnExited_SystemError): Invokes Proxy_OnExited_SystemError with the underlying OS read error.
      - EIO Signal (Reader_OnExited_Eio): Linux slave process termination generates syscall.EIO. Proxy_HandleDispatchExitedHandler_Eio inspects Proxy_TerminalCommand.ProcessState.Sys().(syscall.WaitStatus) to route to exactly ONE of three mutually exclusive leaf callbacks:
          - Killed Exit (processWaitStatus.Signaled()): Invokes Proxy_OnExited_Eio_Killed, signaling termination by OS signal (e.g. SIGKILL, SIGTERM).
          - Success Exit (processState.Success()): Invokes Proxy_OnExited_Eio_Success, signaling clean process exit with status code 0.
          - Failure Exit (non-zero exit code): Invokes Proxy_OnExited_Eio_Failure, signaling child process execution failure.

## Direct OS Kernel Stdin Writing & Error Semantics (Pty_MasterFileDescriptor.Write)

User stdin bytes are transmitted directly to the OS PTY master file descriptor via Pty_MasterFileDescriptor.Write(data). This operation bridges user input to the child process across both normal and edge-case execution states:

 1. Normal Operational Path (Unlocked Direct Kernel Write):
    Pty_MasterFileDescriptor.Write executes 100% UNLOCKED without acquiring Proxy_Mutex. Operating system kernel write() system calls on file descriptors are atomic and thread-safe at the OS level, allowing user input to pass directly to the child process stdin stream without contending with background VTE grid updates. On successful write, Write returns (len(data), nil).

 2. Post-Exit Write Edge Case (syscall.EIO / syscall.EPIPE):
    If a caller writes to Pty_MasterFileDescriptor after the slave process has exited (hung up), the kernel write() system call returns (0, syscall.EIO) or (0, syscall.EPIPE). Because the Go runtime ignores SIGPIPE on file descriptors by default, post-exit writes return a standard Go error (*os.PathError) and NEVER crash or panic the server process.

 3. Post-Closure Write Edge Case (os.ErrClosed):
    If Pty_MasterFileDescriptor.Close() has executed during administrative teardown, subsequent calls to Write() immediately return (0, os.ErrClosed) synchronously without issuing a kernel system call.

 4. Terminal Mode Ignorance & OS Kernel Error Determinism:
    The underlying *os.File descriptor operates independently of PtyProxy's internal Proxy_Mode. Calling Pty_MasterFileDescriptor.Write while Proxy_Mode is PtyProxyMode_Exited executes the OS system call directly, with return behavior governed entirely by kernel PTY stream state regardless of the instance mode.

 5. Synchronous Execution & Kernel Backpressure Blocking Semantics:
    Pty_MasterFileDescriptor.Write executes synchronously on the calling goroutine, incurring user-to-kernel context switch latency. Under normal system load with available kernel write ring buffer capacity, Write completes near-instantaneously (though subject to OS CPU scheduling and TTY driver lock contention). However, if the child process stops reading stdin and the kernel write ring buffer saturates, Write BLOCKS on kernel ring buffer backpressure until the child process drains stdin bytes or teardown invalidates the descriptor.

 6. Kernel FIFO Byte Ordering & Wait-Queue Serialization:
    Sequential and concurrent calls to Pty_MasterFileDescriptor.Write maintain strict First-In, First-Out (FIFO) byte ordering. The Linux kernel TTY driver appends incoming bytes to the write ring buffer in exact order of system call commitment. If multiple Write calls block concurrently due to kernel ring buffer backpressure, the kernel manages blocked threads in a FIFO wait queue, waking them sequentially as buffer space becomes available to ensure byte streams are never scrambled or reordered.

## File Descriptor Blocking Mode Architecture (Blocking vs. Non-Blocking & POSIX Symmetry Constraints)

PtyProxy configures its underlying OS master PTY file descriptor in standard **Blocking Mode** (default descriptor flags without `O_NONBLOCK`).

### 1. Architectural Rationale for Current Blocking Choice
- **Simplified High-Throughput Reader Loop**: `PtyReader` relies on a zero-overhead, single-threaded blocking `read()` loop over `Pty_MasterFileDescriptor`. Operating in blocking mode ensures `PtyReader` sleeps efficiently on kernel wait queues when stdout is idle, eliminating CPU-spinning, polling loops, or spurious `syscall.EAGAIN` retry handling.
- **Pragmatic Initial Baseline Execution Semantics**: Writing to `Pty_MasterFileDescriptor` copies data into the kernel's in-memory TTY buffer in RAM, returning to the caller in microseconds without waiting on external hardware or network transmission. Because the slave child process (`bash`, `readline`) continuously drains `stdin` while human keypresses and client frame bursts consume only a small fraction of the kernel's ~64 KB buffer, buffer headroom remains well above blocking thresholds during standard interactive sessions. Consequently, direct blocking writes execute near-instantaneously without stalling calling threads, providing a sound, low-complexity baseline for initial deployment.

### 2. POSIX File Descriptor Symmetry Constraint (The Symmetrical `O_NONBLOCK` Limitation)
For engineers evaluating future write pipeline enhancements or investigating non-blocking write semantics, POSIX file descriptor architecture imposes a key structural constraint:
- **Symmetrical Application Across Operations**: Non-blocking flags (`O_NONBLOCK`) are attached directly to the open file handle in operating system memory. Executing `fcntl(fd, F_SETFL, O_NONBLOCK)` forces **both `read()` and `write()` operations** on that descriptor handle to inherit non-blocking behavior.
- **Impact on `PtyReader`'s Read Loop**: POSIX does not permit assigning `O_NONBLOCK` exclusively to `write()` calls on a shared descriptor handle. If future engineers attempt to set `O_NONBLOCK` directly on `Pty_MasterFileDescriptor`, doing so will force `PtyReader`'s background `read()` loop to also become non-blocking (requiring `EAGAIN` polling or CPU-spinning).
- **Architectural Consideration for Non-Blocking Exploration**: If non-blocking write semantics are ever evaluated for future production needs, engineers exploring `O_NONBLOCK` directly on `Pty_MasterFileDescriptor` should note that doing so would require re-architecting `PtyReader` to handle non-blocking read polling; also, other architectural options exist for enabling non-blocking write semantics beyond modifying descriptor flags directly.

---

## Analysis of Master PTY Write Blocking Scenarios for Proxy Engineers

Although master PTY writes complete near-instantaneously during interactive usage, executing `Pty_MasterFileDescriptor.Write(data)` can block the calling goroutine under specific operational conditions. Server engineers must understand these five macro-level scenarios when designing upstream write pipelines:

### 1. Kernel TTY Buffer Watermark Saturation
- **Buffer Quotas**: The kernel allocates an in-memory input buffer quota (typically ~64 KB) for the PTY pair.
- **High Watermark Threshold**: Writes do **NOT** wait until the buffer is 100% full before blocking. The kernel enforces a high watermark threshold (blocking when remaining free headroom drops below ~4 KB).
- **Automatic Unblocking**: When free space drops below the high watermark, the kernel puts the calling write thread to sleep. As soon as the slave child process reads pending input and free space crosses back below the low watermark, the kernel automatically wakes the waiting writer thread.

### 2. Software Flow Control Suspension
- **User-Space Triggered Flow Control**: When software flow control is active, user-space callers or keypresses can instruct the operating system to pause or resume terminal output processing.
- **Writer Pause State**: While output is suspended, master writes block on kernel wait queues regardless of available buffer space until a resume command is received.
- **Technical APIs & Control Character Aliases**:
  - *Pause Command*: `Ctrl-S` (STOP / `XOFF` / byte `0x13` / `tcflow(fd, TCOOFF)`).
  - *Resume Command*: `Ctrl-Q` (START / `XON` / byte `0x11` / `tcflow(fd, TCOON)`).

### 3. Slow, Unresponsive, or Suspended Slave Child Process
- **Unresponsive Child Process**: If the child process (`bash`, `cat`, `gdb`) pauses input reading (due to heavy CPU work, blocking file I/O, or internal application locks), user input accumulates in the buffer until the watermark threshold is reached.
- **Background Process Suspension**: If the child process is suspended in the background, input reading stops entirely. Subsequent master writes fill the buffer watermark and block until the process is resumed.
- **Technical APIs & Control Signal Aliases**:
  - *Suspension Commands*: `Ctrl-Z` (Job Control Suspend / `SIGTSTP` / `SIGSTOP`).
  - *Resume Commands*: `fg` (Foreground Resume / `SIGCONT`).

### 4. Kernel System Write Serialization
- **Sequential OS Kernel Locks**: Operating system write calls on a single file descriptor are internally serialized by kernel locks (executing one write at a time). If a write system call is currently in progress, any subsequent write call to the same descriptor handle must wait for the preceding write to release the kernel lock before committing its payload.

### 5. OS Resource Exhaustion & Scheduling Delays
- Under severe system RAM exhaustion or CPU starvation, buffer memory allocation and thread wakeups experience OS scheduling delays, increasing overall write latency.

---

## Administrative PTY Master Descriptor Closure (Pty_MasterFileDescriptor.Close)

Closing the master PTY file descriptor via Pty_MasterFileDescriptor.Close() initiates an administrative session teardown from the parent process:

 1. Unlocked Direct Kernel Descriptor Closure:
    Pty_MasterFileDescriptor.Close() executes 100% UNLOCKED without acquiring Proxy_Mutex. Calling Close() invalidates the underlying OS master file descriptor handle immediately.

 2. Synchronous Non-Blocking Execution & Return Determinism:
    Pty_MasterFileDescriptor.Close() executes synchronously on the calling goroutine and returns immediately without blocking on child process exit (Proxy_TerminalCommand.Wait()) or background PtyReader loop termination. On its initial invocation, Close() executes the OS close() system call and returns nil (even if the slave process has already exited with EIO); on any subsequent invocation on the closed file handle, Go's *os.File immediately returns os.ErrClosed without issuing a kernel system call or causing server panics.

 3. Instantaneous PtyReader Unblocking (os.ErrClosed Signal):
    If background PtyReader is currently blocked on a kernel read() system call, closing the master file descriptor invalidates the kernel file handle, forcing read() to unblock immediately and return os.ErrClosed ("file already closed").

 4. Reader Flow Propagation Ownership & Caller Return Value Irrelevance:
    Callers MUST treat the return value of Pty_MasterFileDescriptor.Close() as irrelevant and MUST NOT conditionally alter interactions with PtyProxy based on the return value. All teardown state mutations, OS process reaping, and exit notifications originate exclusively from background PtyReader flow propagation following read loop termination.

 5. Idempotent Execution Semantics:
    Pty_MasterFileDescriptor.Close() is idempotent; repeated invocations safely return os.ErrClosed without side-effects or resource leaks, though standard architectural lifecycle design requires calling Close() at most once per session.

 6. Dual Resource Reaping Invariant (Process vs. Descriptor Cleanup):
    Pty_MasterFileDescriptor.Close() and Proxy_TerminalCommand.Wait() manage two completely independent OS kernel abstractions. Executing Proxy_TerminalCommand.Wait() reaps the child process exit status to prevent zombie processes, but DOES NOT close the master PTY file descriptor handle. Conversely, calling Close() releases the OS file descriptor table handle. Complete session teardown requires both operations, which PtyProxy encapsulates completely inside __executeExitedTeardownPipeline.
    - Consumer Abstraction & Manual Trigger Note: Consumers focus exclusively on triggering session closure via Close() and do not concern themselves with downstream OS resource cleanup side-effects. If a consumer manually calls Close(), PtyProxy's internal exit pipeline guarantees 100% complete process and descriptor reaping while handling redundant Close() calls safely.

## Multi-Threaded Concurrency Map & Synchronization Architecture

PtyProxy operates across two concurrent execution contexts that intersect at shared instance memory:

### 1. Background Reader Execution Context (Goroutine 1: PtyReader.Reader_StartReading)

    Executes continuously on a single background goroutine, reading PTY stdout bytes from the OS master file descriptor unlocked into PtyReader.Reader_StagingBuffer. PTY output and process teardown propagate through nine distinct mutually exclusive code paths.

    Note on Mutual Exclusivity: Because PtyReader.Reader_StartReading operates on a single background goroutine and Proxy_Mode evaluates atomically under Proxy_Mutex, exactly one path executes per flush or exit event. Furthermore, once a teardown path (Paths 7-9) is triggered upon read loop termination, no further flushes can ever occur.

    - Path 1: Optimistic Live Streaming Call Tree (Hot-Path TryFlush in Live Mode)
      PtyReader.Reader_StartReading
        -> Reader_OnTryFlush
          -> Proxy_HandleTryFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Proxy_Mutex.TryLock
              -> Proxy_TerminalState.Write
              -> Proxy_Mutex.Unlock
              -> Proxy_OnOutput_Live (bytes.Clone)

    - Path 2: Saturation Fallback Live Streaming Call Tree (BlockingFlush in Live Mode)
      PtyReader.Reader_StartReading
        -> Reader_OnBlockingFlush
          -> Proxy_HandleBlockingFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Proxy_LockAndReturnTrue
                -> Proxy_Mutex.Lock
              -> Proxy_TerminalState.Write
              -> Proxy_Mutex.Unlock
              -> Proxy_OnOutput_Live (bytes.Clone)

    - Path 3: Optimistic PostSnapshotBuffer Staging Call Tree (TryFlush in PostSnapshot Mode)
      PtyReader.Reader_StartReading
        -> Reader_OnTryFlush
          -> Proxy_HandleTryFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Proxy_Mutex.TryLock
              -> Proxy_TerminalState.Write
              -> Proxy_PostSnapshotBuffer.Write
              -> Proxy_Mutex.Unlock

    - Path 4: Saturation Fallback PostSnapshotBuffer Staging Call Tree (BlockingFlush in PostSnapshot Mode)
      PtyReader.Reader_StartReading
        -> Reader_OnBlockingFlush
          -> Proxy_HandleBlockingFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Proxy_LockAndReturnTrue
                -> Proxy_Mutex.Lock
              -> Proxy_TerminalState.Write
              -> Proxy_PostSnapshotBuffer.Write
              -> Proxy_Mutex.Unlock

    - Path 5: Optimistic Pre-Snapshot VTE Update Call Tree (Hot-Path TryFlush in PreSnapshot Mode)
      PtyReader.Reader_StartReading
        -> Reader_OnTryFlush
          -> Proxy_HandleTryFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Proxy_Mutex.TryLock
              -> Proxy_TerminalState.Write
              -> Proxy_Mutex.Unlock

    - Path 6: Saturation Fallback Pre-Snapshot VTE Update Call Tree (BlockingFlush in PreSnapshot Mode)
      PtyReader.Reader_StartReading
        -> Reader_OnBlockingFlush
          -> Proxy_HandleBlockingFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Proxy_LockAndReturnTrue
                -> Proxy_Mutex.Lock
              -> Proxy_TerminalState.Write
              -> Proxy_Mutex.Unlock

    - Path 7: Descriptor Closure Process Teardown Call Tree
      PtyReader.Reader_StartReading
        -> Reader_OnExited_Closed
          -> Proxy_HandleExited_Closed
            -> __executeExitedTeardownPipeline
              -> Proxy_Mutex.Lock (Proxy_Mode = Exited)
              -> Proxy_Mutex.Unlock
              -> Pty_MasterFileDescriptor.Close
              -> Proxy_TerminalCommand.Wait
              -> Proxy_HandleDispatchExitedHandler_Closed
                -> Proxy_OnExited_Closed

    - Path 8: Child Process EOF Exit Status Teardown Call Tree
      PtyReader.Reader_StartReading
        -> Reader_OnExited_Eio
          -> Proxy_HandleExited_Eio
            -> __executeExitedTeardownPipeline
              -> Proxy_Mutex.Lock (Proxy_Mode = Exited)
              -> Proxy_Mutex.Unlock
              -> Pty_MasterFileDescriptor.Close
              -> Proxy_TerminalCommand.Wait
              -> Proxy_HandleDispatchExitedHandler_Eio
                -> [Mutually Exclusive Leaf Callback - Exactly 1 Dispatched]:
                  - Proxy_OnExited_Eio_Success (if ExitStatus == 0)
                  - Proxy_OnExited_Eio_Failure (if ExitStatus != 0)
                  - Proxy_OnExited_Eio_Killed (if Signaled)

    - Path 9: Kernel System Error Teardown Call Tree
      PtyReader.Reader_StartReading
        -> Reader_OnExited_SystemError
          -> Proxy_HandleExited_SystemError
            -> __executeExitedTeardownPipeline
              -> Proxy_Mutex.Lock (Proxy_Mode = Exited)
              -> Proxy_Mutex.Unlock
              -> Pty_MasterFileDescriptor.Close
              -> Proxy_TerminalCommand.Wait
              -> Proxy_HandleDispatchExitedHandler_SystemError
                -> Proxy_OnExited_SystemError

### 2. Main / Workspace Execution Context

    Executes concurrently on main or workspace request goroutines. Unlike Goroutine 1's single-threaded loop, paths in this context are invoked on-demand across 3 distinct operational categories:

    - Resynchronization Pipeline Category (Paths 10-12):
      Sequential 3-step protocol (LiveToPreSnapshot -> PreToPostSnapshot -> PostSnapshotToLive) executed when transitioning between online and offline modes. Proxy_Mutex protects each step to ensure thread-safe mode updates and Proxy_PostSnapshotBuffer staging.

    - Window Geometry Category (Path 13):
      Invoked on client terminal resizes. Executes pty.Setsize unlocked on the OS master descriptor, then acquires Proxy_Mutex to atomically resize the VTE grid. Concurrent resizes are serialized under lock.

    - Direct OS Kernel I/O Category (Paths 14-15):
      Unlocked OS kernel system calls executing directly on Pty_MasterFileDescriptor. Path 14 (Write) passes stdin bytes to the kernel pipe unlocked (thread safety handled by Linux kernel I/O buffers). Path 15 (Close) atomically closes the master descriptor, inducing an OS EOF/closed failure on PtyReader and triggering Path 7 teardown on Goroutine 1.

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

    - Path 12: PostSnapshotBuffer Replay Call Tree
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

## Isolated Execution Spheres (Where Mutex IS NOT Required)

  - Internal PtyReader Loop: Draining kernel PTY file descriptor into Reader_StagingBuffer, tracking Reader_UnflushedStagingBufferSliceSize, and resetting write head are 100% single-threaded within PtyReader. No mutex is required inside PtyReader itself.
  - Egress Callback Dispatch: Egress callbacks (Proxy_OnOutput_Live, Proxy_OnOutput_Snapshot, Proxy_OnOutput_PostSnapshotBuffer, Proxy_OnExited_*) are dispatched 100% unlocked outside Proxy_Mutex. The parent handler acquires Proxy_Mutex first to update state and clone data, releasing Proxy_Mutex BEFORE dispatching the egress callback to eliminate downstream circular-wait deadlocks and minimize lock contention.

## State Mutation Concurrency Protections (Why Proxy_Mutex Is Mandatory)

  - VTE Grid Memory Race Protection: Prevents concurrent data races between background PtyReader stdout updates (Proxy_TerminalState.Write) and snapshot serialization (xterm.NewSerializeAddon) or window resizes (Proxy_TerminalState.Resize).
  - Mode Evaluation TOCTOU Protection: Prevents Time-Of-Check-To-Time-Of-Use races by evaluating Proxy_Mode and capturing the egress action (live dispatch flag vs. Proxy_PostSnapshotBuffer accumulation) atomically under Proxy_Mutex. This guarantees that stdout bytes arriving in Live mode are safely flagged for egress even if Proxy_Mode transitions on another goroutine before the callback completes outside lock.
  - PostSnapshotBuffer Buffer Race Protection: Prevents memory corruption when accumulating bytes in Proxy_PostSnapshotBuffer during PostSnapshot mode while concurrent mode transitions reset or clone the buffer.

## Domain Invariants

  - 1. Single-Threaded Zero-Lock Reader Core: PtyReader operates 100% single-threaded on a dedicated background goroutine (Reader_StartReading), absorbing kernel PTY stdout bytes into reusable staging memory with zero internal lock acquisition.
  - 2. Lock-Scoped Memory Integrity & Unlocked Egress: Proxy_Mutex protects in-memory state mutations (Proxy_TerminalState, Proxy_PostSnapshotBuffer, Proxy_Mode) exclusively. All external callbacks (Proxy_OnOutput_*, Proxy_OnExited_*) are dispatched 100% unlocked outside Proxy_Mutex with cloned allocations (bytes.Clone), eliminating downstream lock coupling and circular-wait deadlocks.
  - 3. Lossless Mode-Driven Resynchronization: Stdout bytes arriving while serializing and outputting baseline snapshots accumulate in Proxy_PostSnapshotBuffer under lock, ensuring zero stdout data loss across mode transitions.
  - 4. Unidirectional Callback Coupling: PtyProxy owns PtyReader with zero struct back-pointers; PtyReader delegates stream events to PtyProxy strictly through function values.
  - 5. Atomic Lifecycle State Machine: Proxy_Mode evaluates atomically under Proxy_Mutex. Once a teardown path is triggered upon read loop termination, PtyProxy transitions to PtyProxyMode_Exited and no further stdout flushes can ever occur.
  - 6. Callback Execution Contracts: Stdout egress callbacks (Proxy_OnOutput_*) MUST be non-blocking to prevent stalling stream draining. Teardown pipelines execute synchronously on the background reader thread post-loop, blocking on Proxy_TerminalCommand.Wait() outside Proxy_Mutex to reap child process exit status before dispatching exit callbacks.


