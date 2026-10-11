* **launchPtyExec charter:** Starts PTY program through `suprasole-pty-exec`; returns after shell `execve` or precise failure.
  * **Sole caller:** `Spawn_PtyProcess`; orchestrator owning collection decisions plus goroutine starts.
  * **Name semantics:** Launches PtyExec helper carrying command; never plain fork-exec of command itself.
  * **ForkExec parity:** Mirrors `syscall.ForkExec` exec-verdict wait; zero half-failed children reach caller.
    * **Extended verdict:** Wait spans helper `execve`, then shell `execve` inside helper.
* **Input contract:** Preconditions established upstream; function validates nothing itself.
  * **path_PtyExec source:** Launcher flag `-Path_PtyExec`; presence plus validity unchecked by server.
    * **Relative value:** Resolves against server working directory; server never calls `chdir`.
    * **Missing helper:** Spawns fail with `fork/exec <path>`, errno `ENOENT`.
  * **command_PtyProcess source:** `Prepare__Command_PtyProcess` output; 4 fields reach shell `execve` unaltered.
    * **NUL precondition:** Strings carry zero NUL bytes; `check__NUL_PtyCommand` enforces before PTY opens.
      * **Violation outcome:** `ForkExec` rejects with `EINVAL`, blamed on helper path.
    * **Argv precondition:** Non-empty, `argv[0]` included; `Prepare__Command_PtyProcess` supplies 1 element.
      * **Violation outcome:** Helper reports `ARGUMENTS`; `resolve__PathError_PtyExec` panics invalid path.
    * **Empty directory:** Skips `chdir`; shell inherits server working directory.
    * **Environment handling:** Passed verbatim to shell; duplicates kept; nil yields empty environment.
  * **Slave descriptor:** Becomes shell fds 0-2; caller keeps ownership, closes afterward.
* **Return shapes:** Returned pidfd states whether process exists; caller decides collection.
  * **Fork failure:** `(0, nil, error)`; zero process exists.
    * **Prior cleanup:** Failed child, if forked, already collected; pidfd already closed.
  * **Helper failure:** `(0, pidfd, error)`; helper exiting with code 127.
    * **Caller duty:** `Spawn_PtyProcess` starts `reapAbandoned__PtyProcess_Pidfd` without kill.
  * **Shell started:** `(pid, pidfd, nil)`; pid plus pidfd name helper, then shell after `execve`.
    * **Identity persistence:** `execve` keeps PID; pidfd stays valid across image replacement.
* **Report channel:** Dedicated pipe carrying helper verdict back through netpoller.
  * **Pipe creation:** `os.Pipe`: `pipe2` with `O_CLOEXEC`; read end netpoller-registered.
  * **Write end:** Becomes helper fd 3; parent copy closed right after `ForkExec`.
    * **Blocking mode:** `Fd()` switches shared write description to blocking; 16-byte write never waits.
  * **Record format:** 16-byte record; little-endian `uint64` step, then `uint64` errno.
    * **Write atomicity:** Single `write` below `PIPE_BUF` 4096; partial records impossible.
  * **ABSENT status:** EOF before first byte; zero report written.
    * **Shell exec:** Shell `execve` passed `point_of_no_return`; close-on-exec dropped fd 3.
      * **Late exec failure:** Kernel forces `SIGSEGV`; worker later observes process killed.
    * **Helper death:** Signal killed helper before shell `execve`; spawn still succeeds.
      * **Exposure window:** Before `setsid`, helper shares server process group plus session.
      * **Observed outcome:** Worker reports `Killed`; true outcome: process existed.
  * **REPORTED status:** Complete 16-byte record; helper exits 127 immediately after.
  * **Wait cost:** Goroutine parks in netpoller; zero OS threads held during wait.
