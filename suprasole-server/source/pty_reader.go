package source

import (
	"errors"
	"os"
	"syscall"
)

type PtyReader struct {
	Pty_MasterFileDescriptor               *os.File
	Reader_StagingBuffer                   []byte
	Reader_UnflushedStagingBufferSliceSize int
	Reader_OnTryFlush                      func(unflushedStagingBufferSlice []byte) bool
	Reader_OnBlockingFlush                 func(unflushedStagingBufferSlice []byte)
	Reader_OnExited_Closed                 func(readerTerminalSignal error)
	Reader_OnExited_Eio                    func(readerTerminalSignal error)
	Reader_OnExited_SystemError            func(readerTerminalSignal error)
}

func (thisReader *PtyReader) Reader_StartReading() {
	var ptyBytesRed int
	var terminalSignal error
	for {
		ptyBytesRed, terminalSignal = thisReader.Pty_MasterFileDescriptor.Read(thisReader.Reader_StagingBuffer[thisReader.Reader_UnflushedStagingBufferSliceSize:])
		thisReader.Reader_UnflushedStagingBufferSliceSize += ptyBytesRed
		if thisReader.Reader_UnflushedStagingBufferSliceSize > 0 && thisReader.Reader_OnTryFlush(thisReader.Reader_StagingBuffer[:thisReader.Reader_UnflushedStagingBufferSliceSize]) {
			thisReader.Reader_UnflushedStagingBufferSliceSize = 0
		} else if thisReader.Reader_UnflushedStagingBufferSliceSize == len(thisReader.Reader_StagingBuffer) {
			thisReader.Reader_OnBlockingFlush(thisReader.Reader_StagingBuffer[:thisReader.Reader_UnflushedStagingBufferSliceSize])
			thisReader.Reader_UnflushedStagingBufferSliceSize = 0
		}
		if terminalSignal != nil && thisReader.Reader_UnflushedStagingBufferSliceSize == 0 {
			break
		}
	}
	if errors.Is(terminalSignal, os.ErrClosed) {
		thisReader.Reader_OnExited_Closed(terminalSignal)
	} else if errors.Is(terminalSignal, syscall.EIO) {
		thisReader.Reader_OnExited_Eio(terminalSignal)
	} else if terminalSignal != nil {
		thisReader.Reader_OnExited_SystemError(terminalSignal)
	}
}
