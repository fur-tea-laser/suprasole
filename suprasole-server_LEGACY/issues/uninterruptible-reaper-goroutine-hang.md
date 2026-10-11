# Issue: Uninterruptible Process Reaper Goroutine Hang

## Problem
When a child process enters uninterruptible kernel sleep (`TASK_UNINTERRUPTIBLE` / `D` state), it ignores all signals including `SIGKILL`. If teardown escalation timers expire, the dispatcher proceeds with an unresponsive error state, but the background reaper goroutine remains indefinitely parked in `os.Process.Wait()` / `wait4(2)` because Go cannot interrupt a blocking wait syscall or externally terminate a goroutine.

## Context & Invariants
- Each reaper owns an isolated, 1-slot buffered channel (`make(chan _ReapResult_PtyProcess_, 1)`), ensuring no send deadlock or stale channel pollution if the kernel eventually resolves the wait and reaps the process later.
- If the child never exits, the goroutine remains permanently parked in the kernel wait queue with 0 CPU overhead until system shutdown.
