package main

import (
	_BYTES "bytes"
	_FMT "fmt"
	_SYSCALL "syscall"
)

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
