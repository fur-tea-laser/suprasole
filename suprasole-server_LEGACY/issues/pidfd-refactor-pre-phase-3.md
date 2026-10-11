# pidfd Process Lifecycle & Native PTY — Complete Plan Before Phase 3

> **What this file is.** The whole refactor plan, copied on 2026-10-10 from `pidfd-refactor-and-support.md` (unchanged). Phase 2's remaining work and the unified spawn lifecycle (§3H) send every ending through phase 3's `_PtyTeardown_` (§3E–F, §8.1), so the plan is complete only with both.
> - **The spawn path appears here as interface and responsibilities only.** The helper's internals and the deferred hybrid-fork hardening were extracted to `issues/pidfd-refactor-spawn-hardening.md`; §3B and §5A.3 point to it, and nothing else of the plan is left out.
> - **`issues/pidfd-refactor-spawn-hardening.md` is strictly out of scope for pre-phase-3, and must not be read into it.** It is a separate chunk of work, to be started only *after* the pre-phase-3 phases (1–3) are complete. Do not let it scope, constrain, or expand this plan: the helper's contract in §3B is everything pre-phase-3 depends on. It is named here only so the extracted mechanism has a home — reading it before phase 3 lands only risks tainting this plan's scope.
> - **Section numbers follow the original,** so cross-references (§3B, §5A.3, §6-AL, …) still resolve.
> - **Added here:** this note, the status, and the companion-document list.

> **Audit — 2026-10-09 (revision 6: kernel floor 6.9, kernel-aligned state machine, `_PtyTeardown_`, process × PTY interplay, provenance).** Checked against `source/*.go` (including the uncommitted `Terminate_PtyProxy` first pass), `creack/pty` v1.1.24, the Go 1.27.1 standard library (`go.mod` targets 1.25), and Linux 6.12.76. Every kernel, PTY and Go-runtime behavior relied on below was exercised with throwaway harnesses, and the design (§3) was run end-to-end as a prototype — results in §6. Blindspots, invariants and the provenance of every load-bearing behavior (documented vs. designed vs. merely observed) are in §5, changes from the original draft in §7, open choices in §8, and the full interplay between the kernel's process and TTY subsystems — every process event, PTY event and descendant kind — in §9.
>
> **Vocabulary.** "Final state" means a state the machine never leaves. "Terminal" is avoided in that sense, because in this codebase it means the PTY / `PtyTerminal`. "Terminated" and "termination" follow POSIX: a process terminates when it exits or is killed, and its *termination status* is what `waitid` reports.

