package main

import (
	_BYTES "bytes"
	_ERRORS "errors"
	_OS "os"
	_SYSCALL "syscall"
	_TIME "time"
	_UNSAFE "unsafe"
)

func (This *_PtyDispatcher_) RunWorker() {
	for {
		select {
		case order_next := <-This.QueueChannel__Order_PtyDispatcher:
			switch order_next.Execute(This) {
			case CONTINUE_WORKER__Directive__Order_PtyDispatcher:
			case EXIT_WORKER__Directive__Order_PtyDispatcher:
				return
			default:
				panic("invalid path: _PtyDispatcher_ RunWorker")
			}
		case <-This.Timer_FlushPacing.C:
			if len(This.StagingBuffer) > 0 {
				This.OnBlockingFlush__(This.StagingBuffer)
				This.StagingBuffer = This.StagingBuffer[:0]
				This.Timer_FlushPacing.Reset(This.Timeout__Timer_FlushPacing)
			} else {
				This.Status_state = IDLE__Status_PtyDispatcher
			}
		}
	}
}

func (this *_Data__Order_PtyDispatcher_) Execute(
	PtyDispatcher_forwarded *_PtyDispatcher_,
) _Directive__Order_PtyDispatcher_ {
	switch PtyDispatcher_forwarded.Status_state {
	case IDLE__Status_PtyDispatcher:
		PtyDispatcher_forwarded.OnBlockingFlush__(this.ReadBuffer_PtyDescriptor)
		PtyDispatcher_forwarded.Timer_FlushPacing.Reset(PtyDispatcher_forwarded.Timeout__Timer_FlushPacing)
		PtyDispatcher_forwarded.Status_state = PACING__Status_PtyDispatcher
	case PACING__Status_PtyDispatcher:
		PtyDispatcher_forwarded.StagingBuffer = append(
			PtyDispatcher_forwarded.StagingBuffer,
			this.ReadBuffer_PtyDescriptor...,
		)
	default:
		panic("invalid path: _Data__Order_PtyDispatcher_ Execute")
	}
	this.ReadBuffer_PtyDescriptor = this.ReadBuffer_PtyDescriptor[:cap(this.ReadBuffer_PtyDescriptor)]
	PtyDispatcher_forwarded.PoolChannel___Data__Order_PtyDispatcher <- this
	return CONTINUE_WORKER__Directive__Order_PtyDispatcher
}

func (this *_ExitSignal__Order_PtyDispatcher_) Execute(
	PtyDispatcher_forwarded *_PtyDispatcher_,
) _Directive__Order_PtyDispatcher_ {
	PtyDispatcher_forwarded.Timer_FlushPacing.Stop()
	if len(PtyDispatcher_forwarded.StagingBuffer) > 0 {
		PtyDispatcher_forwarded.OnBlockingFlush__(PtyDispatcher_forwarded.StagingBuffer)
		PtyDispatcher_forwarded.StagingBuffer = PtyDispatcher_forwarded.StagingBuffer[:0]
	}
	PtyDispatcher_forwarded.OnExited__(this.ExitSignal)
	return EXIT_WORKER__Directive__Order_PtyDispatcher
}

func (This *_PtyProxy_) HandleBlockingFlush(
	UnflushedSlice_StagingBuffer__PtyDispatcher []byte,
) {
	This.Mutex.Lock()
	mode_PtyProxy_captured := This.Mode_state
	This.PtyTerminal.Write(UnflushedSlice_StagingBuffer__PtyDispatcher)
	switch mode_PtyProxy_captured {
	case LIVE_RUNNING__Mode_PtyProxy:
	case PRE_SNAPSHOT__RUNNING___Mode_PtyProxy:
	case POST_SNAPSHOT__RUNNING___Mode_PtyProxy:
		This.PostSnapshotBuffer.Write(UnflushedSlice_StagingBuffer__PtyDispatcher)
	default:
		// SPAWNING__Mode_PtyProxy
		// EXITED__Mode_PtyProxy
		panic("invalid path: _PtyProxy_ HandleBlockingFlush")
	}
	This.Mutex.Unlock()
	if LIVE_RUNNING__Mode_PtyProxy == mode_PtyProxy_captured {
		This.OnOutput_Live__(
			This.Id_WorkspacePty,
			_BYTES.Clone(UnflushedSlice_StagingBuffer__PtyDispatcher),
		)
	}
}

// *****************************************************************************
// func (This *_WorkspaceController_) HandleOutput_Pty
// *****************************************************************************

func (This *_PtyProxy_) HandleExited(
	exitSignal_PtyReader error,
) {
	_ = This.FileDescriptor_Master__Pty__shared.Close()
	teardownReport := This.TeardownExited_PtyProcess()
	if _ERRORS.Is(exitSignal_PtyReader, _SYSCALL.EIO) {
		This.OnExitOutcome__(
			This.Id_WorkspacePty,
			teardownReport.Resolve__ExitOutcome_PtyProxy(),
		)
	} else if exitSignal_PtyReader != nil && false == _ERRORS.Is(exitSignal_PtyReader, _OS.ErrClosed) {
		This.OnExitOutcome__(
			This.Id_WorkspacePty,
			_SystemError_Descriptor__ExitOutcome_PtyProxy_{
				SystemError_PtyDescriptor: exitSignal_PtyReader,
			},
		)
	} else {
		// _ERRORS.Is(exitSignal_PtyReader, _OS.ErrClosed)
		panic("invalid path: _PtyProxy_ HandleExited")
	}
}

