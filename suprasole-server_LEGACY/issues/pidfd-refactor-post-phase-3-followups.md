# Issue: pidfd Refactor — Follow-ups After Phase 3

Written 2026-10-11, after phase 3 (the `_PtyTeardown_` actor) was implemented and `issues/pidfd-refactor-pre-phase-3.md` was audited against the code.

**None of this blocks the next chunk of work** (`issues/pidfd-refactor-spawn-hardening.md`). Nothing here is a bug that can trigger under the plan's assumptions. These are decisions still to make, small fixes, and documentation that drifted from the code.

How to find things: code is named by function and file. Plan text is quoted, so it can be found with a text search in `issues/pidfd-refactor-pre-phase-3.md`.

**What actually needs you: two things, neither urgent.**
1. When to commit phase 3, and whether `tests/` and the untracked docs go in with it (section 0).
2. Whether error details should be logged on the server and/or sent to the client (decision 1b). This is the one real product decision.

**Everything else has a default:**
- **Decisions 1a, 1c, 1d:** the recommendation stands unless you object.
  - 1a: no group kill in the debugger case.
  - 1c: keep today's shutdown behaviour and document it.
  - 1d: keep the field-52 signal number.
- **Fixes 2a and 2b:** done as part of the spawn-hardening chunk.
- **Section 3:** the code already does the right thing; only the plan's wording needs updating.
- **Section 4:** documentation fixes; no decisions involved.
- **Section 5:** matters only when the unified-spawn-lifecycle draft is picked up.
- **The race detector:** run it whenever a machine with a C compiler is handy.

---

## 0. Housekeeping

- [ ] **Commit phase 3.** It is uncommitted as of this writing. `tests/runTests.sh` passes.
  - New: `source/PtyTeardown.go`, `source/PtyTeardown-THREAD.go`, `tests/PtyTeardown_test.go` (15 tests), `documentation/PtyTeardown.md`, `documentation/PtyTeardown-THREAD.md`.
  - Changed: `source/LifecycleCoordinator_WorkspacePty-THREAD.go`, `source/PtyDispatcher.go`, `source/PtyDispatcher-THREAD.go`, `source/PtyProcess.go`, `source/PtyProxy.go`, `tests/PtyDescriptor_test.go`, `documentation/PtyProcess.md`, `documentation/PtyDispatcher-THREAD.new.md`.
- [ ] **Nothing under `tests/` has ever been committed.** That includes the phase 1 and phase 2 tests and the kernel contract suite (`tests/contract/`). The phase 1–3 companion docs (`documentation/PtyDescriptor.md`, `PtyProcess.md`, `PtyProcess-THREAD.md`, `PtyExec.md`, `launchPtyExec.md`, `PtyTeardown*.md`) and the issues other than `pidfd-refactor-pre-phase-3.md` are untracked too.
- [ ] **The race detector has never been run.** `go test -race` needs cgo, and the dev container has no C compiler. Run it once somewhere that has one.

---

## 1. Decisions about code behaviour

### 1a. A process that exits while a debugger is attached: kill its process group?

- **Today:** the teardown's 1-second "unresponsive" timer can fire after the PTY process has exited but before an attached debugger has acknowledged the exit (state `_Zombie_Withheld__State_PtyProcess_`). In that case `HandleUnresponsive` (`source/PtyTeardown-THREAD.go`) still sends `SIGKILL` to the PTY process's group. That does nothing to the dead process, but it kills any still-living members of its group.
- **The plan contradicts itself:**
  - The teardown's action table says the timer firing means "observe … `Signal(SIGKILL, PIDFD_SIGNAL_PROCESS_GROUP)`; arm Uninterruptible", with no condition.
  - The decision headed "Lingering Processes After the PTY Process Terminates — *decided: unplug only*" says that once the PTY process has exited, the teardown only unplugs the terminal.
  - So a normal exit leaves those group members running, while an exit a debugger delayed kills them.
- **Recommendation:** follow "unplug only". For the kill, treat this state as already exited, but still read /proc so the outcome says "held by debugger".
  - The change is small: `Observe_Unresponsive` on `_Zombie_Withheld__State_PtyProcess_` (`source/PtyTeardown.go`) returns a third status meaning "exited, report withheld: observe, don't kill".
