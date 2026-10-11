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
			inputOrder_next.WriteInput(this.FileDescriptor_Master__Pty__shared)
		}
	}
}

func (this *_Passthrough__InputOrder_PtyWriter_) WriteInput(
	FileDescriptor_Master__Pty__shared *_OS.File,
) {
	if len(this.InputData_PtyDescriptor) > 0 {
		_, _ = FileDescriptor_Master__Pty__shared.Write(this.InputData_PtyDescriptor)
	}
}

func (this *_Paced__InputOrder_PtyWriter_) WriteInput(
	FileDescriptor_Master__Pty__shared *_OS.File,
) {
}
