package main

import (
	_PTY "github.com/creack/pty"
)

func (this *_PtyResizer_) RunWorker(PtyProxy_spawned *_PtyProxy_) {
	for {
		select {
		case <-this.WorkerContext.Done():
			return
		case order_first := <-this.QueueChannel__Order_PtyResizer:
			order_coalesced := this.DrainAndCoalesce(order_first)
			order_coalesced.Execute(PtyProxy_spawned)
		}
	}
}

func (this *_PtyResizer_) DrainAndCoalesce(order_first _Order_PtyResizer_) _Order_PtyResizer_ {
	order__coalesced_result := order_first
	for {
		select {
		case order_next := <-this.QueueChannel__Order_PtyResizer:
			order__coalesced_result = coalescePair__Order_PtyResizer(order__coalesced_result, order_next)
		default:
			return order__coalesced_result
		}
	}
}

func coalescePair__Order_PtyResizer(
	order_existing _Order_PtyResizer_,
	order_incoming _Order_PtyResizer_,
) _Order_PtyResizer_ {
	order_EmitSnapshot__incoming_maybe, _ := order_incoming.(*_ResizePty_EmitSnapshot__Order_PtyResizer_)
	order_ResizeJust__incoming_maybe, _ := order_incoming.(*_ResizePty_Just__Order_PtyResizer_)
	order_EmitSnapshot__existing_maybe, _ := order_existing.(*_ResizePty_EmitSnapshot__Order_PtyResizer_)
	order_ResizeJust__existing_maybe, _ := order_existing.(*_ResizePty_Just__Order_PtyResizer_)
	if order_EmitSnapshot__incoming_maybe != nil {
		return order_EmitSnapshot__incoming_maybe
	} else if order_ResizeJust__incoming_maybe != nil && order_EmitSnapshot__existing_maybe != nil {
		return &_ResizePty_EmitSnapshot__Order_PtyResizer_{
			ColumnCount_PtyTerminal:         order_ResizeJust__incoming_maybe.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:            order_ResizeJust__incoming_maybe.RowCount_PtyTerminal,
			Id_WorkspacePty:                 order_EmitSnapshot__existing_maybe.Id_WorkspacePty,
			Id_WebsocketConnection_expected: order_EmitSnapshot__existing_maybe.Id_WebsocketConnection_expected,
			QueueChannel_WorkspaceOrder:     order_EmitSnapshot__existing_maybe.QueueChannel_WorkspaceOrder,
		}
	} else if order_ResizeJust__incoming_maybe != nil && order_ResizeJust__existing_maybe != nil {
		return order_ResizeJust__incoming_maybe
	} else {
		panic("invalid path: coalescePair__Order_PtyResizer")
	}
}

func (this *_ResizePty_Just__Order_PtyResizer_) Execute(PtyProxy__spawned_maybe *_PtyProxy_) {
	_ = PtyProxy__spawned_maybe.Resize(
		this.ColumnCount_PtyTerminal,
		this.RowCount_PtyTerminal,
	)
}

func (this *_ResizePty_EmitSnapshot__Order_PtyResizer_) Execute(PtyProxy__spawned_maybe *_PtyProxy_) {
	_ = PtyProxy__spawned_maybe.Resize(
		this.ColumnCount_PtyTerminal,
		this.RowCount_PtyTerminal,
	)
	this.QueueChannel_WorkspaceOrder <- _EmitSnapshot_PtyProxy__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: this.Id_WebsocketConnection_expected,
		Id_WorkspacePty:                 this.Id_WorkspacePty,
	}
}

func (This *_PtyProxy_) Resize(
	columnCount_PtyTerminal_next int,
	rowCount_PtyTerminal_next int,
) error {
	This.Mutex.Lock()
	This.PtyTerminal.Resize(
		columnCount_PtyTerminal_next,
		rowCount_PtyTerminal_next,
	)
	This.Mutex.Unlock()
	return _PTY.Setsize(
		This.FileDescriptor_Master__Pty__shared,
		&_PTY.Winsize{
			Rows: uint16(rowCount_PtyTerminal_next),
			Cols: uint16(columnCount_PtyTerminal_next),
		},
	)
}
