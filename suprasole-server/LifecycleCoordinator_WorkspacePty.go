package main

import (
	_CONTEXT "context"
)

type _LifecycleCoordinator_WorkspacePty_ struct {
	OnConnect_PtyWebsocket__     func(id_WebsocketConnection_expected uint64)
	OnDisconnect_PtyWebsocket__  func()
	OnSync_PtyPool__             func(id_WebsocketConnection_expected uint64, message__Batch_SyncPty _Batch_SyncPty__PtyMessage_Ingress_)
	OnExit_PtyProxy__            func(id_WorkspacePty_exited uint32, exitOutcome_PtyProxy _ExitOutcome_PtyProxy_)
	OnSpawnPty__                 func(id_WebsocketConnection_expected uint64, message _SpawnPty__PtyMessage_Ingress_)
	OnStatus_SpawnPty__Success__ func(id_WebsocketConnection_expected uint64, id_WorkspacePty_spawned uint32, ptyProxy_quiescent *_PtyProxy_)
	OnStatus_SpawnPty__Failure__ func(id_WebsocketConnection_expected uint64, id_WorkspacePty_failed uint32, error_Start__PtyCommand error)
	QueueChannel_WorkspaceOrder  chan _WorkspaceOrder_LifecycleCoordinator_
	WorkerContext                _CONTEXT.Context
	WorkerCancel                 _CONTEXT.CancelFunc
}

type _WorkspaceOrder_LifecycleCoordinator_ interface {
	Execute(lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_)
}

type _Connect__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
}

type _Disconnect__WorkspaceOrder_LifecycleCoordinator_ struct{}

type _Sync__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
	Message__Batch_SyncPty          _Batch_SyncPty__PtyMessage_Ingress_
}

type _ExitPty__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WorkspacePty_exited uint32
	ExitOutcome_PtyProxy   _ExitOutcome_PtyProxy_
}

type _SpawnPty__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
	Message_SpawnPty                _SpawnPty__PtyMessage_Ingress_
}

type _Status_SpawnPty__Success__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
	Id_WorkspacePty_spawned         uint32
	PtyProxy_quiescent              *_PtyProxy_
}

type _Status_SpawnPty__Failure__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
	Id_WorkspacePty_failed          uint32
	Error_Start__PtyCommand         error
}

type _MakeApi__LifecycleCoordinator_WorkspacePty_ struct {
	OnConnect_PtyWebsocket__     func(id_WebsocketConnection_expected uint64)
	OnDisconnect_PtyWebsocket__  func()
	OnSync_PtyPool__             func(id_WebsocketConnection_expected uint64, message__Batch_SyncPty _Batch_SyncPty__PtyMessage_Ingress_)
	OnExit_PtyProxy__            func(id_WorkspacePty_exited uint32, exitOutcome_PtyProxy _ExitOutcome_PtyProxy_)
	OnSpawnPty__                 func(id_WebsocketConnection_expected uint64, message _SpawnPty__PtyMessage_Ingress_)
	OnStatus_SpawnPty__Success__ func(id_WebsocketConnection_expected uint64, id_WorkspacePty_spawned uint32, ptyProxy_quiescent *_PtyProxy_)
	OnStatus_SpawnPty__Failure__ func(id_WebsocketConnection_expected uint64, id_WorkspacePty_failed uint32, error_Start__PtyCommand error)
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
