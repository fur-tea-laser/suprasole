* **PtyDispatcher architectural charter:** PtyDispatcher decouples kernel terminal boundaries across 2 orthogonal operating planes.
  * **Data plane boundary:** Dispatcher buffers raw kernel reads into paced network egress plus emulator updates.
    * **Downstream protection:** Rate regulation shields WebSocket client plus parser mutex from burst saturation.
  * **Lifecycle plane boundary:** Dispatcher reports reader end after final flush; _PtyTeardown_ ends session.
    * **Zero cleanup authority:** Dispatcher closes nothing, reaps nothing, signals nothing.
* **Output pacing architecture:** Bimodal state machine harmonizes radical behavioral polarity across terminal byte streams.
  * **Workload polarity conflict:** Single keystroke interactivity conflicts with saturated stdout streaming bursts.
    * **Interactive latency demand:** Single-keystroke human typing requires immediate sub-millisecond echo latency.
    * **Throughput burst frequency:** Compilation logs, package installs, or streaming dumps generate 100s of read chunks.
    * **Unbuffered streaming limits:** Direct raw transmission degrades network transport and parser locking.
      * **WebSocket transport overload:** Unbuffered chunk floods overwhelm downstream WebSocket connection throughput.
      * **Parser lock contention:** High read rates trigger severe mutex contention across server and client parsers.
    * **Pure batching latency:** Static periodic batching adds unacceptable lag during human typing pauses.
  * **Asymmetric state automaton:** Pacing state machine operates across 2 distinct operational phases.
    * **Interactive IDLE phase:** Initial incoming data order optimizes exclusively for human echo latency.
      * **Staging buffer bypass:** Initial data order bypasses staging buffer, flushing synchronously with 0 ms added delay.
      * **Client egress dispatch:** Synchronous flush dispatches live egress to singular connected client while updating PtyTerminal.
      * **Post-flush timer arming:** Arming Timer_FlushPacing post-flush guarantees full undiminished timeout window for subsequent burst.
    * **Active PACING phase:** Subsequent data orders arriving within temporal window optimize exclusively for throughput.
      * **Burst buffer consolidation:** Incoming data orders consolidate into 96 KB staging buffer during active window.
      * **Egress pipeline decoupling:** Staging decouples upstream kernel reads from network egress plus downstream client consumption.
  * **Periodic cadence enforcement:** Pacing enforces strict periodic cadence over sliding debounce delay.
    * **Sliding debounce delay:** Resetting deadlines on incoming data orders introduces unbounded delay during sustained streaming.
    * **Trailing-edge execution invariance:** Pacing timer runs uninterrupted to trailing edge regardless of kernel read frequency.
    * **Predictable flush boundaries:** Enforcing fixed windows guarantees bounded batch flushes every ~Timeout__Timer_FlushPacing interval.
  * **Trailing-edge idle recovery:** Output burst cessation resolves cleanly on trailing edge of pacing window.
    * **Empty buffer expiration:** Timer expiration on empty buffer transitions dispatcher directly into idle sleep.
    * **0 CPU overhead:** Suppressing timer re-arming eliminates CPU overhead during terminal silence.
    * **Fast-path re-priming:** Trailing-edge idle transition automatically re-primes 0-delay fast-path for subsequent keystroke.
  * **Pacing timer mechanics:** Strict channel draining invariants govern Timer_FlushPacing re-arming.
    * **Clean re-arming points:** PtyDispatcher restricts Timer_FlushPacing.Reset to idle-to-pacing transitions plus consumed trailing ticks.
    * **Channel tick consumption:** Trailing-edge reset executes solely after select statement consumes channel tick.
    * **Unprocessed tick prevention:** Invariant scheduling guarantees 0 unconsumed ticks reside in Timer_FlushPacing.C during Reset.
  * **Select arbitration dynamics:** Uniform select resolves simultaneous readiness between data channel plus timer channel.
    * **Pseudo-random ready resolution:** Go select runtime arbitrates pseudo-randomly when QueueChannel__Order_PtyDispatcher plus Timer_FlushPacing.C ready simultaneously.
    * **Non-flush path bounds:** Non-flush branch executes within 300 to 400 nanoseconds via slice appending.
    * **Freelist recycling latency:** Buffer freelist returns complete inside single-digit microseconds during multi-order streaks.
    * **Timer starvation bounds:** Microsecond execution streaks remain negligible against operating system timer jitter.
    * **Contemporaneous batch enrichment:** Multi-order arbitration streaks coalesce additional contemporaneous data orders without starving timer.
