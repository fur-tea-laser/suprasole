# PtyReader: High-Throughput Unidirectional OS PTY Descriptor Reader

## System Context & Architectural Role
`PtyReader` is the dedicated operating system descriptor reader for pseudo-terminal instances in `suprasole-server`. It has a singular responsibility: continuously and unabatedly draining raw output bytes from the Linux kernel master PTY descriptor (`/dev/ptmx`, wrapped in `*os.File`) using non-blocking recycled data orders and pushing them into a bounded channel (`this.PtyFlusher.QueueChannel__Order_PtyFlusher`) owned by `PtyFlusher`.

By isolating descriptor reading from pacing, buffering, and lock-protected proxy state, `PtyReader`:
 1. **Reads Completely Unabated**: Drains the master descriptor continuously without gating on external channel receives or lock contention. Buffer acquisition is handled via the permanent channel freelist (`PoolChannel___Data__Order_PtyFlusher`) without heap allocations or GC evictions, ensuring the read loop is never delayed before querying the kernel.
 2. **Drains Completely Unlocked**: Master descriptor reading executes 100% unlocked, never acquiring `PtyProxy.Mutex`. This guarantees that long-running netpoller sleep states (`runtime.gopark`) cannot block sibling routines (keystroke writes, window resizes, snapshot synchronization).
 3. **Prevents Cross-Layer Deadlocks**: Eliminates circular-wait deadlocks across the Linux kernel boundary where child processes block on `read(2)` waiting for stdin while a master reader holds the lock needed to deliver keystrokes.
 4. **Induces Lossless OS Backpressure at Enqueue**: Reads in chunks up to 4,095 bytes (`SIZE_READ_BUFFER__PtyReader`) and enqueues them into `this.PtyFlusher.QueueChannel__Order_PtyFlusher` (bounded to 24 orders = 96 KB). When downstream consumption slows or pauses, `this.PtyFlusher.QueueChannel__Order_PtyFlusher` fills, pausing `PtyReader.RunWorker` strictly on channel send. Draining of `/dev/ptmx` halts, causing kernel line discipline buffers (4 KB) to fill and safely throttling the child process in `write(2)` via native OS backpressure with zero dropped bytes.
 5. **Decouples Descriptor IO from Egress Pacing**: Hands off polymorphic orders to `PtyFlusher`, allowing the flusher to coordinate the 16 ms / 60 FPS pacing timer, burst coalescing, and proxy lock acquisition without complicating the read loop.

## Byte Ingestion Mechanism & Freelist Pool Channel Recycling
`PtyReader` operates as a dedicated producer using a pre-allocated channel freelist:
 - **Instantaneous Buffer Acquisition (PoolChannel)**: `PtyReader` receives from `this.PtyFlusher.PoolChannel___Data__Order_PtyFlusher`, which yields a pre-allocated order pointer permanently resident in memory. Because the pool holds 26 orders while the downstream queue holds 24, at least 1 buffer is always immediately available. It reads up to 4,095 bytes directly into `dataOrder_PtyFlusher.ReadBuffer_PtyDevice` and forwards the order to `this.PtyFlusher.QueueChannel__Order_PtyFlusher`.
 - **Zero Continuous Allocation Invariant**: Once `PtyFlusher` processes or stages the data, it reslices `ReadBuffer_PtyDevice` to its full capacity and returns the order pointer back to `flusher.PoolChannel___Data__Order_PtyFlusher`. Because order pointers circulate continuously through channels across goroutines, **exactly 0 heap allocations** occur during steady-state stdout streaming.
 - **Strict FIFO Transmission**: Reads are forwarded synchronously to `this.PtyFlusher.QueueChannel__Order_PtyFlusher` in strict chronological order of kernel emission.

## In-Band Exit Signal Order & Clean Teardown
Under Linux/POSIX semantics, when all slave file descriptors close (e.g. child process terminates), the kernel driver unblocks pending master reads with `syscall.EIO` (or `os.ErrClosed` upon explicit descriptor closure).
 - **Polymorphic Exit Order**: Rather than out-of-band struct variables or channel closure, termination is transmitted as a first-class `_ExitSignal__Order_PtyFlusher_` across `this.PtyFlusher.QueueChannel__Order_PtyFlusher`.
 - **Trailing Output Preservation**: If a read returns data alongside a termination signal (`bytesRead > 0`), the data order is forwarded first, immediately followed by the exit signal order.
 - **Clean Loop Termination**: After emitting `_ExitSignal__Order_PtyFlusher_`, `PtyReader.RunWorker` terminates cleanly without calling `close()`, eliminating race conditions or panics on closed channels.

## Domain Invariants
 - **1. Unabated Direct Kernel Reading**: Drains the master file descriptor directly without pre-read blocking or channel gating.
 - **2. 100% Unlocked Execution**: Never touches or contends on `PtyProxy.Mutex` during `Read()` or channel sends.
 - **3. Zero-Allocation Streaming**: Recycles pre-allocated order pointers through `PoolChannel___Data__Order_PtyFlusher`, producing 0 heap allocations during active streaming.
 - **4. Downstream-Only Backpressure**: Backpressure occurs strictly on `this.PtyFlusher.QueueChannel__Order_PtyFlusher <- order` when downstream buffers saturate, preserving native OS-level throttling.
 - **5. In-Band Polymorphic Exit**: Encapsulates process termination within `_ExitSignal__Order_PtyFlusher_`, guaranteeing in-order trailing flush and clean teardown.
