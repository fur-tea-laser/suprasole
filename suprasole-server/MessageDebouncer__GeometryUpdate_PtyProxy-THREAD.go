package main

import (
	_TIME "time"
)

func (this *_MessageDebouncer__GeometryUpdate_PtyProxy_) RunWorker() {
	pendingBatch := &_PendingBatch__GeometryUpdate_PtyProxy_{
		SyncStatus:                      NONE___SyncStatus___PendingBatch__GeometryUpdate_PtyProxy,
		Id_WebsocketConnection_expected: 0,
		TargetBatch_pending:             make(map[uint32]*_Target__GeometryUpdate_PtyProxy_),
	}
	var debounceTimer *_TIME.Timer
	var channel_debounceTimer <-chan _TIME.Time
	for {
		select {
		case <-this.WorkerContext.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
		case order_leading := <-this.QueueChannel:
			this.DrainAndCoalesceQueueChannel(
				order_leading,
				pendingBatch,
			)
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = _TIME.NewTimer(this.DebounceTimeout__)
			channel_debounceTimer = debounceTimer.C
		case <-channel_debounceTimer:
			if SYNC___SyncStatus___PendingBatch__GeometryUpdate_PtyProxy == pendingBatch.SyncStatus || len(pendingBatch.TargetBatch_pending) > 0 {
				this.OnFlush__OrderBatch_GeometryUpdate_PtyProxy__(pendingBatch)
				pendingBatch = &_PendingBatch__GeometryUpdate_PtyProxy_{
					SyncStatus:                      NONE___SyncStatus___PendingBatch__GeometryUpdate_PtyProxy,
					Id_WebsocketConnection_expected: 0,
					TargetBatch_pending:             make(map[uint32]*_Target__GeometryUpdate_PtyProxy_),
				}
			}
			debounceTimer = nil
			channel_debounceTimer = nil
		}
	}
}

func (this *_MessageDebouncer__GeometryUpdate_PtyProxy_) DrainAndCoalesceQueueChannel(
	order_leading _MessageOrder__GeometryUpdate_PtyProxy_,
	pendingBatch *_PendingBatch__GeometryUpdate_PtyProxy_,
) {
	order_leading.Apply(pendingBatch)
	for {
		select {
		case order_next := <-this.QueueChannel:
			order_next.Apply(pendingBatch)
		default:
			return
		}
	}
}

func (this _Resize___MessageOrder__GeometryUpdate_PtyProxy_) Apply(
	pendingBatch *_PendingBatch__GeometryUpdate_PtyProxy_,
) {
	if this.Id_WebsocketConnection_expected < pendingBatch.Id_WebsocketConnection_expected {
		return
	}
	if this.Id_WebsocketConnection_expected > pendingBatch.Id_WebsocketConnection_expected {
		pendingBatch.Id_WebsocketConnection_expected = this.Id_WebsocketConnection_expected
		pendingBatch.SyncStatus = NONE___SyncStatus___PendingBatch__GeometryUpdate_PtyProxy
		clear(pendingBatch.TargetBatch_pending)
	}
	if SYNC___SyncStatus___PendingBatch__GeometryUpdate_PtyProxy == pendingBatch.SyncStatus {
		for _, order_ResizePty_some := range this.Message__Batch_ResizePty.OrderBatch_ResizePty {
			if nil == pendingBatch.TargetBatch_pending[order_ResizePty_some.Id_WorkspacePty] {
				return
			}
		}
	}
	for _, order_ResizePty_some := range this.Message__Batch_ResizePty.OrderBatch_ResizePty {
		target_pending_existing := pendingBatch.TargetBatch_pending[order_ResizePty_some.Id_WorkspacePty]
		if target_pending_existing != nil {
			target_pending_existing.ColumnCount_PtyTerminal = order_ResizePty_some.ColumnCount_PtyTerminal
			target_pending_existing.RowCount_PtyTerminal = order_ResizePty_some.RowCount_PtyTerminal
		} else {
			pendingBatch.TargetBatch_pending[order_ResizePty_some.Id_WorkspacePty] = &_Target__GeometryUpdate_PtyProxy_{
				ColumnCount_PtyTerminal: order_ResizePty_some.ColumnCount_PtyTerminal,
				RowCount_PtyTerminal:    order_ResizePty_some.RowCount_PtyTerminal,
			}
		}
	}
}