- **Stakes:** low. It needs a debugger attached at that exact moment.

### 1b. Error details never reach anyone

- **Spawn failures:**
  - `Prepare__Command_PtyProcess` and `Spawn_PtyProcess` build precise errors, e.g. `chdir /some/dir: no such file or directory` or `fork/exec /bin/zsh: …`.
  - `HandleStatus_SpawnPty__Failure__Coordinator` (`source/LifecycleCoordinator_WorkspacePty-THREAD.go`) receives the error and drops it. The client gets one status byte (`FAILURE__Status_SpawnPty`), and nothing is logged.
- **Exit outcomes:**
  - `encodeStruct__ExitOutcome_PtyProxy` (`source/PtyMessage_Egress.go`) encodes `_SystemError_Descriptor__ExitOutcome_PtyProxy_` and `_SystemError_Process__ExitOutcome_PtyProxy_` identically: disposition SYSTEM_ERROR, exit code -1, signal 0.
  - So these all look the same to the client, and none is logged: "unresponsive to hangup", "unkillable", "held by debugger (TracerPid N)", a failed /proc read, and a read error on the master.
- **The plan assumes otherwise:** it says "so clients see the same text as before", but clients see no text at all.
- **Options:**
  - Log these on the server: cheap, and no protocol change.
  - And/or carry the text, or finer categories, in the messages sent to the client. That is a client change, and the plan currently says the outcome variants "and their egress encoding are unchanged".

### 1c. Server shutdown skips the teardown

- **Today:** `WorkspaceController.Shutdown` (`source/main.go`) stops the coordinator, closes the websocket and the HTTP server, and then the process exits.
  - The kernel closes every master, which hangs up each shell, and most die.
  - A PTY process that ignores `SIGHUP` keeps running, orphaned, and nothing escalates to `SIGKILL`.
- **The plan:** never mentions shutdown.
- **Options:**
  - Keep terminal-emulator behaviour (quitting the app hangs up its shells) and document it.
  - Or, on shutdown, call `HandleTerminate` on every live session's `_PtyTeardown_` and wait for the teardowns before exiting. The wait is bounded: worst case about 2 s.

### 1d. Where "stopped by signal N" gets its number

- **Today:**
  - When the teardown gives up on a stopped process, the client sees "Stopped" plus a signal number, carried by `_Unresponsive_Stopped__TeardownReport_PtyProcess_`.
  - `read__Snapshot_Proc` (`source/PtyTeardown.go`) takes that number from field 52 (`exit_code`) of `/proc/<pid>/stat`.
- **Why it's a question:**
  - The plan's /proc observation is the state letter plus `TracerPid`, and neither carries the signal.
  - proc(5) documents field 52, but not its value for a stopped thread. Observed on this kernel: 19 for SIGSTOP, 5 at a `PTRACE_TRACEME` exec trap, and 0 once the worker has consumed the stop report.
  - No decision depends on it; it is only displayed.
- **Stakes:** low. "Stopped" can only be the final outcome when `SIGKILL` was refused, e.g. because the PTY program switched users like `su`. Otherwise SIGKILL ends stopped processes immediately.
- **Options:**
  - Keep it (current).
  - Always report 0.
  - Use a non-collecting `waitid(P_PIDFD, WSTOPPED|WNOHANG|WNOWAIT)` query. That is documented behaviour, but it brings back the kind of `waitid` probe the plan removed.
- If it is kept, add it to the plan's provenance table as "observed, display only".

---

## 2. Small code fixes in the spawn path (do them with the next chunk, which reworks this code)

### 2a. The helper reports a step the server doesn't know

- The helper (`source/PtyExec/PtyExec_linux_arm64.s`) defines `RT_SIGACTION__Step__PtyExec 7`, the signal-disposition reset added 2026-10-10. Its comment says the step numbers "Must match _Step__PtyExec_ in source/PtyProcess.go". But `_Step__PtyExec_` stops at 6.
- A step-7 report would reach the final `else` of `resolve__PathError_PtyExec` and panic, and that `else`'s comment only lists ARGUMENTS / SETSID / TIOCSCTTY / RT_SIGPROCMASK.
- It can't actually happen: resetting signals 1–64 (except SIGKILL and SIGSTOP) to their default cannot fail.
- **Fix:** add `RT_SIGACTION__Step__PtyExec _Step__PtyExec_ = 7`, and list it among the impossible cases in that comment and in `documentation/PtyProcess.md`.

