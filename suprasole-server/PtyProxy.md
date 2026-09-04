# PtyProxy: Pseudo-Terminal Proxy Engine

## System Context & Operational Mode Architecture

PtyProxy is the thread-safe Pseudo-Terminal Proxy engine that manages OS PTY master file descriptors,
headless VTE terminal state maintenance (*xterm.Terminal), background stream draining via PtyReader,
and atomic mode-driven stdout egress routing with zero-loss PostSnapshotBuffer staging during baseline snapshot serialization.

## State Machine & Operational Mode Transition Complexity

PtyProxy continuously maintains full headless VTE terminal screen state (TerminalState) and uses a formal state machine (Mode_PtyProxy) to synchronize background stdout stream processing with baseline snapshot serialization and session teardown:

 1. Operational State Enum Definitions:
    - SPAWNING__Mode_PtyProxy: Initializing OS child process, PTY descriptors, and callback bindings.
    - RUNNING_LIVE__Mode_PtyProxy: Direct live streaming mode; incoming stdout bytes update VTE state and dispatch live via OnOutput_Live.
    - RUNNING_PRE_SNAPSHOT__Mode_PtyProxy: Pre-snapshot mode; incoming stdout bytes update VTE state while live egress is suppressed in preparation for baseline snapshot serialization.
    - RUNNING_POST_SNAPSHOT__Mode_PtyProxy: Post-snapshot mode; serializing baseline ANSI snapshot under lock while incoming stdout bytes update VTE state and accumulate in PostSnapshotBuffer.
    - EXITED__Mode_PtyProxy: Teardown mode; read loop termination triggers reader cleanup, PTY descriptor cleanup, process reaping (TerminalCommand.Wait()), and exit callback dispatch.

 2. Live Egress Suppression Rationale (LiveToPreSnapshot Stage):
    Transitioning from RUNNING_LIVE__Mode_PtyProxy to RUNNING_PRE_SNAPSHOT__Mode_PtyProxy represents a mode transition from online live streaming to offline snapshot preparation. It suppresses live egress callbacks (OnOutput_Live) while continuing background VTE grid updates under lock, pausing live output streaming while ensuring that TerminalState remains continuously updated prior to baseline snapshot serialization.

 3. Atomic Grid Serialization & Zero-Loss Staging Rationale (PreToPostSnapshot Stage):
    Because xterm.NewSerializeAddon traverses internal grid line pointers and row buffers in TerminalState, concurrent VTE state updates (TerminalState.Write) driven by background stdout flushes during serialization would trigger fatal Go runtime data races and memory panics. Executing serialization strictly under Mutex guarantees snapshot consistency. Concurrently mutating Mode to RUNNING_POST_SNAPSHOT__Mode_PtyProxy under the same lock hold ensures that any stdout bytes arriving during snapshot serialization immediately accumulate in PostSnapshotBuffer, preventing gap-data loss.

 4. Unlocked Replay & Downstream Decoupling Rationale (PostSnapshotToLive Stage):
    Transitioning back to RUNNING_LIVE__Mode_PtyProxy requires extracting and cloning PostSnapshotBuffer.Bytes() under lock before releasing Mutex. Cloning staged bytes under lock allows OnOutput_PostSnapshotBuffer to be dispatched 100% unlocked outside Mutex, eliminating downstream lock coupling and circular-wait deadlocks while seamlessly resynchronizing the stdout stream prior to resuming direct live output.

## Initialization & Process Spawning Synchronization Architecture (NewPtyProxy & OnPtySpawned)

When initializing a new terminal instance via NewPtyProxy, PtyProxy enforces a strict 3-stage synchronous construction pipeline:

 1. Synchronous OS Process Creation & PTY Allocation (pty.Start):
    Allocates the Linux pseudo-terminal master/slave descriptor pair and forks the child process (fork/execve). Upon completion, the kernel PTY master descriptor is bound to newPtyProxyResult and PtyReader.

 2. Upstream Synchronous Handshake Hook (OnPtySpawned):
    Prior to launching background stream draining, NewPtyProxy synchronously invokes api.OnPtySpawned(newPtyProxyResult). This callback hook enables the upstream workspace orchestrator to:
      - Register the _WorkspacePty_ session wrapper in the active PtyPool under mutex.
      - Emit the SpawnPtyStatusEvent (0x0002) frame over WebSocket egress.

 3. Background Reader Goroutine Launch (go PtyReader.StartReading):
    Only after OnPtySpawned completes does NewPtyProxy spawn the background goroutine to execute PtyReader.StartReading().

