package main

func (this *_PtyReader_) RunWorker() {
	var bytesRed_PtyDescriptor int
	var exitSignal_PtyReader_maybe error
	for {
		dataOrder_PtyDispatcher := <-this.PtyDispatcher.PoolChannel___Data__Order_PtyDispatcher
		bytesRed_PtyDescriptor, exitSignal_PtyReader_maybe = this.FileDescriptor_Master__Pty__shared.Read(dataOrder_PtyDispatcher.ReadBuffer_PtyDescriptor)
		if bytesRed_PtyDescriptor > 0 && nil == exitSignal_PtyReader_maybe {
			dataOrder_PtyDispatcher.ReadBuffer_PtyDescriptor = dataOrder_PtyDispatcher.ReadBuffer_PtyDescriptor[:bytesRed_PtyDescriptor]
			this.PtyDispatcher.QueueChannel__Order_PtyDispatcher <- dataOrder_PtyDispatcher
		} else if exitSignal_PtyReader_maybe != nil && 0 == bytesRed_PtyDescriptor {
			dataOrder_PtyDispatcher.ReadBuffer_PtyDescriptor = dataOrder_PtyDispatcher.ReadBuffer_PtyDescriptor[:cap(dataOrder_PtyDispatcher.ReadBuffer_PtyDescriptor)]
			this.PtyDispatcher.PoolChannel___Data__Order_PtyDispatcher <- dataOrder_PtyDispatcher
			this.PtyDispatcher.QueueChannel__Order_PtyDispatcher <- &_ExitSignal__Order_PtyDispatcher_{
				ExitSignal: exitSignal_PtyReader_maybe,
			}
			return
		} else {
			// nil == exitSignal_PtyReader_maybe && 0 == bytesRed_PtyDescriptor
			// exitSignal_PtyReader_maybe != nil && bytesRed_PtyDescriptor > 0
			panic("invalid path: _PtyReader_ RunWorker")
		}
	}
}
