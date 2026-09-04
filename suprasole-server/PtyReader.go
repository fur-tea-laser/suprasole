package main

import (
	_ERRORS "errors"
	_FMT "fmt"
	_OS "os"
	_SYSCALL "syscall"
)

type _PtyReader_ struct {
	OnTryFlush__                     func(unflushedStagingBufferSlice []byte) bool
	OnBlockingFlush__                func(unflushedStagingBufferSlice []byte)
	OnExited_Closed__                func(exitSignal_PtyReader error)
	OnExited_Eio__                   func(exitSignal_PtyReader error)
	OnExited_SystemError__           func(exitSignal_PtyReader error)
	StagingBuffer                    []byte
	UnflushedSliceSize_StagingBuffer int
	MasterFileDescriptor_PtyDevice   *_OS.File
}

func (this *_PtyReader_) RunWorker() {
	var bytesRed_PtyDevice int
	var exitSignal_PtyReader error
	for {
		bytesRed_PtyDevice, exitSignal_PtyReader = this.MasterFileDescriptor_PtyDevice.Read(this.StagingBuffer[this.UnflushedSliceSize_StagingBuffer:])
		this.UnflushedSliceSize_StagingBuffer += bytesRed_PtyDevice
		if this.UnflushedSliceSize_StagingBuffer > 0 && this.OnTryFlush__(this.StagingBuffer[:this.UnflushedSliceSize_StagingBuffer]) {
			this.UnflushedSliceSize_StagingBuffer = 0
		}
		if len(this.StagingBuffer) == this.UnflushedSliceSize_StagingBuffer {
			this.OnBlockingFlush__(this.StagingBuffer[:this.UnflushedSliceSize_StagingBuffer])
			this.UnflushedSliceSize_StagingBuffer = 0
		}
		if exitSignal_PtyReader != nil && 0 == this.UnflushedSliceSize_StagingBuffer {
			break
		}
	}
	if _ERRORS.Is(exitSignal_PtyReader, _OS.ErrClosed) {
		this.OnExited_Closed__(exitSignal_PtyReader)
	} else if _ERRORS.Is(exitSignal_PtyReader, _SYSCALL.EIO) {
		this.OnExited_Eio__(exitSignal_PtyReader)
	} else if exitSignal_PtyReader != nil {
		this.OnExited_SystemError__(exitSignal_PtyReader)
	} else {
		// exitSignal_PtyReader is guaranteed non-nil because a non-nil exitSignal_PtyReader is required to break out of the for loop above
		_FMT.Println("invalid path: _PtyReader_ RunWorker")
	}
}
