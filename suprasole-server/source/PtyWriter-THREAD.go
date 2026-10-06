package main

import (
	_OS "os"
)

func (this *_PtyWriter_) RunWorker() {
	for {
		select {
		case <-this.WorkerContext.Done():
			return
		case inputOrder_next := <-this.QueueChannel_InputOrder:
			inputOrder_next.WriteInput(this.FileDescriptor_Master__PtyDevice)
		}
	}
}

func (this *_Passthrough__InputOrder_PtyWriter_) WriteInput(
	FileDescriptor_Master__PtyDevice *_OS.File,
) {
	if len(this.InputData_PtyDevice) > 0 {
		_, _ = FileDescriptor_Master__PtyDevice.Write(this.InputData_PtyDevice)
	}
}

func (this *_Paced__InputOrder_PtyWriter_) WriteInput(
	FileDescriptor_Master__PtyDevice *_OS.File,
) {
}
