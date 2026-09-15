package main

import (
	_OS "os"
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
