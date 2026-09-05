package main

import (
	_CONTEXT "context"
)

type _WorkspaceOrder_LifecycleCoordinator_ interface {
	Execute(lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_)
}

type _Connect__WorkspaceOrder_LifecycleCoordinator_ struct {
	ExpectedId_WebsocketConnection uint64
}

func (this _Connect__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnConnect_PtyWebsocket__(this.ExpectedId_WebsocketConnection)
}

type _Disconnect__WorkspaceOrder_LifecycleCoordinator_ struct{}

func (this _Disconnect__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnDisconnect_PtyWebsocket__()
}

type _Sync__WorkspaceOrder_LifecycleCoordinator_ struct {
	ExpectedId_WebsocketConnection uint64
	Message_SyncWorkspace          _SyncWorkspace__PtyMessage_Ingress_
}

func (this _Sync__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnSync_PtyPool__(
		this.ExpectedId_WebsocketConnection,
		this.Message_SyncWorkspace,
	)
}

type _ExitPty__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_PtyProxy          uint32
	ExitOutcome_PtyProxy _ExitOutcome_PtyProxy_
}

func (this _ExitPty__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnExit_PtyProxy__(
		this.Id_PtyProxy,
		this.ExitOutcome_PtyProxy,
	)
}

type _LifecycleCoordinator_WorkspacePty_ struct {
	OnConnect_PtyWebsocket__    func(expectedId_WebsocketConnection uint64)
	OnDisconnect_PtyWebsocket__ func()
	OnSync_PtyPool__            func(expectedId_WebsocketConnection uint64, message_SyncWorkspace _SyncWorkspace__PtyMessage_Ingress_)
	OnExit_PtyProxy__           func(exitedId_PtyProxy uint32, exitOutcome_PtyProxy _ExitOutcome_PtyProxy_)
	WorkerContext               _CONTEXT.Context
	WorkerCancel                _CONTEXT.CancelFunc
	QueueChannel_WorkspaceOrder chan _WorkspaceOrder_LifecycleCoordinator_
}

type _NewApi__LifecycleCoordinator_WorkspacePty_ struct {
	OnConnect_PtyWebsocket__    func(expectedId_WebsocketConnection uint64)
	OnDisconnect_PtyWebsocket__ func()
	OnSync_PtyPool__            func(expectedId_WebsocketConnection uint64, message_SyncWorkspace _SyncWorkspace__PtyMessage_Ingress_)
	OnExit_PtyProxy__           func(exitedId_PtyProxy uint32, exitOutcome_PtyProxy _ExitOutcome_PtyProxy_)
}

func New__LifecycleCoordinator_WorkspacePty(
	api _NewApi__LifecycleCoordinator_WorkspacePty_,
) *_LifecycleCoordinator_WorkspacePty_ {
	workerContext, workerCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	return &_LifecycleCoordinator_WorkspacePty_{
		OnConnect_PtyWebsocket__:    api.OnConnect_PtyWebsocket__,
		OnDisconnect_PtyWebsocket__: api.OnDisconnect_PtyWebsocket__,
		OnSync_PtyPool__:            api.OnSync_PtyPool__,
		OnExit_PtyProxy__:           api.OnExit_PtyProxy__,
		WorkerContext:               workerContext,
		WorkerCancel:                workerCancel,
		QueueChannel_WorkspaceOrder: make(chan _WorkspaceOrder_LifecycleCoordinator_, 16),
	}
}

func (this *_LifecycleCoordinator_WorkspacePty_) RunWorker() {
	for {
		select {
		case <-this.WorkerContext.Done():
			return
		case leadingOrder := <-this.QueueChannel_WorkspaceOrder:
			pendingOrders := this.DrainPendingOrders(leadingOrder)
			reconciledOrders := this.ReconcileOrders(pendingOrders)
			for _, currentReconciledOrder := range reconciledOrders {
				currentReconciledOrder.Execute(this)
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
		case nextPendingOrder := <-this.QueueChannel_WorkspaceOrder:
			pendingOrdersResult = append(
				pendingOrdersResult,
				nextPendingOrder,
			)
		default:
			return pendingOrdersResult
		}
	}
}

func isConnect__LastOrder_Network(
	networkOrdersResult []_WorkspaceOrder_LifecycleCoordinator_,
) bool {
	if len(networkOrdersResult) == 0 {
		return false
	} else {
		_, isConnect := networkOrdersResult[len(networkOrdersResult)-1].(_Connect__WorkspaceOrder_LifecycleCoordinator_)
		return isConnect
	}
}

func isSync__LastOrder_Network(
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
	for _, currentPendingOrder := range pendingOrders {
		switch theCurrentPendingOrder := currentPendingOrder.(type) {
		case _ExitPty__WorkspaceOrder_LifecycleCoordinator_:
			exitOrdersResult = append(
				exitOrdersResult,
				theCurrentPendingOrder,
			)
		case _Disconnect__WorkspaceOrder_LifecycleCoordinator_:
			networkOrdersResult = []_WorkspaceOrder_LifecycleCoordinator_{
				theCurrentPendingOrder,
			}
		case _Connect__WorkspaceOrder_LifecycleCoordinator_:
			if isConnect__LastOrder_Network(networkOrdersResult) {
				networkOrdersResult[len(networkOrdersResult)-1] = theCurrentPendingOrder
			} else {
				networkOrdersResult = append(
					networkOrdersResult,
					theCurrentPendingOrder,
				)
			}
		case _Sync__WorkspaceOrder_LifecycleCoordinator_:
			if isSync__LastOrder_Network(networkOrdersResult) {
				networkOrdersResult[len(networkOrdersResult)-1] = theCurrentPendingOrder
			} else {
				networkOrdersResult = append(
					networkOrdersResult,
					theCurrentPendingOrder,
				)
			}
		}
	}
	return append(
		exitOrdersResult,
		networkOrdersResult...,
	)
}
