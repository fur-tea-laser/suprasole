package main

import (
	_CONTEXT "context"
	_OS "os"
)

type _PtyWriter_ struct {
	FileDescriptor_Master__PtyDevice *_OS.File
	QueueChannel_InputOrder          chan _InputOrder_PtyWriter_
	WorkerContext                    _CONTEXT.Context
	WorkerCancel                     _CONTEXT.CancelFunc
}

type _InputOrder_PtyWriter_ interface {
	WriteInput(FileDescriptor_Master__PtyDevice *_OS.File)
}

type _Passthrough__InputOrder_PtyWriter_ struct {
	InputData_PtyDevice []byte
}

type _Paced__InputOrder_PtyWriter_ struct {
}

type _Code__InputOrder_PtyWriter_ byte

const (
	PASSTHROUGH___Code__InputOrder_PtyWriter _Code__InputOrder_PtyWriter_ = 0x00
	PACED___Code__InputOrder_PtyWriter       _Code__InputOrder_PtyWriter_ = 0x01
)
