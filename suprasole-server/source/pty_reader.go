package source

import (
	_ERRORS  "errors"
	_FMT     "fmt"
	_OS      "os"
	_SYSCALL "syscall"
)

type _PtyReader_ struct {
	PtyMasterFileDescriptor         *_OS.File
	StagingBuffer                   []byte
	UnflushedStagingBufferSliceSize int
	OnTryFlush                      func(unflushedStagingBufferSlice []byte) bool
	OnBlockingFlush                 func(unflushedStagingBufferSlice []byte)
	OnExited_Closed                 func(readerTerminalSignal error)
	OnExited_Eio                    func(readerTerminalSignal error)
	OnExited_SystemError            func(readerTerminalSignal error)
}

func (this *_PtyReader_) StartReading() {
	var ptyBytesRed int
	var terminalSignal error
	for {
		ptyBytesRed, terminalSignal = this.PtyMasterFileDescriptor.Read(this.StagingBuffer[this.UnflushedStagingBufferSliceSize:])
		this.UnflushedStagingBufferSliceSize += ptyBytesRed
		if this.UnflushedStagingBufferSliceSize > 0 && this.OnTryFlush(this.StagingBuffer[:this.UnflushedStagingBufferSliceSize]) {
			this.UnflushedStagingBufferSliceSize = 0
		} 
		if len(this.StagingBuffer) == this.UnflushedStagingBufferSliceSize {
			this.OnBlockingFlush(this.StagingBuffer[:this.UnflushedStagingBufferSliceSize])
			this.UnflushedStagingBufferSliceSize = 0
		}
		if terminalSignal != nil && 0 == this.UnflushedStagingBufferSliceSize {
			break
		}
	}
	if _ERRORS.Is(terminalSignal, _OS.ErrClosed) {
		this.OnExited_Closed(terminalSignal)
	} else if _ERRORS.Is(terminalSignal, _SYSCALL.EIO) {
		this.OnExited_Eio(terminalSignal)
	} else if terminalSignal != nil {
		this.OnExited_SystemError(terminalSignal)
	} else {
		// terminalSignal is guaranteed non-nil because a non-nil terminalSignal is required to break out of the for loop above
		_FMT.Println("invalid path: _PtyReader_ StartReading")
	}
}
