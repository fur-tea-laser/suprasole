# Spawn Hardening — Isolating Client-Controlled fork/exec

> **What this is.** The technical plan for the PTY **spawn path**: the `suprasole-pty-exec` helper's internals, the move off `syscall.ForkExec`'s vfork to a real fork whose copy execs the helper at once, and the decisions around it. Extracted 2026-10-10 from `pidfd-refactor-pre-phase-3.md` (§3B, §5A.3) so that document carries the spawn only as an **interface and responsibilities**. Everything here is spawn-path *mechanism*.
>
> **Status.** A deferred chunk of work, to be done **after** the pre-phase-3 work (phases 1–3). The helper exists and is used by phase 2 (§2.1); the hybrid fork (§3) and everything downstream of it is **not implemented**.
>
> **Relationship to pre-phase-3.** `pidfd-refactor-pre-phase-3.md` keeps, and remains the owner of:
> - the helper's **contract** — the server execs it with the slave on fds 0–2 and a close-on-exec report pipe on fd 3; it does `setsid` → `TIOCSCTTY` → `chdir` → reset signal dispositions → `execve` of the shell; it reports the failed `{step, errno}` over fd 3, or closes it cleanly at `execve`; it is collected through its pidfd, holding no OS thread (§3B);
> - the **server-side spawn** — command preparation, the NUL refusal, pidfd ownership and pollability, the abandon reaper, the worker hand-off (§3B);
> - the **unified spawn lifecycle** this hardening enables — the `Spawning` / `Launching` / `NeverStarted` states (§3H).
>
> **Cross-references.** Bare `§` references (`§3B`, `§3H`, `§5A`, `§6-W`, `§5C`, …) point into `pidfd-refactor-pre-phase-3.md` unless stated otherwise. Companion code-level docs: `documentation/PtyExec.md`, `documentation/launchPtyExec.md`.

---

## 1. The hazard this hardens against

**`ForkExec` vforks.** The forking thread cannot be preempted until the child execs. So a child blocked before `execve` — `chdir` into, or `execve` of, a client-chosen path on a hung network filesystem — stalls every stop-the-world and freezes the whole server. Verified: Go 1.27.1 `syscall_runtime_BeforeFork` sets `m.locks++`; `rawVforkSyscall` never enters the scheduler's syscall state; `canPreemptM` refuses `locks > 0`; `stopTheWorldWithSema` loops until every running P stops. Kernel v6.9: the parent is released only at the child's `exec_mm_release` or `exit_mm`.

The helper (§2) already removes the *client's* steps from that window — `ForkExec` now only execs a small local binary, so a hung client path blocks only the helper and the spawning goroutine, not the server's runtime. But `ForkExec` still vforks to launch the helper itself; §3 removes that last window too.

---

## 2. The helper (`suprasole-pty-exec`)

### 2.1 The helper as built today (phase 2)
- **A separate process with no Go runtime.** Its ELF entry point is assembly, set with `-ldflags=-E`, so it carries no Go runtime and shares nothing with the server. A hung helper blocks only itself and the spawning goroutine; no timeout or kill path exists yet (`documentation/launchPtyExec.md` → Known limits).
- **The `{step, errno}` report.** The helper reports the exact failed step and errno over a close-on-exec pipe on fd 3, read through the netpoller — in the same family as the reports of Go, Rust (`Command::spawn`) and VTE. So a bad `Dir` is `chdir <dir>: …`, no longer `fork/exec <shell>: …`. A clean `execve` closes fd 3 with no report. A failed helper is collected through its pidfd, not a blocking `wait4`; no OS thread is ever held.
- **Signal-disposition reset (done 2026-10-10).** The helper resets every signal disposition to default, except `SIGKILL` and `SIGSTOP`. If that step ever failed, the helper would report it as step 7.
  - **Still to do for that item:** step 7's constant in `_Step__PtyExec_`, and the companion documents, which still describe the helper before the reset (`documentation/launchPtyExec.md` lists ignored signals as a known limit).
- **Cost.** One extra `execve` per spawn (under ~81 µs on the dev machine), and a second installed file (§5A.3).

### 2.2 How it is built and supplied
- **The launcher supplies the helper's path:** `-Path_PtyExec <path>`. The server takes it as given, the way it takes `-Address_HttpServer`; it never guesses a location (§5A.3). Like the kernel floor, the path is the launcher's guarantee — present and valid — and the server does not check it at start-up. A wrong path fails each spawn with `fork/exec <path>: no such file or directory`.
- **Build:** the helper is built with the Go toolchain alone: `go build -ldflags='-E main.Entry_PtyExec' -o <path> ./source/PtyExec`. Without `-E`, the binary is a Go program whose empty `main` exits 0. That closes fd 3 without a report, so every spawn appears to succeed and resolves as `Success` at once, silently; nothing detects it at run time.