### Critical Concurrency Protections of the OnPtySpawned Handshake:

 1. Wire Protocol Ingress/Egress Determinism:
    On the client-server WebSocket transport, downstream protocol decoders require that SpawnPtyStatusEvent (0x0002) precedes any PtyOutputEvent (0x0008) or PtyExitEvent (0x0005) for a given PTY ID. Launching PtyReader.StartReading() before session registration and status frame emission would allow fast child process output (e.g. bash startup banners, echo commands) to emit 0x0008 frames to the client before the client is notified of session creation (0x0002).

 2. Fast Process Exit & Race-Free Teardown Registration:
    If a child process terminates near-instantaneously (e.g., nonexistent binary, immediate exit 1, invalid shell script), the kernel PTY master descriptor triggers an instantaneous EIO/EOF on PtyReader, invoking OnExited_Eio_* and calling upstream HandleProcessExit. Invoking OnPtySpawned before launching StartReading guarantees that _WorkspacePty_ is already fully registered in PtyPool before any exit teardown callback can execute, preventing dropped exit events and unindexed terminal states.

## Flush Handling Callback Complexity & Lock Scoping Strategy

The primary architectural complexity in PtyProxy centers on its flush handling callbacks (HandleTryFlush and HandleBlockingFlush) and their underlying helper __flushReaderStagingBufferSliceIfLockAcquired. This pipeline bridges PtyReader's lock-free background read loop with PtyProxy's thread-safe state machine across four critical boundaries:
 1. Downstream Coupling to PtyReader Staging Cushion:
    PtyReader drains the OS PTY descriptor into its pre-allocated staging cushion unlocked. It calls HandleTryFlush via TryLock(). If Mutex is contended, TryLock() returns false immediately, allowing PtyReader to continue absorbing PTY stdout without stalling kernel pipe draining. Only when the staging cushion saturates does PtyReader call HandleBlockingFlush to wait on Mutex.Lock().
 2. Lock Scoping & Deadlock Avoidance:
    __flushReaderStagingBufferSliceIfLockAcquired updates the VTE grid (*xterm.Terminal) and evaluates Mode strictly under Mutex. However, it MUST unlock Mutex BEFORE calling OnOutput_Live. Dispatching egress callbacks outside Mutex eliminates downstream lock coupling and minimizes lock contention.
 3. Memory Safety & Allocation Nuances Across Staging Buffer Consumers:
    - Synchronous In-Memory Writers (TerminalState.Write & PostSnapshotBuffer.Write): Do NOT require byte cloning. Both methods synchronously consume or copy incoming bytes into their own backing memory during the locked execution window, leaving the caller's slice untouched.
    - Asynchronous Egress Callbacks (OnOutput_Live): MUST receive bytes.Clone(unflushedStagingBufferSlice). Because egress callbacks execute outside Mutex, PtyReader could immediately overwrite its reusable staging buffer on the next read iteration. Cloning guarantees complete memory safety across goroutines.
 4. Dynamic Dual-Egress Routing:
    Routes stdout bytes to dual targets (VTE Grid + Secondary Destination) depending on Mode:
      - RUNNING_LIVE__Mode_PtyProxy: Dual Target -> VTE Grid + Live Callback (direct stdout streaming).
      - RUNNING_POST_SNAPSHOT__Mode_PtyProxy: Dual Target -> VTE Grid + PostSnapshotBuffer (PostSnapshotBuffer staging during snapshot delivery).
      - RUNNING_PRE_SNAPSHOT__Mode_PtyProxy / EXITED__Mode_PtyProxy: Single Target -> VTE Grid only (live egress suppressed).

## Exit Handling, Process Reaping & Signal Propagation Pipeline

