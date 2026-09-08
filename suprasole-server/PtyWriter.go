package main

import (
	_CONTEXT "context"
	_OS "os"
)

type _Code__InputOrder_PtyWriter_ byte

const (
	PASSTHROUGH___Code__InputOrder_PtyWriter _Code__InputOrder_PtyWriter_ = 0x00
	PACED___Code__InputOrder_PtyWriter       _Code__InputOrder_PtyWriter_ = 0x01
)

type _InputOrder_PtyWriter_ interface {
	WriteInput(FileDescriptor_Master__PtyDevice *_OS.File)
}

type _Passthrough__InputOrder_PtyWriter_ struct {
	InputData_PtyDevice []byte
}

func (this *_Passthrough__InputOrder_PtyWriter_) WriteInput(
	FileDescriptor_Master__PtyDevice *_OS.File,
) {
	if len(this.InputData_PtyDevice) > 0 {
		_, _ = FileDescriptor_Master__PtyDevice.Write(this.InputData_PtyDevice)
	}
}

type _Paced__InputOrder_PtyWriter_ struct {
}

func (this *_Paced__InputOrder_PtyWriter_) WriteInput(
	FileDescriptor_Master__PtyDevice *_OS.File,
) {
}

type _PtyWriter_ struct {
	FileDescriptor_Master__PtyDevice *_OS.File
	QueueChannel_InputOrder          chan _InputOrder_PtyWriter_
	WorkerContext                    _CONTEXT.Context
	WorkerCancel                     _CONTEXT.CancelFunc
}

func (this *_PtyWriter_) RunWorker() {
	for {
		select {
		case <-this.WorkerContext.Done():
			return
		case nextInputOrder := <-this.QueueChannel_InputOrder:
			nextInputOrder.WriteInput(this.FileDescriptor_Master__PtyDevice)
		}
	}
}
