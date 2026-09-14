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