When background PtyReader stream draining terminates, PtyProxy executes a 3-stage teardown pipeline (__executeExitedTeardown) to safely reap the OS process and dispatch terminal signal notifications:

 1. Atomic Mode Transition & Lock Release Stage:
    PtyProxy acquires Mutex, mutates Mode = EXITED__Mode_PtyProxy, and immediately releases Mutex. Because PtyReader's read loop has already terminated and drained all stdout bytes prior to invoking the exit handler, mutating Mode to EXITED__Mode_PtyProxy aligns instance state with mechanical reality, ensuring any concurrent goroutine querying Mode observes the terminal EXITED__Mode_PtyProxy state. Unlocking Mutex prior to process reaping ensures zero lock hold times during kernel process synchronization.

 2. Lock-Free OS Process Reaping Stage (TerminalCommand.Wait()):
    PtyProxy invokes TerminalCommand.Wait() strictly UNLOCKED outside Mutex. This blocks the background reader thread until the operating system reaps the child process and populates ProcessState, preventing zombie processes without coupling kernel process waits to Mutex.

 3. WaitStatus Signal Disambiguation & Exit Callback Dispatch Stage:
    Following TerminalCommand.Wait(), PtyProxy evaluates the terminal signal received from PtyReader to dispatch the corresponding exit handler:
      - Closed Signal (OnExited_Closed): Invokes OnExited_Closed, signaling administrative descriptor closure.
      - SystemError Signal (OnExited_SystemError): Invokes OnExited_SystemError with the underlying OS read error.
      - EIO Signal (OnExited_Eio): Linux slave process termination generates syscall.EIO. HandleDispatchExited_Eio inspects TerminalCommand.ProcessState.Sys().(syscall.WaitStatus) to route to exactly ONE of three mutually exclusive leaf callbacks:
          - Killed Exit (processWaitStatus.Signaled()): Invokes OnExited_Eio_Killed, signaling termination by OS signal (e.g. SIGKILL, SIGTERM).
          - Success Exit (processState.Success()): Invokes OnExited_Eio_Success, signaling clean process exit with status code 0.
          - Failure Exit (non-zero exit code): Invokes OnExited_Eio_Failure, signaling child process execution failure.

## Direct OS Kernel Stdin Writing & Error Semantics (MasterFileDescriptor_PtyDevice.Write)

User stdin bytes are transmitted directly to the OS PTY master file descriptor via MasterFileDescriptor_PtyDevice.Write(data). This operation bridges user input to the child process across both normal and edge-case execution states:

 1. Normal Operational Path (Unlocked Direct Kernel Write):
    MasterFileDescriptor_PtyDevice.Write executes 100% UNLOCKED without acquiring Mutex. Operating system kernel write() system calls on file descriptors are atomic and thread-safe at the OS level, allowing user input to pass directly to the child process stdin stream without contending with background VTE grid updates. On successful write, Write returns (len(data), nil).

 2. Post-Exit Write Edge Case (syscall.EIO / syscall.EPIPE):
    If a caller writes to MasterFileDescriptor_PtyDevice after the slave process has exited (hung up), the kernel write() system call returns (0, syscall.EIO) or (0, syscall.EPIPE). Because the Go runtime ignores SIGPIPE on file descriptors by default, post-exit writes return a standard Go error (*os.PathError) and NEVER crash or panic the server process.

 3. Post-Closure Write Edge Case (os.ErrClosed):
    If MasterFileDescriptor_PtyDevice.Close() has executed during administrative teardown, subsequent calls to Write() immediately return (0, os.ErrClosed) synchronously without issuing a kernel system call.

 4. Terminal Mode Ignorance & OS Kernel Error Determinism:
    The underlying *os.File descriptor operates independently of PtyProxy's internal Mode. Calling MasterFileDescriptor_PtyDevice.Write while Mode is EXITED__Mode_PtyProxy executes the OS system call directly, with return behavior governed entirely by kernel PTY stream state regardless of the instance mode.

 5. Synchronous Execution & Kernel Backpressure Blocking Semantics:
    MasterFileDescriptor_PtyDevice.Write executes synchronously on the calling goroutine, incurring user-to-kernel context switch latency. Under normal system load with available kernel write ring buffer capacity, Write completes near-instantaneously (though subject to OS CPU scheduling and TTY driver lock contention). However, if the child process stops reading stdin and the kernel write ring buffer saturates, Write BLOCKS on kernel ring buffer backpressure until the child process drains stdin bytes or teardown invalidates the descriptor.

 6. Kernel FIFO Byte Ordering & Wait-Queue Serialization:
    Sequential and concurrent calls to MasterFileDescriptor_PtyDevice.Write maintain strict First-In, First-Out (FIFO) byte ordering. The Linux kernel TTY driver appends incoming bytes to the write ring buffer in exact order of system call commitment. If multiple Write calls block concurrently due to kernel ring buffer backpressure, the kernel manages blocked threads in a FIFO wait queue, waking them sequentially as buffer space becomes available to ensure byte streams are never scrambled or reordered.

