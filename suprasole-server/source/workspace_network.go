package source

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
	HostPortAddress        string
	OnConnected            func()
	OnTakeoverConnected    func()
	OnDisconnected         func(readMessageError error)
	OnTakeoverDisconnected func()
	OnBinaryMessage        func(binaryMessageFrame []byte)
}

func NewWorkspaceNetwork(
	networkApi _NewWorkspaceNetworkApi_,
) *_WorkspaceNetwork_ {
	submissionQueue := make(
		chan _Submission_GetWebsocketConnection_,
		16,
	)
	lifecycleContext, lifecycleCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	__Network_PtyWebsocket := &_WebsocketController_{
		Mutex:                  _SYNC.Mutex{},
		EgressMutex:            _SYNC.Mutex{},
		ConnectionStatus:       STANDBY__WebsocketConnectionStatus,
		IsTakeoverPending:      false,
		WebsocketConnection:    nil,
		ReadDeadlineTimeout:    60 * _TIME.Second,
		WriteDeadlineTimeout:   10 * _TIME.Second,
		LifecycleLoopContext:   lifecycleContext,
		LifecycleLoopCancel:    lifecycleCancel,
		SubmissionQueue:        submissionQueue,
		OnConnected:            networkApi.OnConnected,
		OnTakeoverConnected:    networkApi.OnTakeoverConnected,
		OnDisconnected:         networkApi.OnDisconnected,
		OnTakeoverDisconnected: networkApi.OnTakeoverDisconnected,
		OnBinaryMessage:        networkApi.OnBinaryMessage,
	}
	requestHandlerRouter := _HTTP.NewServeMux()
	requestHandlerRouter.HandleFunc(
		"/pty",
		__Network_PtyWebsocket.HandleGetPtyRequest,
	)
	__Network_HttpServer := &_HTTP.Server{
		Addr:    networkApi.HostPortAddress,
		Handler: requestHandlerRouter,
	}
	return &_WorkspaceNetwork_{
		Mutex:                  _SYNC.Mutex{},
		HttpServer:             __Network_HttpServer,
		PtyWebsocketController: __Network_PtyWebsocket,
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