### 2b. The report pipe has no "accepted by the event loop" check

- `Spawn_PtyProcess` checks that Go's netpoller accepted the pidfd (`SetReadDeadline(time.Time{})`). `await__Report_PtyExec` (`source/PtyProcess.go`) reads the helper's report pipe without the same check.
- If the netpoller refused the pipe (the per-user limit `fs.epoll.max_user_watches` is exhausted, or `ENOMEM`), Go silently falls back to a blocking read. That read holds an OS thread for the whole launch, or indefinitely if the helper hangs on a dead network filesystem.
- Very unlikely. The plan's unified-spawn-lifecycle draft already includes this check, and it does not depend on the hybrid fork.

---

## 3. Phase 3 departures from the plan's wording: confirm, then record them in the plan

Each came from something found while implementing. In each case the code looks closer to correct than the plan's wording.

- [ ] **The drain timer waits for the reader to start.**
  - The plan says "Process final, reader not finished | arm Drain".
  - The problem: the teardown starts at spawn, but the reader only starts when the coordinator handles the spawn-success message, and that comes after a websocket write that can take up to 10 s. A command that prints and exits immediately could finish before any reader exists, and closing the master would then discard its output.
  - The code: the coordinator sends `_ReaderStarted__Order_PtyTeardown_`, and the drain needs both "process final" and "reader running".
  - Test: `TestPtyTeardown_DrainWaitsForReaderStart`.
- [ ] **Terminate never blocks the coordinator.**
  - The plan says "Terminate after escalation began, or after the outcome | ignored — idempotent". But after the outcome the teardown has exited, and nobody reads its queue anymore.
  - The code: `HandleTerminate` sends without blocking into a queue of 3 (`SIZE_QUEUE_BUFFER__Order_PtyTeardown`). A full queue always already holds a Terminate, because the other orders can occupy at most two slots.
  - Test: `TestPtyTeardown_TerminateAfterOutcomeNeverBlocks`.
- [ ] **A finished reader starts escalation only if none is running.**
  - The plan says "If the process isn't final, arm Unresponsive (the rogue path)".
  - But escalation itself closes the master, which ends the reader while the process is being killed. Read literally, the plan's rule restarts escalation: a second kill round, and double the worst-case time.
  - The code re-arms only from "not yet escalated".
- [ ] **No group kill if the process has already exited.**
  - The plan's table kills unconditionally.
  - But its observation rules say "`Zombie`, `Reaped` or `SystemError` — it terminated in the window", and "unplug only" forbids a group kill after exit.
  - The code skips the kill and waits for the final state, which takes microseconds. Decision 1a is the one case this doesn't cover.
- [ ] **/proc classification.**
  - The plan's table says "`t` or `Z` | not one of suprasole's threads | Debugged".
  - "Not one of suprasole's threads" includes "no tracer". So a program whose main thread exited while other threads run on (/proc shows a zombie, but the process is alive) would be reported as "held by debugger (TracerPid 0)".
  - The table also has no row for a debugger letting the process run (`strace -p`), a trace-stop with no tracer, or letters such as `I`, `X`, `P`.
  - The code (`classify__Snapshot_Proc`):
    - `D` (uninterruptible) comes first.
    - Any foreign tracer → debugged.
    - `T`/`t` → stopped.
    - `R`/`S`/`Z` → unresponsive to hangup ("rogue").
    - Anything else → reported as an unrecognized state.
- [ ] **Gaps filled without conflict:**
  - The debugger case resolves to a system error that names the tracer (`_Unresponsive_Debugged__TeardownReport_PtyProcess_`), because the plan keeps the outcome variants fixed.
  - The plan's teardown struct has no `Id_WorkspacePty`, which the outcome callback needs.
  - If the process exits after the teardown gave up but before the reader finished, the real exit wins.

---

