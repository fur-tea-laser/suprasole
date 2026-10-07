package main

import (
	_OS "os"
)

type _PtyReader_ struct {
	FileDescriptor_Master__Pty__shared *_OS.File
	PtyDispatcher                      *_PtyDispatcher_
}
