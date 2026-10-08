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
	channel_reapResult__PtyProcess__initial := spawnReaper_PtyProcess(This.PtyProcess)
	timer_Unresponsive := _TIME.NewTimer(TIMEOUT_UNRESPONSIVE__PtyProcess)
	return selectReapResult_Exited__PtyProcess(
		This.PtyProcess,
		channel_reapResult__PtyProcess__initial,
		timer_Unresponsive,
	)
}

func selectReapResult_Exited__PtyProcess(
	PtyProcess_this *_OS.Process,
	channel_reapResult__PtyProcess__initial <-chan _ReapResult_PtyProcess_,
	timer_Unresponsive *_TIME.Timer,
) _TeardownReport_PtyProcess_ {
	handleStopped_Ptrace := func(waitStatus_PtyProcess _SYSCALL.WaitStatus) _TeardownReport_PtyProcess_ {
		return killExited__PtyProcess_Unreaped(
			PtyProcess_this,
			spawnReaper_PtyProcess(PtyProcess_this),
			_Stopped_Traced__TeardownReport_PtyProcess_{
				StopSignal_Teardown: waitStatus_PtyProcess.StopSignal(),
			},
		)
	}
	return __selectReapResult__PtyProcess(
		channel_reapResult__PtyProcess__initial,
		timer_Unresponsive,
		func(error_wait error) _TeardownReport_PtyProcess_ {
			return _SystemError__TeardownReport_PtyProcess_{
				SystemError_Wait: error_wait,
			}
		},
		func(processState *_OS.ProcessState) _TeardownReport_PtyProcess_ {
			return _Reaped__TeardownReport_PtyProcess_{
				ProcessState: processState,
			}
		},
		handleStopped_Ptrace,
		handleStopped_Ptrace,
		func() _TeardownReport_PtyProcess_ {
			return killExited__PtyProcess_Unreaped(
				PtyProcess_this,
				channel_reapResult__PtyProcess__initial,
				probeStatus__PtyProcess_Unresponsive(PtyProcess_this),
			)
		},
	)
}

type _ReapResult_PtyProcess_ struct {
	ProcessState_maybe *_OS.ProcessState
	Error_Wait_maybe   error
}

func killExited__PtyProcess_Unreaped(
	PtyProcess_this *_OS.Process,
	channel_reapResult__PtyProcess__leading <-chan _ReapResult_PtyProcess_,
	report_originating _TeardownReport_PtyProcess_,
) _TeardownReport_PtyProcess_ {
	return __kill__PtyProcess_Unreaped(
		PtyProcess_this,
		channel_reapResult__PtyProcess__leading,
		func(error_wait error) _TeardownReport_PtyProcess_ {
			return _SystemError__TeardownReport_PtyProcess_{
				SystemError_Wait: error_wait,
			}
		},
		func(_ *_OS.ProcessState) _TeardownReport_PtyProcess_ {
			return report_originating
		},
		func() _TeardownReport_PtyProcess_ {
			return _Unresponsive_Uninterruptible__TeardownReport_PtyProcess_{}
		},
	)
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
	_P_PID___WAITID    = 1
	_CLD_STOPPED       = 5
	_PTRACE_EVENT_EXIT = 6
)

func probeStatus__PtyProcess_Unresponsive(
	PtyProcess_this *_OS.Process,
) _TeardownReport_PtyProcess_ {
	var sigInfo_result _SigInfo_Waitid_
	returnCode_WAITID, _, errorCode_WAITID := _SYSCALL.Syscall6(
		_SYSCALL.SYS_WAITID,
		uintptr(_P_PID___WAITID),
		uintptr(PtyProcess_this.Pid),
		uintptr(_UNSAFE.Pointer(&sigInfo_result)),
		uintptr(_SYSCALL.WSTOPPED|_SYSCALL.WNOHANG|_SYSCALL.WNOWAIT),
		0,
		0,
	)
	if returnCode_WAITID == 0 && errorCode_WAITID == 0 && int32(PtyProcess_this.Pid) == sigInfo_result.Pid && _CLD_STOPPED == sigInfo_result.Code {
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

func (This *_WorkspaceController_) HandleExitOutcome_Pty(
	id_WorkspacePty uint32,
	exitOutcome_result _ExitOutcome_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: id_WorkspacePty,
		ExitOutcome_PtyProxy:   exitOutcome_result,
	}
}
