package main

func (This *_WorkspaceController_) HandleBatch_ResizePty__Debouncer(
	orderBatch_ResizePty_pending map[uint32]_Order_ResizePty_,
) {
	for _, order_ResizePty__pending_some := range orderBatch_ResizePty_pending {
		This.Mutex.Lock()
		WorkspacePty_target := This.PtyPool[order_ResizePty__pending_some.Id_WorkspacePty]
		This.Mutex.Unlock()
		if WorkspacePty_target != nil {
			WorkspacePty_target.State_current.HandleResize_Debouncer(
				order_ResizePty__pending_some.ColumnCount_PtyTerminal,
				order_ResizePty__pending_some.RowCount_PtyTerminal,
			)
		}
	}
}

func (this *_State_Spawning__WorkspacePty_) HandleResize_Debouncer(
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) {
	this.Mutex.Lock()
	this.ResizeGeometry__deferred_maybe = &_ResizeGeometry___State_Spawning__WorkspacePty_{
		ColumnCount_PtyTerminal: columnCount_PtyTerminal,
		RowCount_PtyTerminal:    rowCount_PtyTerminal,
	}
	this.Mutex.Unlock()
}

func (this *_State_Active__WorkspacePty_) HandleResize_Debouncer(
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) {
	_ = this.PtyProxy.Resize(
		columnCount_PtyTerminal,
		rowCount_PtyTerminal,
	)
}

func (this *_State_Exited__WorkspacePty_) HandleResize_Debouncer(
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) {
	_ = this.PtyProxy.Resize(
		columnCount_PtyTerminal,
		rowCount_PtyTerminal,
	)
}
