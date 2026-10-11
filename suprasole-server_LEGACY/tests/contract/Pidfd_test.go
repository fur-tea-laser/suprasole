package contract

import (
	_OS "os"
	_PATHFILEPATH "path/filepath"
	_STRCONV "strconv"
	_STRINGS "strings"
	_SYSCALL "syscall"
	_TESTING "testing"
	_TIME "time"
)

// §5C: CLONE_PIDFD / SysProcAttr.PidFD — the pidfd is created by the fork itself, close-on-exec. (§6-L)
func TestPidfd_CreatedAtForkCloseOnExec(t *_TESTING.T) {
	child := spawn__Child(
		t,
		[]string{"sleep", "30"},
		nil,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	_, descriptorFlags := readFlags__Descriptor(
		t,
		child.Pidfd,
	)
	if 0 == descriptorFlags&_SYSCALL.FD_CLOEXEC {
		t.Fatalf("pidfd from CLONE_PIDFD is not FD_CLOEXEC")
	}
}

// §5C: a failed fork leaves SysProcAttr.PidFD untouched (Go: "*PidFD is changed only if the process starts successfully"). (§6-N)
func TestPidfd_ForkExecFailureLeavesPidfdUntouched(t *_TESTING.T) {
	devNull := open__DevNull(t)
	pidfd := -1
	_, error_forkExec__maybe := _SYSCALL.ForkExec(
		"/bin/sh",
		[]string{"sh"},
		&_SYSCALL.ProcAttr{
			Dir:   "/nonexistent-suprasole-contract",
			Files: []uintptr{uintptr(devNull), uintptr(devNull), uintptr(devNull)},
			Sys:   &_SYSCALL.SysProcAttr{Setsid: true, PidFD: &pidfd},
		},
	)
	if nil == error_forkExec__maybe {
		t.Fatalf("ForkExec with a nonexistent Dir unexpectedly succeeded")
	}
	if pidfd != -1 {
		t.Fatalf(
			"PidFD = %d after a failed fork, want -1",
			pidfd,
		)
	}
}

// §5C, pidfd_open(2): readable exactly once the process terminates and becomes a zombie —
// never while running or stopped. (§6-P, §6-Q)
func TestPidfd_ReadableExactlyWhenTerminated(t *_TESTING.T) {
	child := spawn__Child(
		t,
		[]string{"sleep", "30"},
		nil,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	if isReadable__Pidfd(child.Pidfd) {
		t.Fatalf("pidfd readable while the process is running")
	}
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
	if isReadable__Pidfd(child.Pidfd) {
		t.Fatalf("pidfd readable while the process is stopped")
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
		"pidfd readable after termination",
		func() bool {
			return isReadable__Pidfd(child.Pidfd)
		},
	)
	state := readState__Proc(child.Pid)
	if "Z" != state {
		t.Fatalf(
			"pidfd readable but /proc state = %q, want Z (zombie)",
			state,
		)
	}
}

// §5C, pidfd_send_signal(2): signals the pinned process; ESRCH once it has been reaped. (§6-C, §6-L)
func TestPidfd_SendSignalThenESRCHAfterReap(t *_TESTING.T) {
	child := spawn__Child(
		t,
		[]string{"sleep", "30"},
		nil,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	errorCode__SIGTERM := sendSignal__Pidfd(
		child.Pidfd,
		_SYSCALL.SIGTERM,
		0,
	)
	if errorCode__SIGTERM != 0 {
		t.Fatalf(
			"SIGTERM: %v",
			errorCode__SIGTERM,
		)
	}
	sigInfo_reported := awaitTermination__Pidfd(
		t,
		child.Pidfd,
	)
	if _CLD_KILLED != sigInfo_reported.Code || int32(_SYSCALL.SIGTERM) != sigInfo_reported.Status {
		t.Fatalf(
			"termination = code %d status %d, want CLD_KILLED/SIGTERM",
			sigInfo_reported.Code,
			sigInfo_reported.Status,
		)
	}
	_, errorCode__WAITID := reap__Pidfd(child.Pidfd)
	if errorCode__WAITID != 0 {
		t.Fatalf(
			"reap: %v",
			errorCode__WAITID,
		)
	}
	errorCode__SIGTERM__AfterReap := sendSignal__Pidfd(
		child.Pidfd,
		_SYSCALL.SIGTERM,
		0,
	)
	if _SYSCALL.ESRCH != errorCode__SIGTERM__AfterReap {
		t.Fatalf(
			"signal after reap = %v, want ESRCH",
			errorCode__SIGTERM__AfterReap,
		)
	}
}

// §5C, PIDFD_SIGNAL_PROCESS_GROUP (6.9): one pidfd signal reaches every member of the
// leader's process group — here a SIGHUP-ignoring child sharing the leader's group. (§6-C, §6-M)
func TestPidfd_SignalProcessGroupReachesGroupMembers(t *_TESTING.T) {
	path_memberPid := _PATHFILEPATH.Join(
		t.TempDir(),
		"member.pid",
	)
	leader := spawn__Child(
		t,
		[]string{"sh", "-c", `trap "" HUP; sleep 30 & echo $! > ` + path_memberPid + `; wait`},
		nil,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	var memberPid int
	eventually__Condition(
		t,
		"group member started",
		func() bool {
			memberPidText, error_read__MemberPid__maybe := _OS.ReadFile(path_memberPid)
			if error_read__MemberPid__maybe != nil {
				return false
			}
			memberPidText__trimmed := _STRINGS.TrimSpace(string(memberPidText))
			memberPid, _ = _STRCONV.Atoi(memberPidText__trimmed)
			return memberPid > 0
		},
	)
	memberPgid, error_getpgid__maybe := _SYSCALL.Getpgid(memberPid)
	if error_getpgid__maybe != nil || memberPgid != leader.Pid {
		t.Fatalf(
			"member pgid = %d (%v), want the leader's pid %d",
			memberPgid,
			error_getpgid__maybe,
			leader.Pid,
		)
	}
	errorCode__SIGKILL := sendSignal__Pidfd(
		leader.Pidfd,
		_SYSCALL.SIGKILL,
		_PIDFD_SIGNAL_PROCESS_GROUP,
	)
	if errorCode__SIGKILL != 0 {
		t.Fatalf(
			"group SIGKILL: %v",
			errorCode__SIGKILL,
		)
	}
	sigInfo_reported := awaitTermination__Pidfd(
		t,
		leader.Pidfd,
	)
	if _CLD_KILLED != sigInfo_reported.Code || int32(_SYSCALL.SIGKILL) != sigInfo_reported.Status {
		t.Fatalf(
			"leader termination = code %d status %d, want CLD_KILLED/SIGKILL",
			sigInfo_reported.Code,
			sigInfo_reported.Status,
		)
	}
	eventually__Condition(
		t,
		"group member killed",
		func() bool {
			state := readState__Proc(memberPid)
			return "Z" == state || "gone" == state
		},
	)
}

// §5C, pidfd_send_signal(2): PIDFD_SIGNAL_PROCESS_GROUP is defined for a pidfd that "refers to a
// process-group leader". Setsid makes the PTY process exactly that, for life (§3D) — so the design stays
// inside the documented domain. The non-leader result is logged, not asserted: it is not relied on.
func TestPidfd_SignalProcessGroupIsDefinedForGroupLeaders(t *_TESTING.T) {
	leader := spawn__Child(
		t,
		[]string{"sleep", "30"},
		nil,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	leaderPgid, _ := _SYSCALL.Getpgid(leader.Pid)
	if leaderPgid != leader.Pid {
		t.Fatalf(
			"Setsid child pgid = %d, want its own pid %d (group leader)",
			leaderPgid,
			leader.Pid,
		)
	}
	errorCode_Leader := sendSignal__Pidfd(
		leader.Pidfd,
		0,
		_PIDFD_SIGNAL_PROCESS_GROUP,
	)
	if errorCode_Leader != 0 {
		t.Fatalf(
			"group signal 0 to a group leader: %v",
			errorCode_Leader,
		)
	}
	nonLeader := spawn__Child(
		t,
		[]string{"sleep", "30"},
		nil,
		&_SYSCALL.SysProcAttr{},
	)
	errorCode_NonLeader := sendSignal__Pidfd(
		nonLeader.Pidfd,
		0,
		_PIDFD_SIGNAL_PROCESS_GROUP,
	)
	t.Logf(
		"observed only (not relied on): group signal 0 via a non-leader's pidfd -> %v",
		errorCode_NonLeader,
	)
}

// §5C, Go syscall.RawConn: f "is expected to try to read"; Read blocks only when f returns false.
// A pidfd made non-blocking and wrapped with os.NewFile parks in the netpoller until termination. (§3C)
func TestPidfd_RawConnReadParksUntilTermination(t *_TESTING.T) {
	child := spawn__Child(
		t,
		[]string{"sleep", "30"},
		nil,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	pidfd_duplicate, error_dup__maybe := _SYSCALL.Dup(child.Pidfd)
	if error_dup__maybe != nil {
		t.Fatalf(
			"dup: %v",
			error_dup__maybe,
		)
	}
	error_setNonblock__maybe := _SYSCALL.SetNonblock(
		pidfd_duplicate,
		true,
	)
	if error_setNonblock__maybe != nil {
		t.Fatalf(
			"SetNonblock: %v",
			error_setNonblock__maybe,
		)
	}
	pidfdFile := _OS.NewFile(
		uintptr(pidfd_duplicate),
		"pidfd",
	)
	defer pidfdFile.Close()
	rawConn_Pidfd, error_syscallConn__maybe := pidfdFile.SyscallConn()
	if error_syscallConn__maybe != nil {
		t.Fatalf(
			"SyscallConn: %v",
			error_syscallConn__maybe,
		)
	}
	deadline := _TIME.Now().Add(TIMEOUT__Condition)
	error_setReadDeadline__maybe := pidfdFile.SetReadDeadline(deadline)
	if error_setReadDeadline__maybe != nil {
		t.Fatalf(
			"SetReadDeadline on the pidfd (is it pollable?): %v",
			error_setReadDeadline__maybe,
		)
	}
	go func() {
		_TIME.Sleep(100 * _TIME.Millisecond)
		_ = sendSignal__Pidfd(
			child.Pidfd,
			_SYSCALL.SIGKILL,
			0,
		)
	}()
	count_invocations := 0
	error_read__maybe := rawConn_Pidfd.Read(func(fileDescriptor uintptr) bool {
		count_invocations++
		sigInfo_peek, errorCode__WAITID := peek__Pidfd(int(fileDescriptor))
		if errorCode__WAITID != 0 {
			t.Errorf(
				"peek: %v",
				errorCode__WAITID,
			)
			return true
		}
		return sigInfo_peek.Pid != 0 // false only while nothing is reportable — the pidfd is not readable then
	})
	if error_read__maybe != nil {
		t.Fatalf(
			"RawConn.Read: %v",
			error_read__maybe,
		)
	}
	if count_invocations < 2 {
		t.Fatalf(
			"callback invoked %d time(s), want ≥ 2 (initial attempt + post-readiness)",
			count_invocations,
		)
	}
}
