* **PtyTeardown charter:** one actor per PTY that ends the session (issue §3E–F). It joins two independent kernel event streams, two commands and three timers, performs the one destructive act (closing the master), and emits one outcome, exactly once.
  * **Why an actor separate from `_PtyProcess_`:** different lifetimes.
    * `_PtyProcess_` mirrors the *process*. It holds nothing proxy-scoped (issue §4C), because it outlives the proxy when a process is unkillable or held by a debugger.
    * `_PtyTeardown_` ends the *session*. It needs the master and the outcome callback, and it must finish even when `_PtyProcess_` never does.
  * **What it replaced:** the dispatcher's `HandleExited` / `TeardownExited_PtyProcess`, the free `Terminate_PtyProcess` goroutine, `TerminateCancelled_PtyProxy`, and the `waitid(P_PID)` probe with its five `ProbeStatus_Unresponsive` methods.
  * **`New_PtyTeardown(id_WorkspacePty, onExitOutcome__, ptyProcess, fileDescriptor_master__Pty__shared)`:** builds the actor with its three timers created stopped, and the queue (`SIZE_QUEUE_BUFFER__Order_PtyTeardown` = 3). It takes `Channel_FinalState` from the `_PtyProcess_`.
  * **Started in `Spawn_PtyProxy`,** right beside `_PtyProcess_.RunWorker`, so a cancelled spawn goes through the same machine as every other ending. The proxy keeps it as `_PtyProxy_.PtyTeardown`, and the outcome callback (`OnExitOutcome__`) moved there from the proxy.
  * **Inputs:**
    * **`_Terminate__Order_PtyTeardown_`**, from `HandleTerminatePty__Coordinator`: the user asked to end the session.
    * **`_Cancel__Order_PtyTeardown_`**, from the cancelled branch of `HandleStatus_SpawnPty__Success__Coordinator`: no reader was ever started; end silently.
    * **`_ReaderStarted__Order_PtyTeardown_`**, from the activated branch of the same handler, sent just before it starts the dispatcher and the reader.
    * **`_ReaderFinished__Order_PtyTeardown_{ExitSignal}`**, from PtyDispatcher **after its final flush** (`OnReaderFinished__`): the TTY side has ended — `EIO`, `os.ErrClosed`, or another read error.
    * **`<-Channel_FinalState`**, from `_PtyProcess_`: the process side has ended in `Reaped` or `SystemError`. The field is set to nil once received, which disables that `select` case.
    * **The three timers:** `Timer_Drain` (`TIMEOUT_DRAIN__PtyProxy`, 100 ms), `Timer_Unresponsive` (`TIMEOUT_UNRESPONSIVE__PtyProcess`, 1 s), `Timer_Uninterruptible` (`TIMEOUT_UNINTERRUPTIBLE__PtyProcess`, 1 s).
  * **Senders, one method each,** so no caller touches the queue directly:
    * `HandleReaderFinished(exitSignal)` is the dispatcher's callback; `HandleReaderStarted()` and `HandleCancel()` are the coordinator's. All three send with a blocking send, which cannot block: each order is sent at most once, before `RunWorker` can return.
    * **`HandleTerminate()` never blocks.** The coordinator may still see the PTY `Active` after the outcome was emitted (its `ExitPty` order is still queued), when `RunWorker` has already returned and nobody will read again. So it sends only if there is room, and drops the order otherwise.
      * **Why dropping loses nothing:** besides Terminates, at most two orders ever enter the queue (ReaderStarted and ReaderFinished; Cancel excludes both). With capacity 3, a full queue therefore already holds a Terminate, and Terminate is idempotent.
  * **Its own state:** written only by its `RunWorker` goroutine, so it needs no lock.
    * **`Status_ReaderSide`:** `PENDING` (no reader yet) → `RUNNING` → `FINISHED` (ReaderFinished received, or Cancel: no reader will ever run).
    * **`Status_ProcessSide`:**
      * `UNESCALATED`: not final, no escalation yet.
      * `AWAITING_HANGUP`: `Timer_Unresponsive` is armed.
      * `AWAITING_KILL`: the group `SIGKILL` was sent; `Timer_Uninterruptible` is armed.
      * `AWAITING_FINAL`: an observation found the state machine already terminated; `Channel_FinalState` is about to close.
      * `FINAL`: `Channel_FinalState` received.
      * `GIVEN_UP`: still not final when `Timer_Uninterruptible` fired. A later `Channel_FinalState` still moves it to `FINAL`, if it arrives before the reader finishes.
    * **`Status_Master`:** `OPEN` / `CLOSED`, so every close is "close if still open".
    * **`Status_Cancellation`:** `CANCELED` after Cancel, which suppresses the outcome.
    * **`ExitSignal_PtyReader`** and **`Report_Observation_latest`**: what the outcome is resolved from.
  * **Every master close happens here** (`Close_Master`), on Drain, ReaderFinished, Unresponsive and Cancel. The dispatcher and the coordinator never close it.
    * **The close is the forced unplug:** the slave hangs up, a still-living session leader receives `SIGHUP` + `SIGCONT` from the kernel, and PtyReader returns `os.ErrClosed` in under 1 ms (issue §6-B).
    * **It makes `EIO` final.** While the master stays open, a process reopening `/dev/pts/N` would undo `EIO` (issue §6-Z). Closing on every reader finish removes the device.
  * **`HangUp_PtyProcess`:** `Signal(SIGHUP, PIDFD_SIGNAL_THREAD_GROUP)` then `Signal(SIGCONT, PIDFD_SIGNAL_THREAD_GROUP)`, under the `_PtyProcess_` Mutex. That is exactly what the kernel sends a session leader on hangup: interactive bash ignores `SIGTERM` but not `SIGHUP` (issue §6-G), and a stopped process needs the `SIGCONT` (§6-H). Errors are ignored (for example `EPERM` after a credential change), because escalation follows either way.
  * **Observation — `Observe_Unresponsive()` on each `_State_PtyProcess_`** (issue §3F). It is called under the Mutex, and each state answers for itself, like `Signal`:
    * **`Zombie`, `Reaped`, `SystemError` → `TERMINATED`, no report:** terminated in the window. The worker's path from `Zombie` to the channel close never blocks, so `Channel_FinalState` closes within microseconds.
    * **`Alive`, `Zombie_Withheld` → `UNRESPONSIVE`, with `observe__Pid_Unresponsive(Pid)`:** the kernel's own view. The PID is still ours because the caller holds the Mutex and the worker reaps only inside it (issue §3C).
    * **`read__Snapshot_Proc`:** the state letter and field 52 (`exit_code`) from `/proc/<pid>/stat`, split after the command's last `)`, since the command may itself contain spaces and `)`; and `TracerPid` from `/proc/<pid>/status`.
      * **A read or parse failure** becomes `_SystemError__TeardownReport_PtyProcess_`, never a panic. It is reachable, for example, under a `hidepid` `/proc` after the PTY process changed its uid.
    * **`resolve__Status_Tracer`:** `NONE` (0), `OWN` (`/proc/self/task/<TracerPid>` exists), or `FOREIGN`. `TracerPid` names the tracer *thread* — under `PTRACE_TRACEME`, the OS thread that forked, which in Go need not be the main thread — so comparing it with suprasole's PID would misfile self-tracing as a foreign debugger (issue §6-AI).
    * **`classify__Snapshot_Proc`, first match wins:**
      * `D` → `_Unresponsive_Uninterruptible_`, whatever the tracer: observed directly, no longer inferred from a failed `SIGKILL`.
      * a foreign tracer, any letter → `_Unresponsive_Debugged_{TracerPid}` (new): a debugger holds it in a stop, an exit stop, or as a withheld zombie. Suprasole can neither finish nor collect it (issue §5B.9). The plan's table names `t` and `Z`; a debugger letting its tracee run (`strace -p`) shows `R`/`S` with the same verdict.
      * `T`, or `t` with no foreign tracer → `_Unresponsive_Stopped_{StopSignal}`.
      * `R`, `S`, or `Z` with no foreign tracer → `_Unresponsive_Rogue_`. A `Z` here is a thread-group leader that exited while other threads run on: `/proc/<pid>/stat` shows the leader's state, and the process has not terminated.
      * Anything else (`I`, `X`, `P`, …) → `_SystemError__TeardownReport_PtyProcess_` naming the letter. An unrecognized kernel state is reported, never guessed.
    * **`StopSignal` comes from field 52, display only.** proc(5) documents the field as the thread's exit status "in the form reported by waitpid(2)". For a stopped thread the kernel shows the raw stop signal there (observed: 19 for `SIGSTOP`, 5 at a `PTRACE_TRACEME` exec trap). It reads 0 once the stop report has been collected — the worker consumes any trap report its first peek finds, such as the exec trap of a process already self-traced when the worker starts — or when `/proc` denies the field. No decision depends on it; it only fills the client's `Stopped{signal}`.
    * **Why not a `waitid` probe as before:** a `WSTOPPED|WNOWAIT` probe is blind to `D` and to every debugger-held condition, since stops are reported to the tracer, not the parent. The old probe labelled a debugger-held shell `Rogue`, then `Uninterruptible` after the futile `SIGKILL`.
    * **Observations live in the teardown report, never in `State_state`:** they are snapshots of one instant (issue §4B).
  * **Teardown reports (`_TeardownReport_PtyProcess_`)** resolve themselves into the outcome; their text and egress encoding are unchanged:
    * `_Unresponsive_Rogue_` → `SystemError_Process("… process unresponsive to hangup")`.
    * `_Unresponsive_Stopped_{StopSignal}` → `Stopped{StopSignal}`.
    * `_Unresponsive_Uninterruptible_` → `SystemError_Process("… process unkillable")`.
    * `_Unresponsive_Debugged_{TracerPid}` → `SystemError_Process("… process held by debugger (TracerPid N)")`: there is no `ExitOutcome` variant for it, and the variants stay unchanged (issue §3F).
    * `_SystemError__TeardownReport_` → `SystemError_Process(its error)`, for observation failures.
    * **`_Reaped__State_PtyProcess_` and `_SystemError__State_PtyProcess_` implement it as well,** so the final state *is* the report.
  * **`HandleExitOutcome_Pty`** (the `WorkspaceController` callback that turns the outcome into an `ExitPty` coordinator order) lives beside the reports.
  * **Policy kept from issue §8.1 — unplug only:** once the PTY process has terminated, no group kill. Lingering slave holders lose their terminal but keep running.
