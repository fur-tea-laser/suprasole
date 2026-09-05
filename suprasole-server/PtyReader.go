package main

import (
	_ERRORS "errors"
	_FMT "fmt"
	_OS "os"
	_SYSCALL "syscall"
)

type _PtyReader_ struct {
	OnTryFlush__                     func(unflushedSlice_StagingBuffer []byte) bool
	OnBlockingFlush__                func(unflushedSlice_StagingBuffer []byte)
	OnExited_Closed__                func(exitSignal_PtyReader error)
	OnExited_Eio__                   func(exitSignal_PtyReader error)
	OnExited_SystemError__           func(exitSignal_PtyReader error)
	FileDescriptor_Master__PtyDevice *_OS.File
	StagingBuffer                    []byte
	UnflushedSliceSize_StagingBuffer int
}

func (this *_PtyReader_) RunWorker() {
	var bytesRed_PtyDevice int
	var maybeExitSignal_PtyReader error
	for {
		bytesRed_PtyDevice, maybeExitSignal_PtyReader = this.FileDescriptor_Master__PtyDevice.Read(this.StagingBuffer[this.UnflushedSliceSize_StagingBuffer:])
		this.UnflushedSliceSize_StagingBuffer += bytesRed_PtyDevice
		if this.UnflushedSliceSize_StagingBuffer > 0 && this.OnTryFlush__(this.StagingBuffer[:this.UnflushedSliceSize_StagingBuffer]) {
			this.UnflushedSliceSize_StagingBuffer = 0
		}
		if len(this.StagingBuffer) == this.UnflushedSliceSize_StagingBuffer {
			this.OnBlockingFlush__(this.StagingBuffer[:this.UnflushedSliceSize_StagingBuffer])
			this.UnflushedSliceSize_StagingBuffer = 0
		}
		if maybeExitSignal_PtyReader != nil && 0 == this.UnflushedSliceSize_StagingBuffer {
			break
		}
	}
	if _ERRORS.Is(maybeExitSignal_PtyReader, _OS.ErrClosed) {
		this.OnExited_Closed__(maybeExitSignal_PtyReader)
	} else if _ERRORS.Is(maybeExitSignal_PtyReader, _SYSCALL.EIO) {
		this.OnExited_Eio__(maybeExitSignal_PtyReader)
	} else if maybeExitSignal_PtyReader != nil {
		this.OnExited_SystemError__(maybeExitSignal_PtyReader)
	} else {
		// maybeExitSignal_PtyReader is guaranteed non-nil because a non-nil maybeExitSignal_PtyReader is required to break out of the for loop above
		_FMT.Println("invalid path: _PtyReader_ RunWorker")
	}
}
