package main

import (
	_OS "os"
	_SYSCALL "syscall"
	_TIME "time"
	_UNSAFE "unsafe"
)

func (This *_PtyProcess_) RunWorker() {
	This.Mutex.Lock()
	State__PtyProcess__alive := This.State_state.(*_Alive__State_PtyProcess_)
	This.Mutex.Unlock()
	var status_TerminationReport _Status_TerminationReport__PtyProcess_
	var sigInfo_reported _SigInfo_Waitid_
	var error_peek__maybe error
	rawConn_Pidfd, error_syscallConn__maybe := State__PtyProcess__alive.FileDescriptor_Pidfd.SyscallConn()
	// [unreachable by construction] SyscallConn fails only for a nil *os.File; Spawn_PtyProcess never builds one
	if error_syscallConn__maybe != nil {
		status_TerminationReport = FAILED__Status_TerminationReport__PtyProcess
		error_peek__maybe = error_syscallConn__maybe
	} else {
		error_read__maybe := rawConn_Pidfd.Read(func(fileDescriptor_pidfd uintptr) bool {
			status_TerminationReport, sigInfo_reported, error_peek__maybe = peek__TerminationReport_PtyProcess(fileDescriptor_pidfd)
			return status_TerminationReport != ABSENT__Status_TerminationReport__PtyProcess
		})
		if error_read__maybe != nil {
			status_TerminationReport = FAILED__Status_TerminationReport__PtyProcess
			error_peek__maybe = error_read__maybe
		}
	}
	if WITHHELD__Status_TerminationReport__PtyProcess == status_TerminationReport {
		This.Mutex.Lock()
		This.State_state = &_Zombie_Withheld__State_PtyProcess_{
			FileDescriptor_Pidfd: State__PtyProcess__alive.FileDescriptor_Pidfd,
			Pid:                  State__PtyProcess__alive.Pid,
		}
		This.Mutex.Unlock()
		status_TerminationReport, sigInfo_reported, error_peek__maybe = repeek__TerminationReport_PtyProcess(rawConn_Pidfd)
	}
	switch status_TerminationReport {
	case REPORTED__Status_TerminationReport__PtyProcess:
		termination_PtyProcess := decode__Termination_PtyProcess(sigInfo_reported)
		This.Mutex.Lock()
		This.State_state = &_Zombie__State_PtyProcess_{
			FileDescriptor_Pidfd: State__PtyProcess__alive.FileDescriptor_Pidfd,
			Pid:                  State__PtyProcess__alive.Pid,
			Termination:          termination_PtyProcess,
		}
		This.Mutex.Unlock()
		This.Mutex.Lock()
		var State__PtyProcess__final _State_PtyProcess_
		error_control__maybe := rawConn_Pidfd.Control(func(fileDescriptor_pidfd uintptr) {
			State__PtyProcess__final = collect__PtyProcess(
				fileDescriptor_pidfd,
				termination_PtyProcess,
			)
		})
		// [unreachable by construction] Control fails only on a nil or closed *os.File; the worker closes the pidfd only after this final write
		if error_control__maybe != nil {
			State__PtyProcess__final = &_SystemError__State_PtyProcess_{
				SystemError_Waitid: error_control__maybe,
			}
		}
		This.State_state = State__PtyProcess__final
		This.Mutex.Unlock()
	case FAILED__Status_TerminationReport__PtyProcess:
		This.Mutex.Lock()
		This.State_state = &_SystemError__State_PtyProcess_{
			SystemError_Waitid: error_peek__maybe,
		}
		This.Mutex.Unlock()
	default:
		// ABSENT__Status_TerminationReport__PtyProcess
		// WITHHELD__Status_TerminationReport__PtyProcess
		panic("invalid path: _PtyProcess_ RunWorker")
	}
	_ = State__PtyProcess__alive.FileDescriptor_Pidfd.Close()
	close(This.Channel_FinalState)
}

