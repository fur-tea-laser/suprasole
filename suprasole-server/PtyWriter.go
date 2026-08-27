package main

import (
	_OS "os"
)

type Code_InputOrder_PtyWriter byte

const (
	PASSTHROUGH__Code_InputOrder_PtyWriter Code_InputOrder_PtyWriter = 0x00
	PACED__Code_InputOrder_PtyWriter       Code_InputOrder_PtyWriter = 0x01
)

type _InputOrder_PtyWriter_ interface {
	WriteInput(ptyMasterFileDescriptor *_OS.File)
}

type _Passthrough__InputOrder_PtyWriter_ struct {
	InputData_PtyMasterFileDescriptor []byte
}

func (this *_Passthrough__InputOrder_PtyWriter_) WriteInput(
	ptyMasterFileDescriptor *_OS.File,
) {
	if len(this.InputData_PtyMasterFileDescriptor) > 0 {
		_, _ = ptyMasterFileDescriptor.Write(this.InputData_PtyMasterFileDescriptor)
	}
}

type _Paced__InputOrder_PtyWriter_ struct {
}

func (this *_Paced__InputOrder_PtyWriter_) WriteInput(
	ptyMasterFileDescriptor *_OS.File,
) {
}

type _PtyWriter_ struct {
	PtyMasterFileDescriptor *_OS.File
	QueueChannel_InputOrder chan _InputOrder_PtyWriter_
}

func (this *_PtyWriter_) RunWorker() {
	for inputOrder := range this.QueueChannel_InputOrder {
		inputOrder.WriteInput(this.PtyMasterFileDescriptor)
	}
}
