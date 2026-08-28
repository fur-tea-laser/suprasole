# PtyReader: High-Throughput Unidirectional PTY Reader Engine

## System Context & Read Indirection Motivation
A key operational requirement of suprasole-server is absorbing high-throughput PTY stdout while minimizing kernel read ring buffer backpressure and user-space lock contention. During PTY process hydration and multi-terminal resynchronization (TransitionMode_PreToPostSnapshot), PtyProxy serializes terminal state into ANSI setup sequences under lock.

While OS kernel PTY backpressure (blocking child writes when the 64 KB kernel pipe fills) is standard kernel behavior, user-space deadlocks can occur if any thread holding the proxy lock enters a blocking wait that relies on PTY output being drained.

To minimize kernel PTY pipe backpressure and user-space lock contention, StagingBuffer and PtyReader introduce read indirection with an opportunistic non-blocking flush strategy:
 1. **Latency Cushion & Zero-Allocation Reading**: Reading PTY stdout requires allocated RAM. Pre-allocating a static staging buffer provides a generous headroom cushion to absorb output bursts without continuous heap re-allocations, drastically minimizing kernel read ring buffer backpressure.
 2. **Non-Blocking Opportunistic Flush (TryLock Hot Path)**: The reader loop drains MasterFileDescriptor_PtyDevice 100% unlocked and attempts OnTryFlush. If the proxy lock is busy, the reader does NOT block; it immediately resumes draining the kernel descriptor into free staging space.
 3. **High-Throughput Micro-Batching**: When the proxy lock is briefly acquired by another thread, PTY data accumulates contiguously in StagingBuffer. Once the lock opens, a single flush delivers the entire accumulated batch in one lock acquisition, drastically reducing synchronization overhead and context switches.
 4. **Controlled Backpressure Fallback**: Only when StagingBuffer saturates (100% full) does PtyReader invoke OnBlockingFlush, safely applying kernel backpressure until lock availability resumes.

## Byte Hand-Off Mechanism (Push vs. Pull Trade-Offs)
We intentionally chose a Push Mechanism (OnTryFlush / OnBlockingFlush) over a Consumer Pull Mechanism:
 - **Pull Mechanism Flaw**: A consumer pull model would require mutex locking across StagingBuffer to synchronize reader and consumer threads. If PtyProxy held the lock, PtyReader would block under lock contention during reads, stalling kernel PTY draining.
 - **Push Mechanism Advantage**: The push model allows StartReading() to operate 100% single-threaded and lock-free over StagingBuffer, guaranteeing zero-contention kernel PTY draining during normal operation.
 - **Opportunistic Skip-on-Busy Semantics**: Invoking OnTryFlush does NOT guarantee consumption. If another thread holds the proxy lock, OnTryFlush returns false and flushing is skipped for that iteration.
 - **Cushioning Skipped Emissions**: Skipped flushes leave accumulated data in StagingBuffer, building up contiguously for subsequent iterations. This reinforces the necessity of an expanded static staging buffer to cushion consecutive skipped emissions. Byte hand-off occurs when PtyReader invokes OnTryFlush AND the proxy lock is free.
 - **Tiered Blocking Fallback**: If StagingBuffer reaches 100% capacity (when staging headroom is fully depleted) and OnTryFlush fails, PtyReader invokes OnBlockingFlush with the unflushed slice, safely blocking the reader thread and inducing clean Linux kernel PTY backpressure without crashing the server.

## Transient Slice Memory Ownership Contract & Memory Safety Boundary
 1. **Reusable Buffer Slicing**: The byte slice passed to OnTryFlush, OnBlockingFlush, and downstream egress callbacks is a transient slice header pointing directly into PtyReader.StagingBuffer's reusable memory array in RAM.
 2. **Strict Validity Scope**: The unflushed staging slice is valid ONLY for the synchronous duration of the callback execution. Once the callback returns, PtyReader resets its staging index and will overwrite StagingBuffer on the subsequent read system call.
 3. **Consumer Memory Ownership & Asynchronous Safety**: Passing transient slice references requires a strict memory safety boundary for downstream consumers:
    - *Synchronous Processing (Zero Allocation)*: If a consumer processes stdout bytes synchronously during the callback, zero cloning is needed.
    - *Asynchronous Handoff (Consumer Clones)*: If a consumer hands off stdout bytes to an asynchronous queue, channel, or worker goroutine, the consumer MUST explicitly clone the slice before returning.
    Engineers wiring egress callbacks must strictly enforce this contract to prevent memory race conditions and data corruption under high-throughput stdout.

