package main

import (
	_CONTEXT "context"
)

type _WorkspaceOrder_LifecycleCoordinator_ interface {
	Execute(lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_)
}

type _Connect__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
}

func (this _Connect__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnConnect_PtyWebsocket__(this.Id_WebsocketConnection_expected)
}

type _Disconnect__WorkspaceOrder_LifecycleCoordinator_ struct{}

func (this _Disconnect__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnDisconnect_PtyWebsocket__()
}

type _Sync__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
	Message__Batch_SyncPty          _Batch_SyncPty__PtyMessage_Ingress_
}

func (this _Sync__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnSync_PtyPool__(
		this.Id_WebsocketConnection_expected,
		this.Message__Batch_SyncPty,
	)
}

type _ExitPty__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WorkspacePty_exited uint32
	ExitOutcome_PtyProxy   _ExitOutcome_PtyProxy_
}

func (this _ExitPty__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnExit_PtyProxy__(
		this.Id_WorkspacePty_exited,
		this.ExitOutcome_PtyProxy,
	)
}

type _SpawnPty__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
	Message_SpawnPty                _SpawnPty__PtyMessage_Ingress_
}

func (this _SpawnPty__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnSpawnPty__(
		this.Id_WebsocketConnection_expected,
		this.Message_SpawnPty,
	)
}

type _Status_SpawnPty__Success__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WorkspacePty_spawned uint32
	PtyProxy_quiescent      *_PtyProxy_
}

func (this _Status_SpawnPty__Success__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnStatus_SpawnPty__Success__(
		this.Id_WorkspacePty_spawned,
		this.PtyProxy_quiescent,
	)
}

type _Status_SpawnPty__Failure__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WorkspacePty_failed  uint32
	Error_Start__PtyCommand error
}

func (this _Status_SpawnPty__Failure__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnStatus_SpawnPty__Failure__(
		this.Id_WorkspacePty_failed,
		this.Error_Start__PtyCommand,
	)
}

type _LifecycleCoordinator_WorkspacePty_ struct {
	OnConnect_PtyWebsocket__     func(id_WebsocketConnection_expected uint64)
	OnDisconnect_PtyWebsocket__  func()
	OnSync_PtyPool__             func(id_WebsocketConnection_expected uint64, message__Batch_SyncPty _Batch_SyncPty__PtyMessage_Ingress_)
	OnExit_PtyProxy__            func(id_WorkspacePty_exited uint32, exitOutcome_PtyProxy _ExitOutcome_PtyProxy_)
	OnSpawnPty__                 func(id_WebsocketConnection_expected uint64, message _SpawnPty__PtyMessage_Ingress_)
	OnStatus_SpawnPty__Success__ func(id_WorkspacePty_spawned uint32, ptyProxy_quiescent *_PtyProxy_)
	OnStatus_SpawnPty__Failure__ func(id_WorkspacePty_failed uint32, error_Start__PtyCommand error)
	QueueChannel_WorkspaceOrder  chan _WorkspaceOrder_LifecycleCoordinator_
	WorkerContext                _CONTEXT.Context
	WorkerCancel                 _CONTEXT.CancelFunc
}

type _MakeApi__LifecycleCoordinator_WorkspacePty_ struct {
	OnConnect_PtyWebsocket__     func(id_WebsocketConnection_expected uint64)
	OnDisconnect_PtyWebsocket__  func()
	OnSync_PtyPool__             func(id_WebsocketConnection_expected uint64, message__Batch_SyncPty _Batch_SyncPty__PtyMessage_Ingress_)
	OnExit_PtyProxy__            func(id_WorkspacePty_exited uint32, exitOutcome_PtyProxy _ExitOutcome_PtyProxy_)
	OnSpawnPty__                 func(id_WebsocketConnection_expected uint64, message _SpawnPty__PtyMessage_Ingress_)
	OnStatus_SpawnPty__Success__ func(id_WorkspacePty_spawned uint32, ptyProxy_quiescent *_PtyProxy_)
	OnStatus_SpawnPty__Failure__ func(id_WorkspacePty_failed uint32, error_Start__PtyCommand error)
}

func Make__LifecycleCoordinator_WorkspacePty(
	api _MakeApi__LifecycleCoordinator_WorkspacePty_,
) *_LifecycleCoordinator_WorkspacePty_ {
	workerContext, workerCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	return &_LifecycleCoordinator_WorkspacePty_{
		OnConnect_PtyWebsocket__:     api.OnConnect_PtyWebsocket__,
		OnDisconnect_PtyWebsocket__:  api.OnDisconnect_PtyWebsocket__,
		OnSync_PtyPool__:             api.OnSync_PtyPool__,
		OnExit_PtyProxy__:            api.OnExit_PtyProxy__,
		OnSpawnPty__:                 api.OnSpawnPty__,
		OnStatus_SpawnPty__Success__: api.OnStatus_SpawnPty__Success__,
		OnStatus_SpawnPty__Failure__: api.OnStatus_SpawnPty__Failure__,
		QueueChannel_WorkspaceOrder:  make(chan _WorkspaceOrder_LifecycleCoordinator_, 128),
		WorkerContext:                workerContext,
		WorkerCancel:                 workerCancel,
	}
}

