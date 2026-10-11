package main

import (
	_TIME "time"
)

func (this *_MessageReducer__LayoutUpdate_PtyProxy_) RunWorker() {
	batch__LayoutUpdate_PtyProxy__tentative_state := make(map[uint32]*_LayoutUpdate_PtyProxy_)
	debounceTimer := _TIME.NewTimer(this.DebounceTimeout__)
	debounceTimer.Stop()
	var channel_debounceTimer <-chan _TIME.Time
	for {
		select {
		case <-this.WorkerContext.Done():
			debounceTimer.Stop()
			return
		case order_leading := <-this.QueueChannel:
			switch order__leading_the := order_leading.(type) {
			case _Batch_ResizePty___MessageOrder__LayoutUpdate_PtyProxy_:
				Coalesce__OrderBatch_LayoutUpdate(
					order__leading_the.Message__Batch_ResizePty.OrderBatch_LayoutUpdate,
					batch__LayoutUpdate_PtyProxy__tentative_state,
				)
				debounceTimer.Reset(this.DebounceTimeout__)
				channel_debounceTimer = debounceTimer.C
			case _Batch_SyncPty___MessageOrder__LayoutUpdate_PtyProxy_:
				debounceTimer.Stop()
				channel_debounceTimer = nil
				this.OnFlush__Batch_SyncPty__(order__leading_the)
				batch__LayoutUpdate_PtyProxy__tentative_state = make(map[uint32]*_LayoutUpdate_PtyProxy_)
			case _SpawnPty___MessageOrder__LayoutUpdate_PtyProxy_:
				debounceTimer.Stop()
				channel_debounceTimer = nil
				Coalesce__OrderBatch_LayoutUpdate(
					order__leading_the.Message__SpawnPty.OrderBatch_LayoutUpdate,
					batch__LayoutUpdate_PtyProxy__tentative_state,
				)
				this.OnFlush__SpawnPty__(
					order__leading_the,
					batch__LayoutUpdate_PtyProxy__tentative_state,
				)
				batch__LayoutUpdate_PtyProxy__tentative_state = make(map[uint32]*_LayoutUpdate_PtyProxy_)
			case _Batch_RemovePty___MessageOrder__LayoutUpdate_PtyProxy_:
				debounceTimer.Stop()
				channel_debounceTimer = nil
				Coalesce__OrderBatch_LayoutUpdate(
					order__leading_the.Message__Batch_RemovePty.OrderBatch_LayoutUpdate,
					batch__LayoutUpdate_PtyProxy__tentative_state,
				)
				for _, order_RemovePty__target_current := range order__leading_the.Message__Batch_RemovePty.OrderBatch_RemovePty {
					delete(batch__LayoutUpdate_PtyProxy__tentative_state, order_RemovePty__target_current.Id_WorkspacePty)
				}
				this.OnFlush__Batch_RemovePty__(
					order__leading_the,
					batch__LayoutUpdate_PtyProxy__tentative_state,
				)
				batch__LayoutUpdate_PtyProxy__tentative_state = make(map[uint32]*_LayoutUpdate_PtyProxy_)
			default:
				panic("invalid path: _MessageReducer__LayoutUpdate_PtyProxy_ RunWorker")
			}
		case <-channel_debounceTimer:
			channel_debounceTimer = nil
			if len(batch__LayoutUpdate_PtyProxy__tentative_state) > 0 {
				this.OnFlush__ResizeOnly__(batch__LayoutUpdate_PtyProxy__tentative_state)
				batch__LayoutUpdate_PtyProxy__tentative_state = make(map[uint32]*_LayoutUpdate_PtyProxy_)
			}
		}
	}
}