## File Descriptor Blocking Mode Architecture (Blocking vs. Non-Blocking & POSIX Symmetry Constraints)

PtyProxy configures its underlying OS master PTY file descriptor in standard **Blocking Mode** (default descriptor flags without O_NONBLOCK).

### 1. Architectural Rationale for Current Blocking Choice
- **Simplified High-Throughput Reader Loop**: PtyReader relies on a zero-overhead, single-threaded blocking read() loop over MasterFileDescriptor_PtyDevice. Operating in blocking mode ensures PtyReader sleeps efficiently on kernel wait queues when stdout is idle, eliminating CPU-spinning, polling loops, or spurious syscall.EAGAIN retry handling.
- **Pragmatic Initial Baseline Execution Semantics**: Writing to MasterFileDescriptor_PtyDevice copies data into the kernel's in-memory TTY buffer in RAM, returning to the caller in microseconds without waiting on external hardware or network transmission. Because the slave child process (bash, readline) continuously drains stdin while human keypresses and client frame bursts consume only a small fraction of the kernel's ~64 KB buffer, buffer headroom remains well above blocking thresholds during standard interactive sessions. Consequently, direct blocking writes execute near-instantaneously without stalling calling threads, providing a sound, low-complexity baseline for initial deployment.

### 2. POSIX File Descriptor Symmetry Constraint (The Symmetrical O_NONBLOCK Limitation)
For engineers evaluating future write pipeline enhancements or investigating non-blocking write semantics, POSIX file descriptor architecture imposes a key structural constraint:
- **Symmetrical Application Across Operations**: Non-blocking flags (O_NONBLOCK) are attached directly to the open file handle in operating system memory. Executing fcntl(fd, F_SETFL, O_NONBLOCK) forces **both read() and write() operations** on that descriptor handle to inherit non-blocking behavior.
- **Impact on PtyReader's Read Loop**: POSIX does not permit assigning O_NONBLOCK exclusively to write() calls on a shared descriptor handle. If future engineers attempt to set O_NONBLOCK directly on MasterFileDescriptor_PtyDevice, doing so will force PtyReader's background read() loop to also become non-blocking (requiring EAGAIN polling or CPU-spinning).
- **Architectural Consideration for Non-Blocking Exploration**: If non-blocking write semantics are ever evaluated for future production needs, engineers exploring O_NONBLOCK directly on MasterFileDescriptor_PtyDevice should note that doing so would require re-architecting PtyReader to handle non-blocking read polling; also, other architectural options exist for enabling non-blocking write semantics beyond modifying descriptor flags directly.

---

## Analysis of Master PTY Write Blocking Scenarios for Proxy Engineers

Although master PTY writes complete near-instantaneously during interactive usage, executing MasterFileDescriptor_PtyDevice.Write(data) can block the calling goroutine under specific operational conditions. Server engineers must understand these five macro-level scenarios when designing upstream write pipelines:

### 1. Kernel TTY Buffer Watermark Saturation
- **Buffer Quotas**: The kernel allocates an in-memory input buffer quota (typically ~64 KB) for the PTY pair.
- **High Watermark Threshold**: Writes do **NOT** wait until the buffer is 100% full before blocking. The kernel enforces a high watermark threshold (blocking when remaining free headroom drops below ~4 KB).
- **Automatic Unblocking**: When free space drops below the high watermark, the kernel puts the calling write thread to sleep. As soon as the slave child process reads pending input and free space crosses back below the low watermark, the kernel automatically wakes the waiting writer thread.

### 2. Software Flow Control Suspension
- **User-Space Triggered Flow Control**: When software flow control is active, user-space callers or keypresses can instruct the operating system to pause or resume terminal output processing.
- **Writer Pause State**: While output is suspended, master writes block on kernel wait queues regardless of available buffer space until a resume command is received.
- **Technical APIs & Control Character Aliases**:
  - *Pause Command*: Ctrl-S (STOP / XOFF / byte 0x13 / tcflow(fd, TCOOFF)).
  - *Resume Command*: Ctrl-Q (START / XON / byte 0x11 / tcflow(fd, TCOON)).