func (this *_LifecycleCoordinator_WorkspacePty_) RunWorker() {
	for {
		select {
		case <-this.WorkerContext.Done():
			return
		case order_leading := <-this.QueueChannel_WorkspaceOrder:
			orderBatch_pending := this.Drain__QueueChannel_WorkspaceOrder(order_leading)
			orderBatch_reconciled := this.Reconcile__orderBatch_pending(orderBatch_pending)
			for _, order__reconciled_current := range orderBatch_reconciled {
				order__reconciled_current.Execute(this)
			}
		}
	}
}

func (this *_LifecycleCoordinator_WorkspacePty_) Drain__QueueChannel_WorkspaceOrder(
	order_leading _WorkspaceOrder_LifecycleCoordinator_,
) []_WorkspaceOrder_LifecycleCoordinator_ {
	orderBatch__pending_result := []_WorkspaceOrder_LifecycleCoordinator_{order_leading}
	for {
		select {
		case order__pending_next := <-this.QueueChannel_WorkspaceOrder:
			orderBatch__pending_result = append(
				orderBatch__pending_result,
				order__pending_next,
			)
		default:
			return orderBatch__pending_result
		}
	}
}

func (this *_LifecycleCoordinator_WorkspacePty_) Reconcile__orderBatch_pending(
	orderBatch_pending []_WorkspaceOrder_LifecycleCoordinator_,
) []_WorkspaceOrder_LifecycleCoordinator_ {
	var orderBatch_ExitPty_result []_WorkspaceOrder_LifecycleCoordinator_
	var orderBatch_SpawnPty_result []_WorkspaceOrder_LifecycleCoordinator_
	var orderBatch_Session_result []_WorkspaceOrder_LifecycleCoordinator_
	var orderBatch__Status_SpawnPty__result []_WorkspaceOrder_LifecycleCoordinator_
	for _, order__pending_current := range orderBatch_pending {
		switch order__pending_current_the := order__pending_current.(type) {
		case _ExitPty__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch_ExitPty_result = append(
				orderBatch_ExitPty_result,
				order__pending_current_the,
			)
		case _SpawnPty__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch_SpawnPty_result = append(
				orderBatch_SpawnPty_result,
				order__pending_current_the,
			)
		case _Disconnect__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch_Session_result = []_WorkspaceOrder_LifecycleCoordinator_{
				order__pending_current_the,
			}
		case _Connect__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch_Session_result = upsertTail_Order__orderBatch_Session(
				orderBatch_Session_result,
				order__pending_current_the,
			)
		case _Sync__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch_Session_result = upsertTail_Order__orderBatch_Session(
				orderBatch_Session_result,
				order__pending_current_the,
			)
		case _Status_SpawnPty__Success__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch__Status_SpawnPty__result = append(
				orderBatch__Status_SpawnPty__result,
				order__pending_current_the,
			)
		case _Status_SpawnPty__Failure__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch__Status_SpawnPty__result = append(
				orderBatch__Status_SpawnPty__result,
				order__pending_current_the,
			)
		default:
			panic("invalid path: _LifecycleCoordinator_WorkspacePty_ Reconcile__orderBatch_pending")
		}
	}
	return append(
		orderBatch_ExitPty_result,
		append(
			orderBatch_SpawnPty_result,
			append(
				orderBatch_Session_result,
				orderBatch__Status_SpawnPty__result...,
			)...,
		)...,
	)
}

func upsertTail_Order__orderBatch_Session[
	__Order__ _WorkspaceOrder_LifecycleCoordinator_,
](
	orderBatch_Session_result []_WorkspaceOrder_LifecycleCoordinator_,
	order__pending_current_the __Order__,
) []_WorkspaceOrder_LifecycleCoordinator_ {
	if 0 == len(orderBatch_Session_result) {
		return append(
			orderBatch_Session_result,
			order__pending_current_the,
		)
	}
	switch orderBatch_Session_result[len(orderBatch_Session_result)-1].(type) {
	case __Order__:
		orderBatch_Session_result[len(orderBatch_Session_result)-1] = order__pending_current_the
		return orderBatch_Session_result
	default:
		return append(
			orderBatch_Session_result,
			order__pending_current_the,
		)
	}
}
