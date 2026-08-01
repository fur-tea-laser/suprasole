package source

import (
	"errors"
	"os"
	"syscall"
)

/*
===============================================================================
PtyReader: High-Throughput Unidirectional PTY Reader Engine
===============================================================================

System Context & Read Indirection Motivation:
A key operational requirement of suprasole-server is absorbing high-throughput PTY
stdout while minimizing kernel read ring buffer backpressure and user-space lock contention.
During PTY process hydration and multi-terminal resynchronization (TransitionMode_PreToPostSnapshot),
PtyProxy serializes xtermState into ANSI setup sequences under PtyProxy.Proxy_Mutex.

While OS kernel PTY backpressure (blocking child write() when the 64 KB kernel pipe fills)
is standard kernel behavior, user-space deadlocks can occur if any thread holding Proxy_Mutex
enters a blocking wait that relies on PTY output being drained. 

To minimize kernel PTY pipe backpressure and user-space lock contention, Reader_StagingBuffer
and PtyReader introduce read indirection with an opportunistic non-blocking flush strategy:
 1. Latency Cushion & Zero-Allocation Reading: Reading PTY stdout requires allocated RAM.
    Pre-allocating a static staging buffer (e.g., 512 KB) provides a generous headroom cushion
    to absorb output bursts without continuous heap re-allocations, drastically minimizing kernel read ring buffer backpressure.
 2. Non-Blocking Opportunistic Flush (TryLock Hot Path): The reader loop drains Pty_MasterFileDescriptor
    100% unlocked and attempts Reader_OnTryFlush (Proxy_Mutex.TryLock). If Proxy_Mutex is busy,
    the reader does NOT block; it immediately resumes draining the kernel descriptor into free staging space.
 3. High-Throughput Micro-Batching: When Proxy_Mutex is briefly locked by another thread, PTY data
    accumulates contiguously in Reader_StagingBuffer. Once Proxy_Mutex opens, a single TryLock flushes
    the entire accumulated batch in one lock acquisition, drastically reducing mutex overhead and context switches.
 4. Controlled Backpressure Fallback: Only when Reader_StagingBuffer saturates (100% full) does PtyReader
    invoke Reader_OnBlockingFlush (Proxy_Mutex.Lock()), safely applying kernel backpressure until lock availability resumes.

Byte Hand-Off Mechanism (Push vs. Pull Trade-Offs):
We intentionally chose a Push Mechanism (Reader_OnTryFlush / Reader_OnBlockingFlush) over a Consumer Pull Mechanism:
  - Pull Mechanism Flaw: A consumer pull model would require mutex locking across Reader_StagingBuffer to synchronize
    reader and consumer threads. If PtyProxy held the lock (e.g. during xtermState serialization), PtyReader
    would block under lock contention during reads, stalling kernel PTY draining.
  - Push Mechanism Advantage: The push model allows Run() to operate 100% single-threaded and lock-free over
    Reader_StagingBuffer, guaranteeing zero-contention kernel PTY draining during normal operation.
  - Opportunistic Skip-on-Busy Semantics: Invoking Reader_OnTryFlush does NOT guarantee consumption. If another thread holds
    PtyProxy.Proxy_Mutex, Reader_OnTryFlush returns false (Proxy_Mutex.TryLock fails) and flushing is skipped for that iteration.
  - Cushioning Skipped Emissions: Skipped flushes leave accumulated data in Reader_StagingBuffer, building up contiguously for
    subsequent iterations. This reinforces the necessity of an expanded static staging buffer (e.g. 512 KB) to cushion
    consecutive skipped emissions. Byte hand-off occurs when PtyReader invokes Reader_OnTryFlush AND PtyProxy.Proxy_Mutex is free.
  - Tiered Blocking Fallback: If Reader_StagingBuffer reaches 100% capacity (Reader_UnflushedStagingBufferSliceSize == len(Reader_StagingBuffer))
    and Reader_OnTryFlush fails, PtyReader invokes Reader_OnBlockingFlush(unflushedSlice) which calls Proxy_Mutex.Lock(),
    safely blocking the reader thread and inducing clean Linux kernel PTY backpressure without crashing the server.

Transient Slice Memory Ownership Contract & Memory Safety Boundary:
 1. Reusable Buffer Slicing:
    The byte slice (unflushedStagingBufferSlice) passed to Reader_OnTryFlush, Reader_OnBlockingFlush, and downstream
    egress callbacks (e.g. Proxy_OnOutput_Live) is a transient slice header pointing directly into PtyReader.Reader_StagingBuffer's
    reusable memory array in RAM.
 2. Strict Validity Scope:
    unflushedStagingBufferSlice is valid ONLY for the synchronous duration of the callback execution. Once the callback
    returns, PtyReader resets its staging index and will overwrite Reader_StagingBuffer on the subsequent Read() system call.
 3. Consumer Memory Ownership & Asynchronous Safety:
    Passing transient slice references requires a strict memory safety boundary for downstream consumers:
    - Synchronous Processing (Zero Allocation): If a consumer processes stdout bytes synchronously during the callback, zero cloning is needed.
    - Asynchronous Handoff (Consumer Clones): If a consumer hands off stdout bytes to an asynchronous queue, channel, or worker goroutine,
    the consumer MUST explicitly clone the slice (e.g., via bytes.Clone(ptyOutputData)) before returning.
    Engineers wiring egress callbacks must strictly enforce this contract to prevent memory race conditions and data corruption under high-throughput stdout.

Downstream Deadlock Avoidance & System-Wide Architectural Constraints:
To guarantee that circular wait deadlocks remain 100% mathematically impossible across the server:
 1. The Golden Rule (No External / Asynchronous OS Waits Under Lock):
    Downstream consumers, callbacks, and server threads MUST NEVER execute external OS system calls, blocking I/O, or asynchronous wait operations
    (e.g., cmd.Wait(), blocking PTY descriptor Write(), or blocking network socket sends) while holding PtyProxy.Proxy_Mutex.
    Blocking OS system calls surrender control to the kernel/external entities and act as implicit kernel wait objects. If a thread holds Proxy_Mutex
    while waiting for an OS wait object that is itself waiting on PTY stdout draining, an implicit 2-lock circular wait deadlock will occur.
 2. Distinguishing Synchronous In-Memory CPU Work vs. External OS Waits:
    It is critical to distinguish between CPU-bound in-memory operations and external OS/asynchronous wait calls:
    - Synchronous In-Memory CPU Work (SAFE under Proxy_Mutex): Operations like updating Proxy_TerminalState grid lines or serializing xtermState ANSI
    snapshots are pure, deterministic CPU/RAM computations. They do NOT wait on external processes, kernel pipes, or network sockets, and always complete
    within milliseconds to release Proxy_Mutex.
    - External OS / Asynchronous Waits (FORBIDDEN under Proxy_Mutex): Calls that block waiting on external processes, kernel I/O, or network peers
    can pause indefinitely and MUST NEVER be executed while holding Proxy_Mutex.
 3. System Boundary Synchronization & Lock Ordering:
    Higher-level server components (e.g., workspace schedulers, stream routers, or event dispatchers) maintain their own synchronization structures.
    To maintain system-wide deadlock safety:
    - Callbacks and egress routines MUST NEVER attempt to acquire higher-level locks or execute external blocking operations while holding Proxy_Mutex.
    - Egress callbacks (e.g. Proxy_OnOutput_Live) must hand off stdout bytes to non-blocking queues or asynchronous schedulers
    without executing external blocking operations or acquiring blocking locks under Proxy_Mutex.
    - If future developments require cross-component locking, engineers MUST establish and enforce a strict, unidirectional
    lock acquisition hierarchy across all layer boundaries.

Static Staging Buffer Design & Sizing Rationale:
Instead of a traditional circular ring buffer, Reader_StagingBuffer uses static linear slicing:
  - Ring Buffer Trade-Off: Circular buffers introduce split-slice wrap-arounds, requiring multi-part callbacks or flattening allocations.
  - Linear Reset Accounting: Resetting Reader_UnflushedStagingBufferSliceSize to 0 on every successful flush guarantees downstream callbacks receive a single, contiguous byte slice with zero memory allocations or split-slice handling.

Terminal Signal Handling & Zero-Data-Loss Teardown:
Any non-nil read error (syscall.EIO, os.ErrClosed, or unspecified sysErr) manifests into a terminal signal:
  - Stream Signal Semantic Notice: The error returned as the second result of Read() is named terminalSignal because
    in Linux TTY/PTY stream reading, a non-nil error does not necessarily denote a system failure. For example,
    syscall.EIO is the expected, normal OS signal indicating clean slave process exit and stream termination.
  - Idempotent Terminal PTY Descriptor Reads: PTY file descriptors execute as an instantaneous non-blocking call and return (0, terminalSignal) on post-hangup reads.
  - Guaranteed Staging Buffer Draining: The read loop breaks only when terminalSignal != nil AND Reader_UnflushedStagingBufferSliceSize == 0, guaranteeing 100% of accumulated stdout bytes in Reader_StagingBuffer are delivered to PtyProxy before exit.
  - Post-Loop Signal Dispatched Routing: After loop exit, terminalSignal is evaluated in a single post-loop conditional block, routing via errors.Is to Reader_OnExited_Closed(readerTerminalSignal), Reader_OnExited_Eio(readerTerminalSignal), or fallback Reader_OnExited_SystemError(readerTerminalSignal).

Struct Property Contracts & Semantics:
  - Pty_MasterFileDescriptor (*os.File) [REQUIRED]
    - Purpose: OS master PTY file descriptor bridging kernel stdout/stderr streams to PtyReader.
    - Contract: Must be open, valid, and non-nil before starting Reader_Run().
    - Semantics: Drained 100% unlocked inside Reader_Run(). Read(freeStagingSlice) passes remaining free staging capacity as maximum read bound. If kernel PTY buffer holds more data than free staging space, Read() fills available free space; remaining bytes stay queued in the kernel PTY buffer to be drained on subsequent loop iterations. Returns (0, syscall.EIO) or (0, os.ErrClosed) on Linux slave process exit or file descriptor closure.
  - Reader_StagingBuffer ([]byte) [REQUIRED]
    - Purpose: Pre-allocated contiguous byte slice serving as the staging cushion for PTY stdout reads.
    - Contract: Must be pre-allocated before starting Reader_Run(). Slices passed to callbacks are valid ONLY for the synchronous duration of the callback; consumers MUST clone the slice (e.g. bytes.Clone()) if stored or dispatched asynchronously across goroutines.
    - Semantics: Owned exclusively single-threaded by Reader_Run(). Overwritten linearly on subsequent Read() iterations once flushed.
  - Reader_UnflushedStagingBufferSliceSize (int) [REQUIRED]
    - Purpose: Monotonic integer offset tracking the exact number of unflushed, valid stdout bytes currently residing in Reader_StagingBuffer.
    - Contract: Advanced after every Read() by ptyBytesRed. Must reset strictly to 0 whenever Reader_OnTryFlush returns true or Reader_OnBlockingFlush completes execution.
    - Semantics: Represents the valid slice boundary Reader_StagingBuffer[:UnflushedSize]. Resetting to 0 restores the linear write head without ring-buffer modulo math.
  - Reader_OnTryFlush (func(unflushedStagingBufferSlice []byte) bool) [REQUIRED]
    - Purpose: Non-blocking opportunistic flush callback to deliver unflushed PTY stdout bytes.
    - Trigger Condition: Invoked by Reader_Run() whenever unflushed bytes exist in Reader_StagingBuffer (Reader_UnflushedStagingBufferSliceSize > 0).
    - Consumer Contract: Provided implementation MUST attempt Proxy_Mutex.TryLock() and return false immediately if busy. If TryLock() succeeds, flushes the unflushed byte slice, releases Proxy_Mutex, and returns true. Must NEVER execute blocking network I/O or acquire higher-level component locks while holding Proxy_Mutex.
    - Semantics: Hot-path flush mechanism. Drains accumulated stdout batches in a single lock acquisition when Proxy_Mutex is free, keeping PtyReader non-blocking.
  - Reader_OnBlockingFlush (func(unflushedStagingBufferSlice []byte)) [REQUIRED]
    - Purpose: Fallback blocking flush callback invoked when staging RAM reaches 100% saturation.
    - Trigger Condition: Invoked by Reader_Run() ONLY when Reader_StagingBuffer reaches 100% saturation (Reader_UnflushedStagingBufferSliceSize == len(Reader_StagingBuffer)) and TryLock fails.
    - Consumer Contract: Provided implementation acquires Proxy_Mutex.Lock(), blocking Reader_Run() until available, flushes the saturated staging buffer slice, and releases Proxy_Mutex. Must NEVER execute external blocking calls or acquire higher-level component locks while holding Proxy_Mutex.
    - Semantics: Controlled backpressure mechanism. Deliberately stalls Reader_Run() to slow down child process write() calls when user-space staging RAM is full.
  - Reader_OnExited_Closed (func(readerTerminalSignal error)) [REQUIRED]
    - Purpose: Teardown callback for explicit administrative PTY master descriptor closure.
    - Trigger Condition: Invoked by Reader_Run() post-loop after all accumulated bytes are flushed (Reader_UnflushedStagingBufferSliceSize == 0) when terminalSignal is os.ErrClosed. Mutually exclusive with Eio and SystemError callbacks.
    - Semantics: Signals explicit administrative descriptor closure.
  - Reader_OnExited_Eio (func(readerTerminalSignal error)) [REQUIRED]
    - Purpose: Teardown callback for Linux slave PTY process termination.
    - Trigger Condition: Invoked by Reader_Run() post-loop after full buffer draining (Reader_UnflushedStagingBufferSliceSize == 0) when terminalSignal is syscall.EIO.
    - Semantics: Standard Linux OS signal indicating clean slave process exit/hangup.
  - Reader_OnExited_SystemError (func(readerTerminalSignal error)) [REQUIRED]
    - Purpose: Fallback error teardown callback for unexpected OS read system call errors.
    - Trigger Condition: Invoked by Reader_Run() post-loop when terminalSignal is non-nil and matches neither os.ErrClosed nor syscall.EIO.
    - Semantics: Catches abnormal OS read failures, preventing error cascades across external subsystems.

Domain Invariants:
  - 1. Unidirectional Parent Ownership: PtyProxy owns PtyReader with zero back-pointers to PtyProxy.
  - 2. Single-Threaded Execution: Reader_Run operates 100% single-threaded over Reader_StagingBuffer without internal locks.
  - 3. Unlocked Direct Kernel Reading: Read() reads directly into Reader_StagingBuffer[Reader_UnflushedStagingBufferSliceSize:] 100% UNLOCKED.
  - 4. Non-Blocking TryLock Flushing: Flushes accumulated batches contiguously via Reader_OnTryFlush (Proxy_Mutex.TryLock) whenever Reader_UnflushedStagingBufferSliceSize > 0.
  - 5. Tiered Saturation Fallback: If 100% full (Reader_UnflushedStagingBufferSliceSize == len(Reader_StagingBuffer)) and Reader_OnTryFlush fails, invokes Reader_OnBlockingFlush (Proxy_Mutex.Lock()), applying safe kernel PTY backpressure.
  - 6. Linear Reset-to-Zero Accounting: Resets Reader_UnflushedStagingBufferSliceSize to 0 on flush, guaranteeing downstream callbacks receive contiguous byte slices without ring-buffer modulo math.
  - 7. Guaranteed Drain Before Exit: Read loop breaks ONLY when terminalSignal != nil AND Reader_UnflushedStagingBufferSliceSize == 0, guaranteeing zero data loss on process teardown.
  - 8. Mutually Exclusive Signal Routing: Evaluates terminalSignal post-loop to dispatch the corresponding exit callback.

===============================================================================
*/
type PtyReader struct {
	Pty_MasterFileDescriptor               *os.File
	Reader_StagingBuffer                   []byte
	Reader_UnflushedStagingBufferSliceSize int
	Reader_OnTryFlush                      func(unflushedStagingBufferSlice []byte) bool
	Reader_OnBlockingFlush                 func(unflushedStagingBufferSlice []byte)
	Reader_OnExited_Closed                 func(readerTerminalSignal error)
	Reader_OnExited_Eio                    func(readerTerminalSignal error)
	Reader_OnExited_SystemError            func(readerTerminalSignal error)
}

