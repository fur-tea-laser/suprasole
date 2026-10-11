* **PtyExec charter:** `suprasole-pty-exec`, the spawn helper. It turns a freshly forked child into the PTY program. `launchPtyExec__Command_PtyProcess` `ForkExec`s it (briefing: `launchPtyExec.md`); it does the PTY setup, then `execve`s the shell, reporting the exact step if anything fails (issue §3B).
  * **Why a separate program:** every step that can block on a client-chosen path (`chdir`, `execve`) runs here, outside Go's vfork window. A hang blocks only this process and the spawning goroutine; the server itself never freezes. Nothing kills a hung helper yet: its pidfd stays inside the blocked launch, and there is no timeout (`launchPtyExec.md` → Known limits). See `PtyProcess.md` → `Spawn_PtyProcess`.
  * **No Go runtime ever runs:**
    * Built with `go build -ldflags='-E main.Entry_PtyExec'`, which makes the ELF entry point the assembly function `Entry_PtyExec`.
    * The runtime is still linked in, which is why the file is about 1.5 MB, but it is never initialized: no threads, no signal handlers, no garbage collector. `execve` maps the file and faults in only the pages it touches.
    * Measured on the dev machine: fork + exec + wait of a minimal entry-point binary is about 81 µs. A spawn adds one `execve` of the helper, which is less than that.
  * **Built with the Go toolchain only:** `source/PtyExec/PtyExec.go` declares `Entry_PtyExec()` and an empty `main`.
    * `main` exists only so the package builds. It runs only if the binary is built without `-E`.
    * **Built without `-E`, every spawn falsely succeeds.** The empty `main` exits 0, which closes fd 3 without a report, so `launchPtyExec__Command_PtyProcess` sees `ABSENT` and returns the process as started. The worker then reaps an exit code of 0 and resolves the PTY as `Success`, at once and silently. Nothing detects this at run time: building with `-E` is the build's and launcher's responsibility (issue §5A.3).
    * `go vet`'s `asmdecl` check verifies the assembly against the declaration.
    * The `_linux_arm64.s` suffix builds the assembly only for linux/arm64, the one supported platform.
  * **Contract with `Spawn_PtyProcess`:**
    * `argv` = `[helper, dir, path, shell argv…]`. An empty `dir` means "do not `chdir`", so the shell starts in the server's working directory, as `exec.Cmd` does with an empty `Dir`.
    * `envp` = the shell's environment, passed to `execve` unchanged.
    * fds 0–2 = the slave; fd 3 = the write end of the report pipe.
    * **Report:** on failure, one 16-byte `write` to fd 3 of `{step, errno}` (two little-endian `uint64`s), then `exit_group(127)`. On success, fd 3 is close-on-exec, so the shell's `execve` closes it and the server reads EOF.
  * **Registers at entry:** the kernel starts the process with `RSP` → `argc`, then the `argv` pointers, a NULL, the `envp` pointers, a NULL.
    * `R19` = `argc`.
    * `R20` = `&argv[0]`.
    * `R21` = `&envp[0]` = `R20 + (argc + 1) × 8`.
    * `R22` = the current step, kept for the report. `R23` / `R24` assemble the report.
    * No function is ever called, so no register needs saving.
  * **Error check:** each syscall returns `-errno` in `[-4095, -1]` on failure. `CMN $4095, R0` sets the carry exactly then, so `BCS` branches to the failure path with `R0` still holding `-errno`.
  * **Steps, in order** (the step numbers mirror `_Step__PtyExec_` in `PtyProcess.go`):

    | Label | Syscall | Fails when | Reachable from `Spawn_PtyProcess`? |
    |---|---|---|---|
    | `closeOnExec__Report` | `fcntl(3, F_SETFD, FD_CLOEXEC)` | fd 3 is not open | **No**: fd 3 is always passed. It fails only for a manual run, and then there is nowhere to report, so it exits 127 silently. |
    | `checkCount__Arguments` (1, `ARGUMENTS`) | none: `argc ≥ 4` | fewer arguments; reports `EINVAL` | **No**: `Spawn_PtyProcess` always passes at least 4. Manual runs only. |
    | `setsid` (2) | `setsid()` | the caller already leads a group, or a group with its PID exists | **No**: the helper is a fresh child in the server's process group, and its PID was just allocated. |
    | `tiocsctty` (3) | `ioctl(0, TIOCSCTTY, 0)` | not a session leader, already has a controlling terminal, the terminal belongs to another session, or fd 0 not open for reading; `ENOTTY` if fd 0 is not a terminal | **No**: `setsid` just ran, and fd 0 is a fresh slave opened read-write that was never anyone's controlling terminal. A security module (SELinux, TOMOYO; Landlock from 6.10) could deny the ioctl itself; the launcher guarantees none does (issue §5A.2). A manual run from a non-terminal gets `ENOTTY`. The argument `0` means never take the terminal from another session (Go passes `1`, which lets `CAP_SYS_ADMIN` steal it). |
    | `chdir` (4) | `chdir(argv[1])`, skipped when `argv[1]` is empty | `ENOENT`, `ENOTDIR`, `EACCES`, `ELOOP`, `ENAMETOOLONG`, `EIO`, `ENOMEM` | **Yes**: the client chooses the directory. |
    | `rtSigprocmask` (5) | `rt_sigprocmask(SIG_SETMASK, &∅, NULL, 8)` | bad `how`, size or pointer | **No**: all three are constants. This makes the shell's empty signal mask a stated fact rather than whatever the server's thread had. |
    | `execve` (6) | `execve(argv[2], &argv[3], envp)` | `ENOENT`, `EACCES`, `ENOEXEC`, `E2BIG`, `ETXTBSY`, `ELOOP`, `ENAMETOOLONG`, `ENOMEM`, `EPERM`, `ELIBBAD`, `EIO`, `EMFILE` | **Yes**: the client chooses the path and environment. It returns only on failure, so execution falls through to the report. |

  * **`report__Failure`:** stores `{R22, -R0}` in `report__PtyExec`, a 16-byte `NOPTR` data symbol (the helper never touches its stack), and `write`s it to fd 3. A failed `write` is ignored: there is nothing else to tell, and the exit follows.
  * **`exit__Failure`:** `exit_group(127)`, in a loop, because `exit_group` never returns. 127 is the shell convention for "command could not be executed". The server does not rely on it; the report carries the cause.
  * **What it deliberately does not do (open decisions):**
    * **Reset signal dispositions:** a signal the server ignores (`SIG_IGN`) stays ignored through `execve`. For example, a server started under `nohup` keeps `SIGHUP` ignored (Go honors an inherited `SIG_IGN` for `SIGHUP` / `SIGINT`), so the shell would ignore the `SIGHUP` that Terminate sends. `ForkExec` behaved the same way. One more step — `rt_sigaction(sig, SIG_DFL)` for every signal except `SIGKILL` / `SIGSTOP` — would make the shell's dispositions stated rather than inherited.
    * **Close inherited descriptors:** `close_range(3, ~0U, 0)` before `execve` would settle issue §8.4 on the server's side; that question is kept open between server and launcher.
