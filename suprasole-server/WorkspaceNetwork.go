package main

import (
	_CONTEXT "context"
	_NET "net"
	_HTTP "net/http"
	_SYNC "sync"
	_TIME "time"
)

type _WorkspaceNetwork_ struct {
	Mutex                   _SYNC.Mutex
	HttpServer              *_HTTP.Server
	WebsocketController_Pty *_WebsocketController_
}

type _NewApi__WorkspaceNetwork_ struct {
	Address_HttpServer__                    string
	OnConnected_PtyWebsocket__              func(id_WebsocketConnection_new uint64)
	OnConnected_Takeover__PtyWebsocket__    func(id_WebsocketConnection_new uint64)
	OnDisconnected_PtyWebsocket__           func()
	OnDisconnected_Takeover__PtyWebsocket__ func()
	OnPayload_BinaryMessage__PtyWebsocket__ func(id_WebsocketConnection_expected uint64, payload_binaryMessage []byte)
}

func New__WorkspaceNetwork(
	api _NewApi__WorkspaceNetwork_,
) *_WorkspaceNetwork_ {
	queueChannel__Submission_GetWebsocketConnection := make(chan _Submission_GetWebsocketConnection_, 16)
	workerContext__Submission_GetWebsocketConnection, workerCancel__Submission_GetWebsocketConnection := _CONTEXT.WithCancel(_CONTEXT.Background())
	websocketController_pty__Network := &_WebsocketController_{
		DeadlineTimeout_Read__:                           60 * _TIME.Second,
		DeadlineTimeout_Write__:                          10 * _TIME.Second,
		OnConnected__:                                    api.OnConnected_PtyWebsocket__,
		OnConnected_Takeover__:                           api.OnConnected_Takeover__PtyWebsocket__,
		OnDisconnected__:                                 api.OnDisconnected_PtyWebsocket__,
		OnDisconnected_Takeover__:                        api.OnDisconnected_Takeover__PtyWebsocket__,
		OnPayload_BinaryMessage__:                        api.OnPayload_BinaryMessage__PtyWebsocket__,
		Mutex:                                            _SYNC.Mutex{},
		EgressMutex:                                      _SYNC.Mutex{},
		Status_WebsocketConnection_current:               STANDBY__Status_WebsocketConnection,
		TakeoverStatus_WebsocketConnection_current:       NOT_PENDING__TakeoverStatus_WebsocketConnection,
		QueueChannel__Submission_GetWebsocketConnection:  queueChannel__Submission_GetWebsocketConnection,
		WorkerContext__Submission_GetWebsocketConnection: workerContext__Submission_GetWebsocketConnection,
		WorkerCancel__Submission_GetWebsocketConnection:  workerCancel__Submission_GetWebsocketConnection,
		WebsocketConnection_current:                      nil,
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
		Mutex:                   _SYNC.Mutex{},
		HttpServer:              httpServer_Network,
		WebsocketController_Pty: websocketController_pty__Network,
	}
}

func (this *_WorkspaceNetwork_) Start() error {
	serverListener, error_serverListen_maybe := _NET.Listen(
		"tcp",
		this.HttpServer.Addr,
	)
	if error_serverListen_maybe != nil {
		return error_serverListen_maybe
	}
	go this.WebsocketController_Pty.RunWorker__Submission_GetWebsocketConnection()
	go this.HttpServer.Serve(serverListener)
	return nil
}

func (this *_WorkspaceNetwork_) Stop(
	context_shutdownDeadline__HttpServer _CONTEXT.Context,
) error {
	this.WebsocketController_Pty.WorkerCancel__Submission_GetWebsocketConnection()
	this.WebsocketController_Pty.CloseWithCode_WebsocketConnection(
		1001,
		"Server Shutting Down",
	)
	return this.HttpServer.Shutdown(context_shutdownDeadline__HttpServer)
}
