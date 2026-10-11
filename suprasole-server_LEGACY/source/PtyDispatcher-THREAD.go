package main

import (
	_BYTES "bytes"
)

func (This *_PtyDispatcher_) RunWorker() {
	for {
		select {
		case order_next := <-This.QueueChannel__Order_PtyDispatcher:
			switch order_next.Execute(This) {
			case CONTINUE_WORKER__Directive__Order_PtyDispatcher:
			case EXIT_WORKER__Directive__Order_PtyDispatcher:
				return
			default:
				panic("invalid path: _PtyDispatcher_ RunWorker")
			}
		case <-This.Timer_FlushPacing.C:
			if len(This.StagingBuffer) > 0 {
				This.OnBlockingFlush__(This.StagingBuffer)
				This.StagingBuffer = This.StagingBuffer[:0]
				This.Timer_FlushPacing.Reset(This.Timeout__Timer_FlushPacing)
			} else {
				This.Status_state = IDLE__Status_PtyDispatcher
			}
		}
	}
}

func (this *_Data__Order_PtyDispatcher_) Execute(
	PtyDispatcher_forwarded *_PtyDispatcher_,
) _Directive__Order_PtyDispatcher_ {
	switch PtyDispatcher_forwarded.Status_state {
	case IDLE__Status_PtyDispatcher:
		PtyDispatcher_forwarded.OnBlockingFlush__(this.ReadBuffer_PtyDescriptor)
		PtyDispatcher_forwarded.Timer_FlushPacing.Reset(PtyDispatcher_forwarded.Timeout__Timer_FlushPacing)
		PtyDispatcher_forwarded.Status_state = PACING__Status_PtyDispatcher
	case PACING__Status_PtyDispatcher:
		PtyDispatcher_forwarded.StagingBuffer = append(
			PtyDispatcher_forwarded.StagingBuffer,
			this.ReadBuffer_PtyDescriptor...,
		)
	default:
		panic("invalid path: _Data__Order_PtyDispatcher_ Execute")
	}
	this.ReadBuffer_PtyDescriptor = this.ReadBuffer_PtyDescriptor[:cap(this.ReadBuffer_PtyDescriptor)]
	PtyDispatcher_forwarded.PoolChannel___Data__Order_PtyDispatcher <- this
	return CONTINUE_WORKER__Directive__Order_PtyDispatcher
}

func (this *_ExitSignal__Order_PtyDispatcher_) Execute(
	PtyDispatcher_forwarded *_PtyDispatcher_,
) _Directive__Order_PtyDispatcher_ {
	PtyDispatcher_forwarded.Timer_FlushPacing.Stop()
	if len(PtyDispatcher_forwarded.StagingBuffer) > 0 {
		PtyDispatcher_forwarded.OnBlockingFlush__(PtyDispatcher_forwarded.StagingBuffer)
		PtyDispatcher_forwarded.StagingBuffer = PtyDispatcher_forwarded.StagingBuffer[:0]
	}
	PtyDispatcher_forwarded.OnReaderFinished__(this.ExitSignal)
	return EXIT_WORKER__Directive__Order_PtyDispatcher
}

func (This *_PtyProxy_) HandleBlockingFlush(
	UnflushedSlice_StagingBuffer__PtyDispatcher []byte,
) {
	This.Mutex.Lock()
	mode_PtyProxy_captured := This.Mode_state
	This.PtyTerminal.Write(UnflushedSlice_StagingBuffer__PtyDispatcher)
	switch mode_PtyProxy_captured {
	case LIVE_RUNNING__Mode_PtyProxy:
	case PRE_SNAPSHOT__RUNNING___Mode_PtyProxy:
	case POST_SNAPSHOT__RUNNING___Mode_PtyProxy:
		This.PostSnapshotBuffer.Write(UnflushedSlice_StagingBuffer__PtyDispatcher)
	default:
		// SPAWNING__Mode_PtyProxy
		// EXITED__Mode_PtyProxy
		panic("invalid path: _PtyProxy_ HandleBlockingFlush")
	}
	This.Mutex.Unlock()
	if LIVE_RUNNING__Mode_PtyProxy == mode_PtyProxy_captured {
		This.OnOutput_Live__(
			This.Id_WorkspacePty,
			_BYTES.Clone(UnflushedSlice_StagingBuffer__PtyDispatcher),
		)
	}
}

// *****************************************************************************
// func (This *_WorkspaceController_) HandleOutput_Pty
// *****************************************************************************