/*
===============================================================================
Reader_Run: Continuous Background PTY Read Loop
===============================================================================

Reader_Run executes the continuous background read loop draining Pty_MasterFileDescriptor unlocked into Reader_StagingBuffer.
It insulates kernel PTY reads from user-space lock contention by using opportunistic non-blocking TryLock flushing, falling back to blocking backpressure only under full buffer saturation.
Upon receiving an OS read error or descriptor closure, it guarantees zero data loss by fully flushing accumulated bytes before loop termination, subsequently evaluating terminalSignal to dispatch the corresponding exit callback.

===============================================================================
*/
func (thisReader *PtyReader) Reader_Run() {
	var ptyBytesRed int
	var terminalSignal error
	for {
		ptyBytesRed, terminalSignal = thisReader.Pty_MasterFileDescriptor.Read(thisReader.Reader_StagingBuffer[thisReader.Reader_UnflushedStagingBufferSliceSize:])
		thisReader.Reader_UnflushedStagingBufferSliceSize += ptyBytesRed
		if thisReader.Reader_UnflushedStagingBufferSliceSize > 0 && thisReader.Reader_OnTryFlush(thisReader.Reader_StagingBuffer[:thisReader.Reader_UnflushedStagingBufferSliceSize]) {
			thisReader.Reader_UnflushedStagingBufferSliceSize = 0
		} else if thisReader.Reader_UnflushedStagingBufferSliceSize == len(thisReader.Reader_StagingBuffer) {
			thisReader.Reader_OnBlockingFlush(thisReader.Reader_StagingBuffer[:thisReader.Reader_UnflushedStagingBufferSliceSize])
			thisReader.Reader_UnflushedStagingBufferSliceSize = 0
		}
		if terminalSignal != nil && thisReader.Reader_UnflushedStagingBufferSliceSize == 0 {
			break
		}
	}
	if errors.Is(terminalSignal, os.ErrClosed) {
		thisReader.Reader_OnExited_Closed(terminalSignal)
	} else if errors.Is(terminalSignal, syscall.EIO) {
		thisReader.Reader_OnExited_Eio(terminalSignal)
	} else if terminalSignal != nil {
		thisReader.Reader_OnExited_SystemError(terminalSignal)
	}
}