func Coalesce__OrderBatch_LayoutUpdate(
	orderBatch_LayoutUpdate map[uint32]*_Order_LayoutUpdate_,
	batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_,
) {
	for id_WorkspacePty__target_some, order_LayoutUpdate_some := range orderBatch_LayoutUpdate {
		layoutUpdate_PtyProxy__target_existing := batch__LayoutUpdate_PtyProxy[id_WorkspacePty__target_some]
		if layoutUpdate_PtyProxy__target_existing != nil {
			layoutUpdate_PtyProxy__target_existing.ColumnCount_PtyTerminal = order_LayoutUpdate_some.ColumnCount_PtyTerminal
			layoutUpdate_PtyProxy__target_existing.RowCount_PtyTerminal = order_LayoutUpdate_some.RowCount_PtyTerminal
		} else {
			batch__LayoutUpdate_PtyProxy[id_WorkspacePty__target_some] = &_LayoutUpdate_PtyProxy_{
				ColumnCount_PtyTerminal: order_LayoutUpdate_some.ColumnCount_PtyTerminal,
				RowCount_PtyTerminal:    order_LayoutUpdate_some.RowCount_PtyTerminal,
			}
		}
	}
}

func (This *_WorkspaceController_) HandleFlush__ResizeOnly__Reducer(
	batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_,
) {
	This.SubmitBatch_Visible__Order_PtyResizer(batch__LayoutUpdate_PtyProxy)
}

func (This *_WorkspaceController_) HandleFlush__SpawnPty__Reducer(
	order_SpawnPty _SpawnPty___MessageOrder__LayoutUpdate_PtyProxy_,
	batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_,
) {
	select {
	case <-This.MessageReducer__LayoutUpdate_PtyProxy.WorkerContext.Done():
		return
	case This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _SpawnPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: order_SpawnPty.Id_WebsocketConnection_expected,
		Message_SpawnPty:                order_SpawnPty.Message__SpawnPty,
	}:
	}
	This.SubmitBatch_Visible__Order_PtyResizer(batch__LayoutUpdate_PtyProxy)
}

func (This *_WorkspaceController_) HandleFlush__Batch_RemovePty__Reducer(
	order_Batch_RemovePty _Batch_RemovePty___MessageOrder__LayoutUpdate_PtyProxy_,
	batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_,
) {
	select {
	case <-This.MessageReducer__LayoutUpdate_PtyProxy.WorkerContext.Done():
		return
	case This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _RemovePty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: order_Batch_RemovePty.Id_WebsocketConnection_expected,
		OrderBatch_RemovePty:            order_Batch_RemovePty.Message__Batch_RemovePty.OrderBatch_RemovePty,
	}:
	}
	This.SubmitBatch_Visible__Order_PtyResizer(batch__LayoutUpdate_PtyProxy)
}

func (This *_WorkspaceController_) SubmitBatch_Visible__Order_PtyResizer(
	batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_,
) {
	batch__Submission_PtyResizer__state := make(map[*_PtyResizer_]_Order_PtyResizer_, len(batch__LayoutUpdate_PtyProxy))
	This.Mutex.Lock()
	for id_WorkspacePty__target_some, layoutUpdate_PtyProxy_some := range batch__LayoutUpdate_PtyProxy {
		WorkspacePty_target := This.PtyPool[id_WorkspacePty__target_some]
		if WorkspacePty_target != nil && VISIBLE___Visibility__Client_connected == WorkspacePty_target.Visibility__Client_connected__state {
			batch__Submission_PtyResizer__state[WorkspacePty_target.PtyResizer] = &_ResizePty_Just__Order_PtyResizer_{
				ColumnCount_PtyTerminal: layoutUpdate_PtyProxy_some.ColumnCount_PtyTerminal,
				RowCount_PtyTerminal:    layoutUpdate_PtyProxy_some.RowCount_PtyTerminal,
			}
		}
	}
	This.Mutex.Unlock()
	for PtyResizer__target_some, order_PtyResizer_some := range batch__Submission_PtyResizer__state {
		PtyResizer__target_some.Submit__Order_PtyResizer(order_PtyResizer_some)
	}
}

