package main

import (
	_CONTEXT "context"
)

type _LifecycleCoordinator_WorkspacePty_ struct {
	OnConnect_PtyWebsocket__     func(id_WebsocketConnection_expected uint64)
	OnDisconnect_PtyWebsocket__  func()
	OnSyncVisibility__           func(id_WebsocketConnection_expected uint64, batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_)
	OnEmitSnapshot_PtyProxy__    func(id_WebsocketConnection_expected uint64, id_WorkspacePty uint32)
	OnSpawnPty__                 func(id_WebsocketConnection_expected uint64, message _SpawnPty__PtyMessage_Ingress_)
	OnStatus_SpawnPty__Success__ func(id_WebsocketConnection_expected uint64, id_WorkspacePty_spawned uint32, ptyProxy_quiescent *_PtyProxy_)
	OnStatus_SpawnPty__Failure__ func(id_WebsocketConnection_expected uint64, id_WorkspacePty_failed uint32, error_Start__PtyCommand error)
	OnTerminatePty__             func(id_WebsocketConnection_expected uint64, id_WorkspacePty uint32, terminalSignal_PtyProcess int)
	OnExit_PtyProxy__            func(id_WorkspacePty_exited uint32, exitOutcome_PtyProxy _ExitOutcome_PtyProxy_)
	OnRemovePty__                func(id_WebsocketConnection_expected uint64, orderBatch_RemovePty []_Order_RemovePty_)
	QueueChannel_WorkspaceOrder  chan _WorkspaceOrder_LifecycleCoordinator_
	WorkerContext                _CONTEXT.Context
	WorkerCancel                 _CONTEXT.CancelFunc
}

type _WorkspaceOrder_LifecycleCoordinator_ interface {
	Apply(groupJob_WorkspaceOrder__reconciled_state *_GroupJob_WorkspaceOrder__LifecycleCoordinator_)
	Execute(LifecycleCoordinator_forwarded *_LifecycleCoordinator_WorkspacePty_)
}

type _Connect__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
}

type _Disconnect__WorkspaceOrder_LifecycleCoordinator_ struct{}

type _SyncVisibility__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
	Batch__LayoutUpdate_PtyProxy    map[uint32]*_LayoutUpdate_PtyProxy_
}

type _EmitSnapshot_PtyProxy__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
	Id_WorkspacePty                 uint32
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

type _TerminatePty__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
	Id_WorkspacePty                 uint32
	TerminalSignal_PtyProcess       int
}

type _ExitPty__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WorkspacePty_exited uint32
	ExitOutcome_PtyProxy   _ExitOutcome_PtyProxy_
}

type _RemovePty__WorkspaceOrder_LifecycleCoordinator_ struct {
	Id_WebsocketConnection_expected uint64
	OrderBatch_RemovePty            []_Order_RemovePty_
}

type _GroupJob_WorkspaceOrder__LifecycleCoordinator_ struct {
	OrderBatch_ExitPty         []_ExitPty__WorkspaceOrder_LifecycleCoordinator_
	OrderBatch_TerminatePty    []_TerminatePty__WorkspaceOrder_LifecycleCoordinator_
	OrderBatch_RemovePty       []_RemovePty__WorkspaceOrder_LifecycleCoordinator_
	OrderBatch_SpawnPty        []_SpawnPty__WorkspaceOrder_LifecycleCoordinator_
	OrderBatch__Status_SpawnPty []_WorkspaceOrder_LifecycleCoordinator_
	Order_Disconnect_maybe     *_Disconnect__WorkspaceOrder_LifecycleCoordinator_
	Order_Connect_maybe        *_Connect__WorkspaceOrder_LifecycleCoordinator_
	Order_SyncVisibility_maybe *_SyncVisibility__WorkspaceOrder_LifecycleCoordinator_
	OrderBatch_EmitSnapshot    []_EmitSnapshot_PtyProxy__WorkspaceOrder_LifecycleCoordinator_
}

type _MakeApi__LifecycleCoordinator_WorkspacePty_ struct {
	OnConnect_PtyWebsocket__     func(id_WebsocketConnection_expected uint64)
	OnDisconnect_PtyWebsocket__  func()
	OnSyncVisibility__           func(id_WebsocketConnection_expected uint64, batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_)
	OnEmitSnapshot_PtyProxy__    func(id_WebsocketConnection_expected uint64, id_WorkspacePty uint32)
	OnSpawnPty__                 func(id_WebsocketConnection_expected uint64, message _SpawnPty__PtyMessage_Ingress_)
	OnStatus_SpawnPty__Success__ func(id_WebsocketConnection_expected uint64, id_WorkspacePty_spawned uint32, ptyProxy_quiescent *_PtyProxy_)
	OnStatus_SpawnPty__Failure__ func(id_WebsocketConnection_expected uint64, id_WorkspacePty_failed uint32, error_Start__PtyCommand error)
	OnTerminatePty__             func(id_WebsocketConnection_expected uint64, id_WorkspacePty uint32, terminalSignal_PtyProcess int)
	OnExit_PtyProxy__            func(id_WorkspacePty_exited uint32, exitOutcome_PtyProxy _ExitOutcome_PtyProxy_)
	OnRemovePty__                func(id_WebsocketConnection_expected uint64, orderBatch_RemovePty []_Order_RemovePty_)
}

func Make__LifecycleCoordinator_WorkspacePty(
	api _MakeApi__LifecycleCoordinator_WorkspacePty_,
) *_LifecycleCoordinator_WorkspacePty_ {
	workerContext, workerCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	return &_LifecycleCoordinator_WorkspacePty_{
		OnConnect_PtyWebsocket__:     api.OnConnect_PtyWebsocket__,
		OnDisconnect_PtyWebsocket__:  api.OnDisconnect_PtyWebsocket__,
		OnSyncVisibility__:           api.OnSyncVisibility__,
		OnEmitSnapshot_PtyProxy__:    api.OnEmitSnapshot_PtyProxy__,
		OnSpawnPty__:                 api.OnSpawnPty__,
		OnStatus_SpawnPty__Success__: api.OnStatus_SpawnPty__Success__,
		OnStatus_SpawnPty__Failure__: api.OnStatus_SpawnPty__Failure__,
		OnTerminatePty__:             api.OnTerminatePty__,
		OnExit_PtyProxy__:            api.OnExit_PtyProxy__,
		OnRemovePty__:                api.OnRemovePty__,
		QueueChannel_WorkspaceOrder:  make(chan _WorkspaceOrder_LifecycleCoordinator_, 128),
		WorkerContext:                workerContext,
		WorkerCancel:                 workerCancel,
	}
}
