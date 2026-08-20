package source

import (
	_CONTEXT "context"
	_NET "net"
	_HTTP "net/http"
	_SYNC "sync"
	_TIME "time"
)

type WorkspaceNetwork struct {
	Network_Mutex        _SYNC.Mutex
	Network_HttpServer   *_HTTP.Server
	Network_PtyWebsocket *_WebsocketProxy_
}

type NewWorkspaceNetworkApi struct {
	Server_HostPortAddress           string
	Websocket_OnConnected            func()
	Websocket_OnTakeoverConnected    func()
	Websocket_OnDisconnected         func(err error)
	Websocket_OnTakeoverDisconnected func()
	Websocket_OnBinaryMessage        func(frame []byte)
}

func NewWorkspaceNetwork(
	networkApi NewWorkspaceNetworkApi,
) *WorkspaceNetwork {
	submissionQueue := make(chan _Submission_GetWebsocketConnection_, 16)
	lifecycleContext, lifecycleCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	__Network_PtyWebsocket := &_WebsocketProxy_{
		Websocket_Mutex:                  _SYNC.Mutex{},
		Websocket_Status:                 WebsocketStatus_Standby,
		Websocket_IsTakeoverPending:      false,
		Websocket_ReadDeadlineTimeout:    60 * _TIME.Second,
		Websocket_WriteDeadlineTimeout:   10 * _TIME.Second,
		Websocket_SubmissionQueue:        submissionQueue,
		Websocket_LifecycleLoopContext:   lifecycleContext,
		Websocket_LifecycleLoopCancel:    lifecycleCancel,
		Websocket_OnConnected:            networkApi.Websocket_OnConnected,
		Websocket_OnTakeoverConnected:    networkApi.Websocket_OnTakeoverConnected,
		Websocket_OnDisconnected:         networkApi.Websocket_OnDisconnected,
		Websocket_OnTakeoverDisconnected: networkApi.Websocket_OnTakeoverDisconnected,
		Websocket_OnBinaryMessage:        networkApi.Websocket_OnBinaryMessage,
		Websocket_Connection:             nil,
	}
	requestHandlerRouter := _HTTP.NewServeMux()
	requestHandlerRouter.HandleFunc(
		"/pty",
		__Network_PtyWebsocket.Websocket_HandleGetPtyRequest,
	)
	__Network_HttpServer := &_HTTP.Server{
		Addr:    networkApi.Server_HostPortAddress,
		Handler: requestHandlerRouter,
	}
	return &WorkspaceNetwork{
		Network_Mutex:        _SYNC.Mutex{},
		Network_HttpServer:   __Network_HttpServer,
		Network_PtyWebsocket: __Network_PtyWebsocket,
	}
}

func (thisNetwork *WorkspaceNetwork) Network_StartServer() error {
	serverListener, listenError := _NET.Listen(
		"tcp",
		thisNetwork.Network_HttpServer.Addr,
	)
	if listenError != nil {
		return listenError
	}
	go thisNetwork.Network_PtyWebsocket.Websocket_RunLifecycleLoop()
	go thisNetwork.Network_HttpServer.Serve(serverListener)
	return nil
}

func (thisNetwork *WorkspaceNetwork) Network_StopServer(
	shutdownDeadlineContext _CONTEXT.Context,
) error {
	thisNetwork.Network_PtyWebsocket.Websocket_LifecycleLoopCancel()
	thisNetwork.Network_PtyWebsocket.Websocket_CloseWithCode(
		1001,
		"Server Shutting Down",
	)
	return thisNetwork.Network_HttpServer.Shutdown(shutdownDeadlineContext)
}
