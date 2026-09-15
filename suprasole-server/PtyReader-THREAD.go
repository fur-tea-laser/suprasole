package main

import (
	_BYTES "bytes"
	_ERRORS "errors"
	_FMT "fmt"
	_OS "os"
	_SYSCALL "syscall"
)

func (this *_PtyReader_) RunWorker() {
	var bytesRed_PtyDevice int
	var exitSignal_PtyReader_maybe error
	for {
		bytesRed_PtyDevice, exitSignal_PtyReader_maybe = this.FileDescriptor_Master__PtyDevice.Read(this.StagingBuffer[this.Size_UnflushedSlice__StagingBuffer:])
		this.Size_UnflushedSlice__StagingBuffer += bytesRed_PtyDevice
		if this.Size_UnflushedSlice__StagingBuffer > 0 && this.OnTryFlush__(this.StagingBuffer[:this.Size_UnflushedSlice__StagingBuffer]) {
			this.Size_UnflushedSlice__StagingBuffer = 0
		}
		if len(this.StagingBuffer) == this.Size_UnflushedSlice__StagingBuffer {
			this.OnBlockingFlush__(this.StagingBuffer[:this.Size_UnflushedSlice__StagingBuffer])
			this.Size_UnflushedSlice__StagingBuffer = 0
		}
		if exitSignal_PtyReader_maybe != nil && 0 == this.Size_UnflushedSlice__StagingBuffer {
			break
		}
	}
	if _ERRORS.Is(exitSignal_PtyReader_maybe, _OS.ErrClosed) {
		this.OnExited_Closed__(exitSignal_PtyReader_maybe)
	} else if _ERRORS.Is(exitSignal_PtyReader_maybe, _SYSCALL.EIO) {
		this.OnExited_Eio__(exitSignal_PtyReader_maybe)
	} else if exitSignal_PtyReader_maybe != nil {
		this.OnExited_SystemError__(exitSignal_PtyReader_maybe)
	} else {
		// exitSignal_PtyReader_maybe is guaranteed non-nil because a non-nil exitSignal_PtyReader_maybe is required to break out of the for loop above
		_FMT.Println("invalid path: _PtyReader_ RunWorker")
	}
}

func (This *_PtyProxy_) HandleTryFlush(
	UnflushedSlice_StagingBuffer__PtyReader []byte,
) bool {
	return __flushReaderStagingBufferSliceIfLockAcquired(
		This.Mutex.TryLock,
		This,
		UnflushedSlice_StagingBuffer__PtyReader,
	)
}

func (This *_PtyProxy_) HandleBlockingFlush(
	UnflushedSlice_StagingBuffer__PtyReader []byte,
) {
	__flushReaderStagingBufferSliceIfLockAcquired(
		This.LockAndReturnTrue,
		This,
		UnflushedSlice_StagingBuffer__PtyReader,
	)
}

func (This *_PtyProxy_) LockAndReturnTrue() bool {
	This.Mutex.Lock()
	return true
}

func __flushReaderStagingBufferSliceIfLockAcquired(
	maybeAcquirePtyProxyMutexLock__ func() bool,
	PtyProxy_this *_PtyProxy_,
	UnflushedSlice_StagingBuffer__PtyReader []byte,
) bool {
	if maybeAcquirePtyProxyMutexLock__() {
		mode_PtyProxy_captured := PtyProxy_this.Mode_current
		PtyProxy_this.PtyTerminal.Write(UnflushedSlice_StagingBuffer__PtyReader)
		if LIVE_RUNNING__Mode_PtyProxy == mode_PtyProxy_captured {
		} else if PRE_SNAPSHOT__RUNNING___Mode_PtyProxy == mode_PtyProxy_captured {
		} else if POST_SNAPSHOT__RUNNING___Mode_PtyProxy == mode_PtyProxy_captured {
			PtyProxy_this.PostSnapshotBuffer.Write(UnflushedSlice_StagingBuffer__PtyReader)
		} else {
			// SPAWNING__Mode_PtyProxy == mode_PtyProxy_captured
			// EXITED__Mode_PtyProxy == mode_PtyProxy_captured
			_FMT.Println("invalid path: _PtyProxy_ __flushReaderStagingBufferSliceIfLockAcquired")
		}
		PtyProxy_this.Mutex.Unlock()
		if LIVE_RUNNING__Mode_PtyProxy == mode_PtyProxy_captured {
			PtyProxy_this.OnOutput_Live__(
				PtyProxy_this.Id_WorkspacePty,
				_BYTES.Clone(UnflushedSlice_StagingBuffer__PtyReader),
			)
		}
		return true
	}
	return false
}

func (This *_PtyProxy_) HandleExited_Closed(
	exitSignal_PtyReader error,
) {
	__executeExitedTeardown(
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

func (This *_PtyProxy_) HandleExited_Eio(
	exitSignal_PtyReader error,
) {
	__executeExitedTeardown(
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
		_FMT.Println("invalid path: _PtyProxy_ HandleDispatchExited_Eio")
	}
}

func (This *_PtyProxy_) HandleExited_SystemError(
	exitSignal_PtyReader error,
) {
	__executeExitedTeardown(
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

func __executeExitedTeardown(
	onDispatchExited__ func(exitSignal_PtyReader error),
	PtyProxy_this *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	_ = PtyProxy_this.FileDescriptor_Master__PtyDevice.Close()
	_ = PtyProxy_this.PtyCommand.Wait()
	onDispatchExited__(exitSignal_PtyReader)
}

func (This *_WorkspaceController_) HandleOutput_Pty(
	id_WorkspacePty uint32,
	outputData_PtyProxy []byte,
) {
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_current,
		_PtyOutput__PtyMessage_Egress_{
			Id_PtyProxy:         id_WorkspacePty,
			OutputData_PtyProxy: outputData_PtyProxy,
		},
	)
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

func (This *_WorkspaceController_) HandleExited_Closed__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy:   _Closed__ExitOutcome_PtyProxy_{},
	}
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