func (This *_WorkspaceController_) HandleFlush__Batch_SyncPty__Reducer(
	order_Batch_SyncPty _Batch_SyncPty___MessageOrder__LayoutUpdate_PtyProxy_,
) {
	batch__LayoutUpdate_PtyProxy__SyncPty__state := make(map[uint32]*_LayoutUpdate_PtyProxy_, len(order_Batch_SyncPty.Message__Batch_SyncPty.OrderBatch_LayoutUpdate))
	batch__Submission_PtyResizer__state := make(map[*_PtyResizer_]_Order_PtyResizer_, len(order_Batch_SyncPty.Message__Batch_SyncPty.OrderBatch_LayoutUpdate))
	This.Mutex.Lock()
	for id_WorkspacePty__target_some, order_LayoutUpdate_some := range order_Batch_SyncPty.Message__Batch_SyncPty.OrderBatch_LayoutUpdate {
		batch__LayoutUpdate_PtyProxy__SyncPty__state[id_WorkspacePty__target_some] = &_LayoutUpdate_PtyProxy_{
			ColumnCount_PtyTerminal: order_LayoutUpdate_some.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:    order_LayoutUpdate_some.RowCount_PtyTerminal,
		}
		WorkspacePty_target := This.PtyPool[id_WorkspacePty__target_some]
		if WorkspacePty_target != nil &&
			VISIBLE___Visibility__Client_connected == WorkspacePty_target.Visibility__Client_connected__state {
			batch__Submission_PtyResizer__state[WorkspacePty_target.PtyResizer] = &_ResizePty_Just__Order_PtyResizer_{
				ColumnCount_PtyTerminal: order_LayoutUpdate_some.ColumnCount_PtyTerminal,
				RowCount_PtyTerminal:    order_LayoutUpdate_some.RowCount_PtyTerminal,
			}
		} else if WorkspacePty_target != nil &&
			(NOT_VISIBLE___Visibility__Client_connected == WorkspacePty_target.Visibility__Client_connected__state ||
				DISCONNECTED_UNKNOWN___Visibility__Client_connected == WorkspacePty_target.Visibility__Client_connected__state) {
			batch__Submission_PtyResizer__state[WorkspacePty_target.PtyResizer] = &_ResizePty_EmitSnapshot__Order_PtyResizer_{
				ColumnCount_PtyTerminal:         order_LayoutUpdate_some.ColumnCount_PtyTerminal,
				RowCount_PtyTerminal:            order_LayoutUpdate_some.RowCount_PtyTerminal,
				Id_WorkspacePty:                 id_WorkspacePty__target_some,
				Id_WebsocketConnection_expected: order_Batch_SyncPty.Id_WebsocketConnection_expected,
				QueueChannel_WorkspaceOrder:     This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder,
			}
		} else if nil == WorkspacePty_target {
		} else {
			panic("invalid path: HandleFlush__Batch_SyncPty__Reducer")
		}
	}
	This.Mutex.Unlock()
	select {
	case <-This.MessageReducer__LayoutUpdate_PtyProxy.WorkerContext.Done():
		return
	case This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _SyncVisibility__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: order_Batch_SyncPty.Id_WebsocketConnection_expected,
		Batch__LayoutUpdate_PtyProxy:    batch__LayoutUpdate_PtyProxy__SyncPty__state,
	}:
	}
	for PtyResizer__target_some, order_PtyResizer_some := range batch__Submission_PtyResizer__state {
		PtyResizer__target_some.Submit__Order_PtyResizer(order_PtyResizer_some)
	}
}

func (this *_PtyResizer_) Submit__Order_PtyResizer(
	order_PtyResizer _Order_PtyResizer_,
) {
	select {
	case <-this.WorkerContext.Done():
	default:
		select {
		case this.QueueChannel__Order_PtyResizer <- order_PtyResizer:
		default:
		}
	}
}
