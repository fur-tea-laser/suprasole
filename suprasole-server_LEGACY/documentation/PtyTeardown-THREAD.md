* **`RunWorker` charter:** the event loop of `_PtyTeardown_` (issue §3E). It is the only writer of the actor's state, and it records each fact as it arrives, so the two sides may end in either order (issue §6-W, §9E).
  * **One `select`** over the queue, `Channel_FinalState` and the three timers; after every event, `Settle` decides whether the session is over and returns `CONTINUE_WORKER` or `EXIT_WORKER`, the dispatcher's directive pattern.
  * **Timers:** created stopped in `New_PtyTeardown`, armed with `Reset`. With `go 1.25` in `go.mod`, Go's timer semantics guarantee that neither `Stop` nor `Reset` leaves a stale tick in `C`, so no channel draining is needed.
  * **Situations → actions:**
    * **Terminate, process side `UNESCALATED`:** `HangUp_PtyProcess`, arm `Timer_Unresponsive` → `AWAITING_HANGUP`.
    * **Terminate in any other process-side status:** ignored. Escalation has begun, or the process side has ended and the drain finishes the session. Repeated Terminates therefore never re-arm the timer, so they cannot postpone escalation.
    * **ReaderStarted:** reader `PENDING` → `RUNNING`; if the process side is already `FINAL`, arm `Timer_Drain`.
    * **`Channel_FinalState`:** stop `Timer_Unresponsive` and `Timer_Uninterruptible` → `FINAL`; if the reader is `RUNNING`, arm `Timer_Drain`, because `EIO` may never come while anyone else holds the slave (issue §9D).
    * **Drain fires:** close the master (the forced unplug) → PtyReader returns `os.ErrClosed` → ReaderFinished.
    * **ReaderFinished:** reader `RUNNING` → `FINISHED`; record `ExitSignal`; stop `Timer_Drain`; close the master if still open. If the process side is `UNESCALATED`, this is the rogue path (issue §9E): arm `Timer_Unresponsive` → `AWAITING_HANGUP`. No explicit signal is needed: the master close already hangs up a living session leader.
    * **Unresponsive fires:** close the master if still open; then, in one Mutex critical section, observe and — only when the observation is `UNRESPONSIVE` — send `Signal(SIGKILL, PIDFD_SIGNAL_PROCESS_GROUP)`.
      * `UNRESPONSIVE`: keep the report as `Report_Observation_latest`, arm `Timer_Uninterruptible` → `AWAITING_KILL`.
      * `TERMINATED` (`Zombie`, `Reaped`, `SystemError`): → `AWAITING_FINAL`, with no kill. The PTY process terminated in the window, and the policy after termination is unplug only (issue §8.1).
    * **Uninterruptible fires:** observe again.
      * `UNRESPONSIVE`: keep the report → `GIVEN_UP`. `_PtyProcess_` stays parked on its pidfd (the accepted cost, issue §3C) and completes on its own if the process ever terminates.
      * `TERMINATED`: → `AWAITING_FINAL`, because the channel is microseconds away.
    * **Cancel:** reader must be `PENDING`. Close the master; if the process side is `UNESCALATED`, `HangUp_PtyProcess` and arm `Timer_Unresponsive` → `AWAITING_HANGUP` (it may already be `FINAL`). Then reader → `FINISHED` (no reader will ever run), and mark the session `CANCELED`.
  * **`Settle`:** when the reader is `FINISHED` and the process side is `FINAL` or `GIVEN_UP`, stop all three timers, emit the outcome unless `CANCELED`, and exit. Otherwise continue.
    * **Exactly one outcome per session:** `Settle` emits it once and `RunWorker` returns at once; a Terminate arriving later finds nobody reading (see `HandleTerminate`).
  * **`Resolve__ExitOutcome_PtyProxy`** (issue §3F):
    * **The reader ended with `EIO` or `os.ErrClosed`:** the process side decides — `Resolve__TeardownReport_PtyProcess().Resolve__ExitOutcome_PtyProxy()`.
      * `FINAL`: the final state read under the Mutex — `Reaped` (through its `Termination`: `Exited{0}` → `Success`, `Exited{n}` → `Failure{n}`, `Signaled{sig}` → `Killed{sig}`) or `SystemError` (→ `SystemError_Process`).
      * `GIVEN_UP`: the latest observation.
      * **A final state wins over every observation.** A shell killed by the escalation reports `Killed{SIGKILL}`, the truth, not the observation taken before the kill; and a HUP-ignoring shell that ends on its own after the unplug reports its own exit (dash exits 2 when its terminal read fails).
    * **Any other read error:** `SystemError_Descriptor{error}`, once the process side has ended too.
  * **The drain counts from the reader's start, not only from the process's end.** The plan arms `Timer_Drain` on "process final, reader not finished". Here it is armed on "process final, reader running", and `ReaderStarted` arms it if the process ended first.
    * **Why:** the actor starts in `Spawn_PtyProxy`, but the reader starts only when the coordinator processes the spawn's success order, and that handler first writes to the websocket, which can stall. Arming at the process's end could close the master before any reader ran, and the kernel discards unread output on that close: a quick `printf …; exit` would lose its output. `TestPtyTeardown_DrainWaitsForReaderStart` pins this.
    * **Cost:** one more order, `_ReaderStarted__Order_PtyTeardown_`, and a three-valued reader side instead of two.
  * **Escalation worst case,** for a process that ignores the hangup: `TIMEOUT_UNRESPONSIVE` → unplug + observe + group `SIGKILL` → up to `TIMEOUT_UNINTERRUPTIBLE`, ≈ 2 s (issue §5B.6). The actor knows why the master closed, so it never waits twice.
  * **Invalid paths (panic):** Cancel with a reader already started; Cancel after escalation began (escalation needs Terminate or ReaderFinished, both of which need a reader); ReaderStarted twice; ReaderFinished without a running reader; a nil `ExitSignal` (PtyReader reports only non-nil errors); a non-final state after `Channel_FinalState` closed; an unknown enum value.