---

## 3. Hybrid spawn — real fork, immediate exec of the helper *(decided 2026-10-10, not implemented)*

The server forks (no vfork), and its copy execs the helper at once. This supersedes the `ForkExec` part of §2; the helper itself stays. This hybrid is systemd's design with a real fork instead of vfork.

### 3.1 Why no vfork at all
Any vfork window makes the server wait on a child. Erlang's `erl_child_setup` source gives the same reason: "if the program that is to be executed is on an NFS that is unavailable, the execv can block for a very long time".

### 3.2 Why fork the server, and why exec the helper at once
- A real fork never makes the server wait: the parent returns as soon as the copy exists.
- Until it execs, the copy shares the server's pages copy-on-write. If it hung on a client path, every page the server wrote meanwhile would be duplicated, and memory would creep toward 2×.
- So the copy touches no client path. It only puts descriptors in place and execs the helper. The helper — small, fresh, sharing nothing with the server — then does `chdir` and the shell's `execve`. A dead client filesystem hangs only the helper.
- Precedent, all checked in source: tmux, node-pty (VS Code), kitty, Alacritty and VTE (GNOME Terminal) fork a copy. tmux and node-pty never wait for the exec; VTE waits on a report pipe with a timeout. tmux calls `chdir` in the server itself before forking, so a dead working directory hangs the whole tmux server. That is the pattern this design avoids.
- **Exec first, then set up:** no terminal emulator does this; all of them call `chdir` in the forked copy, and tmux in the server itself. Process managers that fork from a large parent do it:
  - systemd since v255 spawns, immediately execs its internal `systemd-executor` binary, and lets that binary do the setup (working directory included) before exec'ing the service. Its NEWS gives the reasons: copy-on-write traps, the child carrying "the memory footprint … of the manager, but with the memory limits of the service", and APIs "not supposed to be used after a fork but before an exec". systemd still uses vfork for that first exec, but the executor is pinned by file descriptor.
  - runc re-execs itself (`runc init`), which does the `chdir` and the rest before exec'ing the container's process.
  - Erlang's `erl_child_setup` is exec'd once at boot and does every later fork and `chdir`.

### 3.3 What a hung `chdir` or `execve` is (verified at v6.9)
`ps` shows both of the last two as `D`:
- **Interruptible sleep: any signal ends it.** Example: FUSE's first wait phase.
- **Killable sleep: only `SIGKILL` ends it.** NFS RPC waits (`rpc_wait_bit_killable`); page-cache reads of the executable; autofs mounts; fanotify permission checks, such as an antivirus scanning a binary on exec; and FUSE requests the daemon has not yet taken.
- **Uninterruptible sleep: nothing ends it until the kernel work completes.** A FUSE request the daemon already took ("Wait it out"), or a path lookup waiting for a directory's lock. `lookup_slow` takes `inode_lock_shared`, a plain `down_read`, so it waits behind another task stuck on the same dead filesystem.
- **Slow is not the same as dead:** an automount or an antivirus scan can take seconds and then finish.
- **Consequence:** a `SIGKILL` timeout ends most hangs, but not the last kind. That is why the hang must sit in a small, isolated process. We can always stop *waiting* for the helper (the reaper polls without holding a thread), even when we cannot kill it.

### 3.4 Mechanism, server side (arm64 assembly)
- **Legacy `clone`, not `clone3`:** Docker's default seccomp profile answers `clone3` with `ENOSYS` (measured in the dev container). So the fork is `clone(SIGCHLD | CLONE_PIDFD)`, with the pidfd returned through the `parent_tid` argument, as Go's `ForkExec` does it.
- **Signals:** legacy `clone` has no `CLONE_CLEAR_SIGHAND`, so the copy would inherit Go's handlers. The forking thread therefore blocks all signals around the `clone` and restores its mask afterwards, which is Go's own `BeforeFork` discipline. The copy stays fully blocked until its exec discards the handlers. The blocked mask passes into the helper, which clears it as its last step before the shell's exec.
- **The copy runs only assembly:** `dup3` the slave onto fds 0–2 and the report pipe's write end onto fd 3, then `execve` the helper. On failure it writes `{HELPER_EXEC (a new step), errno}` to the report pipe and calls `exit_group(127)`. Before forking, the server makes sure both descriptors are ≥ 4, so the `dup3` calls cannot collide.
- **Cost:** the parent's thread is busy only for the `clone` itself, which copies page tables in proportion to the server's memory. Measured (fork, child exit, wait): 87 µs at 3 MB RSS, 232 µs at 69 MB, 1.0 ms at 262 MB, 8.2 ms at 1 GB. Measured footprint: idle about 7 MB; a terminal with its default 10,000-line scrollback full holds 11.7 MB at 80 columns, 27.5 MB at 200.
- **Committed memory** (`mm/util.c`, v6.9): under strict overcommit (`vm.overcommit_memory=2`) the copy is charged the server's private writable memory, and a fork can fail with `ENOMEM`, loudly. Modes 0 and 1 do not refuse it in practice.