* **Virtual terminal synchronization:** This.PtyTerminal.Write digests UnflushedSlice_StagingBuffer__PtyDispatcher to synchronize virtual terminal emulator state.
  * **Unconditional output consumption:** This.PtyTerminal.Write executes unconditionally before evaluating This.Mode_state.
    * **Pervasive mode support:** Emulator continuously digests output across LIVE_RUNNING, PRE_SNAPSHOT__RUNNING, plus POST_SNAPSHOT__RUNNING modes.
    * **Presence decoupling invariant:** Output ingestion decouples child process terminal progression from client network presence.
    * **Scrollback accumulation safety:** Backgrounded sessions accumulate accurate scrollback history without state drift.
    * **Instant snapshot readiness:** Ingestion persistence guarantees instant snapshot serialization readiness upon client reconnection.
  * **Emulator thread safety:** xterm-go emulator grid contains mutable multi-line structures lacking internal synchronization.
    * **Shared subsystem access:** PtyResizer triggers PtyTerminal.Resize on viewport adjustments during concurrent terminal ingestion.
    * **Snapshot routine contention:** Background snapshot serialization routines traverse active terminal screen buffers concurrently.
    * **Critical section encapsulation:** Executing This.PtyTerminal.Write within This.Mutex prevents memory corruption, visual tearing, and races.
  * **Shared pacing parity:** 2 sinks share 1 unified flushing strategy across emulation plus network egress.
    * **Identical slice binding:** HandleBlockingFlush binds emulation plus network egress to identical UnflushedSlice_StagingBuffer__PtyDispatcher slice.
    * **Temporal lockstep progression:** Terminal mutation plus client packet delivery advance across identical byte chunk boundaries.
    * **Unified buffer management:** Shared staging buffer consolidation eliminates redundant memory allocation across downstream consumers.
    * **Mutex contention amortization:** Applying 60 Hz pacing cadence amortizes This.Mutex acquisition overhead across bursts.
    * **Atomic parser ingestion:** 1 atomic call streams dense multi-kilobyte batches directly into VTE parser.
    * **Concurrent subsystem responsiveness:** Amortized lock hold preserves responsiveness for PtyResizer plus snapshot serialization routines.
    * **Worker loop optimization:** Confining This.PtyTerminal.Write strictly to flush routine prevents delaying timer tick handling.
  * **Synchronous processing bounds:** This.PtyTerminal.Write executes purely in-memory CPU operations without kernel syscalls or I/O.
    * **Linear complexity scaling:** Execution scales linearly O(N) with byte volume across sequence parsing.
    * **Nominal burst duration:** Digesting saturated 96 KB staging buffer requires approximately 1.3 ms to 2.0 ms.
    * **Frame budget fraction:** Processing consumes under 12% of allocated 16.6 ms pacing frame budget.
    * **Worst-case execution ceiling:** Dense full-screen clears via CSI 2J establish worst-case execution boundary.
      * **Screen sweep latency:** Repeated terminal grid sweeps degrade synchronous execution to approximately 200 ms.
      * **Subsystem delay transient:** Heavy screen clearing sweeps temporarily delay concurrent PtyResizer adjustments plus snapshots.
      * **Finite computation bound:** Mutex hold durations remain bounded by finite in-memory CPU computation.
* **Reader-end handoff:** PtyDispatcher stays pure data plane; _PtyTeardown_ owns session end (PtyTeardown.md).
  * **FIFO exit serialization:** Exit signal order shares identical FIFO channel queue alongside data orders.
    * **Preceding data preservation:** Preceding kernel-read pty bytes finish processing before handoff.
    * **Lossless trailing flush:** Residual staged bytes flush synchronously before handoff, guaranteeing 0 trailing byte loss.
  * **Handoff callback:** OnReaderFinished__ forwards raw ExitSignal as _ReaderFinished__Order_PtyTeardown_ after final flush.
    * **Outcome ordering guarantee:** Exit outcome follows final flush; EXITED mode never meets late flush.
    * **Classification relocation:** EIO, os.ErrClosed, other read errors resolve inside _PtyTeardown_, not here.
  * **Zero teardown duties:** No master Close, no drain timer, no process reaping, no signals.
    * **Master ownership:** _PtyTeardown_ performs every master close, including forced unplug.
    * **Reaping ownership:** _PtyProcess_ RunWorker remains sole collector through pidfd.
