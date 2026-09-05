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
	OnConnected_PtyWebsocket__              func(newId_WebsocketConnection uint64)
	OnTakeoverConnected_PtyWebsocket__      func(newId_WebsocketConnection uint64)
	OnDisconnected_PtyWebsocket__           func()
	OnTakeoverDisconnected_PtyWebsocket__   func()
	OnPayload_BinaryMessage__PtyWebsocket__ func(expectedId_WebsocketConnection uint64, payload_binaryMessage []byte)
}

func New__WorkspaceNetwork(
	api _NewApi__WorkspaceNetwork_,
) *_WorkspaceNetwork_ {
	queueChannel__Submission_GetWebsocketConnection := make(chan _Submission_GetWebsocketConnection_, 16)
	workerContext__Submission_GetWebsocketConnection, workerCancel__Submission_GetWebsocketConnection := _CONTEXT.WithCancel(_CONTEXT.Background())
	websocketController_pty__Network := &_WebsocketController_{
		DeadlineTimeout_Read__:                60 * _TIME.Second,
		DeadlineTimeout_Write__:               10 * _TIME.Second,
		OnConnected__:                         api.OnConnected_PtyWebsocket__,
		OnTakeoverConnected__:                 api.OnTakeoverConnected_PtyWebsocket__,
		OnDisconnected__:                      api.OnDisconnected_PtyWebsocket__,
		OnTakeoverDisconnected__:              api.OnTakeoverDisconnected_PtyWebsocket__,
		OnPayload_BinaryMessage__:             api.OnPayload_BinaryMessage__PtyWebsocket__,
		Mutex:                                 _SYNC.Mutex{},
		EgressMutex:                           _SYNC.Mutex{},
		Status_WebsocketConnection:            STANDBY__Status_WebsocketConnection,
		IsTakeoverPending_WebsocketConnection: false,
		QueueChannel__Submission_GetWebsocketConnection:  queueChannel__Submission_GetWebsocketConnection,
		WorkerContext__Submission_GetWebsocketConnection: workerContext__Submission_GetWebsocketConnection,
		WorkerCancel__Submission_GetWebsocketConnection:  workerCancel__Submission_GetWebsocketConnection,
		WebsocketConnection:                              nil,
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

func (this *_WorkspaceNetwork_) StartServer() error {
	serverListener, maybeError_serverListen := _NET.Listen(
		"tcp",
		this.HttpServer.Addr,
	)
	if maybeError_serverListen != nil {
		return maybeError_serverListen
	}
	go this.WebsocketController_Pty.RunWorker__Submission_GetWebsocketConnection()
	go this.HttpServer.Serve(serverListener)
	return nil
}

func (this *_WorkspaceNetwork_) StopServer(
	context_shutdownDeadline__HttpServer _CONTEXT.Context,
) error {
	this.WebsocketController_Pty.WorkerCancel__Submission_GetWebsocketConnection()
	this.WebsocketController_Pty.CloseWithCode_WebsocketConnection(
		1001,
		"Server Shutting Down",
	)
	return this.HttpServer.Shutdown(context_shutdownDeadline__HttpServer)
}
