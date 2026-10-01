package main

import (
	_CONTEXT "context"
	_FLAG "flag"
	_FMT "fmt"
	_HTTP "net/http"
	_NET "net"
	_OS "os"
	_SIGNAL "os/signal"
	_SYNC "sync"
	_SYSCALL "syscall"
	_TIME "time"
)

func main() {
	cliOption__Address_HttpServer := _FLAG.String(
		"Address_HttpServer",
		"127.0.0.1:8000",
		"Network address (host:port) for the HTTP server to listen on",
	)
	_FLAG.Parse()
	WorkspaceController := Make_WorkspaceController(
		_MakeApi_WorkspaceController_{
			Address_HttpServer__: *cliOption__Address_HttpServer,
			OptionConfig_PtyProxy__default__: _OptionConfig_PtyProxy_{
				Count_ScrollbackLine__PtyTerminal:      10000,
				Size_PostSnapshotBuffer__PtyProxy:      64 * 1024,
				Size_QueueBuffer__InputOrder_PtyWriter: 1024,
			},
		},
	)
	error__Listen_HttpServer__maybe := WorkspaceController.Start()
	if error__Listen_HttpServer__maybe != nil {
		_FMT.Fprintf(
			_OS.Stderr,
			"Error starting server: %v\n",
			error__Listen_HttpServer__maybe,
		)
		_OS.Exit(1)
	}
	_FMT.Printf(
		"Suprasole server listening on %s\n",
		*cliOption__Address_HttpServer,
	)
	channel_shutdownSignal := make(chan _OS.Signal, 1)
	_SIGNAL.Notify(
		channel_shutdownSignal,
		_OS.Interrupt,
		_SYSCALL.SIGTERM,
	)
	<-channel_shutdownSignal
	_FMT.Println("Shutting down suprasole server...")
	WorkspaceController.Shutdown()
}

type _MakeApi_WorkspaceController_ struct {
	Address_HttpServer__             string
	OptionConfig_PtyProxy__default__ _OptionConfig_PtyProxy_
}

func Make_WorkspaceController(
	api _MakeApi_WorkspaceController_,
) *_WorkspaceController_ {
	workspaceController_result := &_WorkspaceController_{
		OptionConfig_PtyProxy__default__:          api.OptionConfig_PtyProxy__default__,
		Mutex:                                     _SYNC.Mutex{},
		PtyPool:                                   make(map[uint32]*_WorkspacePty_),
		Id_WorkspacePty_next:                      0,
		WorkspaceNetwork:                          nil,
		LifecycleCoordinator_WorkspacePty:         nil,
		MessageReducer__LayoutUpdate_PtyProxy:     nil,
	}
	workspaceController_result.WorkspaceNetwork = Make_WorkspaceNetwork(
		_MakeApi_WorkspaceNetwork_{
			Address_HttpServer__:                    api.Address_HttpServer__,
			OnConnected_PtyWebsocket__:              workspaceController_result.HandleConnected_PtyWebsocket,
			OnConnected_Takeover__PtyWebsocket__:    workspaceController_result.HandleConnected_Takeover__PtyWebsocket,
			OnDisconnected_PtyWebsocket__:           workspaceController_result.HandleDisconnected_PtyWebsocket,
			OnDisconnected_Takeover__PtyWebsocket__: workspaceController_result.HandleDisconnected_Takeover__PtyWebsocket,
			OnPayload_BinaryMessage__PtyWebsocket__: workspaceController_result.HandlePayload_BinaryMessage__PtyWebsocket,
		},
	)
	workspaceController_result.LifecycleCoordinator_WorkspacePty = Make__LifecycleCoordinator_WorkspacePty(
		_MakeApi__LifecycleCoordinator_WorkspacePty_{
			OnConnect_PtyWebsocket__:     workspaceController_result.HandleConnect_PtyWebsocket__Coordinator,
			OnDisconnect_PtyWebsocket__:  workspaceController_result.HandleDisconnect_PtyWebsocket__Coordinator,
			OnSyncVisibility__:           workspaceController_result.HandleSyncVisibility__Coordinator,
			OnEmitSnapshot_PtyProxy__:    workspaceController_result.HandleEmitSnapshot_PtyProxy__Coordinator,
			OnSpawnPty__:                 workspaceController_result.HandleSpawnPty__Coordinator,
			OnStatus_SpawnPty__Success__: workspaceController_result.HandleStatus_SpawnPty__Success__Coordinator,
			OnStatus_SpawnPty__Failure__: workspaceController_result.HandleStatus_SpawnPty__Failure__Coordinator,
			OnTerminatePty__:             workspaceController_result.HandleTerminatePty__Coordinator,
			OnExit_PtyProxy__:            workspaceController_result.HandleExit_PtyProxy__Coordinator,
			OnRemovePty__:                workspaceController_result.HandleRemovePty__Coordinator,
		},
	)
	workspaceController_result.MessageReducer__LayoutUpdate_PtyProxy = Make__MessageReducer__LayoutUpdate_PtyProxy(
		_MakeApi__MessageReducer__LayoutUpdate_PtyProxy_{
			DebounceTimeout__:           50 * _TIME.Millisecond,
			OnFlush__ResizeOnly__:        workspaceController_result.HandleFlush__ResizeOnly__Reducer,
			OnFlush__SpawnPty__:          workspaceController_result.HandleFlush__SpawnPty__Reducer,
			OnFlush__Batch_RemovePty__:   workspaceController_result.HandleFlush__Batch_RemovePty__Reducer,
			OnFlush__Batch_SyncPty__:     workspaceController_result.HandleFlush__Batch_SyncPty__Reducer,
		},
	)
	return workspaceController_result
}

