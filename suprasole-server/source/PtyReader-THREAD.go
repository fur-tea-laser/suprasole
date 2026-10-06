package main

func (this *_PtyReader_) RunWorker() {
	var bytesRed_PtyDevice int
	var exitSignal_PtyReader_maybe error
	for {
		dataOrder_PtyDispatcher := <-this.PtyDispatcher.PoolChannel___Data__Order_PtyDispatcher
		bytesRed_PtyDevice, exitSignal_PtyReader_maybe = this.FileDescriptor_Master__PtyDevice.Read(dataOrder_PtyDispatcher.ReadBuffer_PtyDevice)
		if bytesRed_PtyDevice > 0 && nil == exitSignal_PtyReader_maybe {
			dataOrder_PtyDispatcher.ReadBuffer_PtyDevice = dataOrder_PtyDispatcher.ReadBuffer_PtyDevice[:bytesRed_PtyDevice]
			this.PtyDispatcher.QueueChannel__Order_PtyDispatcher <- dataOrder_PtyDispatcher
		} else if exitSignal_PtyReader_maybe != nil && 0 == bytesRed_PtyDevice {
			dataOrder_PtyDispatcher.ReadBuffer_PtyDevice = dataOrder_PtyDispatcher.ReadBuffer_PtyDevice[:cap(dataOrder_PtyDispatcher.ReadBuffer_PtyDevice)]
			this.PtyDispatcher.PoolChannel___Data__Order_PtyDispatcher <- dataOrder_PtyDispatcher
			this.PtyDispatcher.QueueChannel__Order_PtyDispatcher <- &_ExitSignal__Order_PtyDispatcher_{
				ExitSignal: exitSignal_PtyReader_maybe,
			}
			return
		} else {
			// nil == exitSignal_PtyReader_maybe && 0 == bytesRed_PtyDevice
			// exitSignal_PtyReader_maybe != nil && bytesRed_PtyDevice > 0
			panic("invalid path: _PtyReader_ RunWorker")
		}
	}
}
