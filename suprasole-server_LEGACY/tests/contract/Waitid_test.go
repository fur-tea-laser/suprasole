package contract

import (
	_RUNTIME "runtime"
	_SYSCALL "syscall"
	_TESTING "testing"
)

// §5C, waitid(2) WNOHANG: si_pid == 0 (and no error) while there is nothing to report. (§6-P)
func TestWaitid_WnohangReportsNothingWhileAlive(t *_TESTING.T) {
	child := spawn__Child(
		t,
		[]string{"sleep", "30"},
		nil,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	sigInfo_peek, errorCode__WAITID := peek__Pidfd(child.Pidfd)
	if errorCode__WAITID != 0 || sigInfo_peek.Pid != 0 {
		t.Fatalf(
			"peek on a live child = si_pid %d errno %v, want 0 / none",
			sigInfo_peek.Pid,
			errorCode__WAITID,
		)
	}
}

// §5C, waitid(2) WNOWAIT "leave the child in a waitable state": the peek reports a
// termination without collecting it; only the reap collects. (§6-Q)
func TestWaitid_PeekDoesNotCollectReapDoes(t *_TESTING.T) {
	child := spawn__Child(
		t,
		[]string{"sh", "-c", "exit 3"},
		nil,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	sigInfo_first := awaitTermination__Pidfd(
		t,
		child.Pidfd,
	)
	if _CLD_EXITED != sigInfo_first.Code || 3 != sigInfo_first.Status {
		t.Fatalf(
			"peek = code %d status %d, want CLD_EXITED/3",
			sigInfo_first.Code,
			sigInfo_first.Status,
		)
	}
	state_afterPeek := readState__Proc(child.Pid)
	if "Z" != state_afterPeek {
		t.Fatalf(
			"after peek /proc state = %q, want Z (not collected)",
			state_afterPeek,
		)
	}
	sigInfo_second, errorCode__WAITID__Second := peek__Pidfd(child.Pidfd)
	if errorCode__WAITID__Second != 0 || sigInfo_second.Code != sigInfo_first.Code || sigInfo_second.Status != sigInfo_first.Status {
		t.Fatalf(
			"second peek = code %d status %d errno %v, want identical to the first",
			sigInfo_second.Code,
			sigInfo_second.Status,
			errorCode__WAITID__Second,
		)
	}
	sigInfo_reaped, errorCode__WAITID__Reap := reap__Pidfd(child.Pidfd)
	if errorCode__WAITID__Reap != 0 || _CLD_EXITED != sigInfo_reaped.Code || 3 != sigInfo_reaped.Status {
		t.Fatalf(
			"reap = code %d status %d errno %v, want CLD_EXITED/3",
			sigInfo_reaped.Code,
			sigInfo_reaped.Status,
			errorCode__WAITID__Reap,
		)
	}
	state_afterReap := readState__Proc(child.Pid)
	if "gone" != state_afterReap {
		t.Fatalf(
			"after reap /proc state = %q, want gone",
			state_afterReap,
		)
	}
	_, errorCode__WAITID__SecondReap := reap__Pidfd(child.Pidfd)
	if _SYSCALL.ECHILD != errorCode__WAITID__SecondReap {
		t.Fatalf(
			"second reap = %v, want ECHILD",
			errorCode__WAITID__SecondReap,
		)
	}
}

// §5C, waitid(2): only WEXITED|WSTOPPED|WCONTINUED select reports, and the worker passes only
// WEXITED — so an untraced child's stop and continue are invisible to it. (§6-P)
func TestWaitid_UntracedStopAndContinueInvisibleToTerminationPeek(t *_TESTING.T) {
	child := spawn__Child(
		t,
		[]string{"sleep", "30"},
		nil,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	errorCode__SIGSTOP := sendSignal__Pidfd(
		child.Pidfd,
		_SYSCALL.SIGSTOP,
		0,
	)
	if errorCode__SIGSTOP != 0 {
		t.Fatalf(
			"SIGSTOP: %v",
			errorCode__SIGSTOP,
		)
	}
	eventually__Condition(
		t,
		"/proc state T",
		func() bool {
			return "T" == readState__Proc(child.Pid)
		},
	)
	sigInfo_stopped, errorCode__WAITID__Stopped := peek__Pidfd(child.Pidfd)
	if errorCode__WAITID__Stopped != 0 || sigInfo_stopped.Pid != 0 {
		t.Fatalf(
			"peek on a stopped child = si_pid %d code %d errno %v, want nothing",
			sigInfo_stopped.Pid,
			sigInfo_stopped.Code,
			errorCode__WAITID__Stopped,
		)
	}
	errorCode__SIGCONT := sendSignal__Pidfd(
		child.Pidfd,
		_SYSCALL.SIGCONT,
		0,
	)
	if errorCode__SIGCONT != 0 {
		t.Fatalf(
			"SIGCONT: %v",
			errorCode__SIGCONT,
		)
	}
	eventually__Condition(
		t,
		"/proc state not T",
		func() bool {
			return "T" != readState__Proc(child.Pid)
		},
	)
	sigInfo_continued, errorCode__WAITID__Continued := peek__Pidfd(child.Pidfd)
	if errorCode__WAITID__Continued != 0 || sigInfo_continued.Pid != 0 {
		t.Fatalf(
			"peek on a continued child = si_pid %d code %d errno %v, want nothing",
			sigInfo_continued.Pid,
			sigInfo_continued.Code,
			errorCode__WAITID__Continued,
		)
	}
}

// §5C, waitid(2) + ptrace(2): when suprasole is the tracer (child called PTRACE_TRACEME),
// trap-stops are reported even under WEXITED alone and stay pending under WNOWAIT; a
// WSTOPPED-only wait consumes them and can never collect — not even a zombie. (§6-J, §6-R)
// Its exact return on a zombie (ECHILD, §6-AF) is deliberately not asserted: not relied on.
func TestWaitid_TracedTrapConsumedByStopOnlyWaitWhichNeverCollects(t *_TESTING.T) {
	_RUNTIME.LockOSThread() // the forking thread becomes the tracer
	defer _RUNTIME.UnlockOSThread()
	child := spawn__Child(
		t,
		[]string{"sleep", "30"},
		nil,
		&_SYSCALL.SysProcAttr{Setsid: true, Ptrace: true},
	)
	var sigInfo_trap _SigInfo_Waitid_
	eventually__Condition(
		t,
		"exec trap reported",
		func() bool {
			sigInfo_peek, errorCode__WAITID := peek__Pidfd(child.Pidfd)
			if errorCode__WAITID != 0 {
				t.Fatalf(
					"peek: %v",
					errorCode__WAITID,
				)
			}
			sigInfo_trap = sigInfo_peek
			return sigInfo_peek.Pid != 0
		},
	)
	if _CLD_TRAPPED != sigInfo_trap.Code || int32(_SYSCALL.SIGTRAP) != sigInfo_trap.Status {
		t.Fatalf(
			"first report = code %d status %d, want CLD_TRAPPED/SIGTRAP",
			sigInfo_trap.Code,
			sigInfo_trap.Status,
		)
	}
	sigInfo_trap__Again, _ := peek__Pidfd(child.Pidfd)
	if _CLD_TRAPPED != sigInfo_trap__Again.Code {
		t.Fatalf(
			"second peek = code %d, want the trap still pending (WNOWAIT)",
			sigInfo_trap__Again.Code,
		)
	}
	sigInfo_consumed, errorCode__WAITID__Consume := waitid__Pidfd(
		child.Pidfd,
		_SYSCALL.WSTOPPED|_SYSCALL.WNOHANG,
	)
	if errorCode__WAITID__Consume != 0 || _CLD_TRAPPED != sigInfo_consumed.Code {
		t.Fatalf(
			"WSTOPPED-only wait = code %d errno %v, want CLD_TRAPPED",
			sigInfo_consumed.Code,
			errorCode__WAITID__Consume,
		)
	}
	sigInfo_afterConsume, errorCode__WAITID__AfterConsume := peek__Pidfd(child.Pidfd)
	if errorCode__WAITID__AfterConsume != 0 || sigInfo_afterConsume.Pid != 0 {
		t.Fatalf(
			"peek after consuming = si_pid %d errno %v, want nothing pending",
			sigInfo_afterConsume.Pid,
			errorCode__WAITID__AfterConsume,
		)
	}
	errorCode__SIGKILL := sendSignal__Pidfd(
		child.Pidfd,
		_SYSCALL.SIGKILL,
		0,
	)
	if errorCode__SIGKILL != 0 {
		t.Fatalf(
			"SIGKILL: %v",
			errorCode__SIGKILL,
		)
	}
	eventually__Condition(
		t,
		"zombie",
		func() bool {
			return "Z" == readState__Proc(child.Pid)
		},
	)
	_, _ = waitid__Pidfd(
		child.Pidfd,
		_SYSCALL.WSTOPPED|_SYSCALL.WNOHANG,
	)
	state_afterStopOnlyWait := readState__Proc(child.Pid)
	if "Z" != state_afterStopOnlyWait {
		t.Fatalf(
			"a WSTOPPED-only wait collected the zombie: /proc state = %q",
			state_afterStopOnlyWait,
		)
	}
	sigInfo_reported := awaitTermination__Pidfd(
		t,
		child.Pidfd,
	)
	if _CLD_KILLED != sigInfo_reported.Code || int32(_SYSCALL.SIGKILL) != sigInfo_reported.Status {
		t.Fatalf(
			"termination = code %d status %d, want CLD_KILLED/SIGKILL",
			sigInfo_reported.Code,
			sigInfo_reported.Status,
		)
	}
}