## 4. Stale documentation

None of these change what the code should do.

### In `issues/pidfd-refactor-pre-phase-3.md`

- [ ] **Status:**
  - It still lists phase 3 as "pending", and doesn't say phase 2 is committed (`1e88d9a`).
  - The companion-document list lacks `documentation/PtyTeardown.md` and `documentation/PtyTeardown-THREAD.md`.
  - The implementation-phases table still says phase 3 is "Pending".
  - The notes beginning "Now commented out, pending phase 3" describe code that is now deleted.
- [ ] **Spawn:**
  - It describes the design from before the helper: "`Spawn_PtyProcess(ptyCommand, slave)` builds the whole `ProcAttr`", with `Dir` and `Setsid`/`Setctty`.
  - Today `launchPtyExec__Command_PtyProcess` sets only `Env`, fds 0–3 and `PidFD`, and the helper does setsid, takes the controlling terminal and runs chdir.
  - Nearby text places the pidfd's `O_NONBLOCK` handling in `Spawn_PtyProcess`; it is in `wrap__Pidfd`.
- [ ] **Helper steps:**
  - The division-of-labor table says "the helper does `setsid`, `TIOCSCTTY`, `chdir`, empty signal mask, `execve`".
  - The spawn section says "`setsid` → `TIOCSCTTY` → `chdir` → reset signal dispositions → `execve`".
  - Actual order: reset signal dispositions → setsid → take the terminal → chdir → clear the signal mask → execve.
- [ ] **Signals:**
  - It says a `Control` failure "is read as "already reaped"", and two bullets later that it "is still returned rather than assumed away".
  - The code (`sendSignal_Pidfd`) asserts it cannot happen and panics, which was a phase 2 decision.
- [ ] **Known limits, "Command resolution unchanged":**
  - It says `exec.Command` "still describes the command" and duplicate environment keys "still collapse to the last one".
  - Neither has been true since `exec.Cmd` was dropped; `TestSpawn_PtyProcess_EnvironmentPassesThroughUnchanged` proves duplicates pass through.
- [ ] **Known limits, a shell that traces itself:**
  - It says the outcome is "reported as `Stopped` via `CLD_TRAPPED`", but that described the stopgap probe.
  - With phase 3 the outcome is "killed by SIGKILL", because the final state wins.
- [ ] **Interplay table:**
  - It says "Collected by someone else … `ECHILD` → `SystemError`".
  - The code panics on `ECHILD`, which is unreachable while nothing else reaps the child.
- [ ] **Escalation:**
  - It says the escalation reaches "the leader plus its foreground group".
  - It actually reaches the PTY process's own group. Under a job-control shell, the foreground group is the job's.
- [ ] **Evidence row on a full descriptor table:**
  - It says "where the bit is the dead `CLONE_DETACHED`". That's the wrong bit: `CLONE_PIDFD` is 0x1000, while `CLONE_DETACHED` is 0x00400000 (see Go's `syscall/exec_linux.go`).
  - Before Linux 5.2 that bit was unassigned, and the legacy `clone` ignores unknown bits.
- [ ] **Launcher requirements:**
  - It says "How the helper is built … are in `pidfd-refactor-spawn-hardening.md`".
  - But building with `go build -ldflags='-E main.Entry_PtyExec'` is required today; without it every spawn falsely "succeeds". State it here, or point to `documentation/PtyExec.md`.
- [ ] **Minor:**
  - "a close-on-exec report pipe on fd 3": fd 3 arrives without close-on-exec; the helper sets it.
  - "The parent closes the slave immediately after `ForkExec`": it closes once `Spawn_PtyProcess` returns, after the helper's verdict.
  - "All three `waitid` call sites": there are four (the peek, the trap consume, the collect, the abandon reaper).
  - "one per "Depends" row": 21 tests cover 12 rows.
  - "`checkFloor` implements exactly this": the function is `checkFloor__Kernel`.
  - The constants list omits `PIDFD_SIGNAL_THREAD_GROUP` and `POLLIN`.
  - The division-of-labor table omits `TIOCGPTN`.
  - "the `chdir`-failure policy (refuse)" is listed as deferred, though refusing is already today's behaviour.
  - "Problem Statement (current code)" describes the code from before the refactor.
