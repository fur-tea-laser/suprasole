package main

import (
	_ERRORS "errors"
	_FMT "fmt"
	_OS "os"
	_SYSCALL "syscall"
)

type _PtyReader_ struct {
	OnTryFlush__                       func(UnflushedSlice_StagingBuffer []byte) bool
	OnBlockingFlush__                  func(UnflushedSlice_StagingBuffer []byte)
	OnExited_Closed__                  func(exitSignal_PtyReader error)
	OnExited_Eio__                     func(exitSignal_PtyReader error)
	OnExited_SystemError__             func(exitSignal_PtyReader error)
	FileDescriptor_Master__PtyDevice   *_OS.File
	StagingBuffer                      []byte
	Size_UnflushedSlice__StagingBuffer int
}

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
