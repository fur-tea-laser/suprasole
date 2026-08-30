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
	HostPortAddress                     string
	OnConnected_PtyWebsocket            func(id_WebsocketConnection uint64)
	OnTakeoverConnected_PtyWebsocket    func(id_WebsocketConnection uint64)
	OnDisconnected_PtyWebsocket         func(id_WebsocketConnection uint64, readMessageError error)
	OnTakeoverDisconnected_PtyWebsocket func(id_WebsocketConnection uint64)
	OnBinaryMessageFrame_PtyWebsocket   func(id_WebsocketConnection uint64, binaryMessageFrame []byte)
}

func New__WorkspaceNetwork(
	networkApi _NewApi__WorkspaceNetwork_,
) *_WorkspaceNetwork_ {
	__QueueChannel__Submission_GetWebsocketConnection := make(
		chan _Submission_GetWebsocketConnection_,
		16,
	)
	__WorkerContext__Submission_GetWebsocketConnection, __WorkerCancel__Submission_GetWebsocketConnection := _CONTEXT.WithCancel(_CONTEXT.Background())
	__WebsocketController_Pty__Network := &_WebsocketController_{
		Mutex:                _SYNC.Mutex{},
		EgressMutex:          _SYNC.Mutex{},
		ConnectionStatus:     STANDBY__WebsocketConnectionStatus,
		IsTakeoverPending:    false,
		ReadDeadlineTimeout:  60 * _TIME.Second,
		WriteDeadlineTimeout: 10 * _TIME.Second,
		WorkerContext__Submission_GetWebsocketConnection: __WorkerContext__Submission_GetWebsocketConnection,
		WorkerCancel__Submission_GetWebsocketConnection:  __WorkerCancel__Submission_GetWebsocketConnection,
		QueueChannel__Submission_GetWebsocketConnection:  __QueueChannel__Submission_GetWebsocketConnection,
		OnConnected:            networkApi.OnConnected_PtyWebsocket,
		OnTakeoverConnected:    networkApi.OnTakeoverConnected_PtyWebsocket,
		OnDisconnected:         networkApi.OnDisconnected_PtyWebsocket,
		OnTakeoverDisconnected: networkApi.OnTakeoverDisconnected_PtyWebsocket,
		OnBinaryMessageFrame:   networkApi.OnBinaryMessageFrame_PtyWebsocket,
		WebsocketConnection:    nil,
	}
	requestHandlerRouter := _HTTP.NewServeMux()
	requestHandlerRouter.HandleFunc(
		"/pty",
		__WebsocketController_Pty__Network.HandleRequest_GetWebsocketConnection,
	)
	__HttpServer_Network := &_HTTP.Server{
		Addr:    networkApi.HostPortAddress,
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