## Downstream Deadlock Avoidance & System-Wide Architectural Constraints
To guarantee that circular wait deadlocks remain 100% mathematically impossible across the server:
 1. **The Golden Rule (No External / Asynchronous OS Waits Under Lock)**: Downstream consumers, callbacks, and server threads MUST NEVER execute external OS system calls, blocking I/O, or asynchronous wait operations while holding the proxy lock. Blocking OS system calls surrender control to the kernel/external entities and act as implicit kernel wait objects. If a thread holds the proxy lock while waiting for an OS wait object that is itself waiting on PTY stdout draining, an implicit circular wait deadlock will occur.
 2. **Distinguishing Synchronous In-Memory CPU Work vs. External OS Waits**: It is critical to distinguish between CPU-bound in-memory operations and external OS/asynchronous wait calls:
    - *Synchronous In-Memory CPU Work (SAFE Under Lock)*: Operations like updating terminal grid lines or serializing ANSI snapshots are pure, deterministic CPU/RAM computations. They do NOT wait on external processes, kernel pipes, or network sockets, and always complete within milliseconds to release the lock.
    - *External OS / Asynchronous Waits (FORBIDDEN Under Lock)*: Calls that block waiting on external processes, kernel I/O, or network peers can pause indefinitely and MUST NEVER be executed while holding the lock.
 3. **System Boundary Synchronization & Lock Ordering**: Higher-level server components maintain their own synchronization structures. To maintain system-wide deadlock safety:
    - Callbacks and egress routines MUST NEVER attempt to acquire higher-level locks or execute external blocking operations while holding the proxy lock.
    - Egress callbacks must hand off stdout bytes to non-blocking queues or asynchronous schedulers without executing external blocking operations or acquiring blocking locks under the proxy lock.
    - If future developments require cross-component locking, engineers MUST establish and enforce a strict, unidirectional lock acquisition hierarchy across all layer boundaries.

## Static Staging Buffer Design & Sizing Rationale
Instead of a traditional circular ring buffer, StagingBuffer uses static linear slicing:
 - **Ring Buffer Trade-Off**: Circular buffers introduce split-slice wrap-arounds, requiring multi-part callbacks or flattening allocations.
 - **Linear Reset Accounting**: Resetting the unflushed staging buffer slice offset to zero on every successful flush guarantees downstream callbacks receive a single, contiguous byte slice with zero memory allocations or split-slice handling.

## Exit Signal Handling & Zero-Data-Loss Teardown
Any non-nil read error (syscall.EIO, os.ErrClosed, or unspecified system errors) manifests into an exit signal:
 - **Stream Exit Signal Semantic Notice**: The error returned as the second result of reading is named exitSignal_PtyReader because in Linux TTY/PTY stream reading, a non-nil error does not necessarily denote a system failure; rather, it can indicate clean slave process exit and stream termination.
 - **Idempotent Exit PTY Descriptor Reads**: PTY file descriptors execute as an instantaneous non-blocking call and return zero bytes alongside an exit signal on post-hangup reads.
 - **Guaranteed Staging Buffer Draining**: The read loop breaks only when an exit signal has been encountered AND all pending stdout bytes in the staging buffer have been fully flushed, guaranteeing 100% of accumulated stdout bytes in StagingBuffer are delivered to PtyProxy before exit.
 - **Post-Loop Signal Dispatched Routing**: After loop exit, exitSignal_PtyReader is evaluated in a single post-loop conditional block, routing to OnExited_Closed, OnExited_Eio, or fallback OnExited_SystemError.

## Domain Invariants
 - **1. Unidirectional Parent Ownership**: PtyProxy owns PtyReader with zero back-pointers to PtyProxy.
 - **2. Single-Threaded Execution**: RunWorker operates 100% single-threaded over StagingBuffer without internal locks.
 - **3. Unlocked Direct Kernel Reading**: Drains the master file descriptor directly into available StagingBuffer headroom 100% UNLOCKED.
 - **4. Non-Blocking TryLock Flushing**: Flushes accumulated batches contiguously via OnTryFlush whenever unflushed staging data is pending.
 - **5. Tiered Saturation Fallback**: If the staging buffer reaches full capacity and OnTryFlush fails, PtyReader invokes OnBlockingFlush, applying safe kernel PTY backpressure.
 - **6. Linear Reset-to-Zero Accounting**: Resets the unflushed staging buffer slice offset to zero on flush, guaranteeing downstream callbacks receive contiguous byte slices without ring-buffer modulo math.
 - **7. Guaranteed Drain Before Exit**: Read loop breaks ONLY when an exit signal is encountered AND the staging buffer has been completely drained, guaranteeing zero data loss on process teardown.
 - **8. Mutually Exclusive Signal Routing**: Evaluates the terminal signal post-loop to dispatch the corresponding exit callback.