type _MakeApi_WorkspaceNetwork_ struct {
	Address_HttpServer__                    string
	OnConnected_PtyWebsocket__              func(id_WebsocketConnection_new uint64)
	OnConnected_Takeover__PtyWebsocket__    func(id_WebsocketConnection_new uint64)
	OnDisconnected_PtyWebsocket__           func()
	OnDisconnected_Takeover__PtyWebsocket__ func()
	OnPayload_BinaryMessage__PtyWebsocket__ func(id_WebsocketConnection_expected uint64, payload_binaryMessage []byte)
}

func Make_WorkspaceNetwork(
	api _MakeApi_WorkspaceNetwork_,
) *_WorkspaceNetwork_ {
	queueChannel__Submission_GetWebsocketConnection := make(chan _Submission_GetWebsocketConnection_, 16)
	workerContext__Submission_GetWebsocketConnection, workerCancel__Submission_GetWebsocketConnection := _CONTEXT.WithCancel(_CONTEXT.Background())
	websocketController_pty__Network := &_WebsocketController_{
		DeadlineTimeout_Read__:                   60 * _TIME.Second,
		DeadlineTimeout_Write__:                  10 * _TIME.Second,
		PingPeriod__:                             15 * _TIME.Second,
		OnConnected__:                            api.OnConnected_PtyWebsocket__,
		OnConnected_Takeover__:                   api.OnConnected_Takeover__PtyWebsocket__,
		OnDisconnected__:                         api.OnDisconnected_PtyWebsocket__,
		OnDisconnected_Takeover__:                api.OnDisconnected_Takeover__PtyWebsocket__,
		OnPayload_BinaryMessage__:                api.OnPayload_BinaryMessage__PtyWebsocket__,
		Mutex:                                    _SYNC.Mutex{},
		EgressMutex:                              _SYNC.Mutex{},
		Status_WebsocketConnection_state:         STANDBY__Status_WebsocketConnection,
		TakeoverStatus_WebsocketConnection_state: NOT_PENDING__TakeoverStatus_WebsocketConnection,
		QueueChannel__Submission_GetWebsocketConnection:     queueChannel__Submission_GetWebsocketConnection,
		WorkerContext__Submission_GetWebsocketConnection:    workerContext__Submission_GetWebsocketConnection,
		WorkerCancel__Submission_GetWebsocketConnection:     workerCancel__Submission_GetWebsocketConnection,
		WebsocketConnection_state:                           nil,
		WorkerCancel_ClientPing__WebsocketConnection__state: nil,
	}
	router_requestHandler__HttpServer := _HTTP.NewServeMux()
	router_requestHandler__HttpServer.HandleFunc(
		"/pty",
		websocketController_pty__Network.HandleRequest_GetWebsocketConnection,
	)
	httpServer_Network := &_HTTP.Server{
		Addr:    api.Address_HttpServer__,
		Handler: router_requestHandler__HttpServer,
	}
	return &_WorkspaceNetwork_{
		HttpServer:              httpServer_Network,
		WebsocketController_Pty: websocketController_pty__Network,
	}
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

type _MakeApi__MessageReducer__LayoutUpdate_PtyProxy_ struct {
	DebounceTimeout__         _TIME.Duration
	OnFlush__ResizeOnly__      func(batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_)
	OnFlush__SpawnPty__        func(order_SpawnPty _SpawnPty___MessageOrder__LayoutUpdate_PtyProxy_, batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_)
	OnFlush__Batch_RemovePty__ func(order_Batch_RemovePty _Batch_RemovePty___MessageOrder__LayoutUpdate_PtyProxy_, batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_)
	OnFlush__Batch_SyncPty__   func(order_Batch_SyncPty _Batch_SyncPty___MessageOrder__LayoutUpdate_PtyProxy_)
}

func Make__MessageReducer__LayoutUpdate_PtyProxy(
	api _MakeApi__MessageReducer__LayoutUpdate_PtyProxy_,
) *_MessageReducer__LayoutUpdate_PtyProxy_ {
	workerContext, workerCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	return &_MessageReducer__LayoutUpdate_PtyProxy_{
		DebounceTimeout__:         api.DebounceTimeout__,
		OnFlush__ResizeOnly__:      api.OnFlush__ResizeOnly__,
		OnFlush__SpawnPty__:        api.OnFlush__SpawnPty__,
		OnFlush__Batch_RemovePty__: api.OnFlush__Batch_RemovePty__,
		OnFlush__Batch_SyncPty__:   api.OnFlush__Batch_SyncPty__,
		QueueChannel:              make(chan _MessageOrder__LayoutUpdate_PtyProxy_, 512),
		WorkerContext:             workerContext,
		WorkerCancel:              workerCancel,
	}
}

func (This *_WorkspaceController_) Start() error {
	go This.MessageReducer__LayoutUpdate_PtyProxy.RunWorker()
	go This.LifecycleCoordinator_WorkspacePty.RunWorker()
	return This.WorkspaceNetwork.Start()
}

func (this *_WorkspaceNetwork_) Start() error {
	listener_HttpServer, error__Listen_HttpServer__maybe := _NET.Listen(
		"tcp",
		this.HttpServer.Addr,
	)
	if error__Listen_HttpServer__maybe != nil {
		return error__Listen_HttpServer__maybe
	}
	go this.WebsocketController_Pty.RunWorker__Submission_GetWebsocketConnection()
	go this.HttpServer.Serve(listener_HttpServer)
	return nil
}

func (This *_WorkspaceController_) Shutdown() {
	This.MessageReducer__LayoutUpdate_PtyProxy.WorkerCancel()
	This.LifecycleCoordinator_WorkspacePty.WorkerCancel()
	This.WorkspaceNetwork.Shutdown()
}

func (this *_WorkspaceNetwork_) Shutdown() {
	context_shutdownDeadline__HttpServer, cancel_shutdownDeadline__HttpServer := _CONTEXT.WithTimeout(
		_CONTEXT.Background(),
		5*_TIME.Second,
	)
	this.WebsocketController_Pty.WorkerCancel__Submission_GetWebsocketConnection()
	this.WebsocketController_Pty.CloseWithCode_WebsocketConnection(
		1001,
		"Server Shutting Down",
	)
	_ = this.HttpServer.Shutdown(context_shutdownDeadline__HttpServer)
	cancel_shutdownDeadline__HttpServer()
}
