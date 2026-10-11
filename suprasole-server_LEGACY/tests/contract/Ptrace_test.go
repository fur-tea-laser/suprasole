package contract

import (
	_FMT "fmt"
	_OS "os"
	_RUNTIME "runtime"
	_SYSCALL "syscall"
	_TESTING "testing"
)

// §5C, ptrace(2) + kernel/exit.c ("a zombie ptracee is only visible to its ptracer"), and
// pidfd_open(2) (readable once a zombie): under a foreign debugger, a terminated child is
// readable on its pidfd yet unreportable to its real parent — Zombie_Withheld — until the
// debugger collects its report; then the real parent's peek reports it. (§6-V, §6-AD, §6-AE)
func TestPtrace_ForeignDebuggerWithholdsZombieUntilItCollects(t *_TESTING.T) {
	child := spawn__Child(
		t,
		[]string{"sleep", "30"},
		nil,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	foreignDebugger := start__ForeignDebugger(
		t,
		child.Pid,
	)
	tracerPid := readTracerPid__Proc(child.Pid)
	if tracerPid != foreignDebugger.ThreadId {
		t.Fatalf(
			"/proc TracerPid = %d, want the debugger's tracing thread %d",
			tracerPid,
			foreignDebugger.ThreadId,
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
		"pidfd readable (terminated)",
		func() bool {
			return isReadable__Pidfd(child.Pidfd)
		},
	)
	state_withheld := readState__Proc(child.Pid)
	if "Z" != state_withheld {
		t.Fatalf(
			"/proc state = %q, want Z",
			state_withheld,
		)
	}
	sigInfo_withheld, errorCode__WAITID__Withheld := peek__Pidfd(child.Pidfd)
	if errorCode__WAITID__Withheld != 0 || sigInfo_withheld.Pid != 0 {
		t.Fatalf(
			"real parent's peek while withheld = si_pid %d code %d errno %v, want nothing reportable",
			sigInfo_withheld.Pid,
			sigInfo_withheld.Code,
			errorCode__WAITID__Withheld,
		)
	}
	foreignDebugger.Collect(t)
	sigInfo_reported := awaitTermination__Pidfd(
		t,
		child.Pidfd,
	)
	if _CLD_KILLED != sigInfo_reported.Code || int32(_SYSCALL.SIGKILL) != sigInfo_reported.Status {
		t.Fatalf(
			"termination after release = code %d status %d, want CLD_KILLED/SIGKILL",
			sigInfo_reported.Code,
			sigInfo_reported.Status,
		)
	}
	_, errorCode__WAITID__Reap := reap__Pidfd(child.Pidfd)
	if errorCode__WAITID__Reap != 0 {
		t.Fatalf(
			"reap after release: %v",
			errorCode__WAITID__Reap,
		)
	}
}

// §5C, proc(5) + ptrace(2): after PTRACE_TRACEME the tracer is the *thread* that forked, and
// /proc TracerPid names that thread — not necessarily suprasole's PID, because Go forks from any
// OS thread. "Traced by suprasole" therefore means: TracerPid is one of suprasole's threads
// (/proc/self/task/<TracerPid> exists). State letter t while trace-stopped. (§3F, §6-AI)
func TestPtrace_TracemeTracerIsTheForkingThread(t *_TESTING.T) {
	_RUNTIME.LockOSThread() // the forking thread becomes the tracer
	defer _RUNTIME.UnlockOSThread()
	threadId_forking := _SYSCALL.Gettid()
	child := spawn__Child(
		t,
		[]string{"sleep", "30"},
		nil,
		&_SYSCALL.SysProcAttr{Setsid: true, Ptrace: true},
	)
	eventually__Condition(
		t,
		"trace-stopped at exec",
		func() bool {
			return "t" == readState__Proc(child.Pid)
		},
	)
	tracerPid := readTracerPid__Proc(child.Pid)
	t.Logf(
		"process pid %d, forking thread %d, /proc TracerPid %d",
		_OS.Getpid(),
		threadId_forking,
		tracerPid,
	)
	if tracerPid != threadId_forking {
		t.Fatalf(
			"/proc TracerPid = %d, want the forking thread %d (process pid %d)",
			tracerPid,
			threadId_forking,
			_OS.Getpid(),
		)
	}
	path_tracerTask := _FMT.Sprintf(
		"/proc/self/task/%d",
		tracerPid,
	)
	_, error_stat__TracerTask__maybe := _OS.Stat(path_tracerTask)
	if error_stat__TracerTask__maybe != nil {
		t.Fatalf(
			"TracerPid %d is not one of this process's threads: %v",
			tracerPid,
			error_stat__TracerTask__maybe,
		)
	}
}
