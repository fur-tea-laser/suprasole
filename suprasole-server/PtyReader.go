package main

import (
	_ERRORS "errors"
	_FMT "fmt"
	_OS "os"
	_SYSCALL "syscall"
)

type _PtyReader_ struct {
	MasterFileDescriptor_PtyDevice  *_OS.File
	StagingBuffer                   []byte
	UnflushedStagingBufferSliceSize int
	OnTryFlush                      func(unflushedStagingBufferSlice []byte) bool
	OnBlockingFlush                 func(unflushedStagingBufferSlice []byte)
	OnExited_Closed                 func(exitSignal_PtyReader error)
	OnExited_Eio                    func(exitSignal_PtyReader error)
	OnExited_SystemError            func(exitSignal_PtyReader error)
}

func (this *_PtyReader_) RunWorker() {
	var ptyBytesRed int
	var exitSignal_PtyReader error
	for {
		ptyBytesRed, exitSignal_PtyReader = this.MasterFileDescriptor_PtyDevice.Read(this.StagingBuffer[this.UnflushedStagingBufferSliceSize:])
		this.UnflushedStagingBufferSliceSize += ptyBytesRed
		if this.UnflushedStagingBufferSliceSize > 0 && this.OnTryFlush(this.StagingBuffer[:this.UnflushedStagingBufferSliceSize]) {
			this.UnflushedStagingBufferSliceSize = 0
		}
		if len(this.StagingBuffer) == this.UnflushedStagingBufferSliceSize {
			this.OnBlockingFlush(this.StagingBuffer[:this.UnflushedStagingBufferSliceSize])
			this.UnflushedStagingBufferSliceSize = 0
		}
		if exitSignal_PtyReader != nil && 0 == this.UnflushedStagingBufferSliceSize {
			break
		}
	}
	if _ERRORS.Is(exitSignal_PtyReader, _OS.ErrClosed) {
		this.OnExited_Closed(exitSignal_PtyReader)
	} else if _ERRORS.Is(exitSignal_PtyReader, _SYSCALL.EIO) {
		this.OnExited_Eio(exitSignal_PtyReader)
	} else if exitSignal_PtyReader != nil {
		this.OnExited_SystemError(exitSignal_PtyReader)
	} else {
		// exitSignal_PtyReader is guaranteed non-nil because a non-nil exitSignal_PtyReader is required to break out of the for loop above
		_FMT.Println("invalid path: _PtyReader_ RunWorker")
	}
}