func (This *_PtyProxy_) TeardownExited_PtyProcess() _TeardownReport_PtyProcess_ {
	channel_reapResult__PtyProcess__initial := spawnReaper_PtyProcess(This.PtyCommand.Process)
	timer_Unresponsive := _TIME.NewTimer(TIMEOUT_UNRESPONSIVE__PtyProcess)
	select {
	case reapResult_only := <-channel_reapResult__PtyProcess__initial:
		if reapResult_only.ProcessState_maybe != nil && false == reapResult_only.ProcessState_maybe.Sys().(_SYSCALL.WaitStatus).Stopped() {
			return _Reaped__TeardownReport_PtyProcess_{
				ProcessState: reapResult_only.ProcessState_maybe,
			}
		} else if reapResult_only.ProcessState_maybe != nil && reapResult_only.ProcessState_maybe.Sys().(_SYSCALL.WaitStatus).Stopped() {
			return kill__PtyProcess_Unreaped(
				This.PtyCommand.Process,
				spawnReaper_PtyProcess(This.PtyCommand.Process),
				_Stopped_Traced__TeardownReport_PtyProcess_{
					StopSignal_Teardown: reapResult_only.ProcessState_maybe.Sys().(_SYSCALL.WaitStatus).StopSignal(),
				},
			)
		} else if _ERRORS.Is(reapResult_only.Error_Wait_maybe, _SYSCALL.ECHILD) || _ERRORS.Is(reapResult_only.Error_Wait_maybe, _SYSCALL.EINVAL) || _ERRORS.Is(reapResult_only.Error_Wait_maybe, _SYSCALL.EBADF) {
			return _SystemError__TeardownReport_PtyProcess_{
				SystemError_Wait: reapResult_only.Error_Wait_maybe,
			}
		} else {
			panic("invalid path: _PtyProxy_ TeardownExited_PtyProcess")
		}
	case <-timer_Unresponsive.C:
		return kill__PtyProcess_Unreaped(
			This.PtyCommand.Process,
			channel_reapResult__PtyProcess__initial,
			probeStatus__PtyProcess_Unresponsive(This.PtyCommand.Process),
		)
	}
}

const (
	TIMEOUT_UNRESPONSIVE__PtyProcess    = 1 * _TIME.Second
	TIMEOUT_UNINTERRUPTIBLE__PtyProcess = 1 * _TIME.Second
)

type _TeardownReport_PtyProcess_ interface {
	Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_
}

type _Reaped__TeardownReport_PtyProcess_ struct {
	ProcessState *_OS.ProcessState
}

func (this _Reaped__TeardownReport_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	waitStatus_PtyProcess := this.ProcessState.Sys().(_SYSCALL.WaitStatus)
	if waitStatus_PtyProcess.Exited() && 0 == waitStatus_PtyProcess.ExitStatus() {
		return _Success__ExitOutcome_PtyProxy_{}
	} else if waitStatus_PtyProcess.Exited() {
		return _Failure__ExitOutcome_PtyProxy_{
			ExitCode_PtyProcess: this.ProcessState.ExitCode(),
		}
	} else if waitStatus_PtyProcess.Signaled() {
		return _Killed__ExitOutcome_PtyProxy_{
			ExitSignal_PtyProcess: int(waitStatus_PtyProcess.Signal()),
		}
	} else {
		panic("invalid path: _Reaped__TeardownReport_PtyProcess_ Resolve__ExitOutcome_PtyProxy")
	}
}

type _Stopped_Traced__TeardownReport_PtyProcess_ struct {
	StopSignal_Teardown _SYSCALL.Signal
}

func (this _Stopped_Traced__TeardownReport_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return _Stopped__ExitOutcome_PtyProxy_{
		StopSignal_PtyProcess: int(this.StopSignal_Teardown),
	}
}

type _SystemError__TeardownReport_PtyProcess_ struct {
	SystemError_Wait error
}

func (this _SystemError__TeardownReport_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return _SystemError_Process__ExitOutcome_PtyProxy_{
		SystemError_PtyProcess: this.SystemError_Wait,
	}
}

type _Unresponsive_Uninterruptible__TeardownReport_PtyProcess_ struct{}

func (_Unresponsive_Uninterruptible__TeardownReport_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return _SystemError_Process__ExitOutcome_PtyProxy_{
		SystemError_PtyProcess: _ERRORS.New("unresolved process exit state: process unkillable"),
	}
}

type _Unresponsive_Stopped__TeardownReport_PtyProcess_ struct {
	StopSignal_Teardown _SYSCALL.Signal
}

func (this _Unresponsive_Stopped__TeardownReport_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return _Stopped__ExitOutcome_PtyProxy_{
		StopSignal_PtyProcess: int(this.StopSignal_Teardown),
	}
}

