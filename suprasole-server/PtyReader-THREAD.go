package main

func (this *_PtyReader_) RunWorker() {
	var bytesRed_PtyDevice int
	var exitSignal_PtyReader_maybe error
	for {
		dataOrder_PtyFlusher := <-this.PtyFlusher.PoolChannel___Data__Order_PtyFlusher
		bytesRed_PtyDevice, exitSignal_PtyReader_maybe = this.FileDescriptor_Master__PtyDevice.Read(dataOrder_PtyFlusher.ReadBuffer_PtyDevice)
		if bytesRed_PtyDevice > 0 && nil == exitSignal_PtyReader_maybe {
			dataOrder_PtyFlusher.ReadBuffer_PtyDevice = dataOrder_PtyFlusher.ReadBuffer_PtyDevice[:bytesRed_PtyDevice]
			this.PtyFlusher.QueueChannel__Order_PtyFlusher <- dataOrder_PtyFlusher
		} else if exitSignal_PtyReader_maybe != nil && 0 == bytesRed_PtyDevice {
			dataOrder_PtyFlusher.ReadBuffer_PtyDevice = dataOrder_PtyFlusher.ReadBuffer_PtyDevice[:cap(dataOrder_PtyFlusher.ReadBuffer_PtyDevice)]
			this.PtyFlusher.PoolChannel___Data__Order_PtyFlusher <- dataOrder_PtyFlusher
			this.PtyFlusher.QueueChannel__Order_PtyFlusher <- &_ExitSignal__Order_PtyFlusher_{
				ExitSignal: exitSignal_PtyReader_maybe,
			}
			break
		} else {
			// nil == exitSignal_PtyReader_maybe && 0 == bytesRed_PtyDevice
			// exitSignal_PtyReader_maybe != nil && bytesRed_PtyDevice > 0
			panic("invalid path: _PtyReader_ RunWorker")
		}
	}
}
