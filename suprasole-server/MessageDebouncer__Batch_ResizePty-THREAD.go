package main

import (
	_TIME "time"
)

func (this *_MessageDebouncer__Batch_ResizePty_) RunWorker() {
	orderBatch_ResizePty_pending := make(map[uint32]_Order_ResizePty_)
	var debounceTimer *_TIME.Timer
	var channel_debounceTimer <-chan _TIME.Time
	for {
		select {
		case <-this.WorkerContext.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
		case message_leading := <-this.QueueChannel:
			this.DrainAndCoalesceQueueChannel(
				message_leading,
				orderBatch_ResizePty_pending,
			)
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = _TIME.NewTimer(this.DebounceTimeout__)
			channel_debounceTimer = debounceTimer.C
		case <-channel_debounceTimer:
			if len(orderBatch_ResizePty_pending) > 0 {
				this.OnFlush__OrderBatch_ResizePty__(orderBatch_ResizePty_pending)
				clear(orderBatch_ResizePty_pending)
			}
			debounceTimer = nil
			channel_debounceTimer = nil
		}
	}
}

func (this *_MessageDebouncer__Batch_ResizePty_) DrainAndCoalesceQueueChannel(
	message_leading _Batch_ResizePty__PtyMessage_Ingress_,
	orderBatch_ResizePty_pending map[uint32]_Order_ResizePty_,
) {
	for _, order_ResizePty_some := range message_leading.OrderBatch_ResizePty {
		orderBatch_ResizePty_pending[order_ResizePty_some.Id_WorkspacePty] = order_ResizePty_some
	}
	for {
		select {
		case message_next := <-this.QueueChannel:
			for _, order_ResizePty_some := range message_next.OrderBatch_ResizePty {
				orderBatch_ResizePty_pending[order_ResizePty_some.Id_WorkspacePty] = order_ResizePty_some
			}
		default:
			return
		}
	}
}

func (This *_WorkspaceController_) HandleBatch_ResizePty__Debouncer(
	orderBatch_ResizePty_pending map[uint32]_Order_ResizePty_,
) {
	for _, order_ResizePty__pending_some := range orderBatch_ResizePty_pending {
		This.Mutex.Lock()
		var State__WorkspacePty_target__captured _State_WorkspacePty_
		if WorkspacePty_target := This.PtyPool[order_ResizePty__pending_some.Id_WorkspacePty]; WorkspacePty_target != nil {
			State__WorkspacePty_target__captured = WorkspacePty_target.State_state
		}
		This.Mutex.Unlock()
		if State__WorkspacePty_target__captured != nil {
			State__WorkspacePty_target__captured.HandleResize_Debouncer(
				order_ResizePty__pending_some.ColumnCount_PtyTerminal,
				order_ResizePty__pending_some.RowCount_PtyTerminal,
			)
		}
	}
}

func (This *_State_Spawning__WorkspacePty_) HandleResize_Debouncer(
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) {
	This.Mutex.Lock()
	This.ResizeGeometry__deferred_maybe = &_ResizeGeometry___State_Spawning__WorkspacePty_{
		ColumnCount_PtyTerminal: columnCount_PtyTerminal,
		RowCount_PtyTerminal:    rowCount_PtyTerminal,
	}
	This.Mutex.Unlock()
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