### 3. Slow, Unresponsive, or Suspended Slave Child Process
- **Unresponsive Child Process**: If the child process (bash, cat, gdb) pauses input reading (due to heavy CPU work, blocking file I/O, or internal application locks), user input accumulates in the buffer until the watermark threshold is reached.
- **Background Process Suspension**: If the child process is suspended in the background, input reading stops entirely. Subsequent master writes fill the buffer watermark and block until the process is resumed.
- **Technical APIs & Control Signal Aliases**:
  - *Suspension Commands*: Ctrl-Z (Job Control Suspend / SIGTSTP / SIGSTOP).
  - *Resume Commands*: fg (Foreground Resume / SIGCONT).

### 4. Kernel System Write Serialization
- **Sequential OS Kernel Locks**: Operating system write calls on a single file descriptor are internally serialized by kernel locks (executing one write at a time). If a write system call is currently in progress, any subsequent write call to the same descriptor handle must wait for the preceding write to release the kernel lock before committing its payload.

### 5. OS Resource Exhaustion & Scheduling Delays
- Under severe system RAM exhaustion or CPU starvation, buffer memory allocation and thread wakeups experience OS scheduling delays, increasing overall write latency.

---

## Administrative PTY Master Descriptor Closure (MasterFileDescriptor_PtyDevice.Close)

Closing the master PTY file descriptor via MasterFileDescriptor_PtyDevice.Close() initiates an administrative session teardown from the parent process:

 1. Unlocked Direct Kernel Descriptor Closure:
    MasterFileDescriptor_PtyDevice.Close() executes 100% UNLOCKED without acquiring Mutex. Calling Close() invalidates the underlying OS master file descriptor handle immediately.

 2. Synchronous Non-Blocking Execution & Return Determinism:
    MasterFileDescriptor_PtyDevice.Close() executes synchronously on the calling goroutine and returns immediately without blocking on child process exit (TerminalCommand.Wait()) or background PtyReader loop termination. On its initial invocation, Close() executes the OS close() system call and returns nil (even if the slave process has already exited with EIO); on any subsequent invocation on the closed file handle, Go's *os.File immediately returns os.ErrClosed without issuing a kernel system call or causing server panics.

 3. Instantaneous PtyReader Unblocking (os.ErrClosed Signal):
    If background PtyReader is currently blocked on a kernel read() system call, closing the master file descriptor invalidates the kernel file handle, forcing read() to unblock immediately and return os.ErrClosed ("file already closed").

 4. Reader Flow Propagation Ownership & Caller Return Value Irrelevance:
    Callers MUST treat the return value of MasterFileDescriptor_PtyDevice.Close() as irrelevant and MUST NOT conditionally alter interactions with PtyProxy based on the return value. All teardown state mutations, OS process reaping, and exit notifications originate exclusively from background PtyReader flow propagation following read loop termination.

 5. Idempotent Execution Semantics:
    MasterFileDescriptor_PtyDevice.Close() is idempotent; repeated invocations safely return os.ErrClosed without side-effects or resource leaks, though standard architectural lifecycle design requires calling Close() at most once per session.

 6. Dual Resource Reaping Invariant (Process vs. Descriptor Cleanup):
    MasterFileDescriptor_PtyDevice.Close() and TerminalCommand.Wait() manage two completely independent OS kernel abstractions. Executing TerminalCommand.Wait() reaps the child process exit status to prevent zombie processes, but DOES NOT close the master PTY file descriptor handle. Conversely, calling Close() releases the OS file descriptor table handle. Complete session teardown requires both operations, which PtyProxy encapsulates completely inside __executeExitedTeardown.
    - Consumer Abstraction & Manual Trigger Note: Consumers focus exclusively on triggering session closure via Close() and do not concern themselves with downstream OS resource cleanup side-effects. If a consumer manually calls Close(), PtyProxy's internal exit pipeline guarantees 100% complete process and descriptor reaping while handling redundant Close() calls safely.

## Multi-Threaded Concurrency Map & Synchronization Architecture

PtyProxy operates across two concurrent execution contexts that intersect at shared instance memory:

### 1. Background Reader Execution Context (Goroutine 1: PtyReader.StartReading)

    Executes continuously on a single background goroutine, reading PTY stdout bytes from the OS master file descriptor unlocked into PtyReader.StagingBuffer. PTY output and process teardown propagate through nine distinct mutually exclusive code paths.

    Note on Mutual Exclusivity: Because PtyReader.StartReading operates on a single background goroutine and Mode evaluates atomically under Mutex, exactly one path executes per flush or exit event. Furthermore, once a teardown path (Paths 7-9) is triggered upon read loop termination, no further flushes can ever occur.

    - Path 1: Optimistic Live Streaming Call Tree (Hot-Path TryFlush in Live Mode)
      PtyReader.StartReading
        -> OnTryFlush__
          -> HandleTryFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Mutex.TryLock
              -> TerminalState.Write
              -> Mutex.Unlock
              -> OnOutput_Live (bytes.Clone)

    - Path 2: Saturation Fallback Live Streaming Call Tree (BlockingFlush in Live Mode)
      PtyReader.StartReading
        -> OnBlockingFlush__
          -> HandleBlockingFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> LockAndReturnTrue
                -> Mutex.Lock
              -> TerminalState.Write
              -> Mutex.Unlock
              -> OnOutput_Live (bytes.Clone)

    - Path 3: Optimistic PostSnapshotBuffer Staging Call Tree (TryFlush in PostSnapshot Mode)
      PtyReader.StartReading
        -> OnTryFlush__
          -> HandleTryFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Mutex.TryLock
              -> TerminalState.Write
              -> PostSnapshotBuffer.Write
              -> Mutex.Unlock

    - Path 4: Saturation Fallback PostSnapshotBuffer Staging Call Tree (BlockingFlush in PostSnapshot Mode)
      PtyReader.StartReading
        -> OnBlockingFlush__
          -> HandleBlockingFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> LockAndReturnTrue
                -> Mutex.Lock
              -> TerminalState.Write
              -> PostSnapshotBuffer.Write
              -> Mutex.Unlock

    - Path 5: Optimistic Pre-Snapshot VTE Update Call Tree (Hot-Path TryFlush in PreSnapshot Mode)
      PtyReader.StartReading
        -> OnTryFlush__
          -> HandleTryFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> Mutex.TryLock
              -> TerminalState.Write
              -> Mutex.Unlock

    - Path 6: Saturation Fallback Pre-Snapshot VTE Update Call Tree (BlockingFlush in PreSnapshot Mode)
      PtyReader.StartReading
        -> OnBlockingFlush__
          -> HandleBlockingFlush
            -> __flushReaderStagingBufferSliceIfLockAcquired
              -> LockAndReturnTrue
                -> Mutex.Lock
              -> TerminalState.Write
              -> Mutex.Unlock

    - Path 7: Descriptor Closure Process Teardown Call Tree
      PtyReader.StartReading
        -> OnExited_Closed__
          -> HandleExited_Closed
            -> __executeExitedTeardown
              -> Mutex.Lock (Mode = EXITED__Mode_PtyProxy)
              -> Mutex.Unlock
              -> MasterFileDescriptor_PtyDevice.Close
              -> TerminalCommand.Wait
              -> HandleDispatchExited_Closed
                -> OnExited_Closed__

    - Path 8: Child Process EOF Exit Status Teardown Call Tree
      PtyReader.StartReading
        -> OnExited_Eio__
          -> HandleExited_Eio
            -> __executeExitedTeardown
              -> Mutex.Lock (Mode = EXITED__Mode_PtyProxy)
              -> Mutex.Unlock
              -> MasterFileDescriptor_PtyDevice.Close
              -> TerminalCommand.Wait
              -> HandleDispatchExited_Eio
                -> [Mutually Exclusive Leaf Callback - Exactly 1 Dispatched]:
                  - OnExited_Eio_Success (if ExitStatus == 0)
                  - OnExited_Eio_Failure (if ExitStatus != 0)
                  - OnExited_Eio_Killed (if Signaled)

    - Path 9: Kernel System Error Teardown Call Tree
      PtyReader.StartReading
        -> OnExited_SystemError__
          -> HandleExited_SystemError
            -> __executeExitedTeardown
              -> Mutex.Lock (Mode = EXITED__Mode_PtyProxy)
              -> Mutex.Unlock
              -> MasterFileDescriptor_PtyDevice.Close
              -> TerminalCommand.Wait
              -> HandleDispatchExited_SystemError
                -> OnExited_SystemError

