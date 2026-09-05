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
	OnPayload_BinaryMessage__PtyWebsocket__ func(id_WebsocketConnection uint64, payload_binaryMessage []byte)
}

func New__WorkspaceNetwork(
	api _NewApi__WorkspaceNetwork_,
) *_WorkspaceNetwork_ {
	queueChannel_Submission_GetWebsocketConnection := make(chan _Submission_GetWebsocketConnection_, 16)
	workerContext_Submission_GetWebsocketConnection, workerCancel_Submission_GetWebsocketConnection := _CONTEXT.WithCancel(_CONTEXT.Background())
	websocketController_Pty_Network := &_WebsocketController_{
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
		QueueChannel__Submission_GetWebsocketConnection:  queueChannel_Submission_GetWebsocketConnection,
		WorkerContext__Submission_GetWebsocketConnection: workerContext_Submission_GetWebsocketConnection,
		WorkerCancel__Submission_GetWebsocketConnection:  workerCancel_Submission_GetWebsocketConnection,
		WebsocketConnection:                              nil,
	}
	requestHandlerRouter := _HTTP.NewServeMux()
	requestHandlerRouter.HandleFunc(
		"/pty",
		websocketController_Pty_Network.HandleRequest_GetWebsocketConnection,
	)
	httpServer_Network := &_HTTP.Server{
		Addr:    api.Address_HttpServer__,
		Handler: requestHandlerRouter,
	}
	return &_WorkspaceNetwork_{
		Mutex:                   _SYNC.Mutex{},
		HttpServer:              httpServer_Network,
		WebsocketController_Pty: websocketController_Pty_Network,
	}
}

func (this *_WorkspaceNetwork_) StartServer() error {
	serverListener, listenError := _NET.Listen(
		"tcp",
		this.HttpServer.Addr,
	)
	if listenError != nil {
		return listenError
	}
	go this.WebsocketController_Pty.RunWorker__Submission_GetWebsocketConnection()
	go this.HttpServer.Serve(serverListener)
	return nil
}

func (this *_WorkspaceNetwork_) StopServer(
	shutdownDeadlineContext_HttpServer _CONTEXT.Context,
) error {
	this.WebsocketController_Pty.WorkerCancel__Submission_GetWebsocketConnection()
	this.WebsocketController_Pty.CloseWithCode(
		1001,
		"Server Shutting Down",
	)
	return this.HttpServer.Shutdown(shutdownDeadlineContext_HttpServer)
}