* **Error attribution:** Failures name real cause: helper, directory, or executable.
  * **Fork failure error:** `fork/exec <path_PtyExec>`; helper missing, resource limits, descriptor ceiling.
  * **CHDIR report error:** `chdir <Path_Directory>` carrying reported errno; matches `os.StartProcess` wording.
  * **EXECVE report error:** `fork/exec <Path_Executable>` carrying reported errno; matches prior exec wording.
  * **Unreachable step reports:** `ARGUMENTS`, `SETSID`, `TIOCSCTTY`, `RT_SIGPROCMASK`; `resolve__PathError_PtyExec` panics invalid path.
  * **E2BIG misattribution:** Size limits surface at helper exec, named against helper path.
    * **Superset cause:** Helper argv plus envp contain shell's, plus 3 extra strings.
    * **String threshold:** Single string above `MAX_ARG_STRLEN`: 32 pages, 128 KiB at 4 KiB.
    * **Total threshold:** Strings plus pointers above `max(ARG_MAX, min(6 MiB, RLIMIT_STACK / 4))`; 2 MiB typical.
    * **Reported text:** `fork/exec <path_PtyExec>: argument list too long`; client command responsible.
* **PtyExec helper:** Static arm64 program turning fresh child into PTY program.
  * **Runtime absence:** ELF entry `Entry_PtyExec` via `-ldflags=-E`; Go runtime linked, never initialized.
    * **Absent machinery:** Zero threads, zero signal handlers, zero garbage collector.
    * **Environment inertness:** Client `GODEBUG`, `GOGC` configure nothing; envp passes through untouched.
    * **Launch cost:** 81 µs fork-exec-wait cycle measured; spawn adds 1 helper `execve`.
      * **Go alternative:** Go-runtime helper measured 387 µs; rejected.
  * **Inherited state:** fds 0-2 slave, fd 3 report, forking thread's signal mask, `SIG_IGN` dispositions.
  * **Step catalog:** Close-on-exec mark plus 6 numbered steps; numbers mirror `_Step__PtyExec_`.
    * **Close-on-exec mark:** `fcntl(3, F_SETFD, FD_CLOEXEC)`; unnumbered, unreported.
      * **Failure handling:** Only without fd 3; exits 127 silently; unreachable from server.
    * **ARGUMENTS step:** argc below 4 reports `EINVAL`; manual runs only.
    * **SETSID step:** New session led by helper; failure unreachable.
      * **Unreachability basis:** Fresh child leads nothing; new PID matches zero process-group IDs.
      * **Policy immunity:** Kernel `setsid` path carries zero LSM hooks at 6.9.
    * **TIOCSCTTY step:** Slave becomes controlling terminal; argument 0 forbids stealing foreign terminals.
      * **Kernel conditions:** Session leader, zero existing terminal, unowned slave, read access satisfied here.
      * **Policy dependence:** SELinux, TOMOYO mediate ioctl; Landlock from 6.10; launcher guarantees permission.
    * **CHDIR step:** Client directory; skipped for empty string; failures client-reachable.
      * **Blocking exposure:** Hung network filesystem blocks helper only; server keeps running.
    * **RT_SIGPROCMASK step:** Empty mask set explicitly; shell mask stated, never inherited.
    * **EXECVE step:** Shell image replaces helper; success never returns; failures client-reachable.
  * **Failure exit:** Record written to fd 3; `exit_group(127)` follows regardless.
  * **Register discipline:** R19-R24 hold state; kernel preserves registers except x0 across `SVC`.
    * **Reserved registers:** R18 platform register untouched; R26-R28 left for Go toolchain.
  * **Build dependency:** `-E main.Entry_PtyExec` required; default build mode, linux/arm64 only.