func (this _Sync___MessageOrder__GeometryUpdate_PtyProxy_) Apply(
	pendingBatch *_PendingBatch__GeometryUpdate_PtyProxy_,
) {
	if this.Id_WebsocketConnection_expected < pendingBatch.Id_WebsocketConnection_expected {
		return
	}
	pendingBatch.Id_WebsocketConnection_expected = this.Id_WebsocketConnection_expected
	pendingBatch.SyncStatus = SYNC___SyncStatus___PendingBatch__GeometryUpdate_PtyProxy
	clear(pendingBatch.TargetBatch_pending)
	for id_WorkspacePty_sync, order_SyncPty := range this.Message__Batch_SyncPty.OrderBatch_SyncPty {
		pendingBatch.TargetBatch_pending[id_WorkspacePty_sync] = &_Target__GeometryUpdate_PtyProxy_{
			ColumnCount_PtyTerminal: order_SyncPty.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:    order_SyncPty.RowCount_PtyTerminal,
		}
	}
}

func (This *_WorkspaceController_) HandleBatch_GeometryUpdate__Debouncer(
	pendingBatch *_PendingBatch__GeometryUpdate_PtyProxy_,
) {
	This.WorkspaceNetwork.WebsocketController_Pty.Mutex.Lock()
	status_WebsocketConnection_captured := This.WorkspaceNetwork.WebsocketController_Pty.Status_WebsocketConnection_state
	id_WebsocketConnection_captured := This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_state
	This.WorkspaceNetwork.WebsocketController_Pty.Mutex.Unlock()

	isConnectionValid := CONNECTED__Status_WebsocketConnection == status_WebsocketConnection_captured

	isSyncValid := SYNC___SyncStatus___PendingBatch__GeometryUpdate_PtyProxy == pendingBatch.SyncStatus &&
		isConnectionValid &&
		id_WebsocketConnection_captured == pendingBatch.Id_WebsocketConnection_expected

	if isSyncValid {
		This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _SyncVisibility__WorkspaceOrder_LifecycleCoordinator_{
			Id_WebsocketConnection_expected: pendingBatch.Id_WebsocketConnection_expected,
			TargetBatch_pending:             pendingBatch.TargetBatch_pending,
		}
	}

	for id_WorkspacePty_target, target_pending := range pendingBatch.TargetBatch_pending {
		This.Mutex.Lock()
		WorkspacePty_target := This.PtyPool[id_WorkspacePty_target]
		var visibility_current _Visibility__Client_connected_
		if WorkspacePty_target != nil {
			visibility_current = WorkspacePty_target.Visibility__Client_connected__state
		}
		This.Mutex.Unlock()

		if nil == WorkspacePty_target {
			continue
		}

		if isSyncValid && VISIBLE___Visibility__Client_connected != visibility_current {
			WorkspacePty_target.PtyResizer.QueueChannel__Order_PtyResizer <- _ResizePty_EmitSnapshot__Order_PtyResizer_{
				ColumnCount_PtyTerminal:         target_pending.ColumnCount_PtyTerminal,
				RowCount_PtyTerminal:            target_pending.RowCount_PtyTerminal,
				Id_WorkspacePty:                 id_WorkspacePty_target,
				Id_WebsocketConnection_expected: pendingBatch.Id_WebsocketConnection_expected,
				QueueChannel_WorkspaceOrder:     This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder,
			}
		} else {
			WorkspacePty_target.PtyResizer.QueueChannel__Order_PtyResizer <- _ResizePty_Just__Order_PtyResizer_{
				ColumnCount_PtyTerminal: target_pending.ColumnCount_PtyTerminal,
				RowCount_PtyTerminal:    target_pending.RowCount_PtyTerminal,
			}
		}
	}
}
