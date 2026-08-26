package main

import (
	_CONTEXT "context"
	_NET     "net"
	_HTTP    "net/http"
	_SYNC    "sync"
	_TIME    "time"
)

type _WorkspaceNetwork_ struct {
	Mutex                  _SYNC.Mutex
	HttpServer             *_HTTP.Server
	PtyWebsocketController *_WebsocketController_
}

type _NewWorkspaceNetworkApi_ struct {
	HostPortAddress                     string
	OnConnected_PtyWebsocket            func()
	OnTakeoverConnected_PtyWebsocket    func()
	OnDisconnected_PtyWebsocket         func(readMessageError error)
	OnTakeoverDisconnected_PtyWebsocket func()
	OnBinaryMessageFrame_PtyWebsocket   func(binaryMessageFrame []byte)
}

func NewWorkspaceNetwork(
	networkApi _NewWorkspaceNetworkApi_,
) *_WorkspaceNetwork_ {
	submissionQueue := make(
		chan _Submission_GetWebsocketConnection_,
		16,
	)
	lifecycleContext, lifecycleCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	__PtyWebsocketController_Network := &_WebsocketController_{
		Mutex:                  _SYNC.Mutex{},
		EgressMutex:            _SYNC.Mutex{},
		ConnectionStatus:       STANDBY__WebsocketConnectionStatus,
		IsTakeoverPending:      false,
		ReadDeadlineTimeout:    60 * _TIME.Second,
		WriteDeadlineTimeout:   10 * _TIME.Second,
		LifecycleLoopContext:   lifecycleContext,
		LifecycleLoopCancel:    lifecycleCancel,
		SubmissionQueue:        submissionQueue,
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
		__PtyWebsocketController_Network.HandleRequest_GetWebsocketConnection,
	)
	__HttpServer_Network := &_HTTP.Server{
		Addr:    networkApi.HostPortAddress,
		Handler: requestHandlerRouter,
	}
	return &_WorkspaceNetwork_{
		Mutex:                  _SYNC.Mutex{},
		HttpServer:             __HttpServer_Network,
		PtyWebsocketController: __PtyWebsocketController_Network,
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
	go this.PtyWebsocketController.RunLifecycleLoop()
	go this.HttpServer.Serve(serverListener)
	return nil
}

func (this *_WorkspaceNetwork_) StopServer(
	shutdownDeadlineContext _CONTEXT.Context,
) error {
	this.PtyWebsocketController.LifecycleLoopCancel()
	this.PtyWebsocketController.CloseWithCode(
		1001,
		"Server Shutting Down",
	)
	return this.HttpServer.Shutdown(shutdownDeadlineContext)
}
