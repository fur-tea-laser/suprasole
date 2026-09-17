package main

import (
	_CONTEXT "context"
)

type _PtyResizer_ struct {
	QueueChannel__Order_PtyResizer chan _Order_PtyResizer_
	BindChannel__PtyProxy_spawned  chan *_PtyProxy_
	WorkerContext                  _CONTEXT.Context
	WorkerCancel                   _CONTEXT.CancelFunc
}

type _Order_PtyResizer_ interface {
	Execute(PtyProxy__spawned_maybe *_PtyProxy_)
}

type _ResizePty_Just__Order_PtyResizer_ struct {
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

type _ResizePty_EmitSnapshot__Order_PtyResizer_ struct {
	ColumnCount_PtyTerminal         int
	RowCount_PtyTerminal            int
	Id_WorkspacePty                 uint32
	Id_WebsocketConnection_expected uint64
	QueueChannel_WorkspaceOrder     chan<- _WorkspaceOrder_LifecycleCoordinator_
}
