package main

import (
	_CONTEXT "context"
)

type _WorkspaceOrder_LifecycleCoordinator_ interface {
	Execute(lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_)
}

type _Connect__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection uint64
}

func (this _Connect__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnConnect_PtyWebsocket(this.Id_WebsocketConnection)
}

type _Disconnect__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection uint64
}

func (this _Disconnect__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnDisconnect_PtyWebsocket(this.Id_WebsocketConnection)
}

type _Sync__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection uint64
	Message_SyncWorkspace  _SyncWorkspace_Message_
}

func (this _Sync__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnSync_PtyPool(this.Id_WebsocketConnection, this.Message_SyncWorkspace)
}

type _ExitPty__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_PtyProxy uint32
	ExitOutcome _ExitOutcome_PtyProxy_
}

func (this _ExitPty__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnExit_PtyProxy(this.Id_PtyProxy, this.ExitOutcome)
}

type _LifecycleCoordinator_WorkspacePty_ struct {
	WorkerContext             _CONTEXT.Context
	WorkerCancel              _CONTEXT.CancelFunc
	QueueChannel              chan _WorkspaceOrder_LifecycleCoordinator_
	OnConnect_PtyWebsocket    func(id_WebsocketConnection uint64)
	OnDisconnect_PtyWebsocket func(id_WebsocketConnection uint64)
	OnSync_PtyPool            func(id_WebsocketConnection uint64, message_SyncWorkspace _SyncWorkspace_Message_)
	OnExit_PtyProxy           func(id_PtyProxy uint32, exitOutcome _ExitOutcome_PtyProxy_)
}

type _NewApi__LifecycleCoordinator_WorkspacePty_ struct {
	OnConnect_PtyWebsocket    func(id_WebsocketConnection uint64)
	OnDisconnect_PtyWebsocket func(id_WebsocketConnection uint64)
	OnSync_PtyPool            func(id_WebsocketConnection uint64, message_SyncWorkspace _SyncWorkspace_Message_)
	OnExit_PtyProxy           func(id_PtyProxy uint32, exitOutcome _ExitOutcome_PtyProxy_)
}

func New__LifecycleCoordinator_WorkspacePty(
	api _NewApi__LifecycleCoordinator_WorkspacePty_,
) *_LifecycleCoordinator_WorkspacePty_ {
	__WorkerContext, __WorkerCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	return &_LifecycleCoordinator_WorkspacePty_{
		WorkerContext:             __WorkerContext,
		WorkerCancel:              __WorkerCancel,
		QueueChannel:              make(chan _WorkspaceOrder_LifecycleCoordinator_, 16),
		OnConnect_PtyWebsocket:    api.OnConnect_PtyWebsocket,
		OnDisconnect_PtyWebsocket: api.OnDisconnect_PtyWebsocket,
		OnSync_PtyPool:            api.OnSync_PtyPool,
		OnExit_PtyProxy:           api.OnExit_PtyProxy,
	}
}

func (this *_LifecycleCoordinator_WorkspacePty_) RunWorker() {
	for {
		select {
		case <-this.WorkerContext.Done():
			return
		case leadingOrder := <-this.QueueChannel:
			pendingOrders := this.DrainPendingOrders(leadingOrder)
			reconciledOrders := this.ReconcileOrders(pendingOrders)
			for _, someReconciledOrder := range reconciledOrders {
				someReconciledOrder.Execute(this)
			}
		}
	}
}

func (this *_LifecycleCoordinator_WorkspacePty_) DrainPendingOrders(
	leadingOrder _WorkspaceOrder_LifecycleCoordinator_,
) []_WorkspaceOrder_LifecycleCoordinator_ {
	pendingOrdersResult := []_WorkspaceOrder_LifecycleCoordinator_{leadingOrder}
	for {
		select {
		case nextOrder := <-this.QueueChannel:
			pendingOrdersResult = append(
				pendingOrdersResult,
				nextOrder,
			)
		default:
			return pendingOrdersResult
		}
	}
}

func isLastOrderConnect(
	networkOrdersResult []_WorkspaceOrder_LifecycleCoordinator_,
) bool {
	if len(networkOrdersResult) == 0 {
		return false
	} else {
		_, isConnect := networkOrdersResult[len(networkOrdersResult)-1].(_Connect__WorkspaceOrder_LifecycleCoordinator_)
		return isConnect
	}
}

func isLastOrderSync(
	networkOrdersResult []_WorkspaceOrder_LifecycleCoordinator_,
) bool {
	if len(networkOrdersResult) == 0 {
		return false
	} else {
		_, isSync := networkOrdersResult[len(networkOrdersResult)-1].(_Sync__WorkspaceOrder_LifecycleCoordinator_)
		return isSync
	}
}

func (this *_LifecycleCoordinator_WorkspacePty_) ReconcileOrders(
	pendingOrders []_WorkspaceOrder_LifecycleCoordinator_,
) []_WorkspaceOrder_LifecycleCoordinator_ {
	var exitOrdersResult []_WorkspaceOrder_LifecycleCoordinator_
	var networkOrdersResult []_WorkspaceOrder_LifecycleCoordinator_
	for _, somePendingOrder := range pendingOrders {
		switch typedPendingOrder := somePendingOrder.(type) {
		case _ExitPty__WorkspaceOrder_LifecycleCoordinator_:
			exitOrdersResult = append(
				exitOrdersResult,
				typedPendingOrder,
			)
		case _Disconnect__WorkspaceOrder_LifecycleCoordinator_:
			networkOrdersResult = []_WorkspaceOrder_LifecycleCoordinator_{
				typedPendingOrder,
			}
		case _Connect__WorkspaceOrder_LifecycleCoordinator_:
			if isLastOrderConnect(networkOrdersResult) {
				networkOrdersResult[len(networkOrdersResult)-1] = typedPendingOrder
			} else {
				networkOrdersResult = append(
					networkOrdersResult,
					typedPendingOrder,
				)
			}
		case _Sync__WorkspaceOrder_LifecycleCoordinator_:
			if isLastOrderSync(networkOrdersResult) {
				networkOrdersResult[len(networkOrdersResult)-1] = typedPendingOrder
			} else {
				networkOrdersResult = append(
					networkOrdersResult,
					typedPendingOrder,
				)
			}
		}
	}
	return append(
		exitOrdersResult,
		networkOrdersResult...,
	)
}