type _Unresponsive_Rogue__TeardownReport_PtyProcess_ struct{}

func (_Unresponsive_Rogue__TeardownReport_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return _SystemError_Process__ExitOutcome_PtyProxy_{
		SystemError_PtyProcess: _ERRORS.New("unresolved process exit state: process unresponsive to hangup"),
	}
}

type _ReapResult_PtyProcess_ struct {
	ProcessState_maybe *_OS.ProcessState
	Error_Wait_maybe   error
}

func spawnReaper_PtyProcess(
	process_Pty *_OS.Process,
) chan _ReapResult_PtyProcess_ {
	channel_reapResult__PtyProcess := make(chan _ReapResult_PtyProcess_, 1)
	go func() {
		processState, error_wait := process_Pty.Wait()
		channel_reapResult__PtyProcess <- _ReapResult_PtyProcess_{
			ProcessState_maybe: processState,
			Error_Wait_maybe:   error_wait,
		}
	}()
	return channel_reapResult__PtyProcess
}

func kill__PtyProcess_Unreaped(
	process_Pty *_OS.Process,
	channel_reapResult__PtyProcess__leading <-chan _ReapResult_PtyProcess_,
	report_originating _TeardownReport_PtyProcess_,
) _TeardownReport_PtyProcess_ {
	pid_PtyProcess := process_Pty.Pid
	_ = _SYSCALL.Kill(-pid_PtyProcess, _SYSCALL.SIGKILL)
	_ = _SYSCALL.Kill(pid_PtyProcess, _SYSCALL.SIGKILL)
	timer_Uninterruptible := _TIME.NewTimer(TIMEOUT_UNINTERRUPTIBLE__PtyProcess)
	channel_reapResult__PtyProcess__next := channel_reapResult__PtyProcess__leading
	for {
		select {
		case reapResult_only := <-channel_reapResult__PtyProcess__next:
			if reapResult_only.ProcessState_maybe != nil && false == reapResult_only.ProcessState_maybe.Sys().(_SYSCALL.WaitStatus).Stopped() {
				return report_originating
			} else if reapResult_only.ProcessState_maybe != nil && reapResult_only.ProcessState_maybe.Sys().(_SYSCALL.WaitStatus).Stopped() {
				channel_reapResult__PtyProcess__next = spawnReaper_PtyProcess(process_Pty)
				continue
			} else if _ERRORS.Is(reapResult_only.Error_Wait_maybe, _SYSCALL.ECHILD) || _ERRORS.Is(reapResult_only.Error_Wait_maybe, _SYSCALL.EINVAL) || _ERRORS.Is(reapResult_only.Error_Wait_maybe, _SYSCALL.EBADF) {
				return _SystemError__TeardownReport_PtyProcess_{
					SystemError_Wait: reapResult_only.Error_Wait_maybe,
				}
			} else {
				panic("invalid path: kill__PtyProcess_Unreaped")
			}
		case <-timer_Uninterruptible.C:
			return _Unresponsive_Uninterruptible__TeardownReport_PtyProcess_{}
		}
	}
}

type _SigInfo_Waitid_ struct {
	Signo  int32
	Errno  int32
	Code   int32
	_pad   int32
	Pid    int32
	Uid    uint32
	Status int32
	_rest  [128 - 28]byte
}

const (
	_P_PID___WAITID = 1
	_CLD_STOPPED    = 5
)

func probeStatus__PtyProcess_Unresponsive(
	process_Pty *_OS.Process,
) _TeardownReport_PtyProcess_ {
	var sigInfo_result _SigInfo_Waitid_
	returnCode_WAITID, _, errorCode_WAITID := _SYSCALL.Syscall6(
		_SYSCALL.SYS_WAITID,
		uintptr(_P_PID___WAITID),
		uintptr(process_Pty.Pid),
		uintptr(_UNSAFE.Pointer(&sigInfo_result)),
		uintptr(_SYSCALL.WSTOPPED|_SYSCALL.WNOHANG|_SYSCALL.WNOWAIT),
		0,
		0,
	)
	if returnCode_WAITID == 0 && errorCode_WAITID == 0 && int32(process_Pty.Pid) == sigInfo_result.Pid && _CLD_STOPPED == sigInfo_result.Code {
		return _Unresponsive_Stopped__TeardownReport_PtyProcess_{
			StopSignal_Teardown: _SYSCALL.Signal(sigInfo_result.Status),
		}
	} else if returnCode_WAITID == 0 && errorCode_WAITID == 0 && 0 == sigInfo_result.Pid {
		return _Unresponsive_Rogue__TeardownReport_PtyProcess_{}
	} else {
		return _SystemError__TeardownReport_PtyProcess_{
			SystemError_Wait: _ERRORS.New("unresolved process exit state: process reaping took too long (you should play the lottery)"),
		}
	}
}

func (This *_WorkspaceController_) HandleExitOutcome_Pty(
	id_WorkspacePty uint32,
	exitOutcome_result _ExitOutcome_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: id_WorkspacePty,
		ExitOutcome_PtyProxy:   exitOutcome_result,
	}
}