## Status — 2026-10-10
- **Phase 1 (§3A):** committed, `b507ca7`.
- **Phase 2 (§3B–D, §4), spawning through the `suprasole-pty-exec` helper** (§3B): `tests/runTests.sh` passes.
- **Spawn hardening** (`pidfd-refactor-spawn-hardening.md`; §3H's unified lifecycle builds on it): its own chunk of work, not implemented, except the helper's signal-disposition reset (done 2026-10-10). That document carries its own status and remaining items.
- **Open** (§3H): the backstop timeout; the client protocol for launching → running; a cancel while `Spawning`.
- **Phase 3 (§3E–F):** pending.

## Companion Documents
Code-level detail lives beside the code:
- `documentation/PtyDescriptor.md` — the native PTY (phase 1).
- `documentation/PtyProcess.md`, `documentation/PtyProcess-THREAD.md` — `_PtyProcess_` and its worker.
- `documentation/PtyExec.md`, `documentation/launchPtyExec.md` — the helper and its launch, including the 2026-10-10 launch audit (`launchPtyExec.md`, "Known limits").
- `issues/pidfd-refactor-spawn-hardening.md` — the spawn-path mechanism: the helper's internals and the deferred hybrid-fork hardening, extracted from §3B / §5A.3 (kept there as interface only).

---

## 0. Division of Labor

| Concern | Owner | Mechanism |
|---|---|---|
| PTY device: open, unlock, size, slave | **suprasole** (replaces `creack/pty`) | `/dev/ptmx` + `TIOCSPTLCK`, `TIOCSWINSZ`, `TIOCGPTPEER` |
| fork/exec: dir, env, fds 0–2, `setsid`, controlling tty | **suprasole** (helper `suprasole-pty-exec`) | `syscall.ForkExec` of the helper (fds 0–3, `PidFD`); the helper does `setsid`, `TIOCSCTTY`, `chdir`, empty signal mask, `execve` of the `_Command_PtyProcess_` exactly as given (§3B) |
| pidfd creation | **kernel, via Go** | `SysProcAttr.PidFD` → `CLONE_PIDFD` at fork |
| Observe termination, then reap | **suprasole** | netpoller-parked pidfd; `waitid(P_PIDFD)` peek (`WNOWAIT`), then reap |
| Signal the PTY process (`Signal(…, PIDFD_SIGNAL_THREAD_GROUP)`) | **suprasole** | `pidfd_send_signal(pidfd, sig, NULL, PIDFD_SIGNAL_THREAD_GROUP)` |
| Signal its process group (`Signal(…, PIDFD_SIGNAL_PROCESS_GROUP)`) | **suprasole** | `pidfd_send_signal(pidfd, sig, NULL, PIDFD_SIGNAL_PROCESS_GROUP)` |
| pidfd capability (kernel ≥ 6.9, seccomp) | **launcher** | §5A |
| Descriptors inherited when suprasole itself was exec'd | **open: suprasole or launcher** (§8.4) | `close_range(…, CLOSE_RANGE_CLOEXEC)`, on either side |
| End the session: join the process side and the TTY side, Terminate / Cancel, timers, every master close, the outcome | **suprasole** — `_PtyTeardown_` (§3E) | one actor per PTY |
| Data plane | unchanged (the dispatcher no longer closes the master) | PtyReader / PtyWriter / PtyDispatcher / PtyResizer |

* **Every signal goes through the pidfd.** No signal anywhere addresses a PID number, so no lock or reap-state bookkeeping is needed to keep signals away from recycled PIDs (§3D).
* **Out of scope for both suprasole and its launcher:** reaping orphaned descendants. These are processes re-parented to PID 1 or a subreaper after their PTY process dies.
* **Why Go still forks.** Forking a multithreaded Go runtime by hand is not safe; `syscall.ForkExec` is the primitive `os/exec` itself is built on. It carries no process object, no handle, and no hidden state.
* **Why not `exec.Cmd` / `os.Process`.** `os.Process` keeps its own pidfd and a status machine that cannot follow suprasole's reaping. Its `Wait()` ties up an OS thread, and it breaks when the shell makes suprasole its tracer (§1.4, §6-I). A copy of its handle shares `O_NONBLOCK` with Go's (§6-K). Using it would mean creating a second owner only to `Release()` it.
  - `exec.Cmd` is no longer used (2026-10-10). Its implicit policies were: `LookPath` against suprasole's `PATH`; `Environ()`'s duplicate removal and silent NUL drop; inheriting suprasole's environment when `Env` is nil; and `[Path]` when `Args` is empty. They are replaced by `_Command_PtyProcess_`, whose fields reach `execve` exactly. The one policy kept, looking up a bare name in suprasole's `PATH`, is now explicit in `resolve__Path_ShellBinary` (§3B).
* **Where `SysProcAttr.PidFD` comes from.** `syscall.SysProcAttr` holds the Linux fork/exec options (`Setsid`, `Setctty`, …); `exec.Cmd.SysProcAttr` points at the same struct. Go 1.22 added `PidFD *int`. When it is non-nil, Go's fork passes `CLONE_PIDFD` to `clone(2)`, and the kernel returns a pidfd for the child to the parent **atomically with the fork** — there is no window in which the child exists without a handle. Go writes the fd into `*PidFD` only if the process started, and writes `-1` if the kernel cannot provide one. The fd is `O_CLOEXEC` and blocking. `os.StartProcess` uses this same field internally to obtain its own handle.

---

## 1. Problem Statement (current code)
1. **PID Recycling Races**: On this toolchain `os.Process.Signal` is already pidfd-backed; the exposure is the raw numeric calls:
   - `_SYSCALL.Kill(-pid, SIGTERM)` in `Terminate_PtyProxy` and `TerminateCancelled_PtyProxy`;
   - `Kill(-pid | pid, SIGKILL)` in `__kill__PtyProcess_Unreaped`;
   - `waitid(P_PID)` in `probeExited__PtyProcess` and `probeStatus__PtyProcess_Unresponsive`.

   A numeric PID (equal to the PGID, since `Setsid`) is safe exactly while the shell is unreaped — a zombie still owns its PID. A `TerminatePty` order that still finds the PTY `Active` after `HandleExited` has reaped the shell runs `Kill(-pid, …)` against a freed number.
2. **Absence of Wait Timeouts in POSIX**: `wait4` / `waitid` either block indefinitely or poll. The code works around this with detached reaper goroutines (`spawnReaper_PtyProcess` + timer selects) and a 5 ms `WNOHANG` polling loop (`ticker_probe`).
3. **Uninterruptible Reaper Goroutine Hangs**: A `D`-state child ignores `SIGKILL`, so its `spawnReaper_PtyProcess` goroutine waits forever in `os.Process.Wait()` and ties up an OS thread (`uninterruptible-reaper-goroutine-hang.md`). After the refactor the residual cost is the accepted cost in §3C.
4. **Stop / ptrace reports: reachable, and `os.Process` cannot survive them**:
   - The shell can make suprasole its tracer by calling `PTRACE_TRACEME`; suprasole never has to call ptrace itself. Trace-stops are then reported to suprasole's waits even under `WEXITED` alone.
   - Go's `Wait()` returns that trap-stop, marks the process done and closes its handle. From then on `Signal`, `Wait` and `WithHandle` all refuse, and the child sits trace-stopped and unreaped (§6-I).
   - The existing re-reaper recursion cannot recover from this: its second `Wait()` gets `ECHILD`.
   - Under the new design the worker peeks without reaping and consumes a `CLD_TRAPPED` report with a `WSTOPPED`-only `waitid`, which can never reap (§6-R). It keeps waiting (§6-J). The recursion is deleted, not ported.
5. **Forced Unplug Is a No-Op Today**:
   - `creack/pty` v1.1.24 performs every ioctl through `os.File.Fd()` (`ioctl.go`: "Fall back to blocking io"), including inside `Open()` and `Setsize()`. `Fd()` clears `O_NONBLOCK` on files Go itself made non-blocking, so the master is in **blocking mode** (§6-A).
   - Then `masterFD.Close()` neither interrupts PtyReader's parked `read(2)` nor closes the descriptor (`poll.FD` defers `close(2)` until the read returns), so no hangup reaches the slave. Each idle PtyReader also ties up an OS thread.
   - Combined with interactive bash ignoring `SIGTERM` (§6-G), the current `Terminate_PtyProxy` stalls on an idle shell until it happens to print something.
6. **Natural Exit With a Lingering Job Never Completes**:
   - `EIO` depends on every slave holder, not on the shell. A job still running when an interactive shell `exit`s lives in its own process group, gets no `SIGHUP`, and keeps the slave open (§6-F).
   - The PTY never reaches `HandleExited`, and the shell stays an unreaped zombie until the job exits.

---

## 2. Kernel Primitives — Floor: Linux 6.9

| Primitive | Kernel | Semantics relied on |
|---|---|---|
| `TIOCGPTPEER` | 4.13 | Opens the slave straight from the master fd — no `/dev/pts/N` path lookup, immune to devpts-namespace mismatch. |
| `pidfd_send_signal(2)` | 5.1 | Signals the pinned process. To a **zombie** it returns success but has no effect (§6-S); after the reap, `ESRCH`. |
| `CLONE_PIDFD` | 5.2 | pidfd returned by the fork itself (`SysProcAttr.PidFD`). |
| `POLLIN` on pidfd | 5.3 | Readable strictly when the process terminates; stops never wake it. |
| `waitid(P_PIDFD, …)` | 5.4 | Inspect / reap through the handle. `WNOHANG`: `si_pid == 0` ⇒ nothing reportable. `WNOWAIT`: report without collecting — a zombie stays a zombie (§6-Q). `WSTOPPED` without `WEXITED`: consumes stop reports and never reaps; on a zombie it returns `ECHILD` (§6-R). |
| `PIDFD_SIGNAL_PROCESS_GROUP` | 6.9 | Documented for a pidfd that refers to a **process-group leader** (`pidfd_send_signal(2)`); `Setsid` makes the PTY process one for life (through a non-leader's pidfd: `ESRCH`, §6-AH). Signals the process group led by the pidfd's process, addressed through the kernel's process object rather than a number. Before the reap it reaches the whole group (§6-C); after the reap it still reaches members left in that group and returns `ESRCH` once the group is empty (§6-O). It can never reach a group that later reuses the number. |

* **Why 6.9:** it is the release that lets group signals go through the pidfd. Below it, group signals need numeric `kill(-pgid)`, plus a Mutex and reap state so that the kill can never land after the reap.
* **What 6.9 excludes:** hosts still on Debian 12 (6.1), Ubuntu 22.04 (5.15), Ubuntu 24.04 GA (6.8) or RHEL 9 (5.14) kernels.
* **What it covers:** Docker Desktop's VM (6.12 here) and Apple `container`'s default Kata kernel (reported as 6.18).
* **The floor is the host's kernel, not the image's.** Containers share the host kernel; for VM-based runtimes it is the VM's kernel. The image contributes only userland.
* **Not used:** `PIDFD_NONBLOCK` (5.10) applies only to `pidfd_open`, and the pidfd comes from `CLONE_PIDFD` instead. `fcntl` `O_NONBLOCK` (needed by the netpoller) plus `WNOHANG` covers it.
* **Constants:** `syscall` defines `TIOCSPTLCK`, `TIOCSWINSZ` and `SYS_WAITID`, but not:
  - `TIOCGPTPEER` = `0x5441`;
  - `SYS_PIDFD_SEND_SIGNAL` = 424 (the post-5.1 unified syscall table, identical on amd64 and arm64);
  - `PIDFD_SIGNAL_PROCESS_GROUP` = `1 << 2`;
  - `P_PIDFD` = 3;
  - `CLD_EXITED` = 1, `CLD_KILLED` = 2, `CLD_DUMPED` = 3, `CLD_TRAPPED` = 4, `CLD_STOPPED` = 5.

  Declare these locally beside `_SigInfo_Waitid_`, keeping the codebase's raw `_SYSCALL.Syscall6` style. `creack/pty` leaves `go.mod`.

---

## 3. Architecture

### A. PTY Descriptor (replaces `creack/pty`)
```go
fileDescriptor_master, _ := _SYSCALL.Open("/dev/ptmx", _SYSCALL.O_RDWR|_SYSCALL.O_NOCTTY|_SYSCALL.O_CLOEXEC|_SYSCALL.O_NONBLOCK, 0)
ioctl(fileDescriptor_master, _SYSCALL.TIOCSPTLCK, &zero)       // unlockpt
ioctl(fileDescriptor_master, _SYSCALL.TIOCSWINSZ, &winsize)    // size before the shell starts
fileDescriptor_slave := ioctl(fileDescriptor_master, TIOCGPTPEER__Linux, O_RDWR|O_NOCTTY|O_CLOEXEC) // blocking slave for the child
FileDescriptor_Master__Pty__shared := _OS.NewFile(uintptr(fileDescriptor_master), "/dev/ptmx")
```
- The master is created non-blocking. `_OS.NewFile` therefore registers it with the netpoller and leaves `File.nonblock` false, so even a stray `Fd()` cannot flip it to blocking.
- `PtyResizer` issues `TIOCSWINSZ` through `SyscallConn().Control` instead of `_PTY.Setsize`. The reference count Go holds during `Control` also prevents a resize that races the master's close from hitting a reused fd number.
- No termios changes — same as `creack/pty` (kernel defaults; the shell configures its own modes).

### B. Process Spawn (replaces `exec.Command` + `_PTY.StartWithSize`)

> **Spawn through the `suprasole-pty-exec` helper (interface and responsibilities — mechanism in `pidfd-refactor-spawn-hardening.md`).** The client-controlled setup steps do not run in the server. `Spawn_PtyProcess` execs a small, isolated local helper (code in `source/PtyExec/`, companion doc `documentation/PtyExec.md`), and the helper does `setsid` → `TIOCSCTTY` → `chdir` → reset signal dispositions → `execve` of the shell.
>
> **Why a helper.** A `chdir` into, or `execve` of, a client-chosen path on a hung network filesystem can block the forking thread and freeze the whole server (`pidfd-refactor-spawn-hardening.md` §1). Running those steps in a separate process with no Go runtime means a dead client filesystem hangs only the helper and the spawning goroutine, never the server's runtime.
>
> **The helper's contract — all the spawn depends on:**
> - **In:** the command (`Path_Executable`, `Argv`, `Path_Directory`, `Environment`), to `execve` exactly; the slave on fds 0–2; a close-on-exec report pipe on fd 3.
> - **Out:** the exact failed `{step, errno}` over fd 3 — so a bad `Dir` is `chdir <dir>: …`, not `fork/exec <shell>: …` — or, on success, fd 3 closes silently at `execve` (close-on-exec).
> - **Collection:** through its pidfd, like any `_PtyProcess_`; no OS thread is ever held.
>
> **Deferred — spawn hardening** (`pidfd-refactor-spawn-hardening.md`, its own chunk of work after the pre-phase-3 phases). Replacing the remaining `ForkExec` vfork with a real fork whose copy execs the pinned helper at once (so no vfork window can make the server wait on a client path), ending a hung helper, the `chdir`-failure policy (refuse), the two-tier `E2BIG` pre-check, and pinning the helper by descriptor all live there. None of it changes the contract above or the process lifecycle below. **That document is strictly out of scope for pre-phase-3 — do not read it into this plan until phases 1–3 are done.**
>
> The sketch below shows the server-side pidfd / worker core, with the shell exec'd directly — the first phase-2 shape; the notes after it record how the committed code routes that `execve` *through* the helper.

```go
path_ShellBinary, error_lookPath := _EXEC.LookPath(api.Path_ShellBinary__PtyCommand) // same resolution exec.Command uses today
fileDescriptor_pidfd := -1
pid, error_forkExec := _SYSCALL.ForkExec(path_ShellBinary, []string{path_ShellBinary}, &_SYSCALL.ProcAttr{
    Dir:   api.DirectoryPath_PtyCommand,
    Env:   api.EnvironmentVariables_PtyCommand,
    Files: []uintptr{fileDescriptor_slave, fileDescriptor_slave, fileDescriptor_slave},
    Sys:   &_SYSCALL.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0, PidFD: &fileDescriptor_pidfd},
})
_ = _SYSCALL.Close(fileDescriptor_slave)              // always — success or failure (§5B.5)
_ = _SYSCALL.SetNonblock(fileDescriptor_pidfd, true)  // so NewFile registers it with the netpoller
ptyProcess := &_PtyProcess_{
    Channel_FinalState: make(chan struct{}),
    State_state: &_Alive__State_PtyProcess_{
        FileDescriptor_Pidfd: _OS.NewFile(uintptr(fileDescriptor_pidfd), "pidfd"),
        Pid:                  pid, // valid exactly while unreaped — used only to observe /proc (§3F), never to signal
    },
}
go ptyProcess.RunWorker() // here, not in the coordinator — see §4D.1
```
- On `ForkExec` failure: close the master and return the error (→ `FAILURE__Status_SpawnPty`); `PidFD` stays `-1` (§6-N).
- `PidFD == -1` after a successful fork happens only on a kernel that ignores `CLONE_PIDFD`, i.e. before 5.2; a full descriptor table fails the fork itself (§6-AM). That is below the floor, which the launcher guarantees (§5A), so suprasole does not handle it — §5A states what happens if the check is skipped.
- **Implemented (phase 2)** in `Spawn_PtyProxy` and `Spawn_PtyProcess` (`source/PtyProcess.go`). Differences from the sketch above:
  - **The command is a `_Command_PtyProcess_`** (`Path_Executable`, `Argv`, `Path_Directory`, `Environment`), passed through the helper to `execve` exactly. `Prepare__Command_PtyProcess` builds it (the first call in `Spawn_PtyProxy`), with two policies chosen 2026-10-10:
    - **Shell name:** `resolve__Path_ShellBinary` looks up a bare name (no `/`) in suprasole's `PATH` with `exec.LookPath`, the rule `exec.Command` applied; a name with a slash is used as given. `argv[0]` is the name as the client sent it, not the resolved path.
    - **Environment:** passed exactly as the client sent it, duplicates included, so the shell receives precisely what was requested.
  - **`Spawn_PtyProcess(ptyCommand, slave)` builds the whole `ProcAttr`:** `Dir` and `Env` from the command description, the terminal's fields (`Files`, `Setsid` / `Setctty` / `Ctty`), and `PidFD`, all in one literal (the former `AttachSlave_PtyDescriptor` is inlined). `Spawn_PtyProxy` keeps what is its own: describing the command (checking `Err` before any PTY exists), opening the PTY, and closing the slave copy it opened.
  - **A failed `ForkExec` is wrapped as `*os.PathError{Op: "fork/exec"}`** — the same error `exec` returned, so `FAILURE__Status_SpawnPty` reads as before.
  - **A NUL byte is refused before anything starts,** in `Prepare__Command_PtyProcess` (`check__NUL_PtyCommand`, run before the shell-name lookup; `Spawn_PtyProxy` calls the constructor before opening the PTY). The error names the field: shell path → `fork/exec <path>` / `EINVAL`; directory → `chdir <dir>` / `EINVAL`; an environment entry → the message `exec.Cmd.Start` gave ("exec: environment variable contains NUL", Go #56284), so clients see the same text as before. `Spawn_PtyProcess` takes NUL-free strings as a precondition; a NUL that still reached it would fail loudly in `ForkExec` (`EINVAL`).
  - **`Spawn_PtyProxy` starts `RunWorker`** right after `Spawn_PtyProcess` succeeds — the constructor builds, the orchestrator starts workers (and never the coordinator, whose cancelled branch would leave the process uncollected, §4D.1). Starting after the fork loses nothing: the zombie waits for its parent, and `RawConn.Read` peeks before it ever waits.
  - **A spawn whose pidfd the worker could never watch is abandoned, at spawn time.** Right after `ForkExec`, `Spawn_PtyProcess` sets `O_NONBLOCK`, wraps the pidfd with `NewFile`, and checks pollability with `SetReadDeadline(time.Time{})` — Go documents that `NewFile` "will attempt to return a pollable file" and that "only pollable files support" `SetReadDeadline`, so it fails (`os.ErrNoDeadline`) exactly when the pidfd cannot be parked in the netpoller. Its one real cause is the netpoller refusing to register it (`epoll_ctl`: `ENOMEM`, or `ENOSPC` from the per-user `fs.epoll.max_user_watches`); the spawn then fails with that error, and the process is never handed to a worker that could only lose it.
    - **Setting `O_NONBLOCK` cannot fail, so its error is asserted:** the success and netpoll branches both require it nil, and the final `else` lists it and panics. Derivation (Go 1.27.1, kernel v6.9 source), under §5A.1 and §5A.2:
      - `syscall.SetNonblock` is `fcntl(F_GETFL)`, then — unless already set — `fcntl(F_SETFL, flags | O_NONBLOCK)`.
      - `fcntl` returns `EBADF` only for a missing descriptor: excluded, since the pidfd is valid (the floor) and known only to `Spawn_PtyProcess` until `NewFile` (the child copied its descriptor table before the kernel installed the pidfd in ours).
      - `F_GETFL` returns `f_flags` — exactly `O_RDWR` for a process pidfd (`__pidfd_prepare` → `pidfs_alloc_file(pid, O_RDWR)` → `dentry_open`), never in Go's errno range.
      - `setfl` fails only on a refused change of `O_APPEND`, `O_NOATIME` or `O_DIRECT`, or through `f_op->check_flags` / `f_op->fasync`. Only `O_NONBLOCK` changes, and `pidfs_file_operations` has only `.poll` and `.show_fdinfo`.
      - The in-tree `file_fcntl` security hooks all pass: SELinux checks only `FD__USE`, skipped because the file carries suprasole's own SID (allocated by its thread during `clone`); Smack returns 0 for the `S_PRIVATE` pidfs inode; TOMOYO acts only on an `O_APPEND` change; AppArmor and Landlock have no such hook.
      - Neither command can sleep, so there is no `EINTR`.
    - **Abandoning, inline in `Spawn_PtyProcess`:** group `SIGKILL` through the pidfd (`sendSignal_Pidfd` with `PIDFD_SIGNAL_PROCESS_GROUP`), then Spawn starts `reapAbandoned__PtyProcess_Pidfd`, a goroutine that collects it with `waitid(P_PIDFD, WEXITED|WNOHANG)`, sleeping `INTERVAL_REAP_ABANDONED__PtyProcess` between empty answers, then `File.Close()`. It works on the `*os.File`, not the raw number: once `NewFile` owns the descriptor, closing the bare number would let the file's finalizer later close a reused one. Every access goes through `Control`; a `Control` failure (only a nil or closing file — impossible, since this goroutine alone owns and closes it) is asserted in the final `else`. The kill's own `errno` is ignored deliberately: whether or not it lands (e.g. `EPERM` after a credential change), the reaper collects the process whenever it terminates.
    - It has no trap branch: kernel source rules a trap report out here (§6-AN); the final `else` lists the case and panics.
    - **No OS thread is ever held:** it never blocks in the kernel — the netpoller refused the pidfd, so it cannot be waited on there, and a blocking `waitid` would pin a thread for as long as a `D`-state or debugger-held process survives. A sleeping goroutine holds only a runtime timer. Cost: one goroutine waking once per interval while the killed process is not yet collectable — the same class as `Zombie_Withheld`'s re-peek, but with its own constant, `INTERVAL_REAP_ABANDONED__PtyProcess` (1 s), because it waits for a different thing: a process we just killed finishing its death (microseconds), not a debugger releasing a zombie (human time). Nobody waits on the abandon reaper, so a long interval costs nothing and minimizes wake-ups if the killed process cannot die.
    - **Iteration algebra** (bottom-up, from code facts and documented semantics — not from sampling):
      - *Givens.* G1: the first `waitid` runs strictly after the group `SIGKILL` returned (code). G2: suprasole issues no ptrace request anywhere (0 uses in `source/`), so if it traces the child C, C called `PTRACE_TRACEME` itself: options 0, never resumed. G3: nobody else collects C (§5B.2). G4: `wait(2)` — `WNOHANG` with nothing to report gives `si_pid == 0`; without `WNOWAIT` a returned report is consumed; `EINTR` only "if `WNOHANG` was not set". G5: `ptrace(2)` "Death under ptrace" — `SIGKILL` causes no signal-delivery-stop and no syscall-exit-stop; its only possible stop, the exit stop, needs `PTRACE_O_TRACEEXIT`, settable only by a tracer. G6: a ptrace-stopped tracee stays stopped until its tracer resumes it, it is killed, or the tracer detaches. G7: a traced child's stops go only to its tracer; a foreign-traced zombie reaches the real parent only after that tracer collects or detaches.
      - *Outcome space of one call:* an error / 0 — and for 0: `si_pid == 0`, or `si_pid != 0` with `CLD_TRAPPED`, or `si_pid != 0` with a termination code — `CLD_EXITED`, `CLD_KILLED` or `CLD_DUMPED`, named positively in the condition. `CLD_STOPPED` and `CLD_CONTINUED` cannot appear (they need `WSTOPPED` / `WCONTINUED`, and a traced child's stops arrive as `CLD_TRAPPED`); they are the final `else`'s remainder. All three terminations are possible here: `CLD_KILLED` from our `SIGKILL`, `CLD_EXITED` or `CLD_DUMPED` if the program exited or crashed before the kill landed — never `CLD_DUMPED` from our own kill, whose action is terminate-without-core (`signal(7)`). The branches partition it, so the final `else` is unreachable by construction.
      - *`EINTR`: impossible* — G4; the call always passes `WNOHANG`. So it has no branch: it was removed from this loop, from `peek__TerminationReport_PtyProcess` and from `collect__PtyProcess` (now a single call). *Error: 0* under G3 (valid flags and pidfd leave only `ECHILD`, i.e. someone else collected C). So the loop has no error branch: the real kernel case is documented here instead, and an error falls into the final `else`, which lists it and panics — a violation of G3 is caught rather than assumed away.
      - *`CLD_TRAPPED`: ≤ 1 by documentation.* Only suprasole-as-tracer stops reach the call (G7); after the kill no new stop can arise (G5 with G2); before it, C is in at most one stop and cannot leave it (G6, G2); a returned report is consumed (G4). Whether a report pending at the kill is still reportable afterwards, the documentation does not say. Kernel source does — verified at v6.9, designed since 5.19 (§6-AN): the fatal wake inside `kill` clears `JOBCTL_TRACED` unless `JOBCTL_PTRACE_FROZEN`, which only the tracer's own `ptrace(2)` requests set (G2: none); `wait` reports a ptrace stop only while `JOBCTL_TRACED` is set; and a task with a fatal signal pending enters no new stop. So: ≤ 1 by documentation, 0 by design. So the loop has no trap branch: the case the man pages allow is documented here instead, and the final `else` lists it and panics.
      - *Nothing yet:* with t₀ the first call and t_z the moment C becomes a zombie reportable to suprasole, the count is 0 if t_z ≤ t₀, else 1 + ⌊(t_z − t₀) / `INTERVAL_REAP_ABANDONED__PtyProcess`⌋. A `SIGKILL` death takes microseconds, so normally 0 or 1; infinite only if t_z = ∞ (uninterruptible sleep that never ends, or a foreign tracer holding C forever).
      - *Terminated: exactly 1* whenever t_z is finite — that call collects C and ends the loop.
      - *Total:* nothing-yet + 1 — normally 1–2; unbounded only through the sleep branch, as a goroutine on a timer.

### C. `_PtyProcess_` Worker — Observe, Collect, Transition
Two `waitid` calls, each with one job:
- **The peek** (`WEXITED|WNOHANG|WNOWAIT`) observes termination without collecting, so the kernel's zombie becomes suprasole's `Zombie`.
- **The reap** (`WEXITED|WNOHANG`) is the only call in suprasole that collects a process.

One readiness check separates "not terminated" from "terminated but withheld by a tracer".
```go
func (this *_PtyProcess_) RunWorker() {
    State_alive := this.State_state.(*_Alive__State_PtyProcess_) // RunWorker is the only writer of State_state
    rawConn, _ := State_alive.FileDescriptor_Pidfd.SyscallConn()
    var status_TerminationReport _Status_TerminationReport__PtyProcess_ // REPORTED | WITHHELD | FAILED: what the waking peek showed
    var sigInfo_reported _SigInfo_Waitid_
    var error_watch error
    error_read := rawConn.Read(func(fd uintptr) bool {
        for {
            sigInfo_peek, errorCode_peek := waitid__PtyProcess(fd, _SYSCALL.WEXITED|_SYSCALL.WNOHANG|_SYSCALL.WNOWAIT) // observe, never collect
            switch {
            case _SYSCALL.EINTR == errorCode_peek:
                continue
            case errorCode_peek != 0: // ECHILD ⇒ someone else reaped (§5B.2)
                status_TerminationReport, error_watch = FAILED__Status_TerminationReport__PtyProcess, errorCode_peek
                return true
            case 0 == sigInfo_peek.Pid && readable__Pidfd(fd):
                // Terminated (pidfd_open(2): readable once a zombie), yet nothing reportable to its real parent:
                // a tracer withholds it ("a zombie ptracee is only visible to its ptracer" — §5C). Leave Read.
                status_TerminationReport = WITHHELD__Status_TerminationReport__PtyProcess
                return true
            case 0 == sigInfo_peek.Pid:
                return false // not terminated (running, stopped, D, exit stop alike) and the pidfd is not ready → park until POLLIN
            case CLD_TRAPPED__SIGCHLD__Linux == sigInfo_peek.Code:
                // We are its tracer (§1.4). WNOWAIT leaves the trap report pending, so consume it with a WSTOPPED-only
                // call, which cannot collect (waitid(2): only WEXITED reports terminations). Its result is not relied on (§5C).
                _, _ = waitid__PtyProcess(fd, _SYSCALL.WSTOPPED|_SYSCALL.WNOHANG)
                continue
            default:
                status_TerminationReport, sigInfo_reported = REPORTED__Status_TerminationReport__PtyProcess, sigInfo_peek
                return true
            }
        }
    })
    if error_read != nil { // poller failure; the pidfd is closed only below
        status_TerminationReport, error_watch = FAILED__Status_TerminationReport__PtyProcess, error_read
    }
    if WITHHELD__Status_TerminationReport__PtyProcess == status_TerminationReport {
        this.Transition(&_Zombie_Withheld__State_PtyProcess_{FileDescriptor_Pidfd: State_alive.FileDescriptor_Pidfd, Pid: State_alive.Pid})
        // The pidfd is already readable, so it offers nothing further to wait on: re-peek on an interval
        // until the tracer collects its report or detaches (§6-AE). Only a debugger can bring us here.
        status_TerminationReport, sigInfo_reported, error_watch = repeek__TerminationReport_PtyProcess(rawConn, INTERVAL_REPEEK__PtyProcess) // REPORTED or FAILED
    }
    switch status_TerminationReport {
    case REPORTED__Status_TerminationReport__PtyProcess:
        termination := decode__Termination_PtyProcess(sigInfo_reported) // CLD_EXITED | CLD_KILLED | CLD_DUMPED; anything else: panic("invalid path")
        this.Transition(&_Zombie__State_PtyProcess_{FileDescriptor_Pidfd: State_alive.FileDescriptor_Pidfd, Pid: State_alive.Pid, Termination: termination})
        this.Mutex.Lock() // reap and final write are one critical section: while State_state is non-final, the PID is still ours (§3F)
        _ = rawConn.Control(func(fd uintptr) {
            this.State_state = collect__PtyProcess(fd, termination) // the only collecting waitid (EINTR retried): Reaped{termination}, or SystemError
        })
        this.Mutex.Unlock()
    case FAILED__Status_TerminationReport__PtyProcess:
        this.Transition(&_SystemError__State_PtyProcess_{SystemError_Waitid: error_watch})
    }
    _ = State_alive.FileDescriptor_Pidfd.Close() // the handle ends with the last state that carries it
    close(this.Channel_FinalState)               // broadcast, strictly after the final state is written
}
```
- `Transition` is `Mutex.Lock(); State_state = next; Mutex.Unlock()`. *Implemented (phase 2) without the helper:* `RunWorker` writes each transition inline under the Mutex, so no method takes a lock out of its caller's sight.
- **The reap runs inside the Mutex.** The kernel releases the PID at the reap. Holding the Mutex across it means anyone who reads a non-final state (`Alive` / `Zombie_Withheld` / `Zombie`) under the Mutex, and stays inside it, can trust the PID still names our child. That is what lets the teardown observe `/proc/<pid>` safely (§3F).
- **Contract-compliant by construction.** `syscall.RawConn.Read` documents that `f` "is expected to try to read" and that `Read` blocks only when `f` returns `false`. So the callback returns `false` **only when the pidfd is not readable**. The design therefore never depends on the Go netpoller's trigger mode (edge-triggered `EPOLLET` today — an implementation detail, §5C).
- **`Zombie_Withheld` is detected deterministically,** from two designed behaviors (§5C):
  - the pidfd is readable — documented as "terminated and became a zombie";
  - the peek reports nothing — a zombie ptracee is visible only to its tracer.

  The re-peek interval (`INTERVAL_REPEEK__PtyProcess`, e.g. 1 s) is the worker's only timer. It runs only while a debugger withholds the process. (The abandon reaper, §3B, has its own: `INTERVAL_REAP_ABANDONED__PtyProcess`.)
- `CLD_STOPPED` and `CLD_CONTINUED` cannot appear: the peek never passes `WUNTRACED` / `WCONTINUED`, and untraced group-stops are invisible to it (§6-P, §6-R).
- **Implemented (phase 2)** in `source/PtyProcess-THREAD.go`. The `Read` callback is one line: it delegates to `peek__TerminationReport_PtyProcess`, which returns a `_Status_TerminationReport__PtyProcess_`: `ABSENT` (nothing has terminated — the only result that parks), `REPORTED`, `WITHHELD` (a tracer holds the report) or `FAILED`. The re-peek, `repeek__TerminationReport_PtyProcess`, reuses the same function under `Control`. `poll__Pidfd` returns the `POLLIN` bit as `_Status_PollIn__Pidfd_` (`SET` / `CLEAR`) and retries `EINTR` (`poll(2)` does not exempt a zero timeout), while the `waitid` calls carry no `EINTR` branch — every one passes `WNOHANG`, and `wait(2)` raises `EINTR` only without it. They have no `ECHILD` branch either — a real kernel case unreachable while §5B.2 holds, documented here and in the companion doc: an `ECHILD` falls into the chain's final `else`, which lists it and panics; a pidfd is always pollable and never `read(2)`-able (`EINVAL`), so the status names the bit, not "pollability" or "read status".
- **Accepted cost** *(decided)*: a process that survives `SIGKILL` (`D` state) never terminates, so its worker stays parked and its state stays — truthfully — `Alive`. The cost is one goroutine (≥ 2 KiB stack), one `*_OS.File` + poll descriptor and one fd: zero OS threads, zero CPU, zero timers.

### D. Signals — All Through the pidfd
- **Process group, briefly.** `Setsid` puts the PTY process in a new process group whose ID equals its PID, and makes it the group's leader for life. Processes it runs in the foreground without job control (`bash -c 'a | b'`, a program's own worker children) inherit that group. Job-control jobs (`cmd &` in an interactive shell) get groups of their own, so they are not members.
- **`Signal(signal, PIDFD_SIGNAL_THREAD_GROUP)`** — the PTY process alone: `pidfd_send_signal(fd, signal, NULL, PIDFD_SIGNAL_THREAD_GROUP)` — explicit, rather than flags `0`, which makes the kernel infer the same scope from the pidfd's type.
- **`Signal(signal, PIDFD_SIGNAL_PROCESS_GROUP)`** — the whole group: the same call with `PIDFD_SIGNAL_PROCESS_GROUP`.
- **Both dispatch on the state.** Read `State_state` under `Mutex`:
  - **`Alive`** — send through its pidfd with `SyscallConn().Control`.
  - **`Zombie_Withheld` / `Zombie`** — same handle. The kernel accepts a signal to a terminated process and does nothing with it (§6-S). A group signal still reaches live members of the group. **A successful `Signal` is therefore not evidence the process is alive** — only `State_state` is.
  - **`Reaped` / `SystemError`** — no handle, nothing to signal; "signal a reaped process" cannot even be expressed.
  - If the worker closes the pidfd between the read and the send, `Control` fails — with poll's "use of closed file", which `os` translates to `os.ErrClosed` only for its own I/O methods; the value is undocumented, so any `Control` failure is read as "already reaped" (§6-AJ). Its reference count guarantees a signal never goes to a reused fd number.
  - **Implemented (phase 2):** each state implements `Signal(signalNumber_PtyProcess, flags__PIDFD_SEND_SIGNAL)`, where the caller passes the kernel's own scope flag — `PIDFD_SIGNAL_THREAD_GROUP` or `PIDFD_SIGNAL_PROCESS_GROUP` (both Linux 6.9) — straight through; there is no enum of our own to translate (the kernel rejects unknown bits or two scope flags with `EINVAL`).
    - **Why each state answers for itself:** only `Alive`, `Zombie_Withheld` and `Zombie` hold a pidfd. `Reaped` and `SystemError` have none (invariant 2, §4B), so there is no syscall to make: `Reaped` returns `os.ErrProcessDone`, `SystemError` its own error. Had the handle been kept, the kernel would answer `ESRCH` after the reap — but after a poller failure the process may still be alive, and the signal would land.
    - **The caller holds the Mutex,** in its own body: `Mutex.Lock()` → `State_state.Signal(…)` → `Mutex.Unlock()`. `_PtyProcess_` has no method that locks internally, so no lock is taken out of the caller's sight and none can nest.
    - **Sending under the Mutex closes the race in the previous bullet:** the worker closes the pidfd only after writing a final state under that same Mutex. A `Control` failure is still returned rather than assumed away.
- **Why PID recycling is not a concern:** both calls name the kernel's process object, never a number. Even if the reap lands between the state read and the send, `Signal(…, PIDFD_SIGNAL_THREAD_GROUP)` gets `ESRCH`, and `Signal(…, PIDFD_SIGNAL_PROCESS_GROUP)` reaches only members left in that same group (`ESRCH` once it is empty — §6-O). The Mutex guards only the state field; it plays no part in signal safety.
- **Termination:** `Signal(SIGHUP, PIDFD_SIGNAL_THREAD_GROUP)` then `Signal(SIGCONT, PIDFD_SIGNAL_THREAD_GROUP)` — exactly what the kernel sends a session leader on hangup.
  - Interactive bash ignores `SIGTERM` but forwards `SIGHUP` to its jobs (§6-G).
  - A stopped shell needs the `SIGCONT` (§6-H).
- **Escalation:** `Signal(SIGKILL, PIDFD_SIGNAL_PROCESS_GROUP)` — the leader plus its foreground group, matching what the current code does with `kill(-pid)`. §6-M shows why the group matters: a HUP-ignoring foreground child died with the shell.

### E. `_PtyTeardown_` — Ending the Session *(adopted)*
Ending a session joins:
- two independent kernel event streams that arrive in either order (§6-W);
- two commands;
- three timers;
- one destructive act (closing the master);
- one output, emitted exactly once.

One actor per PTY owns all of it. It replaces the dispatcher's drain arm, `HandleExited`'s teardown, the free `go Terminate_PtyProxy()` goroutine and `TerminateCancelled_PtyProxy`.

**Why a separate actor from `_PtyProcess_`:** the two have different lifetimes.
- `_PtyProcess_` mirrors the *process*. It must hold nothing proxy-scoped (§4C), because it outlives the proxy for unkillable or debugger-held processes.
- `_PtyTeardown_` ends the *session*. It needs the master and the outcome callback, and it must finish even when `_PtyProcess_` never does.

```go
type _PtyTeardown_ struct {
    QueueChannel__Order_PtyTeardown    chan _Order_PtyTeardown_ // Terminate, Cancel, ReaderFinished{ExitSignal}
    Channel_FinalState                 <-chan struct{}          // _PtyProcess_'s broadcast; nil-ed once received
    PtyProcess                         *_PtyProcess_
    FileDescriptor_Master__Pty__shared *_OS.File                // _PtyTeardown_ is the only code that closes the master
    OnExitOutcome__                    func(id_WorkspacePty uint32, exitOutcome _ExitOutcome_PtyProxy_)
    Timer_Drain                        *_TIME.Timer             // TIMEOUT_DRAIN__PtyProxy
    Timer_Unresponsive                 *_TIME.Timer             // TIMEOUT_UNRESPONSIVE__PtyProcess
    Timer_Uninterruptible              *_TIME.Timer             // TIMEOUT_UNINTERRUPTIBLE__PtyProcess
    // plus its own state: which of the two sides has finished, whether it is escalating, the latest observation (§3F)
}
```

| Input | From | Meaning |
|---|---|---|
| `_Terminate__Order_PtyTeardown_` | `HandleTerminatePty__Coordinator` | the user asked to end the session |
| `_Cancel__Order_PtyTeardown_` | the cancelled branch of `HandleStatus_SpawnPty__Success__Coordinator` | no reader was ever started; end silently |
| `_ReaderFinished__Order_PtyTeardown_{ExitSignal}` | PtyDispatcher, **after its final flush** | the TTY side has ended: `EIO`, `os.ErrClosed`, or another read error |
| `<-Channel_FinalState` | `_PtyProcess_` | the process side has ended: `Reaped` or `SystemError` |
| the three timers | itself | drain, unresponsive, uninterruptible |

| Situation | Action |
|---|---|
| Terminate, process not final | `Signal(SIGHUP, PIDFD_SIGNAL_THREAD_GROUP)` + `Signal(SIGCONT, PIDFD_SIGNAL_THREAD_GROUP)`; arm Unresponsive |
| Process final, reader not finished | arm Drain — `EIO` may never come (§9D) |
| Drain fires | close the master (forced unplug) → the reader finishes with `os.ErrClosed` |
| Reader finished | close the master if still open — this is what makes `EIO` final (§6-Z). If the process isn't final, arm Unresponsive (the rogue path, §9E). |
| Unresponsive fires | close the master if still open; **observe** (§3F); `Signal(SIGKILL, PIDFD_SIGNAL_PROCESS_GROUP)`; arm Uninterruptible |
| Uninterruptible fires | **observe again** (§3F); give up on the process side — `_PtyProcess_` stays parked (accepted cost) |
| Reader finished **and** (process final **or** given up) | emit the outcome once (§3F), stop the timers, return |
| Cancel | close the master; `Signal(SIGHUP, PIDFD_SIGNAL_THREAD_GROUP)` + `Signal(SIGCONT, PIDFD_SIGNAL_THREAD_GROUP)`; count the reader as finished; arm Unresponsive. On final or give-up, return **without** an outcome. |
| Terminate after escalation began, or after the outcome | ignored — idempotent |

Properties:
- **Every master close happens here.** The dispatcher and the coordinator never close it.
- **Order-independent by construction:** the two facts are recorded as they arrive, and the outcome waits for both (§9E).
- **It knows why the master closed,** so an unresponsive shell is waited on once, not twice. Worst case drops from ≈ 3 s to ≈ 2 s (§5B.6).
- **Bounded, proxy-scoped lifetime.** After the master closes, the reader finishes within milliseconds (§6-B), and the process side is bounded by Unresponsive + Uninterruptible.
- **Started in `Spawn_PtyProxy`,** alongside `_PtyProcess_.RunWorker`, so cancelled spawns go through the same machine.
- **PtyDispatcher becomes a pure data plane:** `_ExitSignal__Order_PtyDispatcher_.Execute` flushes, then sends `_ReaderFinished__Order_PtyTeardown_`. The dispatcher has no master close, no drain arm and no `HandleExited`.
- Deleted along the way: `ticker_probe`, `probeExited__PtyProcess`, `probeBufferDrained__MasterPty`. The kernel delivers every pending byte before `EIO` (§6-E), so `EIO` itself is the drain signal.
- **The forced unplug is itself a kernel hangup.** A still-living session leader receives `SIGHUP` + `SIGCONT`, and PtyReader returns `os.ErrClosed` in under 1 ms (§6-B). Both exit paths need it: even after a successful Terminate, a `setsid`'d process that kept the slave and a disowned job survive and hold it, so `EIO` never comes (§6-U).

### F. Teardown Observation & Outcome
**Observation** (when Unresponsive or Uninterruptible fires): read `State_state` under `Mutex`.
- **`Zombie`, `Reaped` or `SystemError`** — it terminated in the window; `Channel_FinalState` closes within microseconds.
- **`Alive` or `Zombie_Withheld`** — still inside the Mutex, so the PID is still ours (§3C). **Observe the kernel's own view** in `/proc/<Pid>/stat` (state letter) and `/proc/<Pid>/status` (`TracerPid`). Both fields are documented in `proc(5)`.

  | State letter | `TracerPid` | Observation (teardown report) |
  |---|---|---|
  | `R` / `S` | 0 | `_Unresponsive_Rogue_` — running or sleeping; it ignored the hangup |
  | `T` | 0 | `_Unresponsive_Stopped_` — group-stopped |
  | `t` | **one of suprasole's own threads** (`/proc/self/task/<TracerPid>` exists) | `_Unresponsive_Stopped_` — trace-stopped under its own `PTRACE_TRACEME`. `TracerPid` names the tracer *thread* — the OS thread that forked, which in Go need not be the main thread — so comparing it with suprasole's PID would misfile this as a foreign debugger (§6-AI). |
  | `t` or `Z` | not one of suprasole's threads | **`_Unresponsive_Debugged_{TracerPid}`** (new) — a debugger holds it: in a stop, an exit stop, or (state `Zombie_Withheld`) a terminated zombie. Suprasole can neither finish nor collect it (§6-V, §6-AA–AE). |
  | `D` | — | `_Unresponsive_Uninterruptible_` — **observed** directly, no longer inferred from a failed `SIGKILL` |

  These are observations at one instant, so they live in the teardown report and never in `State_state` (§4B).

  Why not `waitid` as today: a `WSTOPPED|WNOWAIT` probe is blind to `D` and to every debugger-held condition, since stops are reported to the tracer, not the parent. Today's probe therefore labels a debugger-held shell `Rogue`. After the futile `SIGKILL` it ends up reported as `Uninterruptible`, which is wrong both times.

**Outcome** (emitted once by `_PtyTeardown_`):
- **Reader finished with an error other than `EIO` / `os.ErrClosed`** → `_SystemError_Descriptor__ExitOutcome_PtyProxy_`, as today.
- **Otherwise, the process side.** The final state **is** the teardown report: `_Reaped__State_PtyProcess_` (through its `Termination`) and `_SystemError__State_PtyProcess_` implement `_TeardownReport_PtyProcess_`. If the process side was given up, the latest observation is the report.

  | Process side | `_ExitOutcome_PtyProxy_` |
  |---|---|
  | `Reaped{_Exited__Termination_PtyProcess_{0}}` | `Success` |
  | `Reaped{_Exited__Termination_PtyProcess_{n}}` | `Failure{n}` |
  | `Reaped{_Signaled__Termination_PtyProcess_{sig, any core-dump status}}` | `Killed{sig}` |
  | `SystemError{err}` | `SystemError_Process{err}` |
  | given up: `_Unresponsive_*` observation | its `Resolve__ExitOutcome_PtyProxy()`, as today |
- `_ExitOutcome_PtyProxy_` variants and their egress encoding are unchanged.

### G. Refactor Map — Today's Code → Proposed
This proposal is a refactor of existing code. Every row names something that exists in `source/` today.

| Today | Where | Proposed |
|---|---|---|
| `_PtyProxy_.PtyProcess *_OS.Process` | `PtyProxy.go` | `*_PtyProcess_` (§4) |
| `Spawn_PtyProxy`: `exec.Command` + `_PTY.StartWithSize` | `LifecycleCoordinator_WorkspacePty-THREAD.go` | Native PTY + `syscall.ForkExec(PidFD)`; starts `RunWorker` (§3A–B) |
| `(*_PtyProxy_).Resize`: `_PTY.Setsize` | `PtyResizer-THREAD.go` | `TIOCSWINSZ` via `SyscallConn().Control` (§3A) |
| `spawnReaper_PtyProcess` — a goroutine blocked in `os.Process.Wait()`, sending… | `PtyProcess.go` | `RunWorker` — parked on the pidfd, driving `Alive → (Zombie_Withheld →) Zombie → Reaped` (§3C) |
| …`_ReapResult_PtyProcess_{ProcessState_maybe, Error_Wait_maybe}` | `PtyDispatcher-THREAD.go` | **Deleted.** Its two nil-able fields become states: the `Termination` carried by `Zombie` / `Reaped`, and `SystemError`'s error (§4B). |
| `_Reaped__TeardownReport_PtyProcess_{ProcessState}` + its `Resolve__ExitOutcome_PtyProxy` | `PtyDispatcher-THREAD.go` | **Deleted.** `_Reaped__State_PtyProcess_` (via its `Termination`) and `_SystemError__State_PtyProcess_` implement `Resolve__ExitOutcome_PtyProxy` themselves; the final state *is* the teardown report (§3F). |
| `__selectReapResult__PtyProcess`, `__kill__PtyProcess_Unreaped`, `selectReapResult_Exited__PtyProcess`, `killExited__PtyProcess_Unreaped`, `selectReapResult_Cancelled__PtyProcess`, `killCancelled__PtyProcess_Unreaped` | `PtyProcess.go`, `PtyDispatcher-THREAD.go`, `LifecycleCoordinator_WorkspacePty-THREAD.go` | **Deleted.** Replaced by `_PtyTeardown_`'s event loop (§3E). |
| `_Stopped_Traced__TeardownReport_PtyProcess_` | `PtyDispatcher-THREAD.go` | **Deleted.** It came from `Wait()` returning trace-stops. The worker now consumes those, and a traced shell is reported by the probe as `_Unresponsive_Stopped_` (§3F). |
| `_Unresponsive_{Stopped,Rogue,Uninterruptible}__TeardownReport_PtyProcess_` | `PtyDispatcher-THREAD.go` | **Kept** — observations, not states (§4B). `Uninterruptible` is now observed (`D`), not inferred. **Added:** `_Unresponsive_Debugged__TeardownReport_PtyProcess_{TracerPid}`. |
| `_SystemError__TeardownReport_PtyProcess_` | `PtyDispatcher-THREAD.go` | **Kept** for probe failures other than `ECHILD`. Reap failures become `_SystemError__State_PtyProcess_`. |
| `probeStatus__PtyProcess_Unresponsive` (`waitid(P_PID)`) | `PtyDispatcher-THREAD.go` | Replaced by a `/proc/<Pid>` observation (state letter + `TracerPid`) inside the Mutex. A `waitid` probe is blind to `D` and to debugger-held processes (§3F). |
| `(*_PtyProxy_).HandleExited`, `TeardownExited_PtyProcess` | `PtyDispatcher-THREAD.go` | **Deleted.** Absorbed into `_PtyTeardown_` (§3E, §3F). |
| `_ExitSignal__Order_PtyDispatcher_.Execute` → `OnExited__` | `PtyDispatcher-THREAD.go`, `PtyDispatcher.go` | Final flush, then `_ReaderFinished__Order_PtyTeardown_`. The dispatcher no longer closes the master (§3E). |
| `Terminate_PtyProxy`, `TerminateCancelled_PtyProxy` | `LifecycleCoordinator_WorkspacePty-THREAD.go` | **Deleted.** `HandleTerminatePty__Coordinator` sends `_Terminate__Order_PtyTeardown_` instead of `go Terminate_PtyProxy()`. The cancelled spawn branch sends `_Cancel__Order_PtyTeardown_` (§3E). |
| `probeExited__PtyProcess`, `probeBufferDrained__MasterPty`, `ticker_probe` (uncommitted) | `LifecycleCoordinator_WorkspacePty-THREAD.go` | **Deleted** (§3E). |
| `P_PID__WAITID__Linux`, `_PTRACE_EVENT_EXIT` | `PtyDispatcher-THREAD.go` | **Deleted.** `_SigInfo_Waitid_` and `CLD_STOPPED__SIGCHLD__Linux` stay; pidfd / `CLD_*` constants are added (§2). |
| — | — | **New:** `_PtyTeardown_` in `PtyTeardown.go` / `PtyTeardown-THREAD.go`, started in `Spawn_PtyProxy` (§3E). |
| `creack/pty` | `go.mod` | Removed. |

### H. Unified Spawn Lifecycle — *draft 2026-10-10, not implemented*
**Goal:** one owner for a PTY's process from before its fork to its reap. Every ending goes through the same teardown (§3E) and the same collector, and every state is backed by a kernel fact. This replaces today's blocking exec verdict inside `Spawn`, which has its own wait, reaper and error channel separate from the process lifecycle (the split-brain found in the 2026-10-10 launch audit). It builds on the hybrid spawn (the spawn hardening — `pidfd-refactor-spawn-hardening.md`).

**Timeline**
1. **`Spawn_PtyProxy`, synchronous.** A failure here creates nothing:
   - `Prepare__Command_PtyProcess`: the NUL check, the two-tier `E2BIG` check against the launcher-probed limits (the spawn hardening — `pidfd-refactor-spawn-hardening.md`), and shell-name resolution.
   - `Open_PtyDescriptor`: master and slave.
   - `Spawn_PtyProcess`: a pure constructor with no syscalls, returning a `_PtyProcess_` in `Spawning`.
   - It builds the PtyProxy (reader, writer, dispatcher, terminal) and its `_PtyTeardown_`, then starts the workers, `RunWorker` included.
   - The pinned helper descriptor (the spawn hardening — `pidfd-refactor-spawn-hardening.md`) is opened once, at server startup, so it is never a per-spawn failure.
2. **`RunWorker` phase 1, spawning.** The worker is the only writer of the state:
   - **Report pipe:** create it and check that the netpoller accepted its read end. If not, the state becomes final `NeverStarted{SystemError}`. Go would otherwise silently fall back to a blocking read, which holds an OS thread for the whole launch.
   - **Fork** (the hybrid real fork, whose copy execs the pinned helper — `pidfd-refactor-spawn-hardening.md`). On error, the state becomes `NeverStarted{fork error}`.
   - **Publish:** close the server's copies of the slave and of the pipe's write end, and publish `Launching{pidfd, pid}`.
   - **Watchable pidfd:** check that the netpoller accepted the pidfd. If not: `SIGKILL` the process, collect it by polling, and end in `SystemError`, worded truthfully. The server terminated the process because it cannot observe it: its event loop refused the pidfd, with `ENOSPC` once the user's epoll registrations reach `fs.epoll.max_user_watches`, or `ENOMEM`. Go does not expose which of the two.
   - **Wait on the report pipe.** Watching the pipe alone is enough, because any death before the shell's exec closes it:
     - End-of-file while the pidfd shows the process alive → the exec is confirmed (only an exec or a death closes fd 3) → `Alive`.
     - End-of-file with the process already terminated → collect → `Reaped`; the outcome is "ended before exec was confirmed", plus the termination.
     - A `{step, errno}` report → collect → `Reaped`; the outcome is the setup failure: `chdir <dir>: …`, `fork/exec <shell>: …`, or a failure to exec the helper itself.
3. **`RunWorker` phase 2, spawned:** today's worker. `Alive → (Zombie_Withheld →) Zombie → Reaped`.

**New states**
- `Spawning`: no PID yet; holds what the fork needs (command, slave, helper descriptor).
- `Launching`: pidfd and PID; exec pending.
- `NeverStarted`: final; no PID ever existed.

**Failures land in exactly three places, matching reality**
- Before the constructor (steps 1–2 of the timeline): a synchronous spawn failure, with nothing to clean up.
- Phase 1 before the fork: `NeverStarted`.
- After the fork: the process lifecycle's outcome.

**Teardown integration (per PTY, §3E)**
- `_PtyTeardown_` exists before the fork, so a cancel in any phase has an owner.
- `Signal` works through the pidfd from `Launching` on.
- Escalation must `SIGKILL` the process itself as well as its group. The group exists only after the helper's `setsid`; a `PIDFD_SIGNAL_PROCESS_GROUP` before then reaches nobody (verified).
- **Open point, a cancel while `Spawning`:** teardown records it under the Mutex, and the worker checks it under the Mutex before forking (→ `NeverStarted{cancelled}`). A cancel that lands mid-fork takes effect once teardown observes `Launching`.

**What disappears**
- the blocking verdict wait inside the launch;
- `Spawn_PtyProcess`'s reaper start, and the netpoll abandon branch;
- `reapAbandoned__PtyProcess_Pidfd`, which becomes the worker's polling mode for an unwatchable pidfd;
- any separate timeout, cancel or kill code.

**Invariants**
- one writer: the worker;
- one collector: the worker;
- every transition backed by a kernel fact: the fork's result, end-of-file plus liveness, a report record, pidfd readability, `waitid`.

**Still open**
- the backstop timeout, a teardown policy;
- the client protocol for launching → running: either an event, or the coordinator holds the `SpawnPty` reply until the verdict without blocking a goroutine.

---

## 4. `_PtyProcess_` Subsystem & Memory Isolation

### A. Architectural Symmetry
`PtyReader-THREAD.go`, `PtyWriter-THREAD.go`, `PtyDispatcher-THREAD.go` and `PtyResizer-THREAD.go` each run a `RunWorker`. `_PtyProcess_` joins them as a first-class subsystem, replacing `_PtyProxy_.PtyProcess *_OS.Process`:
- `PtyProcess.go` — `_PtyProcess_`, the `_*__State_PtyProcess_` and `_*__Termination_PtyProcess_` types with their `Signal` and `Resolve__ExitOutcome_PtyProxy` methods, the `_Status_CoreDump__` / `_Status_TerminationReport__PtyProcess_` and `_Status_PollIn__Pidfd_` enums, `Spawn_PtyProcess`, constants.
- `PtyProcess-THREAD.go` — `RunWorker` (§3C).

Its proxy-scoped counterpart `_PtyTeardown_` (§3E), in `PtyTeardown.go` / `PtyTeardown-THREAD.go`, ends the session. `_PtyProcess_` only mirrors the process.

### B. State Machine — Aligned With the Kernel's Process Lifecycle
**Design rule: a state records only what suprasole is *notified* of or *does itself*. Anything that can change without notice is an observation, not a state.**

The kernel's lifecycle for a child is: alive → terminated but uncollected (a *zombie*, holding its PID and termination status) → collected by its parent's wait (*reaped*, PID and process entry released). Suprasole is notified of termination (the pidfd becomes readable) and performs the collection itself (the reap).

One kernel rule splits the middle step in two: **a zombie ptracee is visible only to its tracer** until the tracer collects its report or detaches. At the termination notification, the peek therefore tells suprasole which of two things happened:
- the termination is reportable → `Zombie`;
- a debugger withholds it → `Zombie_Withheld`.

```
Alive{pidfd, Pid} ──terminated, reportable (peek)──────────────────▶ Zombie{pidfd, Pid, Termination} ──collected (reap)──▶ Reaped{Termination}
      └─terminated, withheld by a tracer (readable, peek empty)──▶ Zombie_Withheld{pidfd, Pid} ──tracer releases (re-peek)──┘
      └─────────────── waitid / poller failure, from any non-final state ────────────────────────────────────────────▶ SystemError{error}

Termination = Exited{ExitCode}  |  Signaled{Signal, Status_CoreDump}
```

| State | Kernel counterpart | `/proc/<pid>/stat` | Meaning |
|---|---|---|---|
| `_Alive__State_PtyProcess_{FileDescriptor_Pidfd, Pid}` | no `exit_state` — the process has not terminated | `R`, `S`, `D`, `T`, `t` | Not terminated **as of the last notification**. Deliberately covers running, sleeping, uninterruptible, stopped and traced alike — suprasole cannot tell them apart (§6-P), and they change silently. *Not* "running". It also covers a debugger's **exit stop**, correctly: the process is in its exit path but has not terminated (§6-AA). Telling an exit stop apart from other trace stops would need kernel-internal task flags, which are not a stable ABI (§5C). |
| `_Zombie_Withheld__State_PtyProcess_{FileDescriptor_Pidfd, Pid}` | `EXIT_ZOMBIE`, ptraced by another process | `Z`, `TracerPid` ≠ 0 | Terminated, but the kernel shows the termination only to the tracer until it collects its report or detaches. The pidfd is readable; the termination status is not yet visible to suprasole. Signals are accepted and ignored. Exists only while a debugger is attached (§6-AE). |
| `_Zombie__State_PtyProcess_{FileDescriptor_Pidfd, Pid, Termination}` | `EXIT_ZOMBIE` | `Z` (§6-Q) | Terminated; the kernel still holds its PID and termination status until suprasole collects it. The status is already known (the peek reads it). Signals are accepted and ignored (§6-S). Lasts only from the peek to the reap — microseconds — but it is a real kernel state with real semantics, and the type says so. |
| `_Reaped__State_PtyProcess_{Termination}` | released by the reap | entry gone (§6-Q) | Collected. PID and process entry released; only suprasole's record of the termination remains. No handle. |
| `_SystemError__State_PtyProcess_{SystemError_Waitid}` | — | — | Suprasole lost the ability to watch the process: the netpoller failed or `ppoll` failed (`poll(2)`: `ENOMEM`). *Since the spawn's pollability check (§3B), a failed netpoll registration fails the spawn instead and never reaches the worker; whether this state keeps any reachable producer is an open question (parked).* The fate is unknowable, and the state says exactly that rather than inventing a termination. *Implemented (phase 2):* a `waitid` failure — only `ECHILD`, i.e. someone else collected the process — is a real kernel case that §5B.2 rules out, so it has no branch — it is documented here and listed in each chain's final `else`; if it ever occurs, the worker panics with "invalid path" instead of entering this state. |

| Termination | `si_code` | Meaning |
|---|---|---|
| `_Exited__Termination_PtyProcess_{ExitCode_PtyProcess}` | `CLD_EXITED` | Called `exit(code)` (POSIX `WIFEXITED`). |
| `_Signaled__Termination_PtyProcess_{Signal_PtyProcess, Status_CoreDump}` | `CLD_KILLED` / `CLD_DUMPED` | Terminated by *any* signal, not just `SIGKILL` (POSIX `WIFSIGNALED`). `DUMPED__Status_CoreDump__PtyProcess` for `CLD_DUMPED`. |

- **Notification lag, stated precisely.**
  - `Zombie` / `Zombie_Withheld` lag the kernel only by the worker's wake-up latency: between the kernel marking the process `EXIT_ZOMBIE` and the peek, `State_state` still says `Alive`.
  - With `Zombie_Withheld` split out, no state stays wrong for longer than that. Before the split, a debugger-held zombie read as `Alive` indefinitely (§6-V).
  - `Reaped` never lags, because suprasole performs the reap itself.
- **No pre-spawn state** (no `Spawning` / `Launching` / `NotActive`):
  - A `_PtyProcess_` is constructed only after `ForkExec` has returned a pidfd. `CLONE_PIDFD` creates the process and its handle in the same syscall, so there is no instant where the process exists without a handle.
  - If the fork fails, no `_PtyProcess_` is ever built (§3B).
  - A pre-spawn state would be a `_PtyProcess_` with no process and no pidfd — exactly the kind of encoding that misrepresents reality.
  - Spawning is already modeled one level up: `_State_Spawning__WorkspacePty_` (with its cancellation status) and `SPAWNING__Mode_PtyProxy`.
- **Encoding invariants — what the types make unrepresentable:**
  1. *A termination status before termination:* only `Zombie` and `Reaped` carry a `Termination`. There is no nil-able result field.
  2. *A handle or PID after collection:* only `Alive`, `Zombie_Withheld` and `Zombie` carry the pidfd and the PID — the states in which the kernel still holds the process, so the PID cannot have been recycled. The worker closes the pidfd after the final transition. The PID is used only to observe `/proc` inside the Mutex (§3F), never to signal.
  3. *A stop presented as a termination:* `CLD_TRAPPED` is consumed inside the worker loop. `CLD_STOPPED` / `CLD_CONTINUED` cannot be reported to the peek, and would be `panic("invalid path")`.
  4. *Skipping or reversing the kernel's order:* `Zombie` always precedes `Reaped`, because the only other `waitid` that runs before the reap (the `WSTOPPED`-only consume) cannot collect (§6-R). `Zombie_Withheld` occurs only between `Alive` and `Zombie`. `Reaped` and `SystemError` are final.
- **Concurrency:**
  - `RunWorker` is the single writer. It makes at most three transitions under `Mutex` (`→ Zombie_Withheld`, `→ Zombie`, `→` final), closes the pidfd, then closes `Channel_FinalState`. The reap runs inside the same critical section as the final write (§3C).
  - Anyone who sees the channel closed sees a final state. Readers either wait on the channel, or peek under `Mutex` (the signal methods, the teardown's observation) and may catch `Zombie_Withheld` or `Zombie`.
  - The channel is the wake-up; `State_state` is the truth.
- **Observations stay where they are:** the teardown's stopped / rogue / uninterruptible classifications (§3F) are snapshots taken at one instant. They belong in `_TeardownReport_PtyProcess_`, which already models them that way, and never in `State_state`.

### C. Memory Isolation & GC Invariant
- **Risk:** any goroutine holding `_PtyProxy_` while its process is hung pins:
  - `PtyTerminal` (scrollback and grids — megabytes per session);
  - `PostSnapshotBuffer`;
  - the dispatcher's 96 KiB staging buffer and 26-buffer freelist.
- **Model:** `_PtyProcess_` holds zero references to `_PtyProxy_`, `PtyTerminal` or buffers; references flow one way (`_PtyProxy_.PtyProcess`).
  ```go
  type _PtyProcess_ struct {
      Channel_FinalState chan struct{}      // closed by RunWorker strictly after the final transition
      Mutex              _SYNC.Mutex        // guards State_state
      State_state        _State_PtyProcess_ // Alive → (Zombie_Withheld →) Zombie → Reaped, or → SystemError (§4B)
  }

  type _State_PtyProcess_ interface {
      compiletimemarker_State_PtyProcess()
  }

  type _Alive__State_PtyProcess_ struct {
      FileDescriptor_Pidfd *_OS.File // CLONE_PIDFD + O_NONBLOCK, netpoller-registered; raw access only via SyscallConn()
      Pid                  int       // pinned while unreaped; read /proc/<Pid> only inside Mutex (§3F)
  }

  type _Zombie_Withheld__State_PtyProcess_ struct {
      FileDescriptor_Pidfd *_OS.File // same handle — the kernel still holds the process (for its tracer)
      Pid                  int
  }

  type _Zombie__State_PtyProcess_ struct {
      FileDescriptor_Pidfd *_OS.File // same handle as Alive — the kernel still holds the process
      Pid                  int
      Termination          _Termination_PtyProcess_
  }

  type _Reaped__State_PtyProcess_ struct {
      Termination _Termination_PtyProcess_
  }

  type _SystemError__State_PtyProcess_ struct {
      SystemError_Waitid error
  }

  type _Termination_PtyProcess_ interface {
      Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_
  }

  type _Exited__Termination_PtyProcess_ struct {
      ExitCode_PtyProcess int
  }

  type _Signaled__Termination_PtyProcess_ struct {
      Signal_PtyProcess int
      Status_CoreDump    _Status_CoreDump__PtyProcess_
  }

  type _Status_CoreDump__PtyProcess_ uint8

  const (
      NOT_DUMPED__Status_CoreDump__PtyProcess _Status_CoreDump__PtyProcess_ = 0
      DUMPED__Status_CoreDump__PtyProcess     _Status_CoreDump__PtyProcess_ = 1
  )
  ```
  `_Reaped__State_PtyProcess_` and `_SystemError__State_PtyProcess_` also implement `_TeardownReport_PtyProcess_` (§3F). `Alive`, `Zombie_Withheld` and `Zombie` do not, so a non-final state cannot be resolved into an exit outcome.

  **Implemented (phase 2):** instead of a compile-time marker, `_State_PtyProcess_` carries behavior, like `_State_WorkspacePty_`: `Signal(signalNumber_PtyProcess, flags__PIDFD_SEND_SIGNAL)` (§3D) and, until phase 3, `ProbeStatus_Unresponsive()` — the interim teardown probe (§10). Both depend on what a state holds — a pidfd, a PID, or neither — so each state answers for itself; callers hold the Mutex around the call. The signal scope is the kernel's own `pidfd_send_signal` flag, passed straight through. Concrete names follow `_Success__ExitOutcome_PtyProxy_`: `_Alive__State_PtyProcess_`, …, and `_Status_CoreDump__PtyProcess_`.

### D. Worker Lifecycle
1. **Spawn:** started inside `Spawn_PtyProxy` (§3B), alongside `_PtyTeardown_`. Not in `HandleStatus_SpawnPty__Success__Coordinator`, whose cancelled branch returns before starting any worker — cancelled spawns would never be reaped.
2. **Alive:** parked in the netpoller — zero CPU, timers or OS threads.
3. **Proxy reclamation:** the proxy is **not** dropped on websocket disconnect — it is kept for snapshot-on-reconnect. It is dropped only by `HandleRemovePty__Coordinator` (requires `Exited`) and by the cancelled spawn branch. The worker outlives its proxy only for cancelled spawns, or after `_PtyTeardown_` reported an `Unresponsive_*` outcome.
4. **Termination:** `POLLIN` → peek → (`Zombie_Withheld` → re-peek until released →) `Zombie` → reap → `Reaped` → close the pidfd → close `Channel_FinalState`. The channel close is the process-side input to `_PtyTeardown_` (§3E).

---

## 5. Blindspots, Invariants & Known Limits

### A. Launcher Requirements
Suprasole makes no assumption about what launches it (systemd, a container runtime, a wrapper script) or what runs as PID 1. The launcher guarantees:
1. **pidfd capability.** Kernel ≥ 6.9, with the pidfd syscalls not blocked by seccomp — restrictive profiles can block them even on new kernels.
   - Suggested check: Go's own (`pidfd_open(self)`, then `waitid(P_PIDFD)` → `ECHILD`, then `pidfd_send_signal(…, 0, …, 0)`), plus `pidfd_send_signal(self, 0, NULL, PIDFD_SIGNAL_PROCESS_GROUP)`. That last call is a harmless existence check: `EINVAL` means the flag is unknown (below 6.9); `ESRCH` means the flag is supported but the caller leads no process group, so it counts as a pass (§6-AH). `tests/contract/Harness_test.go` `checkFloor` implements exactly this.
   - Unlike Go's `os` package, suprasole has no fallback for kernels below the floor. If the check is skipped: below 5.2 the fork succeeds without a pidfd, `O_NONBLOCK` on `-1` fails (`EBADF`), and `Spawn_PtyProcess` panics `invalid path` — the server stops at the first spawn, leaving that shell running, since suprasole holds no handle to stop it (§3B, §6-AM); from 5.2 to 6.8 spawns work, but group signals fail with `EINVAL`.
2. **No security policy refuses or alters the system calls of suprasole or its helper.** No seccomp filter returns an errno for them. No security module — SELinux, AppArmor, Smack, TOMOYO, Landlock, or a BPF-LSM program — denies the calls the server or `suprasole-pty-exec` make. Executing the helper causes no security-domain transition.
   - Any of these can make a call fail with any error. Every unreachability derivation in this document — `EINTR`, `ECHILD`, `ESRCH`, `EINVAL`, the `Control` failures, `O_NONBLOCK` (§3B) — takes the kernel's own code paths as given; a filtered call would land in that chain's panicking final `else`.
   - This generalizes item 1's seccomp clause from the pidfd syscalls to every call. It was always assumed; it is written down here so the assumption is explicit.
   - **Security modules count as well (added 2026-10-10 from the launch audit).** Policy is set outside the server, so this is the launcher's guarantee. Example: the helper's `TIOCSCTTY` on the PTY slave is checked by SELinux (`FILE__IOCTL`) and TOMOYO at v6.9; Landlock adds an ioctl right (`LANDLOCK_ACCESS_FS_IOCTL_DEV`) in 6.10. A denial reaches the helper's `TIOCSCTTY` step and panics `Spawn_PtyProcess`. The server's own PTY ioctls (`TIOCSPTLCK`, `TIOCGPTN`, `TIOCGPTPEER`, `TIOCSWINSZ`) need the same kind of permission.
   - **No domain transition on executing the helper:** under SELinux, a domain change re-checks inherited descriptors and can revoke them, including the slave on fds 0–2 and the report pipe on fd 3.
3. **The launcher supplies the spawn helper's path:** `-Path_PtyExec <path>`. The server takes it as given, the way it takes `-Address_HttpServer`. It never guesses a location.
   - Like the kernel floor, the path is the launcher's guarantee — present and valid — and the server does not check it at start-up. A wrong path fails each spawn with `fork/exec <path>: no such file or directory`.
   - The filesystem holding the helper must be healthy: the server `execve`s this file during launch. (How the helper is built, and how the launch avoids blocking on a slow filesystem, are in `pidfd-refactor-spawn-hardening.md`.)

### B. Suprasole's Own Invariants & Known Limits
1. **A shell that calls `PTRACE_TRACEME`.**
   - The worker survives it (§6-J, §6-R), but the shell then enters a signal-delivery-stop on *every* signal and nobody resumes it. `SIGHUP` / `SIGCONT` never take effect, so Terminate always ends in `SIGKILL` escalation, reported as `Stopped` via `CLD_TRAPPED`.
   - Suprasole cannot `PTRACE_CONT` / `PTRACE_DETACH` cheaply: ptrace requests must come from the exact OS thread that forked, which would mean pinning a goroutine to that thread for the PTY's lifetime.
2. **Exclusive reaping.** Nothing in the server may set `SIGCHLD` to `SIG_IGN`, call `wait4(-1)` / `waitid(P_ALL)`, or `Wait()` a PTY child through `os/exec`. Any of these can steal a reap; the worker then sees `ECHILD` → `SystemError`.
   - **Why it holds today (derivation).** Per `wait(2)`, a child can be collected only by a thread of its parent's own process, through a wait call naming it (by PID, pidfd, group or wildcard), or by the kernel itself when the parent's `SIGCHLD` disposition is `SIG_IGN` or carries `SA_NOCLDWAIT`. No other process can — a foreign debugger receives its tracee's reports but cannot collect a child that is not its own (§5C). Each of those doors is closed:
     - *Inherited `SIG_IGN`:* Go's runtime installs its own `SIGCHLD` handler at startup; it honors an inherited `SIG_IGN` only for `SIGHUP` / `SIGINT` (`runtime.sigInstallGoHandler`).
     - *`SA_NOCLDWAIT`:* the runtime's handlers use `SA_SIGINFO|SA_ONSTACK|SA_RESTORER|SA_RESTART` and never set it.
     - *`signal.Ignore(SIGCHLD)`:* the only `signal.*` call in `source/` is `main`'s `signal.Notify` for `SIGINT` / `SIGTERM`, which installs no disposition for `SIGCHLD`.
     - *Other waits:* the only `waitid` is `waitid__PtyProcess`, on a pidfd private to one owner; there is no `wait4` in `source/`, no `P_ALL` / `P_PGID`, no `os.StartProcess`, and `exec.Cmd` is never started. The one `wait4` underneath is `syscall.ForkExec`'s own: when the child fails before `execve`, `forkExec` calls `Wait4(pid)` on that child before returning the error (Go 1.27.1 `syscall/exec_unix.go`). It cannot steal a reap — it names only the PID it just created, and that child never becomes a `_PtyProcess_`. It holds the calling OS thread only while the failed child finishes exiting: `CLONE_VFORK` already released the parent at the child's `exit_mm`, and the wait covers the rest of `do_exit` (closing the child's copies of suprasole's descriptors, then `exit_notify`). That is unbounded only if a descriptor's `flush` blocks (a network or FUSE file inherited from the launcher, §8.4 — suprasole itself opens only `/dev/ptmx`) or a foreign debugger following suprasole's forks holds the zombie; the wait is a scheduler-visible syscall, so it blocks one thread, never the runtime.
     - *One collecting call per child:* the worker's `collect__PtyProcess` or the abandon reaper, never both; the peek uses `WNOWAIT` and the trap consume is `WSTOPPED`-only, so neither collects.
     - *Dependencies:* xterm-go and gorilla/websocket never wait on children or touch `SIGCHLD`.
   - **What kind of guarantee that is.** Unlike `EINTR` (ruled out by `wait(2)`) or the kernel floor (enforced by the launcher), nothing outside suprasole enforces this: it rests on the audit above staying true. A future `signal.Ignore(syscall.SIGCHLD)`, wildcard wait, or reaping dependency would break it silently. Removing an error branch does not remove the case — an error falls into the chain's final `else` and panics. All three `waitid` call sites now do exactly that — the abandon reaper, `peek__TerminationReport_PtyProcess` and `collect__PtyProcess` have no error branch; the case is documented here and listed in each final `else`, so a violation panics with "invalid path"; so `SystemError` (§4B) now means only that the watching itself failed.
3. **Group kill reaches only the PTY process's own group.** Job-control jobs live in their own groups, and a session-wide kill would need `/proc` enumeration — see §8.1 and §9D.
4. **Close-on-exec everywhere.** The master, slave and pidfd are all `O_CLOEXEC` (§6-L). A master leaked into another PTY's shell would keep that terminal alive after suprasole closes it: the hangup only happens when the last reference closes, so the forced unplug would silently fail.
   - This covers only descriptors suprasole (or Go) creates. A descriptor suprasole *inherited* when it was exec'd carries no close-on-exec and reaches every shell (§6-AK). Who closes that gap is open (§8.4).
5. **The parent closes the slave immediately after `ForkExec`** (and on failure). Holding it means `EIO` never arrives.
6. **Escalation latency.** For a HUP-ignoring shell: Terminate waits `TIMEOUT_UNRESPONSIVE` (1 s) → unplug + observe + `SIGKILL` → up to `TIMEOUT_UNINTERRUPTIBLE` (1 s). Worst case ≈ 2 s. Previously it was ≈ 3 s, because `HandleExited` waited a second time without knowing why the master had closed; `_PtyTeardown_` knows (§3E).
7. **64-bit only.** `_SigInfo_Waitid_` assumes 64-bit padding after `si_code` (amd64 / arm64).
8. **Command resolution unchanged.** `exec.Command` still describes the command (§3B), so the shell resolves against suprasole's `PATH`, not the child's `Env`; `argv[0]` stays the name as given; and duplicate environment keys still collapse to the last one — exactly as before phase 2.
9. **A debugger can hold the PTY process beyond suprasole's reach.** Attaching requires the same uid (or `CAP_SYS_PTRACE`) — e.g. the user running `gdb -p` / `strace -p` from inside one of their own terminals.
   - **Ordinary attach:** `SIGKILL` works, and the process terminates, but the zombie is withheld until the debugger collects its report or detaches. The worker records this as `Zombie_Withheld` (§6-V, §6-AE).
   - **With `PTRACE_O_TRACEEXIT`:** in the observed kernel (§6-AA–AC), every exit stops in the exit path, **including one caused by `SIGKILL`**. While held there:
     - the process has not terminated, so the pidfd is not readable and the state is — correctly — `Alive`;
     - the peek reports nothing;
     - a further `SIGKILL` is accepted and has no effect.

     The design does not depend on any of this (§5C): `_PtyTeardown_` reports whatever `/proc` shows.
   - In every case suprasole can neither finish nor collect the process until the debugger lets go. `_PtyTeardown_` reports `_Unresponsive_Debugged_{TracerPid}` and ends the session; `_PtyProcess_` completes on its own once the debugger releases the process (§6-AE).
   - **The exit stop and `Zombie_Withheld` are independent.** One happens *before* termination (the process paused in its exit path: still `Alive`); the other *after* (a zombie hidden from suprasole: `Zombie_Withheld`). An ordinary attach produces only `Zombie_Withheld`. A `PTRACE_O_TRACEEXIT` debugger produces both, in sequence, unless it detaches in between. Neither's handling depends on the other.
   - **Reachability depends on the host,** not on the design. Without the Yama LSM (as in this environment), any same-uid process may attach. With Yama `ptrace_scope=1`, the common distribution default, only an ancestor of the PTY process or a holder of `CAP_SYS_PTRACE` may. A tool run inside the PTY is a descendant, so it cannot attach.
10. **`EIO` means "no slave fd is open *right now*" — and suprasole treats the first one as the end.**
    - The kernel's `EIO` is a condition, not a verdict. While the master stays open, any process allowed to open `/dev/pts/N` can reconnect, and the master then reads again (§6-Z).
    - Suprasole makes it final deliberately: PtyReader stops at the first `EIO`, and `_PtyTeardown_` closes the master on "reader finished", which removes `/dev/pts/N`.
    - In the gap between the two, a reconnecting writer's bytes are simply never read. Nothing restarts, and no second teardown is triggered.
    - **Consequence:** a PTY process that closes *every* copy of the terminal and then reopens `/dev/tty` would be cut off in between. Shells can't trigger this, because they keep their own extra copy (§6-AG); their children closing theirs never produces `EIO`. Only a non-shell PTY process doing exactly that close-then-reopen dance could. This is accepted, as the same "nobody holds the terminal" rule terminal emulators apply.

### C. Provenance — What the Design Builds On
Goal: build only on **documented** or **designed** behavior, never on implementation details or environment quirks that experiments happened to reveal. Every load-bearing behavior, classified:

| Behavior | Source | Class | Design's reliance |
|---|---|---|---|
| pidfd readable (`EPOLLIN`) once the process terminates and becomes a zombie; `EPOLLHUP` once reaped | `pidfd_open(2)` | **Documented** | Depends: worker wake-up, `Zombie_Withheld` detection |
| `CLONE_PIDFD` / `SysProcAttr.PidFD`; `pidfd_send_signal` (`ESRCH` once gone); `PIDFD_SIGNAL_PROCESS_GROUP` (6.9) | `clone(2)`, `pidfd_send_signal(2)`, Go `syscall` docs | **Documented** | Depends |
| `waitid`: `P_PIDFD`; `WNOHANG` ⇒ `si_pid == 0` when nothing to report; `WNOWAIT` "leave the child in a waitable state"; only `WEXITED` reports terminations, so a `WSTOPPED`-only call cannot collect | `waitid(2)` | **Documented** | Depends: peek, reap, trap consumption |
| Stops of a traced child go to the tracer; "a zombie ptracee is only visible to its ptracer" until it collects or detaches | `ptrace(2)`; comment in `kernel/exit.c` `wait_consider_task()` (that logic had a bug, fixed in 2014, in the case where the tracer *is* the real parent) | **Designed** | Depends: `Zombie_Withheld`, `CLD_TRAPPED` handling |
| `/proc/<pid>/stat` state letters (`R S D T t Z`); `/proc/<pid>/status` `TracerPid` (a *thread* ID — §6-AI) | `proc(5)` | **Documented** | Depends: teardown observation |
| A PID is not reused until its zombie is collected | POSIX (process lifetime) | **Documented** | Depends: reading `/proc/<Pid>` inside the Mutex |
| `TIOCGPTPEER`, `TIOCSPTLCK`, `TIOCSWINSZ`, `TIOCGPTN` | `ioctl_tty(2)` | **Documented** | Depends: native PTY |
| A descriptor without close-on-exec survives `execve`, so one inherited at suprasole's own exec survives each shell's too; `close_range(first, last, CLOSE_RANGE_CLOEXEC)` (5.11) sets the flag on a range without closing anything | `execve(2)` ("by default, file descriptors remain open across an execve()"), `fcntl(2)`, `close_range(2)` | **Documented** | Depends, for whichever side takes §8.4 |
| Master `read` returns buffered data first, then `EIO`, once no slave fd is open | kernel commit messages (e.g. "pty: Fix input race when closing", "tty: Fix pty master read() after slave closes" — races fixed long before 6.9); **not in `pty(7)`** | **Designed** | Depends: the reader's end-of-stream and the drain. A regression would truncate trailing output, never hang. |
| Closing the master hangs up the slave (`SIGHUP` + `SIGCONT` to the session leader; slave fds hung up; `/dev/pts/N` removed) | TTY / pty driver; what every terminal emulator relies on when a window closes | **Designed** | Depends: forced unplug |
| Reopening the slave after `EIO` undoes it while the master is open | pty driver (`pty_open` clears the master's other-closed flag) | **Designed** | **Defended against only:** the master is closed on every reader finish (§3E) |
| Session leader's exit → `SIGHUP` to the foreground group; a newly orphaned group with a stopped member → `SIGHUP` + `SIGCONT` | POSIX `_exit()` | **Documented** | Descriptive only (§9C–D) |
| Interactive bash ignores `SIGTERM`, forwards `SIGHUP` to its jobs, skips disowned ones; GNU `nohup` redirects all three standard fds | `bash(1)`, coreutils manual | **Documented** (shell policy) | Not depended on: it motivates choosing `SIGHUP` (§3D), and non-shell PTY processes simply lack the policy |
| `os.NewFile` on a non-blocking fd is pollable; `File.Fd()` disables deadlines ("For most uses prefer the f.SyscallConn method") | Go `os` docs | **Documented** | Depends: master and pidfd are created non-blocking and accessed only via `SyscallConn` |
| `RawConn.Read`: `f` "is expected to try to read"; `Read` blocks only when `f` returns `false` | Go `syscall` docs | **Documented** | Depends: the worker returns `false` only when the pidfd is not readable (§3C) |
| Go's netpoller is edge-triggered (`EPOLLET` in `runtime/netpoll_epoll.go`) | Go runtime source | **Implementation detail** | **Not relied on** (the `Zombie_Withheld` state removed the only dependency) |
| A debugger's `PTRACE_EVENT_EXIT` stop also occurs on a `SIGKILL` death | `ptrace(2)` says exit stops happen for "signal deaths (except SIGKILL)", but its BUGS section notes that `SIGKILL` *may* still cause one. rr's author documents that modern kernels always generate it (§6-AC) | **Documented as version-dependent** | **Never relied on:** the teardown reports what `/proc` shows |
| A further `SIGKILL` does not release a debugger's exit stop | Consistent with `ptrace(2)` ("the tracer cannot prevent the exit from happening at this point" — the exit is already under way), but how a pending `SIGKILL` interacts with the stop is the same version-dependent area (§6-AB) | **Observed; documented only as version-dependent** | **Never relied on** |
| A second pidfd wake-up when a tracer releases a zombie | The pidfd polling commit wakes pollers "at the same moment the parent is notified of a child's death (`do_notify_parent`)". For a ptraced zombie that notification is "cascaded to the real parent when the ptracer detaches" (`kernel/exit.c`), so a second parent notification brings a second wake-up (§6-V). The notify conditions were reworked in 2024 | **Designed, not documented as a contract; actively changing** | **Not relied on:** `Zombie_Withheld` re-peeks on an interval |
| A `WSTOPPED`-only `waitid` on a zombie returns `ECHILD` — traced *or not* | Kernel `do_wait`: `ECHILD` means no child could *ever* satisfy the requested state changes, and a zombie can never stop. My first source-based prediction ("only when traced") was **wrong**; both cases return `ECHILD` (§6-AF) | **Designed (kernel source), undocumented** | **Not relied on:** its result is ignored |
| A pidfd signal to a zombie succeeds without effect | `kill(2)`, under `ESRCH`: "an existing process might be a zombie" — a zombie still exists, so the call does not fail (§6-S) | **Documented** | **Not relied on:** success is never read as "alive" (§3D) |
| `EIO` vs pidfd ordering | **No contract exists.** The kernel releases a dying process's fds before it becomes a zombie (`do_exit`), but the two notifications travel independent paths to user space (§6-W) | **Unspecified** | **Never relied on:** order-independent by construction (§3E) |
| Wake-up latencies, the 29/30 split | measured here (§6-W) | **Environment-specific** | Never relied on |

**Environment-specific aspects of the experiments, none of which the design depends on:**
- **No Yama LSM (linuxkit),** so a same-uid debugger could attach. This changes *reachability* only (§5B.9).
- **Docker's default seccomp profile** permitted the pidfd syscalls, `ptrace` and `clone3`. Other profiles may not; that is the launcher's check (§5A).
- **Running inside a PID namespace** with `dumb-init` as PID 1 affects orphan reaping only, which is out of scope.
- **Timings** (29/30 ordering, wake-up latencies) are environment-dependent; nothing depends on them.
- **bash 5.2 and GNU coreutils 9.7** behaviors are documented shell / utility policy, not kernel behavior.

---

## 6. Verification Evidence
**Executable form:** `suprasole-server/tests/contract/` — 21 kernel contract tests, one per "Depends" row of §5C, each citing the row below that it replaces. Run `go test ./tests/contract -count=1 -v` on every target host. Here: all pass, stable over 20 runs (≈ 0.66 s per run).
Linux 6.12.76 (linuxkit, arm64) · Go 1.27.1. Rows A–H used `creack/pty` v1.1.24 (with a hand re-wrapped non-blocking master where noted); I–K used `os/exec`; L–AG ran the §3 design (or its primitives) as a prototype; AA–AE add a foreign debugger as a separate process.

| # | Scenario | Observed |
|---|---|---|
| A | `creack/pty` master as returned, child `sleep 30`; `Close()` after 100 ms | `O_NONBLOCK` = false; reader still blocked 1.5 s after `Close()`; child alive (no hangup). |
| B | Same master re-wrapped non-blocking, then `pty.Setsize`, then `Close()` | Still non-blocking after `Setsize`; reader `file already closed` in < 1 ms; child killed by `SIGHUP`. |
| C | `bash -c 'trap "" HUP; sleep 30 & exit 3'`; pidfd group `SIGKILL` **before** the reap | pidfd readable 2 ms after exit; no `EIO` after 1.5 s; `PIDFD_SIGNAL_PROCESS_GROUP` `SIGKILL` on the unreaped leader → `EIO`; reap `CLD_EXITED`/3; plain pidfd signal after reap → `ESRCH`. |
| D | pidfd watcher, 200 ms `SetReadDeadline`, live process | `i/o timeout` — deadlines work on the netpoller-parked pidfd. |
| E | Child writes 2,000,000 bytes then exits | All 2,000,000 bytes read before `EIO`. |
| F | Interactive bash: `sleep 30 &`, `exit 7` | Shell exited (status 7); no `EIO` after 2 s; forced `Close()` → reader `file already closed`. |
| G | Interactive bash + running job: `SIGTERM`, then `SIGHUP` | Alive 500 ms after `SIGTERM`; `SIGHUP` killed bash (`CLD_KILLED`/1) and its job (`EIO`). |
| H | `SIGSTOP`'d interactive bash: `SIGHUP`, then `SIGCONT` | Not exited 500 ms after `SIGHUP`; exited by `SIGHUP` once continued. |
| I | Child traced by suprasole (`PTRACE_TRACEME`); Go's `Wait()` | Returned the trap-stop (`SIGTRAP`). Then `Signal` → "process already finished", 2nd `Wait` → `ECHILD`, `WithHandle` → "process already finished"; child left trace-stopped (`t`), unreaped. |
| J | Same child, own non-blocking pidfd worker; external pidfd `SIGKILL` at 500 ms | `waitid(WEXITED)` returned `CLD_TRAPPED`/5 → skipped; next call → parked; reaped `CLD_KILLED`/9 at 501 ms. |
| K | Copy of Go's handle via `WithHandle` + `O_NONBLOCK` | Go's `Wait()` on a live process returned immediately with `EAGAIN` — the flag is shared through the open file description. |
| L | Prototype: native PTY + `ForkExec(PidFD)` + interactive bash | Master and pidfd `O_NONBLOCK` + `FD_CLOEXEC`; `stty size` = 24 80, `tty` = `/dev/pts/N`; `SIGHUP`+`SIGCONT` → `CLD_KILLED`/1 and reader `EIO`; after the worker's close, `Signal` → closed file. |
| M | Prototype: `bash -c 'trap "" HUP TERM; sleep 30'` | Alive 500 ms after `SIGHUP`+`SIGCONT`; group `SIGKILL` → `CLD_KILLED`/9 and `EIO` (the foreground `sleep` died too). This run used numeric `kill(-pgid)`; the pidfd form is rows C and O. |
| N | Prototype: `ForkExec` with a nonexistent `Dir` | Error returned; `PidFD` left at `-1`. |
| O | Prototype: `bash -c 'trap "" HUP; sleep 30 & exit 4'`; pidfd group `SIGKILL` **after** the reap | Child's PGID == shell's PID. Shell reaped (`CLD_EXITED`/4), reader blocked; group `SIGKILL` through the reaped shell's pidfd → `nil`, then `EIO` (child killed). Once the group was empty → `ESRCH`. |
| P | Untraced child; worker-style `waitid(WEXITED\|WNOHANG)` while running, after `SIGSTOP` (`/proc` state `T`), after `SIGCONT`, after `SIGKILL` | Running, stopped and continued all report `si_pid == 0` — indistinguishable, and no `CLD_STOPPED` / `CLD_CONTINUED` ever appears. `SIGKILL` → `CLD_KILLED`/9 (reaped); next call → `ECHILD`. |
| Q | Untraced `sh -c 'exit 3'`: peek (`WNOWAIT`), peek again, reap | Peek: `CLD_EXITED`/3 while `/proc` shows `Z`; second peek identical (nothing collected); reap: `CLD_EXITED`/3, `/proc` entry gone; next call → `ECHILD`. |
| R | Traced child: peek, `WSTOPPED`-only consume, peek; then `SIGKILL`, `WSTOPPED`-only call, peek, reap. Also an untraced `SIGSTOP`'d child under the peek | Peek `CLD_TRAPPED` (stays pending under `WNOWAIT`); consume `CLD_TRAPPED`, `/proc` still `t` (not reaped); next peek `si_pid == 0`. After `SIGKILL`: the `WSTOPPED`-only call → `ECHILD` with `/proc` still `Z` (did **not** reap); peek `CLD_KILLED`/9; reap → gone. Untraced `T` child: peek reports nothing. |
| S | Zombie (`/proc` `Z`): pidfd `SIGTERM` with flags 0 and with `PIDFD_SIGNAL_PROCESS_GROUP`; then the same after the reap | Zombie: both calls succeed, process still `Z` (no effect). After the reap: both `ESRCH`. |
| T | Interactive bash with six descendants (§9D: running job, stopped job, `nohup`, `setsid` keeping the slave, disowned job, fully detached daemon); natural `exit`; then group `SIGKILL` via the zombie's pidfd; then forced unplug | Shell exited; **no `EIO`** (running job, `setsid` process and disowned job hold the slave). Stopped job gone (orphaned-process-group `SIGHUP` + `SIGCONT`); all others alive. Group `SIGKILL` → success, **nobody died** (every job has its own group). Unplug → reader `os.ErrClosed`; every survivor still alive, slave fds hung up. |
| U | Same six descendants; Terminate (pidfd `SIGHUP` + `SIGCONT`); then unplug | Shell `CLD_KILLED`/`SIGHUP`. Running and stopped jobs gone (bash forwarded the hangup). `nohup`, `setsid` and disowned processes survive; **still no `EIO`** (`setsid` and disowned hold the slave). Unplug → `os.ErrClosed`; survivors alive. |
| V | Foreign tracer (separate process, same uid) attached to the PTY process; pidfd `SIGKILL`; worker running | `/proc` `Z` while the tracer holds it. Worker callback #1 (start): nothing → park. **#2 (pidfd readable at exit): nothing reportable to the real parent → park.** Tracer collects its report → **#3: `CLD_KILLED`/9** → reaped. |
| W | 30 natural exits (`sh -c 'exit 0'`): which arrives first at user level, `EIO` or pidfd termination? | pidfd termination first 29×, `EIO` first 1× — no guaranteed order. |
| X | `bash -c 'sleep 30 & exit 0'` (no job control: child in the shell's group, not ignoring `SIGHUP`) | Shell reaped; `EIO`; child gone — the foreground group gets `SIGHUP` when the session leader exits. |
| Y | A process **outside the session** opens `/dev/pts/N` by path and holds it; PTY process exits; unplug | Open succeeded; PTY process reaped; **no `EIO`**. Unplug: outsider alive and not signaled; `/dev/pts/N` removed (`ENOENT`). |
| Z | Only slave holder exits → `EIO`; then reopen `/dev/pts/N` by path while the master is open; write; then close the master and reopen | Reopen after `EIO` **succeeded**; master read `"late\r\n"` — `EIO` was undone. After the master closed: reopen → `ENOENT`. |
| AA | Foreign debugger seizes the child with `PTRACE_O_TRACEEXIT`; child exits normally (status 5); debugger holds the exit stop 1 s | Debugger gets `PTRACE_EVENT_EXIT`. Throughout the hold, suprasole sees `/proc` `t`, pidfd **not** readable, peek nothing reportable. After release and the debugger's own collection: `Z`, readable, `CLD_EXITED`/5, reaped. |
| AB | Same, but suprasole sends pidfd `SIGKILL` while the exit stop is held | `SIGKILL` accepted (`0`) and **without effect**: still `t` until the debugger continues; the final status is still `CLD_EXITED`/5, not killed. |
| AC | Same debugger; suprasole's `SIGKILL` is the cause of death | The exit stop happens **for `SIGKILL` too**. Held `t`, not readable, nothing reportable; after release: `CLD_KILLED`/9, reaped. |
| AD | `/proc/<pid>/status` of a seized process | `TracerPid:` shows the debugger's PID. |
| AE | Proposed worker (§3C) under an ordinarily attached debugger; pidfd `SIGKILL` at +100 ms; debugger collects its report at +800 ms | `Alive`; at +100 ms pidfd readable + peek empty → **`Zombie_Withheld`** (`/proc` `Z`); re-peek reports after the release → `Zombie` `CLD_KILLED`/9 → `Reaped`. The callback returned `false` only while the pidfd was not readable. |
| AF | Prediction derived from kernel source: a `WSTOPPED`-only `waitid` on a zombie returns `ECHILD` only if the zombie is traced. Tested with an untraced and a traced (`PTRACE_TRACEME`) zombie | **Prediction wrong:** both return `ECHILD` (`/proc` `Z` in both). The corrected root: `ECHILD` means no child could ever satisfy the requested state changes. |
| AG | File descriptors of an interactive bash on a PTY | fds 0, 1, 2 **and** 5 → `/dev/pts/N`: the shell keeps its own extra copy of the terminal open, so its children closing theirs can never produce `EIO` while it lives. |
| AH | `PIDFD_SIGNAL_PROCESS_GROUP`, signal 0, through a group leader's pidfd vs. a non-leader's (and the test process's own) | Leader: `0`. Non-leader: **`ESRCH`** — the flag addresses the group *led by* the process. `EINVAL` (flag unknown) would mean a kernel below 6.9. |
| AI | `PTRACE_TRACEME` child; `/proc/<pid>/status` `TracerPid` vs. the forking OS thread and the process PID, 3 runs | `TracerPid` always equals the **forking thread**; in one run that thread (80235) differed from the process PID (80231). Hence §3F checks thread membership, not PID equality. |
| AJ | `RawConn.Control` on an `os.File` after `Close()` (`tests/PtyDescriptor_test.go`, `TestResize_PtyDescriptor`) | Fails with poll's **"use of closed file"**, not `os.ErrClosed`: `os` translates that only for its own I/O methods (`File.wrapErr`). The value is undocumented, so only "fails" is relied on (§3D). |
| AK | A launcher passes suprasole an extra descriptor (`exec.Cmd.ExtraFiles` → fd 3); suprasole spawns a child with `ForkExec`. Then the same with `close_range(3, ~0U, CLOSE_RANGE_CLOEXEC)` first (`tests/contract/CloseOnExec_test.go`) | As inherited: fd 3 flags `0` (no `FD_CLOEXEC`); **the child holds fd 3** → the launcher's file. After `close_range`: flags `FD_CLOEXEC`; suprasole still holds fd 3; the child does not. |
| AL | Phase 2 `_PtyProcess_` (`tests/PtyProcess_test.go`) | `exit 0` / `exit 3` → `Reaped{Exited}` → `Success` / `Failure{3}`. `Signal(SIGTERM)` → `Reaped{Signaled{15}}` → `Killed{15}`. `Signal(SIGKILL, PIDFD_SIGNAL_PROCESS_GROUP)` also kills a HUP-ignoring group member. Signals after `Reaped` → `os.ErrProcessDone`. Self-traced (the PTY process calls `PTRACE_TRACEME` itself, then execs): `Alive` while `/proc` shows `t`, traps consumed, then `Reaped{Signaled{9}}`. Foreign debugger: `Zombie_Withheld`, then `Reaped{Signaled{9}}` one re-peek (1.00 s) after it collects. Failed fork: no process, a `fork/exec` `*os.PathError`. The abandon sequence (group `SIGKILL` + `reapAbandoned__PtyProcess_Pidfd`) kills and collects (`/proc` entry gone). All pass, 10 runs. |
| AM | Descriptor table exhausted (`RLIMIT_NOFILE` 64, every slot taken except the two Go's exec-status pipe needs); `ForkExec` of `true` without and with `PidFD` | Without: succeeds. **With: `too many open files`, no child created, `PidFD` left at `-1`.** The kernel fails the whole `clone` when it cannot allocate the pidfd (`clone(2)`: `EMFILE` with `CLONE_PIDFD`, since 5.2) — so exhaustion takes `Spawn_PtyProcess`'s fork-error branch; only a kernel that ignores `CLONE_PIDFD` (before 5.2, where the bit is the dead `CLONE_DETACHED`) yields `-1` after a successful fork — below the floor, so not handled (§5A). |
| AN | *Illustration only — the bounds are derived algebraically in §3B; sampling can show "not observed", never "impossible".* Trap-survival experiment, 1000 trials per variant: a `PTRACE_TRACEME` child at its exec trap (suprasole its tracer), group `SIGKILL` through the pidfd, and the abandon reaper's loop with per-branch counters (1 ms sleep instead of 1 s). **A** trap confirmed pending (`/proc` `t` + a non-consuming `WNOWAIT` peek), kill, reap; **B** the same, with one blocking `waitid(WEXITED)` (the old reaper); **C** kill the moment `ForkExec` returns (the real abandon timing); **D** reaper already spinning when the kill lands | **A, B, C: the trap branch was never taken — 0 of 3000 trials,** 2000 of them with the trap confirmed pending just before the kill. A and C: every trial got exactly one empty answer, then `CLD_KILLED` — right after `kill` returns, neither the trap nor the death is reportable. **D: the trap branch ran exactly once in every trial (1000/1000), never twice:** the spinning reaper consumed the report before the kill, and consuming it does not resume the tracee. `EINTR` never occurred (0 in ≈ 10.2 million `WNOHANG` calls). **Root cause, verified in kernel source at v6.9 (designed since 5.19, commit 31cae1ea):** `complete_signal()` handles `SIGKILL` inside `kill` by calling `signal_wake_up(t, 1)` on every thread; that fatal wake clears `JOBCTL_STOPPED | JOBCTL_TRACED` unless `JOBCTL_PTRACE_FROZEN` (set only by the tracer's own `ptrace(2)` requests — suprasole makes none); `task_stopped_code()` reports a ptrace stop only while `task_is_traced()`; and `ptrace_stop()` refuses to stop with a fatal signal pending. `ptrace(2)` is silent on the case (it states only the intent: "SIGKILL is meant to always immediately kill tasks even under ptrace"). On this basis the abandon reaper has no `CLD_TRAPPED` branch (§3B) — its final `else` lists the case and panics; `TestReapAbandoned_PtyProcess_Pidfd_ReapsPastTrapReports` would panic if a trap ever arrived. **Documented bound: at most one such report, at most once** — only suprasole-as-tracer traps reach the real parent (a foreign debugger receives its own); suprasole never resumes a tracee, so a self-traced process stops once and stays stopped; after `SIGKILL` no new stop can arise (`ptrace(2)`: no signal-delivery-stop for `SIGKILL`; the exit stop needs `PTRACE_O_TRACEEXIT` and syscall stops need `PTRACE_SYSCALL`, neither ever set by suprasole); and the reaper's `waitid` without `WNOWAIT` consumes the report. |

---

## 7. Changes From the Original Draft
- **Added** problems 5 and 6. Neither forced unplug nor natural exit with a lingering job works in today's code.
- **Replaced** `creack/pty` with a native PTY descriptor, and `exec.Command` / `os.Process` with `syscall.ForkExec` + `SysProcAttr.PidFD`. One owner of the process from fork to reap; no `Release()`, no duplicated handles.
- **Revised** problem 4: stop reports are real (via `PTRACE_TRACEME`) and fatal to `os.Process`. The recursion is replaced by a peek loop that consumes trap reports without ever reaping, and the probe must accept `CLD_TRAPPED`.
- **Set** the kernel floor to 6.9. Every signal, group signals included, goes through the pidfd, so no lock or reap bookkeeping is needed for signal safety, and `_PtyProcess_` needs no `Pid`. `PIDFD_NONBLOCK` is not needed: `fcntl` `O_NONBLOCK` + `WNOHANG` covers it.
- **Modeled** `_PtyProcess_` as a state machine aligned with the kernel's process lifecycle (§4B):
  - `Alive{pidfd}` → `Zombie{pidfd, Termination}` → `Reaped{Termination}`, or → `SystemError`.
  - `Termination` = `Exited{ExitCode}` | `Signaled{Signal, Status_CoreDump}`.
  - The worker observes termination with a non-reaping peek, so `Zombie` mirrors the kernel's `EXIT_ZOMBIE`.
  - Running / stopped / traced / `D` are observations, not states, because suprasole is never notified of them (§6-P).
  - This replaces the nil-able reap-result struct.
- **Renamed** the broadcast channel to `Channel_FinalState`, since it closes on entering a final state (`Reaped` or `SystemError`), not on exit. "Final" is used instead of "terminal" to avoid colliding with `PtyTerminal`.
- **Changed** `SIGHUP`-only termination to `SIGHUP` + `SIGCONT`. Escalation stays a group `SIGKILL`; the original draft's `pidfd_send_signal(SIGKILL, 0)` would have dropped the group.
- **Moved** the worker start from the coordinator to `Spawn_PtyProxy`.
- **Added** the dispatcher-owned post-exit drain timer. Removed the reliance on a PtyReader "exitSignal channel" (it does not exist).
- **Corrected** the claim that disconnect drops the proxy, and the "~64 bytes, zero leaked" footprint. Recorded the `D`-state worker as an accepted cost.
- **Scoped** the launcher's responsibility to the pidfd capability check (§5A), and declared orphan reaping out of scope for both suprasole and its launcher.
- **Mapped** the full interplay between the kernel's process and TTY subsystems (§9). Verified, among other things (§6-T–Z):
  - descendant fates under both exit paths;
  - foreign-tracer zombie withholding;
  - the lack of any `EIO` / pidfd ordering;
  - outsiders holding the slave;
  - `EIO`'s reversibility.

  Consequences: the forced unplug is required on the Terminate path too; the unbounded `Alive` lag under a foreign tracer is now documented; and group `SIGKILL` cannot clean up lingering processes.
- **Corrected** an earlier claim that `PTRACE_EVENT_EXIT` "cannot occur". It cannot be *reported to suprasole*, but a debugger with `PTRACE_O_TRACEEXIT` causes it, even on `SIGKILL`, and suprasole can neither see, end nor collect the process until the debugger releases it (§6-AA–AD, §5B.9).
- **Replaced** the teardown's `waitid` probe with a `/proc/<Pid>` observation (state letter + `TracerPid`), read inside the Mutex. The reap now runs inside the Mutex, so the PID is still ours while the state says `Alive` / `Zombie`. This adds `_Unresponsive_Debugged_` and observes `D` instead of inferring it (§3F).
- **Re-keyed** §9D by kernel properties (process group, session, slave fd, `SIGHUP` disposition / run state). "Job" and "disown" are shell bookkeeping; a disowned job is kernel-identical to a running one.
- **Adopted** `_PtyTeardown_` (§3E), which absorbs the dispatcher's drain arm, `HandleExited`, `Terminate_PtyProxy` and `TerminateCancelled_PtyProxy`. It owns every master close and all timers, and emits the outcome once. Worst-case escalation drops from ≈ 3 s to ≈ 2 s.
- **Added** the `Zombie_Withheld` state (a debugger-held zombie: pidfd readable, peek empty), detected deterministically from documented and designed behavior (§6-AE). `Alive` is now never false beyond wake-up latency, and the worker no longer depends on the netpoller being edge-triggered.
- **Added** §5C, the provenance of every load-bearing behavior. Observed-only behaviors (`SIGKILL` vs. exit stop, second pidfd wake-up, `ECHILD` from a `WSTOPPED`-only wait, signal-to-zombie success, event ordering) are explicitly not relied on.
- **Added** naming angles (e)–(h) (§8.2).
- **Root-caused** every behavior previously marked "observed only" (§5C): the exit stop on `SIGKILL` is documented as version-dependent; the second wake-up is designed but not a contract; `ECHILD` is designed kernel logic (a source-based prediction was tested and found wrong, then corrected — §6-AF); zombie signal success is documented in `kill(2)`; `EIO` / pidfd ordering has no contract at all. None are relied on.
- **Added** §5B.10 (`EIO` means no slave fd *right now*; suprasole makes the first one final) and §6-AG (shells keep an extra copy of the terminal).
- **Added** the kernel contract suite `tests/contract/` (19 tests). Writing it corrected two claims: `TracerPid` names a *thread*, so self-tracing is detected by thread membership (§3F, §6-AI); and `PIDFD_SIGNAL_PROCESS_GROUP` is defined for group leaders, so the launcher check treats `ESRCH` as a pass (§5A, §6-AH).
- **Phase 1 implemented** (§10): native PTY in `source/PtyDescriptor.go`; `creack/pty` removed. Its tests corrected one more claim: `RawConn.Control` on a closed file fails with "use of closed file", not `os.ErrClosed` (§3D, §6-AJ).
- **Phase 2 implemented** (§10): `_PtyProcess_` (`source/PtyProcess.go`, `source/PtyProcess-THREAD.go`) owns the PTY process from `ForkExec` to reap; `exec.Command` survives only as the command's description (§3B), so shells start exactly as before. Today's teardown is rewired onto `Channel_FinalState` until phase 3.
- **Added** §8.4: descriptors inherited when suprasole was exec'd reach every shell, a gap §5B.4 did not cover. Whether suprasole or its launcher closes it is open. Two contract tests (§6-AK).

---

## 8. Open Decisions

### 1. Lingering Processes After the PTY Process Terminates — *decided: unplug only (Option A)*
- **What lingers (§9D, §6-T, §6-U):**
  - running job-control jobs (after a natural `exit` only);
  - `setsid`'d processes that kept the slave;
  - disowned jobs;
  - same-group children that ignore `SIGHUP`;
  - outsiders holding `/dev/pts/N` by path.
- *As proposed:* forced unplug only. The PTY is reclaimed; lingering processes lose their terminal (reads see EOF, writes fail) but keep running — what happens when a terminal-emulator window closes.
- *Alternative:* also `Signal(SIGKILL, PIDFD_SIGNAL_PROCESS_GROUP)` after the reap. The evidence says it buys almost nothing:
  - Through the zombie shell's pidfd it "succeeded" and **killed no one** (§6-T). Every lingering kind above lives outside the shell's process group, except the HUP-ignoring same-group child.
  - It needs a pidfd after collection, which breaks invariant 2 (handle only in `Alive` / `Zombie` — §4B).
  - Reaching the rest would need a `/proc` scan of the session, and `setsid`'d processes and outsiders aren't even in the session.

  **Decision: unplug only.** Implemented by `_PtyTeardown_`'s drain timer in phase 3 (§10).

### 2. State Names — *chosen, tentatively*
**Current choice:** `Alive` → (`Zombie_Withheld` →) `Zombie` → `Reaped`, or → `SystemError`. These are kernel nouns throughout, with one qualifier on the single state where suprasole's view differs from the kernel's. The rename is mechanical, so this stays open to revision. The analysis that led here follows.

Each state can be named from three angles. The current names already mix two of them: `Alive` and `Zombie` describe the **kernel's** condition, while `Reaped` is the **parent's** action.

| Angle | `Alive` | `Zombie` | `Reaped` | `SystemError` |
|---|---|---|---|---|
| Kernel: process condition | alive | zombie | gone | unknown |
| Kernel: process entry (PID + task) | held | held | released (the kernel's verb: `release_task`) | unknown |
| Suprasole: what it knows / did | watching — no termination notified | observed — termination read, not collected | collected | lost track |

- **(a) Keep single names.** The §4B tables carry the other angles.
- **(b) `Alive_Entered` / `Zombie_Entered` / `Reaped_Removed`, or `AliveEntry` / `ZombieEntry` / `ReapedErased`.** "Entered" is true of every state, so it adds no information. "Removed" / "Erased" captures the process-entry angle, but the kernel's own word is "released".
- **(c) Condition × entry,** e.g. `Alive_Held`, `Zombie_Held`, `Gone_Released`. Accurate, but the second half is derivable from the first: the kernel holds the entry exactly while the process is alive or a zombie.
- **(d) Condition × knowledge,** e.g. `Alive_Watched`, `Zombie_Observed`, `Released_Collected`, `Unknown_Lost` (or keep `SystemError`, to match the `_SystemError_*` taxonomy).
  - The second half is **not** derivable. It is exactly where the notification lag lives: under a foreign tracer the kernel says zombie while suprasole's knowledge is still "watched" (§6-V).
  - `Released_Collected` names one event from both sides: the kernel releases, the parent collects.

**Which names can be false?** With `Zombie_Withheld` split out (§4B), **none** — beyond the worker's wake-up latency.
- `Alive` also covers a debugger's exit stop, and that is correct: the process has not terminated (§6-AA).
- `Zombie_Withheld` and `Zombie` are each entered on a termination notification and a specific peek result.
- `Reaped` is suprasole's own act.

Before the split, `Alive` was false for a debugger-withheld zombie, which motivated the mechanism-based angles below. They are still valid choices, now on taste rather than correctness.
- **(e) `waitid`'s own vocabulary:** `Unwaitable` → `Waitable` → `Waited`. The man page's `WNOWAIT` wording is "leave the child in a waitable state".
- **(f) Report lifecycle:** `wait(2)` and `ptrace(2)` speak of state changes being *reported*. `Unreported` → (`Zombie_Withheld`) → `Reported` → `Consumed`.
- **(g) What the kernel retains on suprasole's behalf:** `Pending` (PID + process) → `Retained` (PID + termination status, kept by `WNOWAIT`) → `Released`.
- **(h) Kernel nouns throughout** (the current set): `Alive` → (`Zombie_Withheld`) → `Zombie` → `Reaped`.

**Why `Zombie_Withheld`:** it is a zombie in every kernel sense (`EXIT_ZOMBIE`, `/proc` `Z`), whose termination is withheld from suprasole by its tracer. The name is dual — kernel condition plus suprasole's view — on exactly the one state where those two diverge.
- For symmetry, the plain one could become `Zombie_Reported`. Leaving it as `Zombie` reads as "a zombie, nothing unusual".
- Other kernel-aligned options considered: `TracedZombie`, `Zombie_TracerHeld` (after the kernel's "zombie ptracee").

### 3. A Dedicated Teardown Actor — *adopted*
Adopted as `_PtyTeardown_` (§3E).

### 4. Descriptors Inherited From the Launcher — *open: suprasole or launcher*
- **The gap.** §5B.4 covers descriptors suprasole and Go create: Go opens everything `O_CLOEXEC`, and the child receives only fds 0–2. A descriptor suprasole *inherited* when it was exec'd has no close-on-exec — that is why it survived — so it also survives each shell's `execve` (§6-AK). fds 0–2 are exempt: the child's are always replaced by the slave.
- **Where such descriptors come from.** Anything in the launch chain: a wrapper script, sockets passed by a service manager, descriptors a container runtime preserves, or one an ancestor never marked. Suprasole cannot know which, and uses none today.
- **What a leaked descriptor does:**
  - every shell and all of its descendants hold it, so the resource lives as long as any of them does (a pipe's reader never sees EOF; a socket or lock stays held);
  - any of them can read or write through it.
- **The mechanism, on either side.** `close_range(3, ~0U, CLOSE_RANGE_CLOEXEC)` marks every descriptor from 3 up close-on-exec without closing it: the holder keeps using it, and nothing it execs receives it (Linux 5.11, within the 6.9 floor). With flags `0`, it closes them instead. Go's `SysProcAttr` has no per-spawn equivalent, so it cannot be done inside `ForkExec`.
- **Option A — suprasole, at startup.** One syscall before the first spawn.
  - Holds whatever the launcher does; inherited descriptors stay usable by suprasole itself.
  - Suprasole takes a position on the environment it starts in, which §5A otherwise leaves to the launcher. A launcher that ever wanted a descriptor to reach the shells could not do it.
- **Option B — a launcher requirement (§5A).** The launcher execs suprasole with only fds 0–2 open, or with every other descriptor close-on-exec.
  - Matches the existing division: the launcher owns the environment suprasole starts in, as with the pidfd capability.
  - Suprasole trusts rather than enforces. A launcher that misses it leaks silently; nothing reports an error.
  - **Variant:** suprasole verifies at startup (`/proc/self/fd`, `F_GETFD` on each descriptor from 3 up) and reports or refuses, the way §5A suggests a capability check.
- **Option C — both.** The launcher guarantees it, and suprasole marks anyway.

---

## 9. Interplay — Process Management × TTY Management
Two kernel subsystems govern a PTY session, and neither waits for the other:
- **Process management** (`exit`, `wait`, signals, ptrace) decides when the PTY process terminates and who may collect it. Suprasole sees it through the pidfd.
- **TTY management** (sessions, controlling terminal, hangup, line discipline) decides when the master reports `EIO`: when the **last slave descriptor** closes — held by any process, in any session (§6-Y).

The PTY process's termination and the slave's last close are different events. They have no fixed order (§6-W), and either can happen without the other.

### A. Actors
| Actor | Relationship |
|---|---|
| suprasole | Real parent of the PTY process; sole holder of the master and the pidfd. |
| PTY process | Session leader (`Setsid`); leader of its own process group (`PGID == PID`); controlling terminal = the slave (`Setctty`). |
| Its process group | It, plus children it runs without job control. This is the tty's foreground group unless a job-control shell changes it. |
| Job-control jobs | `cmd &` / `^Z` in an interactive shell: their own process group, same session. |
| Session leavers | `setsid`, daemons: a new session and no controlling terminal — but they may still hold slave fds they inherited. |
| Tracer | Nobody; suprasole (if the PTY process called `PTRACE_TRACEME`); or a foreign debugger (same uid, or `CAP_SYS_PTRACE`). |
| Slave holders | Any process with an open fd on the slave: descendants, session leavers, or outsiders that opened `/dev/pts/N` by path (§6-Y). |

### B. Process Events → What Suprasole Observes
| Event | Reported to | Worker's peek (`WEXITED\|WNOHANG\|WNOWAIT`) | State effect | Evidence |
|---|---|---|---|---|
| Exits / is killed, untraced | suprasole | pidfd readable; `CLD_EXITED` / `CLD_KILLED` / `CLD_DUMPED` | `Alive → Zombie → Reaped` | Q, L |
| Group-stop (`SIGSTOP`, `SIGTSTP`, `SIGTTIN`, `SIGTTOU`) or continue, untraced | suprasole only under `WSTOPPED` / `WCONTINUED`, which the worker does not pass | nothing — indistinguishable from running | none; the teardown probe observes it | P, H |
| Traced by suprasole: exec trap, signal-delivery stops, group-stops | suprasole, as tracer | `CLD_TRAPPED` → consumed by a `WSTOPPED`-only call | none | J, R |
| Traced by suprasole: `PTRACE_EVENT_*` stops, including `PTRACE_EVENT_EXIT` | — | never: event stops need options only the tracer can set (`PTRACE_SETOPTIONS`), and suprasole sets none. Untraced-by-options, `SIGKILL` goes straight to termination (R). | none | R |
| Traced by a foreign debugger: any stop, including `PTRACE_EVENT_EXIT` (debugger set `PTRACE_O_TRACEEXIT`) | the debugger only | nothing; pidfd not readable (the exit stop precedes termination); `/proc` `t`, `TracerPid` = debugger | `Alive` until released. Suprasole's `SIGKILL` cannot end an exit stop. | AA, AB, AC, AD |
| Traced by a foreign tracer: termination | the tracer first; suprasole only after the tracer collects or detaches | pidfd readable but nothing reportable → `Zombie_Withheld`; re-peek until released | `Alive → Zombie_Withheld → Zombie → Reaped` | V, AE |
| Enters `D` (uninterruptible) | — | nothing | `Alive` indefinitely (accepted cost) | — |
| `exec`s another program | — | nothing: same PID and pidfd, still the session leader | none | — |
| Collected by someone else (`SIGCHLD` = `SIG_IGN`, `wait4(-1)`) | — | `ECHILD` | `→ SystemError` | §5B.2 |

### C. TTY Events → What Each Side Observes
| Event | Kernel effect | Suprasole observes | Evidence |
|---|---|---|---|
| Last slave fd closes, by anyone | Master drains pending bytes, then `EIO` — reversible while the master stays open | `EIO` | E, Z |
| The PTY process (session leader) terminates | Its foreground group gets `SIGHUP`; **nobody else is signaled** | pidfd termination; `EIO` only if it was the last slave holder | X, T |
| …leaving a process group orphaned with a stopped member | That group gets `SIGHUP` + `SIGCONT` (POSIX orphaned-process-group rule) | — | T |
| Master closes (forced unplug) | Slave hangup: a still-living session leader gets `SIGHUP` + `SIGCONT`; every slave fd is hung up (reads EOF, writes `EIO`); nobody else is signaled; `/dev/pts/N` is removed | reader `os.ErrClosed` | B, T, Y |
| `TIOCSWINSZ` on the master | `SIGWINCH` to the foreground group | — | — |
| `^C` / `^\` / `^Z` written to the master | `SIGINT` / `SIGQUIT` / `SIGTSTP` to the foreground group | — | — |
| Any signal above while suprasole is the tracer | The PTY process enters a signal-delivery stop and stays there (§5B.1) | `CLD_TRAPPED`, consumed | J, R |

### D. Descendant Fates — Keyed by Kernel Properties, Not Shell Vocabulary
**Does suprasole need to care? Its code doesn't.** Nothing in the design classifies descendants. `_PtyTeardown_` treats every row below identically: if `EIO` hasn't come within `TIMEOUT_DRAIN__PtyProxy` of the PTY process's termination, it unplugs. This section exists for two reasons only:
- to explain user-visible behavior — why a terminal can outlive its shell for 100 ms, and why a `cmd &` server keeps running after `exit`;
- to inform the one policy decision (§8.1).
"Job", "job control", "disown" and "background" are **shell** bookkeeping, not kernel objects. An interactive shell puts each pipeline it launches into its own process group, makes one group the terminal's foreground group, and keeps a job table. `disown` merely deletes an entry from bash's table, and the kernel never learns of it. Suprasole never sees jobs, and the PTY process need not be a shell at all (`python`, `htop`, `vim`).

What decides a descendant's fate is four kernel properties, plus whatever policy the PTY process applies when *it* receives `SIGHUP`:
1. **Process group:** the PTY process's own group, or another. This decides whether `Signal(…, PIDFD_SIGNAL_PROCESS_GROUP)` and the foreground-group `SIGHUP` reach it.
2. **Session:** the PTY process's session, or another. This decides whether the orphaned-group rule and hangup semantics apply.
3. **Holds a slave fd:** this decides whether it blocks `EIO`.
4. **`SIGHUP` disposition and run state:** default, ignored, or stopped. This decides whether a hangup kills it.

| Kernel properties (group · session · slave · state) | Shell constructs that produce it | PTY process `exit`s | Terminate (`SIGHUP`+`SIGCONT` to the PTY process) | Forced unplug | Group `SIGKILL` | Blocks `EIO` |
|---|---|---|---|---|---|---|
| PTY's group · PTY's session · yes · default `SIGHUP` | `bash -c 'a & b'`; a program's worker children | dies: foreground-group `SIGHUP` (X) | dies the same way once the PTY process terminates | — | **reached** (M) | until it dies |
| PTY's group · PTY's session · yes · ignores `SIGHUP` | `trap '' HUP` before forking | survives (C) | survives; dies only if escalation runs, i.e. the PTY process itself is unresponsive (M) | survives; fds hung up | **reached** (C, M) | **yes** |
| own group · PTY's session · yes · running | interactive `cmd &` | **survives** — nothing signals it (T) | **PTY-process policy:** bash forwards `SIGHUP` to its job table → dies (U); a non-shell forwards nothing | survives; fds hung up (T) | not reached (T) | **yes** |
| own group · PTY's session · yes · running, removed from the shell's table | `cmd & disown` — **kernel-identical to the row above** | survives (T) | survives — bash's policy skips it (U) | survives; fds hung up (T, U) | not reached | **yes** |
| own group · PTY's session · yes · stopped | `^Z`, `kill -STOP` | dies — kernel orphaned-group rule: `SIGHUP`+`SIGCONT` (T) | dies — bash sends `SIGHUP`+`SIGCONT` (U); the kernel rule would anyway | — | not reached | no (it dies) |
| own group · PTY's session · **no** · ignores `SIGHUP` | `nohup cmd &` (GNU `nohup` redirects all three fds) | survives (T) | survives (U) | unaffected | not reached | no |
| own group · **new session** · yes | `setsid cmd &` with inherited stdio | survives (T) | survives (U) | survives; fds hung up (T, U) | not reached | **yes** |
| own group · new session · no | daemons; `setsid cmd </dev/null >/dev/null 2>&1 &` | unaffected (T) | unaffected (U) | unaffected | not reached | no |
| unrelated · other session · yes | anything of the user's that opens `/dev/pts/N` by path | unaffected | unaffected | not signaled; fd hung up (Y) | not reached | **yes** |

Reading the table by kernel property:
- Only processes in the **PTY process's own group** are reached by `Signal(…, PIDFD_SIGNAL_PROCESS_GROUP)` or the foreground-group `SIGHUP`.
- Only **stopped** members of the session are hit by the orphaned-group rule.
- Anything holding a **slave fd** blocks `EIO` until the forced unplug.
- Whether a *running* job dies on Terminate is the PTY process's policy, not the kernel's.

### E. Completeness — Every Combination Ends Once
| | Slave fully closed (`EIO`) | Slave still held (no `EIO`) |
|---|---|---|
| **PTY process terminated** | Normal end. `EIO` and `Channel_FinalState` reach `_PtyTeardown_` in either order (§6-W); it emits the outcome once both are in. | A lingering holder (§9D). `_PtyTeardown_`'s drain timer unplugs after `TIMEOUT_DRAIN__PtyProxy` → `os.ErrClosed` → outcome from `Reaped`. |
| **PTY process not terminated** | Rogue: it closed or abandoned the terminal but kept running. `_PtyTeardown_` closes the master → `TIMEOUT_UNRESPONSIVE` → observe → group `SIGKILL` → `TIMEOUT_UNINTERRUPTIBLE`. | A live session. Terminate: `SIGHUP`+`SIGCONT` → `TIMEOUT_UNRESPONSIVE` → unplug + observe + group `SIGKILL` → `TIMEOUT_UNINTERRUPTIBLE`. In `D` state or debugger-held → `_Unresponsive_Uninterruptible_` / `_Unresponsive_Debugged_`. |

- **Order is never assumed.** The kernel closes a dying process's descriptors before marking it a zombie, yet at user level the pidfd usually arrives first (29 of 30 — §6-W).
- **Exactly one ending per session.** Every cell ends in exactly one outcome (`_PtyTeardown_` emits it once, after PtyReader has returned once) and at most one final state (`RunWorker` writes it once — or never, for a `D` / debugger-held process, the accepted cost).
- **The master is closed on every path,** so `EIO`'s reversibility (§6-Z) cannot reopen a finished session.

---

## 10. Implementation Phases
| Phase | Scope | Status |
|---|---|---|
| Prep | Discard the uncommitted Terminate first pass, keeping only the termination-signal parameter purge that HEAD (commit 9055901) needs to compile | **Done** |
| Contract | `tests/contract/` — kernel contract suite, one test per "Depends" row of §5C | **Done:** 21 tests, stable |
| 1 | **Native PTY** (§3A): `Open_PtyDescriptor` / `Resize_PtyDescriptor` in `source/PtyDescriptor.go`; `Spawn_PtyProxy` hands the slave to the child and closes its own copy; `PtyResizer` resizes through `RawConn.Control`; `creack/pty` removed from `go.mod`. The forced unplug now works: closing the master unblocks the reader and hangs up the session leader. | **Done:** 4 tests in `tests/PtyDescriptor_test.go`, run by `tests/runTests.sh` |
| 2 | **`_PtyProcess_`** (§3B–D, §4): `syscall.ForkExec` + `SysProcAttr.PidFD` replaces `exec.Command`; the `Alive → (Zombie_Withheld →) Zombie → Reaped` worker replaces `spawnReaper_PtyProcess` and the stop recursion | **Done:** 16 tests in `tests/PtyProcess_test.go` (§6-AL) |
| 3 | **`_PtyTeardown_`** (§3E–F): absorbs the drain timer, `HandleExited`, `Terminate_PtyProcess`, `TerminateCancelled_PtyProxy`; Terminate becomes an order; `SIGHUP` + `SIGCONT`; `/proc` observation; unplug-only policy (§8.1) | Pending |

**Interim between phases** (deliberate, so each phase changes one thing):
- **Now commented out, pending phase 3** (each block tagged `// [phase 3]`): the interim teardown below — `HandleExited`, `TeardownExited_PtyProcess`, the `waitid(P_PID)` probe and the five `ProbeStatus_Unresponsive` methods, `Terminate_PtyProcess`, `TerminateCancelled_PtyProxy`, the `OnExited__` field and its wiring and call, the two `TIMEOUT_*` constants, and the three imports only they used. The build, vet and every test still pass, but **until phase 3 lands**: a shell's exit is never reported (the worker still reaps it, but the master stays open and the workspace stays active), Terminate does nothing, and a spawn cancelled mid-flight leaves its shell running. The teardown report types (`_TeardownReport_PtyProcess_`, `_Unresponsive_*`, `_SystemError__TeardownReport_`) stay live — §3G keeps them.
- **Phase 2 rewires today's teardown onto `Channel_FinalState`,** keeping its policy (phase 3 changes the policy):
  - `TeardownExited_PtyProcess` (after `EIO` or the master's close) waits up to `TIMEOUT_UNRESPONSIVE`; then, in one critical section, probes and sends `Signal(SIGKILL, PIDFD_SIGNAL_PROCESS_GROUP)` — the leader is always in its own group, so the separate leader kill is gone — and waits up to `TIMEOUT_UNINTERRUPTIBLE`. The former `killExited` / final-report helpers are folded in, so every lock is taken in the function's own body.
  - `TerminateCancelled_PtyProxy` waits up to `TIMEOUT_UNRESPONSIVE`, then sends `Signal(SIGKILL, PIDFD_SIGNAL_PROCESS_GROUP)` and returns: the worker reaps on its own, so there is nothing left to wait for.
  - **The `waitid(P_PID)` probe now runs under the Mutex, on the PID the state machine guarantees** (`ProbeStatus_Unresponsive` on each state), so it can no longer race the reap. It also reports `CLD_TRAPPED` (a self-traced shell in a signal-delivery-stop) as `Stopped`, keeping the outcome the deleted `_Stopped_Traced_` report produced. A debugger-held zombie still probes as a `SystemError` until phase 3's `/proc` observation (§3F).
  - **Deleted:** `spawnReaper_PtyProcess`, `__selectReapResult__PtyProcess`, `__kill__PtyProcess_Unreaped`, the `selectReapResult_*` / `kill*__PtyProcess_Unreaped` helpers, `_ReapResult_PtyProcess_`, `_Reaped__TeardownReport_PtyProcess_` (the final states resolve the outcome themselves) and `_Stopped_Traced__TeardownReport_PtyProcess_`. `_SigInfo_Waitid_` and `CLD_STOPPED__SIGCHLD__Linux` moved to `PtyProcess.go`.
- `Terminate_PtyProcess` still sends `SIGTERM`, now as a pidfd group signal; an idle interactive bash ignores it (§6-G). Phase 3 replaces it with a Terminate order to `_PtyTeardown_`.
- `TerminateCancelled_PtyProxy` already benefits from phase 1: its master close now really hangs up the shell.