### 2. Main / Workspace Execution Context

    Executes concurrently on main or workspace request goroutines. Unlike Goroutine 1's single-threaded loop, paths in this context are invoked on-demand across 3 distinct operational categories:

    - Resynchronization Pipeline Category (Paths 10-12):
      Sequential 3-step protocol (TransitionMode_LiveToPreSnapshot -> TransitionMode_PreToPostSnapshot -> TransitionMode_PostSnapshotToLive) executed when transitioning between online and offline modes. Mutex protects each step to ensure thread-safe mode updates and PostSnapshotBuffer staging.

    - Window Geometry Category (Path 13):
      Invoked on client terminal resizes. Executes pty.Setsize unlocked on the OS master descriptor, then acquires Mutex to atomically resize the VTE grid. Concurrent resizes are serialized under lock.

    - Direct OS Kernel I/O Category (Paths 14-15):
      Unlocked OS kernel system calls executing directly on MasterFileDescriptor_PtyDevice. Path 14 (Write) passes stdin bytes to the kernel pipe unlocked (thread safety handled by Linux kernel I/O buffers). Path 15 (Close) atomically closes the master descriptor, inducing an OS EOF/closed failure on PtyReader and triggering Path 7 teardown on Goroutine 1.

    - Path 10: Pre-Snapshot Mode Transition Call Tree
      TransitionMode_LiveToPreSnapshot
        -> Mutex.Lock (Mode = RUNNING_PRE_SNAPSHOT__Mode_PtyProxy)
        -> Mutex.Unlock

    - Path 11: Baseline Snapshot Serialization Call Tree
      TransitionMode_PreToPostSnapshot
        -> Mutex.Lock (Mode = RUNNING_POST_SNAPSHOT__Mode_PtyProxy)
        -> PostSnapshotBuffer.Reset
        -> xterm.NewSerializeAddon
        -> serializeAddon.Serialize
        -> Mutex.Unlock
        -> OnOutput_Snapshot

    - Path 12: PostSnapshotBuffer Replay Call Tree
      TransitionMode_PostSnapshotToLive
        -> Mutex.Lock (Mode = RUNNING_LIVE__Mode_PtyProxy)
        -> bytes.Clone(PostSnapshotBuffer)
        -> Mutex.Unlock
        -> OnOutput_PostSnapshotBuffer

    - Path 13: Terminal Window Resizing Call Tree
      Resize
        -> Mutex.Lock
        -> PtyTerminal.Resize
        -> Mutex.Unlock
        -> pty.Setsize

    - Path 14: Terminal User Input / Stdin Writing Call Tree
      MasterFileDescriptor_PtyDevice.Write

    - Path 15: Master Descriptor Administrative Closure Call Tree
      MasterFileDescriptor_PtyDevice.Close

## Terminal Window Resizing Synchronization & Order of Operations (Resize)

When resizing a pseudo-terminal session via `Resize(nextColumnCount, nextRowCount)`, `PtyProxy` enforces a strict 2-phase execution sequence:

 1. In-Memory VTE Grid Resize Under Lock (PtyTerminal.Resize):
    `PtyProxy` acquires `Mutex` and synchronously resizes the headless `*xterm.Terminal` emulator grid buffers and line wrap pointers before releasing `Mutex`.

 2. Kernel Window Size Syscall Outside Lock (pty.Setsize):
    After releasing `Mutex`, `PtyProxy` issues the `ioctl(TIOCSWINSZ)` syscall via `pty.Setsize` on `MasterFileDescriptor_PtyDevice` 100% unlocked.

### Critical Invariants of the Resize Sequence:

 1. Elimination of the SIGWINCH Redraw Race Hazard:
    Issuing `pty.Setsize` triggers the Linux kernel to send `SIGWINCH` to the child process (`vim`, `htop`, `tmux`), which immediately emits full-screen redraw bytes over stdout. Resizing `PtyTerminal` **first** guarantees that the in-memory emulator is already sized and ready to consume the incoming redraw output, preventing visual wrapping corruption that would occur if stdout arrived while the grid was still at the old geometry.

 2. Zero Syscall Blocking Under Mutex:
    Executing `pty.Setsize` outside `Mutex` ensures that kernel `ioctl` operations never hold the in-memory state lock, eliminating lock contention and circular-wait deadlocks with background reader flushes.

 3. Exited Session Visual Integrity & The Absence of SIGWINCH Egress:
    - **The Exited Resizing Asymmetry**: For a running session, the client receives visual updates because the child process catches `SIGWINCH` and actively emits ANSI redraw chunks over stdout. When a process has exited, the child is dead and `MasterFileDescriptor_PtyDevice` is closed. No `SIGWINCH` is caught, and zero stdout bytes are produced.
    - **Current Dual-Sided Resolution Strategy**:
      1. *Client-Side Local Reflow (Visible Sessions)*: For sessions currently focused in the UI, the client-side terminal engine (`xterm.js`) retains the scrollback buffer locally and natively reflows text on the DOM upon viewport resize with 0ms latency, eliminating the need for server-driven redraw streams.
      2. *Server-Side Authoritative VTE Grid*: Executing `PtyTerminal.Resize` synchronously under lock keeps the server's in-memory emulator strictly synchronized with the client viewport geometry. Any `EBADF` descriptor error from `pty.Setsize` on the closed file descriptor is cleanly ignored.
      3. *On-Demand Catch-Up Snapshots (Hidden & Reconnect Flows)*: If an exited tab was backgrounded during resizing or if the client drops/reconnects, switching visibility to the tab triggers `TransitionMode_PreToPostSnapshot`. Because `Mode == EXITED`, it serializes the newly reflowed `PtyTerminal` state and dispatches a full ANSI snapshot (`OnOutput_Snapshot`) to establish pristine visual parity.

