package source

import (
	_ERRORS  "errors"
	_FMT     "fmt"
	_OS      "os"
	_SYSCALL "syscall"
)

type _PtyReader_ struct {
	Pty_MasterFileDescriptor               *_OS.File
	Reader_StagingBuffer                   []byte
	Reader_UnflushedStagingBufferSliceSize int
	Reader_OnTryFlush                      func(unflushedStagingBufferSlice []byte) bool
	Reader_OnBlockingFlush                 func(unflushedStagingBufferSlice []byte)
	Reader_OnExited_Closed                 func(readerTerminalSignal error)
	Reader_OnExited_Eio                    func(readerTerminalSignal error)
	Reader_OnExited_SystemError            func(readerTerminalSignal error)
}

func (thisReader *_PtyReader_) Reader_StartReading() {
	var ptyBytesRed int
	var terminalSignal error
	for {
		ptyBytesRed, terminalSignal = thisReader.Pty_MasterFileDescriptor.Read(thisReader.Reader_StagingBuffer[thisReader.Reader_UnflushedStagingBufferSliceSize:])
		thisReader.Reader_UnflushedStagingBufferSliceSize += ptyBytesRed
		if thisReader.Reader_UnflushedStagingBufferSliceSize > 0 && thisReader.Reader_OnTryFlush(thisReader.Reader_StagingBuffer[:thisReader.Reader_UnflushedStagingBufferSliceSize]) {
			thisReader.Reader_UnflushedStagingBufferSliceSize = 0
		} 
		if len(thisReader.Reader_StagingBuffer) == thisReader.Reader_UnflushedStagingBufferSliceSize {
			thisReader.Reader_OnBlockingFlush(thisReader.Reader_StagingBuffer[:thisReader.Reader_UnflushedStagingBufferSliceSize])
			thisReader.Reader_UnflushedStagingBufferSliceSize = 0
		}
		if terminalSignal != nil && 0 == thisReader.Reader_UnflushedStagingBufferSliceSize {
			break
		}
	}
	if _ERRORS.Is(terminalSignal, _OS.ErrClosed) {
		thisReader.Reader_OnExited_Closed(terminalSignal)
	} else if _ERRORS.Is(terminalSignal, _SYSCALL.EIO) {
		thisReader.Reader_OnExited_Eio(terminalSignal)
	} else if terminalSignal != nil {
		thisReader.Reader_OnExited_SystemError(terminalSignal)
	} else {
		// terminalSignal is guaranteed non-nil because a non-nil terminalSignal is required to break out of the for loop above
		_FMT.Println("invalid path: _PtyReader_ Reader_StartReading")
	}
}
