package main

import (
	_BYTES "bytes"
	_ERRORS "errors"
	_OS "os"
	_SYSCALL "syscall"
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
		PtyDispatcher_forwarded.OnBlockingFlush__(this.ReadBuffer_PtyDevice)
		PtyDispatcher_forwarded.Timer_FlushPacing.Reset(PtyDispatcher_forwarded.Timeout__Timer_FlushPacing)
		PtyDispatcher_forwarded.Status_state = PACING__Status_PtyDispatcher
	case PACING__Status_PtyDispatcher:
		PtyDispatcher_forwarded.StagingBuffer = append(
			PtyDispatcher_forwarded.StagingBuffer,
			this.ReadBuffer_PtyDevice...,
		)
	default:
		panic("invalid path: _Data__Order_PtyDispatcher_ Execute")
	}
	this.ReadBuffer_PtyDevice = this.ReadBuffer_PtyDevice[:cap(this.ReadBuffer_PtyDevice)]
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
	if _ERRORS.Is(this.ExitSignal, _OS.ErrClosed) {
		PtyDispatcher_forwarded.OnExited_Closed__(this.ExitSignal)
	} else if _ERRORS.Is(this.ExitSignal, _SYSCALL.EIO) {
		PtyDispatcher_forwarded.OnExited_Eio__(this.ExitSignal)
	} else if this.ExitSignal != nil {
		PtyDispatcher_forwarded.OnExited_SystemError__(this.ExitSignal)
	} else {
		panic("invalid path: _ExitSignal__Order_PtyDispatcher_ Execute")
	}
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

func (This *_PtyProxy_) HandleExited_Closed(
	exitSignal_PtyReader error,
) {
	__executeTeardown_Exited(
		This.HandleDispatchExited_Closed,
		This,
		exitSignal_PtyReader,
	)
}

func (This *_PtyProxy_) HandleDispatchExited_Closed(
	_ error,
) {
	This.OnExited_Closed__(This)
}

func (This *_WorkspaceController_) HandleExited_Closed__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy:   _Closed__ExitOutcome_PtyProxy_{},
	}
}

func (This *_PtyProxy_) HandleExited_Eio(
	exitSignal_PtyReader error,
) {
	__executeTeardown_Exited(
		This.HandleDispatchExited_Eio,
		This,
		exitSignal_PtyReader,
	)
}

func (This *_PtyProxy_) HandleDispatchExited_Eio(
	_ error,
) {
	waitStatus_PtyProcess := This.PtyCommand.ProcessState.Sys().(_SYSCALL.WaitStatus)
	if waitStatus_PtyProcess.Signaled() {
		This.OnExited_Eio_Killed__(This)
	} else if waitStatus_PtyProcess.Exited() && 0 == waitStatus_PtyProcess.ExitStatus() {
		This.OnExited_Eio_Success__(This)
	} else if waitStatus_PtyProcess.Exited() {
		This.OnExited_Eio_Failure__(This)
	} else {
		panic("invalid path: _PtyProxy_ HandleDispatchExited_Eio")
	}
}

func (This *_WorkspaceController_) HandleExited_Eio_Success__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy:   _Success__ExitOutcome_PtyProxy_{},
	}
}

func (This *_WorkspaceController_) HandleExited_Eio_Failure__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy: _Failure__ExitOutcome_PtyProxy_{
			ExitCode_PtyProcess: PtyProxy_exited.PtyCommand.ProcessState.ExitCode(),
		},
	}
}

func (This *_WorkspaceController_) HandleExited_Eio_Killed__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	waitStatus_PtyProcess := PtyProxy_exited.PtyCommand.ProcessState.Sys().(_SYSCALL.WaitStatus)
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy: _Killed__ExitOutcome_PtyProxy_{
			ExitSignal_PtyProcess: int(waitStatus_PtyProcess.Signal()),
		},
	}
}

func (This *_PtyProxy_) HandleExited_SystemError(
	exitSignal_PtyReader error,
) {
	__executeTeardown_Exited(
		This.HandleDispatchExited_SystemError,
		This,
		exitSignal_PtyReader,
	)
}

func (This *_PtyProxy_) HandleDispatchExited_SystemError(
	exitSignal_PtyReader error,
) {
	This.OnExited_SystemError__(
		This,
		exitSignal_PtyReader,
	)
}

func (This *_WorkspaceController_) HandleExited_SystemError__Pty(
	PtyProxy_exited *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy: _SystemError__ExitOutcome_PtyProxy_{
			SystemError_PtyDevice: exitSignal_PtyReader,
		},
	}
}

func __executeTeardown_Exited(
	onDispatchExited__ func(exitSignal_PtyReader error),
	PtyProxy_this *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	_ = PtyProxy_this.FileDescriptor_Master__PtyDevice.Close()
	_ = PtyProxy_this.PtyCommand.Wait()
	onDispatchExited__(exitSignal_PtyReader)
}
