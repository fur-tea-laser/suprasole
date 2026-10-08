package main

import (
	_ERRORS "errors"
	_OS "os"
	_SYSCALL "syscall"
	_TIME "time"
)

// type _PtyProcess_ = _OS.Process

const (
	TIMEOUT_UNRESPONSIVE__PtyProcess    = 1 * _TIME.Second
	TIMEOUT_UNINTERRUPTIBLE__PtyProcess = 1 * _TIME.Second
)

func spawnReaper_PtyProcess(
	PtyProcess_this *_OS.Process,
) chan _ReapResult_PtyProcess_ {
	channel_reapResult__PtyProcess := make(chan _ReapResult_PtyProcess_, 1)
	go func() {
		processState, error_wait := PtyProcess_this.Wait()
		channel_reapResult__PtyProcess <- _ReapResult_PtyProcess_{
			ProcessState_maybe: processState,
			Error_Wait_maybe:   error_wait,
		}
	}()
	return channel_reapResult__PtyProcess
}

func __selectReapResult__PtyProcess[__SelectResult__ any](
	channel_reapResult__PtyProcess <-chan _ReapResult_PtyProcess_,
	timer_Unresponsive *_TIME.Timer,
	onSystemError__ func(error_wait error) __SelectResult__,
	onReaped__ func(processState *_OS.ProcessState) __SelectResult__,
	onStopped_Ptrace_Exit func(waitStatus_PtyProcess _SYSCALL.WaitStatus) __SelectResult__,
	onStopped_Ptrace func(waitStatus_PtyProcess _SYSCALL.WaitStatus) __SelectResult__,
	onUnresponsive func() __SelectResult__,
) __SelectResult__ {
	select {
	case reapResult_only := <-channel_reapResult__PtyProcess:
		if _ERRORS.Is(reapResult_only.Error_Wait_maybe, _SYSCALL.ECHILD) || _ERRORS.Is(reapResult_only.Error_Wait_maybe, _SYSCALL.EINVAL) || _ERRORS.Is(reapResult_only.Error_Wait_maybe, _SYSCALL.EBADF) {
			return onSystemError__(reapResult_only.Error_Wait_maybe)
		} else if nil == reapResult_only.ProcessState_maybe {
			panic("invalid path: __selectReapResult__PtyProcess [NIL_PROCESS_STATE]")
		}
		waitStatus_PtyProcess := reapResult_only.ProcessState_maybe.Sys().(_SYSCALL.WaitStatus)
		if false == waitStatus_PtyProcess.Stopped() {
			return onReaped__(reapResult_only.ProcessState_maybe)
		} else if waitStatus_PtyProcess.Stopped() && _PTRACE_EVENT_EXIT == waitStatus_PtyProcess.TrapCause() {
			return onStopped_Ptrace_Exit(waitStatus_PtyProcess)
		} else if waitStatus_PtyProcess.Stopped() {
			return onStopped_Ptrace(waitStatus_PtyProcess)
		} else {
			panic("invalid path: __selectReapResult__PtyProcess [INVALID_WAIT_STATUS]")
		}
	case <-timer_Unresponsive.C:
		return onUnresponsive()
	}
}

func __kill__PtyProcess_Unreaped[__KillResult__ any](
	PtyProcess_this *_OS.Process,
	channel_reapResult__PtyProcess__leading <-chan _ReapResult_PtyProcess_,
	onSystemError__ func(error_wait error) __KillResult__,
	onReaped__ func(processState *_OS.ProcessState) __KillResult__,
	onUnresponsive func() __KillResult__,
) __KillResult__ {
	pid_PtyProcess := PtyProcess_this.Pid
	_ = _SYSCALL.Kill(-pid_PtyProcess, _SYSCALL.SIGKILL)
	_ = _SYSCALL.Kill(pid_PtyProcess, _SYSCALL.SIGKILL)
	timer__Unresponsive_Uninterruptible := _TIME.NewTimer(TIMEOUT_UNINTERRUPTIBLE__PtyProcess)
	return __selectReapResult__PtyProcess(
		channel_reapResult__PtyProcess__leading,
		timer__Unresponsive_Uninterruptible,
		onSystemError__,
		onReaped__,
		func(_ _SYSCALL.WaitStatus) __KillResult__ {
			return __selectReapResult__PtyProcess(
				spawnReaper_PtyProcess(PtyProcess_this),
				timer__Unresponsive_Uninterruptible,
				onSystemError__,
				onReaped__,
				func(_ _SYSCALL.WaitStatus) __KillResult__ {
					panic("invalid path: __kill__PtyProcess_Unreaped [STOPPED_PTRACE_EXIT -> STOPPED_PTRACE_EXIT]")
				},
				func(_ _SYSCALL.WaitStatus) __KillResult__ {
					panic("invalid path: __kill__PtyProcess_Unreaped [STOPPED_PTRACE_EXIT -> STOPPED_PTRACE]")
				},
				onUnresponsive,
			)
		},
		func(_ _SYSCALL.WaitStatus) __KillResult__ {
			return __selectReapResult__PtyProcess(
				spawnReaper_PtyProcess(PtyProcess_this),
				timer__Unresponsive_Uninterruptible,
				onSystemError__,
				onReaped__,
				func(_ _SYSCALL.WaitStatus) __KillResult__ {
					return __selectReapResult__PtyProcess(
						spawnReaper_PtyProcess(PtyProcess_this),
						timer__Unresponsive_Uninterruptible,
						onSystemError__,
						onReaped__,
						func(_ _SYSCALL.WaitStatus) __KillResult__ {
							panic("invalid path: __kill__PtyProcess_Unreaped [STOPPED_PTRACE -> STOPPED_PTRACE_EXIT -> STOPPED_PTRACE_EXIT]")
						},
						func(_ _SYSCALL.WaitStatus) __KillResult__ {
							panic("invalid path: __kill__PtyProcess_Unreaped [STOPPED_PTRACE -> STOPPED_PTRACE_EXIT -> STOPPED_PTRACE]")
						},
						onUnresponsive,
					)
				},
				func(_ _SYSCALL.WaitStatus) __KillResult__ {
					panic("invalid path: __kill__PtyProcess_Unreaped [STOPPED_PTRACE -> STOPPED_PTRACE]")
				},
				onUnresponsive,
			)
		},
		onUnresponsive,
	)
}