### 3.5 Helper change
Reset every signal disposition to default except `SIGKILL` and `SIGSTOP`, as node-pty and VTE do (VTE's comment: the parent "may have changed one to be ignored"). Everything else stays as today. (This is the step already done in phase 2 — §2.1.)

### 3.6 What goes away, what stays
- **What goes away:** Go's `syscall.ForkExec` in the spawn path, and with it the vfork window and Go's internal `Wait4`.
- **What stays:**
  - the helper binary (§5A.3);
  - the `{step, errno}` report over a close-on-exec pipe, in the same family as the reports of Go, Rust (`Command::spawn`) and VTE;
  - pidfd ownership, exclusive reaping, and `Spawn_PtyProcess`'s decision chain.

---

## 4. Open decisions

- **Ending a hung helper (finding 1).** The candidate is VTE's model: wait on the report pipe with a timeout and a cancel handle, then kill and reap. We would kill with `SIGKILL` through the pidfd (`PIDFD_SIGNAL_THREAD_GROUP`), because it is the only signal that cannot be blocked, and the helper keeps its signals blocked until just before the shell's exec. VTE sends `SIGHUP`, and guards against hitting its own process group before the child's `setsid`. The timeout value and the cancel wiring are still to be chosen.
- **`chdir` failure policy: decided 2026-10-10, refuse.** The spawn fails with `chdir <dir>: errno`. A fallback, if ever wanted, must be explicit and reported (VTE's model), never silent: tmux, kitty and Alacritty start the shell somewhere the client did not ask for, and still report success.
- **`E2BIG`: decided 2026-10-10. The launcher probes the limit, and the server checks in two tiers before forking.**
  - **ABI versus policy.** The layout of a new process's start-up data is ABI and stable: each string costs its length plus a NUL, each pointer 8 bytes, and the exec'd filename counts as a string. The *limit* is kernel policy: at v6.9 it is `max(128 KiB, min(6 MiB, RLIMIT_STACK / 4))`, plus 128 KiB per string. The server does not copy that policy. The launcher measures it, by exec'ing a trivial program with growing arguments until `E2BIG`, under the same `RLIMIT_STACK` it gives the server, and passes the results as flags.
  - **Tier 1, the client's command alone,** sized as the shell's own exec will see it. Too large → `fork/exec <shell>: argument list too long`.
  - **Tier 2, the helper-augmented exec:** add the directory, `/dev/fd/<n>`, the helper's `argv[0]` and 3 pointers. Too large → an error saying the launch overhead tipped the command over the limit.
  - **After both pass, `E2BIG` at either exec is unreachable,** so it is asserted. If it ever happens, the launcher's number was wrong, which is a broken launcher guarantee, like the kernel floor.
- **Pin the helper by file descriptor (systemd's approach), decided 2026-10-10.** At startup the server opens `-Path_PtyExec` once, close-on-exec, and the forked copy runs `execveat(fd, "", argv, envp, AT_EMPTY_PATH)`. The descriptor names the file itself, not its path: the server always runs the exact helper it opened at startup, whatever later happens to that path on disk.
  - The copy then does no path lookup at all, and the helper cannot be swapped or removed on disk while the server runs.
  - `execveat` is allowed under Docker's default seccomp profile (tested in the dev container, unlike `clone3`).
  - Because the open happens at startup, a bad path stops the server from starting instead of failing every spawn.

---

## 5. Rejected alternatives
- **A Go helper:** it costs a Go runtime start-up (~387 µs measured, against ~81 µs for the entry-point binary), and a client `GODEBUG` would configure it.
- **Our own `clone3` routine in assembly inside the server:** it is assembly, and it either copies page tables (a real fork) or keeps the freeze (vfork).
- **`nosplit` Go plus the runtime's fork hooks (`go:linkname`):** these are kept stable only because gVisor uses them.
- **cgo:** rejected outright; Go toolchain only.
