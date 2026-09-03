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
	HostPortAddress__                     string
	OnConnected_PtyWebsocket__            func(id_WebsocketConnection uint64)
	OnTakeoverConnected_PtyWebsocket__    func(id_WebsocketConnection uint64)
	OnDisconnected_PtyWebsocket__         func(id_WebsocketConnection uint64, readMessageError error)
	OnTakeoverDisconnected_PtyWebsocket__ func(id_WebsocketConnection uint64)
	OnBinaryMessageFrame_PtyWebsocket__   func(id_WebsocketConnection uint64, binaryMessageFrame []byte)
}

func New__WorkspaceNetwork(
	api _NewApi__WorkspaceNetwork_,
) *_WorkspaceNetwork_ {
	__QueueChannel__Submission_GetWebsocketConnection := make(chan _Submission_GetWebsocketConnection_, 16)
	__WorkerContext__Submission_GetWebsocketConnection, __WorkerCancel__Submission_GetWebsocketConnection := _CONTEXT.WithCancel(_CONTEXT.Background())
	__WebsocketController_Pty__Network := &_WebsocketController_{
		Mutex:                    _SYNC.Mutex{},
		EgressMutex:              _SYNC.Mutex{},
		ConnectionStatus:         STANDBY__WebsocketConnectionStatus,
		IsTakeoverPending:        false,
		ReadDeadlineTimeout__:    60 * _TIME.Second,
		WriteDeadlineTimeout__:   10 * _TIME.Second,
		OnConnected__:            api.OnConnected_PtyWebsocket__,
		OnTakeoverConnected__:    api.OnTakeoverConnected_PtyWebsocket__,
		OnDisconnected__:         api.OnDisconnected_PtyWebsocket__,
		OnTakeoverDisconnected__: api.OnTakeoverDisconnected_PtyWebsocket__,
		OnBinaryMessageFrame__:   api.OnBinaryMessageFrame_PtyWebsocket__,
		QueueChannel__Submission_GetWebsocketConnection:  __QueueChannel__Submission_GetWebsocketConnection,
		WorkerContext__Submission_GetWebsocketConnection: __WorkerContext__Submission_GetWebsocketConnection,
		WorkerCancel__Submission_GetWebsocketConnection:  __WorkerCancel__Submission_GetWebsocketConnection,
		WebsocketConnection:                              nil,
	}
	requestHandlerRouter := _HTTP.NewServeMux()
	requestHandlerRouter.HandleFunc(
		"/pty",
		__WebsocketController_Pty__Network.HandleRequest_GetWebsocketConnection,
	)
	__HttpServer_Network := &_HTTP.Server{
		Addr:    api.HostPortAddress__,
		Handler: requestHandlerRouter,
	}
	return &_WorkspaceNetwork_{
		Mutex:                   _SYNC.Mutex{},
		HttpServer:              __HttpServer_Network,
		WebsocketController_Pty: __WebsocketController_Pty__Network,
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
	shutdownDeadlineContext _CONTEXT.Context,
) error {
	this.WebsocketController_Pty.WorkerCancel__Submission_GetWebsocketConnection()
	this.WebsocketController_Pty.CloseWithCode(
		1001,
		"Server Shutting Down",
	)
	return this.HttpServer.Shutdown(shutdownDeadlineContext)
}