* **Server invariants:** Launch path upholds server-wide guarantees from issue §5B.
  * **Exclusive reaping:** 1 collector per child across 4 outcomes.
    * **Fork failure reaper:** `syscall.ForkExec` internal `Wait4` on failed helper exec.
    * **Helper failure reaper:** `reapAbandoned__PtyProcess_Pidfd`, started by `Spawn_PtyProcess`, without kill.
    * **Unwatchable pidfd reaper:** Group `SIGKILL` through pidfd, then same reaper.
    * **Success reaper:** `RunWorker`, started by `Spawn_PtyProxy` after return.
  * **Goroutine ownership:** Function starts zero goroutines; `Spawn_PtyProcess` starts collection goroutines.
  * **Thread holding:** Vfork window spans helper `execve` only; local binary bounds duration.
    * **Client paths:** `chdir` plus shell `execve` run inside helper, outside vfork window.
    * **Residual exposure:** Hung filesystem beneath helper binary still stalls stop-the-world.
      * **Launcher duty:** §5A.3 requires healthy filesystem beneath helper binary.
  * **Descriptor ownership:** Raw pidfd wrapped by `NewFile` before return; callers receive `os.File` only.
  * **Close-on-exec discipline:** Report pipe ends carry `O_CLOEXEC`; helper marks fd 3 before shell.
    * **Concurrent spawns:** Sibling vfork child briefly holds descriptor; closes at own helper exec.
      * **EOF delay:** Bounded by sibling pre-exec window: microseconds.
  * **Kernel floor:** `CLONE_PIDFD` since 5.2; `PIDFD_SIGNAL_PROCESS_GROUP` since 6.9; floor 6.9.
  * **Security axiom:** Unreachability claims assume calls run kernel code paths unfiltered.
    * **Launcher guarantee:** §5A.2 covers seccomp, SELinux, AppArmor, Smack, TOMOYO, Landlock, BPF-LSM.
    * **Domain transitions:** Helper exec triggers zero security-domain change; launcher guarantees.
    * **Denial outcome:** Refused step reaches `resolve__PathError_PtyExec`; spawn panics loudly.
* **Known limits:** Open decisions surfaced by audit 2026-10-10.
  * **Hung helper:** Blocked `chdir` or `execve` parks spawn goroutine indefinitely.
    * **PTY state:** Workspace PTY remains `SPAWNING`; result never reaches coordinator.
    * **Kill path:** Pidfd stays local to blocked call; cancellation never reaches helper.
    * **Server impact:** Coordinator plus other PTYs continue; 1 goroutine parked.
    * **Candidate remedy:** Report-pipe read deadline, then group `SIGKILL`, reap, timeout error.
  * **Misbuilt helper:** Build without `-E` runs empty Go `main`; exit 0 closes fd 3.
    * **Silent success:** `ABSENT` verdict; spawn succeeds; worker resolves `Success` immediately.
    * **Chosen handling:** Documented build requirement; zero runtime detection.
    * **Build responsibility:** Build pipeline plus launcher, per issue §5A.3.
  * **Ignored signals:** Server `SIG_IGN` dispositions survive into shell; `nohup` leaves `SIGHUP` ignored.
    * **Teardown consequence:** Terminate `SIGHUP` ineffective; escalation to `SIGKILL` required.
    * **Candidate remedy:** Helper resets dispositions to `SIG_DFL`, skipping `SIGKILL`, `SIGSTOP`.
  * **E2BIG remedy:** Report helper-exec `E2BIG` against command, not helper; only command strings grow unbounded.
  * **Inherited descriptors:** Non-close-on-exec launcher descriptors reach shell; issue §8.4 undecided.
* **Test coverage:** `Spawn_PtyProcess` tests under `tests/runTests.sh` exercise launch outcomes.
  * **Covered outcomes:** Directory failure, executable failure, missing helper, session plus terminal, environment, abandon.
    * **Session assertions:** Shell leads session plus group; `tty_nr` nonzero; `SigBlk` empty.
  * **Uncovered outcomes:** Misbuilt helper, `E2BIG` attribution, hung helper, concurrent spawns.
  * **Helper build:** `tests/runTests.sh` builds helper with `-E`; exports `SUPRASOLE_PTY_EXEC`.
* **Verification basis:** Go 1.27.1 source, kernel v6.9 source, arm64 disassembly.
  * **Annotation safety:** Annotated assembly assembles to identical machine code.
