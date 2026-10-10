package main

import (
	_BYTES "bytes"
	_ERRORS "errors"
	// [phase 3] _OS "os"
	_SYSCALL "syscall"
	// [phase 3] _TIME "time"
	// [phase 3] _UNSAFE "unsafe"
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
	// [phase 3] PtyDispatcher_forwarded.OnExited__(this.ExitSignal)
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

// [phase 3]
// func (This *_PtyProxy_) HandleExited(
// 	exitSignal_PtyReader error,
// ) {
// 	_ = This.FileDescriptor_Master__Pty__shared.Close()
// 	teardownReport := This.TeardownExited_PtyProcess()
// 	if _ERRORS.Is(exitSignal_PtyReader, _SYSCALL.EIO) || _ERRORS.Is(exitSignal_PtyReader, _OS.ErrClosed) {
// 		This.OnExitOutcome__(
// 			This.Id_WorkspacePty,
// 			teardownReport.Resolve__ExitOutcome_PtyProxy(),
// 		)
// 	} else if exitSignal_PtyReader != nil {
// 		This.OnExitOutcome__(
// 			This.Id_WorkspacePty,
// 			_SystemError_Descriptor__ExitOutcome_PtyProxy_{
// 				SystemError_PtyDescriptor: exitSignal_PtyReader,
// 			},
// 		)
// 	} else {
// 		// nil == exitSignal_PtyReader
// 		panic("invalid path: _PtyProxy_ HandleExited")
// 	}
// }

// [phase 3]
// func (This *_PtyProxy_) TeardownExited_PtyProcess() _TeardownReport_PtyProcess_ {
// 	timer_Unresponsive := _TIME.NewTimer(TIMEOUT_UNRESPONSIVE__PtyProcess)
// 	select {
// 	case <-This.PtyProcess.Channel_FinalState:
// 		This.PtyProcess.Mutex.Lock()
// 		State__PtyProcess__final := This.PtyProcess.State_state
// 		This.PtyProcess.Mutex.Unlock()
// 		switch State__PtyProcess__the := State__PtyProcess__final.(type) {
// 		case *_Reaped__State_PtyProcess_:
// 			return State__PtyProcess__the
// 		case *_SystemError__State_PtyProcess_:
// 			return State__PtyProcess__the
// 		default:
// 			// *_Alive__State_PtyProcess_
// 			// *_Zombie_Withheld__State_PtyProcess_
// 			// *_Zombie__State_PtyProcess_
// 			panic("invalid path: _PtyProxy_ TeardownExited_PtyProcess [FINAL_STATE]")
// 		}
// 	case <-timer_Unresponsive.C:
// 	}
// 	This.PtyProcess.Mutex.Lock()
// 	report_originating := This.PtyProcess.State_state.ProbeStatus_Unresponsive()
// 	_ = This.PtyProcess.State_state.Signal(
// 		_SYSCALL.SIGKILL,
// 		PIDFD_SIGNAL_PROCESS_GROUP__Linux,
// 	)
// 	This.PtyProcess.Mutex.Unlock()
// 	timer__Unresponsive_Uninterruptible := _TIME.NewTimer(TIMEOUT_UNINTERRUPTIBLE__PtyProcess)
// 	select {
// 	case <-This.PtyProcess.Channel_FinalState:
// 		This.PtyProcess.Mutex.Lock()
// 		State__PtyProcess__final := This.PtyProcess.State_state
// 		This.PtyProcess.Mutex.Unlock()
// 		switch State__PtyProcess__the := State__PtyProcess__final.(type) {
// 		case *_Reaped__State_PtyProcess_:
// 			return report_originating
// 		case *_SystemError__State_PtyProcess_:
// 			return State__PtyProcess__the
// 		default:
// 			// *_Alive__State_PtyProcess_
// 			// *_Zombie_Withheld__State_PtyProcess_
// 			// *_Zombie__State_PtyProcess_
// 			panic("invalid path: _PtyProxy_ TeardownExited_PtyProcess [FINAL_STATE_KILLED]")
// 		}
// 	case <-timer__Unresponsive_Uninterruptible.C:
// 		return _Unresponsive_Uninterruptible__TeardownReport_PtyProcess_{}
// 	}
// }

// [phase 3]
// const (
// 	P_PID__WAITID__Linux = 1
// )

// [phase 3]
// func (this *_Alive__State_PtyProcess_) ProbeStatus_Unresponsive() _TeardownReport_PtyProcess_ {
// 	return probeStatus__Pid_Unresponsive(this.Pid)
// }

// [phase 3]
// func (this *_Zombie_Withheld__State_PtyProcess_) ProbeStatus_Unresponsive() _TeardownReport_PtyProcess_ {
// 	return probeStatus__Pid_Unresponsive(this.Pid)
// }

// [phase 3]
// func (this *_Zombie__State_PtyProcess_) ProbeStatus_Unresponsive() _TeardownReport_PtyProcess_ {
// 	return probeStatus__Pid_Unresponsive(this.Pid)
// }

// [phase 3]
// func (this *_Reaped__State_PtyProcess_) ProbeStatus_Unresponsive() _TeardownReport_PtyProcess_ {
// 	return this
// }

// [phase 3]
// func (this *_SystemError__State_PtyProcess_) ProbeStatus_Unresponsive() _TeardownReport_PtyProcess_ {
// 	return this
// }

// [phase 3]
// func probeStatus__Pid_Unresponsive(
// 	pid_PtyProcess int,
// ) _TeardownReport_PtyProcess_ {
// 	var sigInfo_result _SigInfo_Waitid_
// 	returnCode__WAITID, _, errorCode__WAITID := _SYSCALL.Syscall6(
// 		_SYSCALL.SYS_WAITID,
// 		uintptr(P_PID__WAITID__Linux),
// 		uintptr(pid_PtyProcess),
// 		uintptr(_UNSAFE.Pointer(&sigInfo_result)),
// 		uintptr(_SYSCALL.WSTOPPED|_SYSCALL.WNOHANG|_SYSCALL.WNOWAIT),
// 		0,
// 		0,
// 	)
// 	if returnCode__WAITID == 0 && errorCode__WAITID == 0 && int32(pid_PtyProcess) == sigInfo_result.Pid && (CLD_STOPPED__SIGCHLD__Linux == sigInfo_result.Code || CLD_TRAPPED__SIGCHLD__Linux == sigInfo_result.Code) {
// 		return _Unresponsive_Stopped__TeardownReport_PtyProcess_{
// 			StopSignal_Teardown: _SYSCALL.Signal(sigInfo_result.Status),
// 		}
// 	} else if returnCode__WAITID == 0 && errorCode__WAITID == 0 && 0 == sigInfo_result.Pid {
// 		return _Unresponsive_Rogue__TeardownReport_PtyProcess_{}
// 	} else {
// 		return _SystemError__TeardownReport_PtyProcess_{
// 			SystemError_Wait: _ERRORS.New("unresolved process exit state: process reaping took too long (you should play the lottery)"),
// 		}
// 	}
// }

type _TeardownReport_PtyProcess_ interface {
	Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_
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
