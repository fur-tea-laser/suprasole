package main

import (
	_PTY "github.com/creack/pty"
)

func (this *_PtyResizer_) RunWorker() {
	var PtyProxy__spawned_maybe *_PtyProxy_
	var order_PtyResizer__next_maybe _Order_PtyResizer_
	for {
		select {
		case <-this.WorkerContext.Done():
			return
		case PtyProxy_spawned := <-this.BindChannel__PtyProxy_spawned:
			PtyProxy__spawned_maybe = PtyProxy_spawned
			if order_PtyResizer__next_maybe != nil {
				order_PtyResizer__next_maybe.Execute(PtyProxy__spawned_maybe)
				order_PtyResizer__next_maybe = nil
			}
		case order_PtyResizer_next := <-this.QueueChannel__Order_PtyResizer:
			if nil == PtyProxy__spawned_maybe {
				order_PtyResizer__next_maybe = order_PtyResizer_next
			} else {
				order_PtyResizer_next.Execute(PtyProxy__spawned_maybe)
			}
		}
	}
}

func (this _ResizePty_Just__Order_PtyResizer_) Execute(PtyProxy__spawned_maybe *_PtyProxy_) {
	_ = PtyProxy__spawned_maybe.Resize(
		this.ColumnCount_PtyTerminal,
		this.RowCount_PtyTerminal,
	)
}

func (this _ResizePty_EmitSnapshot__Order_PtyResizer_) Execute(PtyProxy__spawned_maybe *_PtyProxy_) {
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
		This.FileDescriptor_Master__PtyDevice,
		&_PTY.Winsize{
			Rows: uint16(rowCount_PtyTerminal_next),
			Cols: uint16(columnCount_PtyTerminal_next),
		},
	)
}