func peek__TerminationReport_PtyProcess(
	fileDescriptor_pidfd uintptr,
) (_Status_TerminationReport__PtyProcess_, _SigInfo_Waitid_, error) {
	for {
		sigInfo_peek, errorCode__WAITID := waitid__PtyProcess(
			fileDescriptor_pidfd,
			_SYSCALL.WEXITED|_SYSCALL.WNOHANG|_SYSCALL.WNOWAIT,
		)
		if 0 == errorCode__WAITID && 0 == sigInfo_peek.Pid {
			status_PollIn, errorCode__PPOLL := poll__Pidfd(fileDescriptor_pidfd)
			if errorCode__PPOLL != 0 {
				return FAILED__Status_TerminationReport__PtyProcess, _SigInfo_Waitid_{}, _OS.NewSyscallError(
					"ppoll",
					errorCode__PPOLL,
				)
			} else if 0 == errorCode__PPOLL && SET__Status_PollIn__Pidfd == status_PollIn {
				return WITHHELD__Status_TerminationReport__PtyProcess, _SigInfo_Waitid_{}, nil
			} else if 0 == errorCode__PPOLL && CLEAR__Status_PollIn__Pidfd == status_PollIn {
				return ABSENT__Status_TerminationReport__PtyProcess, _SigInfo_Waitid_{}, nil
			} else {
				panic("invalid path: peek__TerminationReport_PtyProcess [STATUS_POLLIN]")
			}
		} else if 0 == errorCode__WAITID && sigInfo_peek.Pid != 0 && CLD_TRAPPED__SIGCHLD__Linux == sigInfo_peek.Code {
			_, _ = waitid__PtyProcess(
				fileDescriptor_pidfd,
				_SYSCALL.WSTOPPED|_SYSCALL.WNOHANG,
			)
			continue
		} else if 0 == errorCode__WAITID && sigInfo_peek.Pid != 0 && (CLD_EXITED__SIGCHLD__Linux == sigInfo_peek.Code || CLD_KILLED__SIGCHLD__Linux == sigInfo_peek.Code || CLD_DUMPED__SIGCHLD__Linux == sigInfo_peek.Code) {
			return REPORTED__Status_TerminationReport__PtyProcess, sigInfo_peek, nil
		} else {
			// errorCode__WAITID != 0
			// CLD_STOPPED / CLD_CONTINUED: not selected without WSTOPPED / WCONTINUED
			panic("invalid path: peek__TerminationReport_PtyProcess")
		}
	}
}

func repeek__TerminationReport_PtyProcess(
	rawConn_Pidfd _SYSCALL.RawConn,
) (_Status_TerminationReport__PtyProcess_, _SigInfo_Waitid_, error) {
	for {
		_TIME.Sleep(INTERVAL_REPEEK__PtyProcess)
		var status_TerminationReport _Status_TerminationReport__PtyProcess_
		var sigInfo_repeek _SigInfo_Waitid_
		var error_repeek__maybe error
		error_control__maybe := rawConn_Pidfd.Control(func(fileDescriptor_pidfd uintptr) {
			status_TerminationReport, sigInfo_repeek, error_repeek__maybe = peek__TerminationReport_PtyProcess(fileDescriptor_pidfd)
		})
		// [unreachable by construction] Control fails only on a nil or closed *os.File; the worker closes the pidfd only after its final state
		if error_control__maybe != nil {
			return FAILED__Status_TerminationReport__PtyProcess, _SigInfo_Waitid_{}, error_control__maybe
		}
		switch status_TerminationReport {
		case WITHHELD__Status_TerminationReport__PtyProcess:
		case REPORTED__Status_TerminationReport__PtyProcess, FAILED__Status_TerminationReport__PtyProcess:
			return status_TerminationReport, sigInfo_repeek, error_repeek__maybe
		default:
			// ABSENT__Status_TerminationReport__PtyProcess
			panic("invalid path: repeek__TerminationReport_PtyProcess")
		}
	}
}

