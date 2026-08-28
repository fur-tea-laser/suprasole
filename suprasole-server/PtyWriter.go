package main

import (
	_CONTEXT "context"
	_OS "os"
)

type Code_InputOrder_PtyWriter byte

const (
	PASSTHROUGH__Code_InputOrder_PtyWriter Code_InputOrder_PtyWriter = 0x00
	PACED__Code_InputOrder_PtyWriter       Code_InputOrder_PtyWriter = 0x01
)

type _InputOrder_PtyWriter_ interface {
	WriteInput(masterFileDescriptor_ptyDevice *_OS.File)
}

type _Passthrough__InputOrder_PtyWriter_ struct {
	InputData_PtyMaster []byte
}

func (this *_Passthrough__InputOrder_PtyWriter_) WriteInput(
	masterFileDescriptor_ptyDevice *_OS.File,
) {
	if len(this.InputData_PtyMaster) > 0 {
		_, _ = masterFileDescriptor_ptyDevice.Write(this.InputData_PtyMaster)
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