## Isolated Execution Spheres (Where Mutex IS NOT Required)

  - Internal PtyReader Loop: Draining kernel PTY file descriptor into StagingBuffer, tracking UnflushedSliceSize_StagingBuffer, and resetting write head are 100% single-threaded within PtyReader. No mutex is required inside PtyReader itself.
  - Egress Callback Dispatch: Egress callbacks (OnOutput_Live, OnOutput_Snapshot, OnOutput_PostSnapshotBuffer, OnExited_*) are dispatched 100% unlocked outside Mutex. The parent handler acquires Mutex first to update state and clone data, releasing Mutex BEFORE dispatching the egress callback to eliminate downstream circular-wait deadlocks and minimize lock contention.

## State Mutation Concurrency Protections (Why Mutex Is Mandatory)

  - VTE Grid Memory Race Protection: Prevents concurrent data races between background PtyReader stdout updates (TerminalState.Write) and snapshot serialization (xterm.NewSerializeAddon) or window resizes (TerminalState.Resize).
  - Mode Evaluation TOCTOU Protection: Prevents Time-Of-Check-To-Time-Of-Use races by evaluating Mode and capturing the egress action (live dispatch flag vs. PostSnapshotBuffer accumulation) atomically under Mutex. This guarantees that stdout bytes arriving in Live mode are safely flagged for egress even if Mode transitions on another goroutine before the callback completes outside lock.
  - PostSnapshotBuffer Buffer Race Protection: Prevents memory corruption when accumulating bytes in PostSnapshotBuffer during PostSnapshot mode while concurrent mode transitions reset or clone the buffer.

## Domain Invariants

  - 1. Single-Threaded Zero-Lock Reader Core: PtyReader operates 100% single-threaded on a dedicated background goroutine (StartReading), absorbing kernel PTY stdout bytes into reusable staging memory with zero internal lock acquisition.
  - 2. Lock-Scoped Memory Integrity & Unlocked Egress: Mutex protects in-memory state mutations (TerminalState, PostSnapshotBuffer, Mode) exclusively. All external callbacks (OnOutput_*, OnExited_*) are dispatched 100% unlocked outside Mutex with cloned allocations (bytes.Clone), eliminating downstream lock coupling and circular-wait deadlocks.
  - 3. Lossless Mode-Driven Resynchronization: Stdout bytes arriving while serializing and outputting baseline snapshots accumulate in PostSnapshotBuffer under lock, ensuring zero stdout data loss across mode transitions.
  - 4. Unidirectional Callback Coupling: PtyProxy owns PtyReader with zero struct back-pointers; PtyReader delegates stream events to PtyProxy strictly through function values.
  - 5. Atomic Lifecycle State Machine: Mode evaluates atomically under Mutex. Once a teardown path is triggered upon read loop termination, PtyProxy transitions to EXITED__Mode_PtyProxy and no further stdout flushes can ever occur.
  - 6. Callback Execution Contracts: Stdout egress callbacks (OnOutput_*) MUST be non-blocking to prevent stalling stream draining. Teardown pipelines execute synchronously on the background reader thread post-loop, blocking on TerminalCommand.Wait() outside Mutex to reap child process exit status before dispatching exit callbacks.
  - 7. Spawning Handshake Contract: NewPtyProxy synchronously executes OnPtySpawned to complete session registry insertion and initial wire status emission BEFORE launching go PtyReader.StartReading(), guaranteeing strict network frame ordering and preventing race conditions with fast child process exits.