func decode__Termination_PtyProcess(
	sigInfo_reported _SigInfo_Waitid_,
) _Termination_PtyProcess_ {
	switch sigInfo_reported.Code {
	case CLD_EXITED__SIGCHLD__Linux:
		return _Exited__Termination_PtyProcess_{
			ExitCode_PtyProcess: int(sigInfo_reported.Status),
		}
	case CLD_KILLED__SIGCHLD__Linux:
		return _Signaled__Termination_PtyProcess_{
			Signal_PtyProcess: int(sigInfo_reported.Status),
			Status_CoreDump:   NOT_DUMPED__Status_CoreDump__PtyProcess,
		}
	case CLD_DUMPED__SIGCHLD__Linux:
		return _Signaled__Termination_PtyProcess_{
			Signal_PtyProcess: int(sigInfo_reported.Status),
			Status_CoreDump:   DUMPED__Status_CoreDump__PtyProcess,
		}
	default:
		// CLD_TRAPPED__SIGCHLD__Linux
		// CLD_STOPPED__SIGCHLD__Linux
		// CLD_CONTINUED
		panic("invalid path: decode__Termination_PtyProcess")
	}
}

func collect__PtyProcess(
	fileDescriptor_pidfd uintptr,
	termination_PtyProcess _Termination_PtyProcess_,
) _State_PtyProcess_ {
	sigInfo_reaped, errorCode__WAITID := waitid__PtyProcess(
		fileDescriptor_pidfd,
		_SYSCALL.WEXITED|_SYSCALL.WNOHANG,
	)
	if 0 == errorCode__WAITID && sigInfo_reaped.Pid != 0 {
		return &_Reaped__State_PtyProcess_{
			Termination: termination_PtyProcess,
		}
	} else {
		// errorCode__WAITID != 0
		// 0 == errorCode__WAITID && 0 == sigInfo_reaped.Pid
		panic("invalid path: collect__PtyProcess")
	}
}

func waitid__PtyProcess(
	fileDescriptor_pidfd uintptr,
	options__WAITID int,
) (_SigInfo_Waitid_, _SYSCALL.Errno) {
	var sigInfo_result _SigInfo_Waitid_
	_, _, errorCode__WAITID := _SYSCALL.Syscall6(
		_SYSCALL.SYS_WAITID,
		P_PIDFD__WAITID__Linux,
		fileDescriptor_pidfd,
		uintptr(_UNSAFE.Pointer(&sigInfo_result)),
		uintptr(options__WAITID),
		0,
		0,
	)
	return sigInfo_result, errorCode__WAITID
}

func poll__Pidfd(
	fileDescriptor_pidfd uintptr,
) (_Status_PollIn__Pidfd_, _SYSCALL.Errno) {
	for {
		pollFd_Pidfd := _PollFd_Ppoll_{
			FileDescriptor: int32(fileDescriptor_pidfd),
			Events:         POLLIN__Linux,
		}
		var timeout_immediate _SYSCALL.Timespec
		_, _, errorCode__PPOLL := _SYSCALL.Syscall6(
			_SYSCALL.SYS_PPOLL,
			uintptr(_UNSAFE.Pointer(&pollFd_Pidfd)),
			1,
			uintptr(_UNSAFE.Pointer(&timeout_immediate)),
			0,
			0,
			0,
		)
		if _SYSCALL.EINTR == errorCode__PPOLL {
			continue
		} else if errorCode__PPOLL != 0 && errorCode__PPOLL != _SYSCALL.EINTR {
			return CLEAR__Status_PollIn__Pidfd, errorCode__PPOLL
		} else if 0 == errorCode__PPOLL && pollFd_Pidfd.Revents&POLLIN__Linux != 0 {
			return SET__Status_PollIn__Pidfd, 0
		} else if 0 == errorCode__PPOLL && 0 == pollFd_Pidfd.Revents&POLLIN__Linux {
			return CLEAR__Status_PollIn__Pidfd, 0
		} else {
			panic("invalid path: poll__Pidfd")
		}
	}
}
