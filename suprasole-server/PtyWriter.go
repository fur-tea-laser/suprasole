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
	WriteInput(masterFileDescriptor_ptyDevice *_OS.File)
}

type _Passthrough__InputOrder_PtyWriter_ struct {
	InputData_PtyDevice []byte
}

func (this *_Passthrough__InputOrder_PtyWriter_) WriteInput(
	masterFileDescriptor_ptyDevice *_OS.File,
) {
	if len(this.InputData_PtyDevice) > 0 {
		_, _ = masterFileDescriptor_ptyDevice.Write(this.InputData_PtyDevice)
	}
}

type _Paced__InputOrder_PtyWriter_ struct {
}

func (this *_Paced__InputOrder_PtyWriter_) WriteInput(
	masterFileDescriptor_ptyDevice *_OS.File,
) {
}

type _PtyWriter_ struct {
	MasterFileDescriptor_PtyDevice *_OS.File
	QueueChannel_InputOrder        chan _InputOrder_PtyWriter_
	WorkerContext                  _CONTEXT.Context
	WorkerCancel                   _CONTEXT.CancelFunc
}

func (this *_PtyWriter_) RunWorker() {
	for {
		select {
		case <-this.WorkerContext.Done():
			return
		case inputOrder := <-this.QueueChannel_InputOrder:
			inputOrder.WriteInput(this.MasterFileDescriptor_PtyDevice)
		}
	}
}