- [ ] **Header notes:**
  - One says the spawn-hardening doc is referenced from only two places ("point to it"), but the unified-spawn-lifecycle draft, Status and the companion list reference it too.
  - "**Added here:**" doesn't mention the rewritten spawn, launcher and lifecycle passages.

### In the companion docs

- [ ] `documentation/PtyExec.md` lists "Reset signal dispositions" under what the helper "deliberately does not do", but it does reset them, since 2026-10-10.
- [ ] `documentation/launchPtyExec.md` says "Close-on-exec mark plus 6 numbered steps", but there are 7.
- [ ] `documentation/PtyProcess.md`'s list of impossible helper steps omits step 7 (`RT_SIGACTION`). See fix 2a.
- [ ] `issues/uninterruptible-reaper-goroutine-hang.md` describes the `os.Process.Wait()` reaper, which phase 2 removed. What remains is the plan's accepted cost: a parked worker goroutine, with no thread held. Close it or mark it resolved.

---

## 5. Planning questions and lower-priority notes

- [ ] **Unified spawn lifecycle draft: is it in this plan's scope?**
  - The plan's header counts it as part of this plan ("so the plan is complete only with both"). But the draft builds on the spawn-hardening work, which may start only after phases 1–3.
  - Its core doesn't need the hybrid fork: the `Spawning` / `Launching` / `NeverStarted` states, moving the launch verdict into `RunWorker`, and the teardown owning a cancel while spawning.
  - Only the two-tier `E2BIG` check, the pinned helper descriptor and the fork mechanism do.
  - Decide whether to move it to the spawn-hardening chunk or split it.
- [ ] **The draft vs. the state-machine section:**
  - The draft makes `Spawn_PtyProcess` "a pure constructor with no syscalls, returning a `_PtyProcess_` in `Spawning`".
  - The state-machine section ("No pre-spawn state") argues that such a state misrepresents reality.
  - One of them has to give.
- [ ] **The draft's outcome wording:**
  - It says: "End-of-file with the process already terminated → collect → `Reaped`; the outcome is "ended before exec was confirmed"".
  - That mislabels a shell that exec'd fine and exited at once (e.g. `/bin/false`). End-of-file without a report already proves no setup step failed, so the termination itself is the honest outcome.
- [ ] **`_SystemError__State_PtyProcess_` has no reachable producer.**
  - The plan parks this ("whether this state keeps any reachable producer is an open question").
  - The plan's own assumptions rule out every producer:
    - `SyscallConn`: only fails on a nil file.
    - `RawConn.Read`: fails only on a closing file, a set deadline, or an unpollable file, and all three are excluded.
    - `Control`: the worker owns the file.
    - `ppoll`: with one descriptor the kernel uses an on-stack array, so the plan's example `ENOMEM` can't occur, and `EINTR` is retried.
  - Meanwhile `ECHILD` is asserted, so the two policies disagree.
  - Decide whether to assert these too, or keep the state as a typed home for them.
- [ ] **The spawn contract's "no OS thread is ever held" is too strong:**
  - `ForkExec`'s vfork window, and its `Wait4` after a failed exec, both hold a thread. The exclusive-reaping derivation admits the `Wait4`.
  - See also fix 2b.
- [ ] **The no-trap reasoning covers only the kill path:**
  - `reapAbandoned__PtyProcess_Pidfd` has no branch for a trap report, and the plan's derivation assumes the group `SIGKILL` came first.
  - The second path, where the helper reported a failure and is reaped without a kill, needs its own one-line reason: the helper never execs the PTY program, so it cannot have asked to be traced, and a foreign tracer's stops go to that tracer.
- [ ] **The teardown's lifetime:**
  - "After the master closes, the reader finishes within milliseconds" is only true for the read itself.
  - The teardown also waits for the dispatcher's final flush, which can take up to the websocket write deadline (`DeadlineTimeout_Write__`, 10 s, `source/main.go`) on a stalled client.
- [ ] **Status's open list** omits three items: the open decision on descriptors inherited from the launcher (who runs `close_range`), the tentative state names, and the parked `SystemError` question.
