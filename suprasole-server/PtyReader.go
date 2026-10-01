package main

import (
	_OS "os"
)

type _PtyReader_ struct {
	FileDescriptor_Master__PtyDevice *_OS.File
	PtyFlusher                       *_PtyFlusher_
}
