package main

import (
	_CONTEXT "context"
	_BINARY "encoding/binary"
	_FMT "fmt"
	_MAPS "maps"
	_SYNC "sync"
	_SYSCALL "syscall"
	_TIME "time"
)

type _Defaults_PtyProxy_ struct {
	ScrollbackLineCount_PtyTerminal__       int
	StagingBufferSize_PtyReader__           int
	PostSnapshotBufferSize_PtyProxy__       int
	QueueBufferSize_InputOrder__PtyWriter__ int
}

type _WorkspaceController_ struct {
	Defaults_PtyProxy__               _Defaults_PtyProxy_
	Mutex                             _SYNC.Mutex
	PtyPool                           map[uint32]*_WorkspacePty_
	NextId_PtyProxy                   uint32
	WorkspaceNetwork                  *_WorkspaceNetwork_
	MessageDebouncer_ResizePtys       *_MessageDebouncer_ResizePtys_
	LifecycleCoordinator_WorkspacePty *_LifecycleCoordinator_WorkspacePty_
}

type _NewApi__WorkspaceController_ struct {
	HostPortAddress__   string
	Defaults_PtyProxy__ _Defaults_PtyProxy_
}

func New__WorkspaceController(
	api _NewApi__WorkspaceController_,
) *_WorkspaceController_ {
	newWorkspaceControllerResult := &_WorkspaceController_{
		Defaults_PtyProxy__:               api.Defaults_PtyProxy__,
		Mutex:                             _SYNC.Mutex{},
		PtyPool:                           make(map[uint32]*_WorkspacePty_),
		NextId_PtyProxy:                   0,
		WorkspaceNetwork:                  nil,
		MessageDebouncer_ResizePtys:       nil,
		LifecycleCoordinator_WorkspacePty: nil,
	}
	newWorkspaceControllerResult.WorkspaceNetwork = New__WorkspaceNetwork(_NewApi__WorkspaceNetwork_{
		HostPortAddress__:                       api.HostPortAddress__,
		OnConnected_PtyWebsocket__:              newWorkspaceControllerResult.HandleConnected_PtyWebsocket,
		OnTakeoverConnected_PtyWebsocket__:      newWorkspaceControllerResult.HandleTakeoverConnected_PtyWebsocket,
		OnDisconnected_PtyWebsocket__:           newWorkspaceControllerResult.HandleDisconnected_PtyWebsocket,
		OnTakeoverDisconnected_PtyWebsocket__:   newWorkspaceControllerResult.HandleTakeoverDisconnected_PtyWebsocket,
		OnPayload_BinaryMessage__PtyWebsocket__: newWorkspaceControllerResult.HandlePayload_BinaryMessage__PtyWebsocket,
	})
	newWorkspaceControllerResult.MessageDebouncer_ResizePtys = New__MessageDebouncer_ResizePtys(_NewApi__MessageDebouncer_ResizePtys_{
		DebounceTimeout: 50 * _TIME.Millisecond,
		OnResizePtys:    newWorkspaceControllerResult.HandleResizePtys_Debouncer,
	})
	newWorkspaceControllerResult.LifecycleCoordinator_WorkspacePty = New__LifecycleCoordinator_WorkspacePty(_NewApi__LifecycleCoordinator_WorkspacePty_{
		OnConnect_PtyWebsocket:    newWorkspaceControllerResult.HandleConnect_PtyWebsocket__Coordinator,
		OnDisconnect_PtyWebsocket: newWorkspaceControllerResult.HandleDisconnect_PtyWebsocket__Coordinator,
		OnSync_PtyPool:            newWorkspaceControllerResult.HandleSync_PtyPool__Coordinator,
		OnExit_PtyProxy:           newWorkspaceControllerResult.HandleExit_PtyProxy__Coordinator,
	})
	return newWorkspaceControllerResult
}

func (this *_WorkspaceController_) StartSession() error {
	go this.MessageDebouncer_ResizePtys.RunWorker()
	go this.LifecycleCoordinator_WorkspacePty.RunWorker()
	return this.WorkspaceNetwork.StartServer()
}

func (this *_WorkspaceController_) StopSession(
	shutdownDeadlineContext_HttpServer _CONTEXT.Context,
) error {
	this.MessageDebouncer_ResizePtys.WorkerCancel()
	this.LifecycleCoordinator_WorkspacePty.WorkerCancel()
	return this.WorkspaceNetwork.StopServer(shutdownDeadlineContext_HttpServer)
}

func (this *_WorkspaceController_) HandleConnected_PtyWebsocket(
	newId_WebsocketConnection uint64,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _Connect__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection: newId_WebsocketConnection,
	}
}

func (this *_WorkspaceController_) HandleTakeoverConnected_PtyWebsocket(
	newId_WebsocketConnection uint64,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _Connect__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection: newId_WebsocketConnection,
	}
}

func (this *_WorkspaceController_) HandleDisconnected_PtyWebsocket() {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _Disconnect__WorkspaceOrder_LifecycleCoordinator_{}
}

func (this *_WorkspaceController_) HandleTakeoverDisconnected_PtyWebsocket() {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _Disconnect__WorkspaceOrder_LifecycleCoordinator_{}
}

func (this *_WorkspaceController_) HandlePayload_BinaryMessage__PtyWebsocket(
	id_WebsocketConnection uint64,
	payload_binaryMessage []byte,
) {
	__decodeWebsocketPayload_binaryMessage(
		MAP__DECODE_PAYLOAD___PTY_MESSAGE__INGRESS,
		"pty websocket client",
		this,
		id_WebsocketConnection,
		payload_binaryMessage,
	)
}

func __decodeWebsocketPayload_binaryMessage[
	__Code__Ingress_Message__ ~uint16,
	__Ingress_Message__ interface {
		Execute(
			workspaceController *_WorkspaceController_,
			id_WebsocketConnection uint64,
		)
	},
](
	map__decodePayload_toMessage__ map[__Code__Ingress_Message__]func(payload_binaryMessage []byte) (__Ingress_Message__, error),
	messageSourceLabel_ErrorLog__ string,
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
	payload_binaryMessage []byte,
) {
	if len(payload_binaryMessage) < 2 {
		_FMT.Printf(
			"%s message error: payload too short: %d bytes\n",
			messageSourceLabel_ErrorLog__,
			len(payload_binaryMessage),
		)
		return
	}
	messageCode := __Code__Ingress_Message__(_BINARY.BigEndian.Uint16(payload_binaryMessage[0:2]))
	decodePayload_toMessage := map__decodePayload_toMessage__[messageCode]
	if nil == decodePayload_toMessage {
		_FMT.Printf(
			"%s message error: unrecognized message code: 0x%04x\n",
			messageSourceLabel_ErrorLog__,
			messageCode,
		)
		return
	}
	decodedMessage, decodeError := decodePayload_toMessage(payload_binaryMessage)
	if decodeError != nil {
		_FMT.Printf(
			"%s decode message error: %v\n",
			messageSourceLabel_ErrorLog__,
			decodeError,
		)
		return
	}
	decodedMessage.Execute(
		workspaceController,
		id_WebsocketConnection,
	)
}

func (this *_WorkspaceController_) HandleSpawned_Pty(
	spawnedPtyProxy *_PtyProxy_,
) {
	newWorkspacePty := &_WorkspacePty_{
		IsVisible_Client:          true,
		PtyProxy:                  spawnedPtyProxy,
		MaybeExitOutcome_PtyProxy: nil,
	}
	this.Mutex.Lock()
	this.PtyPool[spawnedPtyProxy.Id] = newWorkspacePty
	this.Mutex.Unlock()
	Emit__PtyMessage_Egress(
		this.WorkspaceNetwork.WebsocketController_Pty,
		this.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection,
		_SpawnPtyStatus__PtyMessage_Egress_{
			Status:      SUCCESS__Status_SpawnPty,
			Id_PtyProxy: spawnedPtyProxy.Id,
		},
	)
}

func (this *_WorkspaceController_) HandleSpawnFailed_Pty(
	failedId_PtyProxy uint32,
) {
	Emit__PtyMessage_Egress(
		this.WorkspaceNetwork.WebsocketController_Pty,
		this.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection,
		_SpawnPtyStatus__PtyMessage_Egress_{
			Status:      FAILURE__Status_SpawnPty,
			Id_PtyProxy: failedId_PtyProxy,
		},
	)
}

func (this *_WorkspaceController_) HandleOutput_Pty(
	sourcePtyProxy *_PtyProxy_,
	outputData_PtyProxy []byte,
) {
	Emit__PtyMessage_Egress(
		this.WorkspaceNetwork.WebsocketController_Pty,
		this.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection,
		_PtyOutput__PtyMessage_Egress_{
			Id_PtyProxy: sourcePtyProxy.Id,
			OutputData:  outputData_PtyProxy,
		},
	)
}

func (this *_WorkspaceController_) HandleExited_Eio_Success__Pty(
	exitedPtyProxy *_PtyProxy_,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy:          exitedPtyProxy.Id,
		ExitOutcome_PtyProxy: _Success__ExitOutcome_PtyProxy_{},
	}
}

func (this *_WorkspaceController_) HandleExited_Eio_Failure__Pty(
	exitedPtyProxy *_PtyProxy_,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: exitedPtyProxy.Id,
		ExitOutcome_PtyProxy: _Failure__ExitOutcome_PtyProxy_{
			ExitCode_PtyProcess: exitedPtyProxy.PtyCommand.ProcessState.ExitCode(),
		},
	}
}

func (this *_WorkspaceController_) HandleExited_Eio_Killed__Pty(
	exitedPtyProxy *_PtyProxy_,
) {
	processWaitStatus_exitedPtyProxy := exitedPtyProxy.PtyCommand.ProcessState.Sys().(_SYSCALL.WaitStatus)
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: exitedPtyProxy.Id,
		ExitOutcome_PtyProxy: _Killed__ExitOutcome_PtyProxy_{
			ExitSignal_PtyProcess: int(processWaitStatus_exitedPtyProxy.Signal()),
		},
	}
}

func (this *_WorkspaceController_) HandleExited_Closed__Pty(
	exitedPtyProxy *_PtyProxy_,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy:          exitedPtyProxy.Id,
		ExitOutcome_PtyProxy: _Closed__ExitOutcome_PtyProxy_{},
	}
}

func (this *_WorkspaceController_) HandleExited_SystemError__Pty(
	exitedPtyProxy *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: exitedPtyProxy.Id,
		ExitOutcome_PtyProxy: _SystemError__ExitOutcome_PtyProxy_{
			SystemError_PtyDevice: exitSignal_PtyReader,
		},
	}
}

func (this *_WorkspaceController_) HandleResizePtys_Debouncer(
	pendingOrders_ResizePtys map[uint32]_ResizePtyOrder_ResizePtys_,
) {
	for _, somePendingOrder := range pendingOrders_ResizePtys {
		this.Mutex.Lock()
		targetWorkspacePty := this.PtyPool[somePendingOrder.Id_PtyProxy]
		this.Mutex.Unlock()
		if targetWorkspacePty != nil {
			_ = targetWorkspacePty.PtyProxy.Resize(
				somePendingOrder.ColumnCount_PtyTerminal,
				somePendingOrder.RowCount_PtyTerminal,
			)
		}
	}
}

func (this *_WorkspaceController_) HandleConnect_PtyWebsocket__Coordinator(
	originalId_WebsocketConnection uint64,
) {
	this.Mutex.Lock()
	ptyBulletinsResult := make([]_PtyBulletin_WorkspaceManifest_, 0, len(this.PtyPool))
	for _, someWorkspacePty := range this.PtyPool {
		ptyBulletinsResult = append(
			ptyBulletinsResult,
			_PtyBulletin_WorkspaceManifest_{
				Id_PtyProxy:               someWorkspacePty.PtyProxy.Id,
				MaybeExitOutcome_PtyProxy: someWorkspacePty.MaybeExitOutcome_PtyProxy,
				IsVisible_Client:          someWorkspacePty.IsVisible_Client,
			},
		)
	}
	this.Mutex.Unlock()
	Emit__PtyMessage_Egress(
		this.WorkspaceNetwork.WebsocketController_Pty,
		originalId_WebsocketConnection,
		_WorkspaceManifest__PtyMessage_Egress_{
			PtyBulletins: ptyBulletinsResult,
		},
	)
}

func (this *_WorkspaceController_) HandleDisconnect_PtyWebsocket__Coordinator() {
	this.Mutex.Lock()
	ptyPoolClone := _MAPS.Clone(this.PtyPool)
	this.Mutex.Unlock()
	for _, someWorkspacePty := range ptyPoolClone {
		switch someWorkspacePty.PtyProxy.Mode {
		case RUNNING_LIVE__Mode_PtyProxy:
			someWorkspacePty.PtyProxy.TransitionMode_LiveToPreSnapshot()
		case RUNNING_POST_SNAPSHOT__Mode_PtyProxy:
			someWorkspacePty.PtyProxy.TransitionMode_PostSnapshotToPreSnapshot()
		case RUNNING_PRE_SNAPSHOT__Mode_PtyProxy:
		case EXITED__Mode_PtyProxy:
		default:
			// SPAWNING__Mode_PtyProxy == someWorkspacePty.PtyProxy.Mode
			_FMT.Println("invalid path: HandleDisconnect_PtyWebsocket__Coordinator")
		}
	}
}

func (this *_WorkspaceController_) HandleSync_PtyPool__Coordinator(
	originalId_WebsocketConnection uint64,
	message_SyncWorkspace _SyncWorkspace__PtyMessage_Ingress_,
) {
	this.Mutex.Lock()
	ptyPoolClone := _MAPS.Clone(this.PtyPool)
	this.Mutex.Unlock()
	for _, someWorkspacePty := range ptyPoolClone {
		maybeSyncPtyOrder := message_SyncWorkspace.SyncPtyOrders[someWorkspacePty.PtyProxy.Id]
		this.Mutex.Lock()
		wasVisible := someWorkspacePty.IsVisible_Client
		if maybeSyncPtyOrder != nil {
			someWorkspacePty.IsVisible_Client = true
		} else {
			someWorkspacePty.IsVisible_Client = false
		}
		this.Mutex.Unlock()
		if maybeSyncPtyOrder != nil && wasVisible {
			_ = someWorkspacePty.PtyProxy.Resize(
				maybeSyncPtyOrder.ColumnCount_PtyTerminal,
				maybeSyncPtyOrder.RowCount_PtyTerminal,
			)
		} else if maybeSyncPtyOrder != nil && false == wasVisible && someWorkspacePty.MaybeExitOutcome_PtyProxy != nil {
			_ = someWorkspacePty.PtyProxy.Resize(
				maybeSyncPtyOrder.ColumnCount_PtyTerminal,
				maybeSyncPtyOrder.RowCount_PtyTerminal,
			)
			__emitSnapshot_SyncPty(
				someWorkspacePty.PtyProxy.EmitSnapshot_Exited,
				this.WorkspaceNetwork.WebsocketController_Pty,
				originalId_WebsocketConnection,
				someWorkspacePty.PtyProxy.Id,
				maybeSyncPtyOrder.ColumnCount_PtyTerminal,
				maybeSyncPtyOrder.RowCount_PtyTerminal,
			)
		} else if maybeSyncPtyOrder != nil && false == wasVisible && nil == someWorkspacePty.MaybeExitOutcome_PtyProxy {
			_ = someWorkspacePty.PtyProxy.Resize(
				maybeSyncPtyOrder.ColumnCount_PtyTerminal,
				maybeSyncPtyOrder.RowCount_PtyTerminal,
			)
			__emitSnapshot_SyncPty(
				someWorkspacePty.PtyProxy.TransitionMode_PreToPostSnapshot,
				this.WorkspaceNetwork.WebsocketController_Pty,
				originalId_WebsocketConnection,
				someWorkspacePty.PtyProxy.Id,
				maybeSyncPtyOrder.ColumnCount_PtyTerminal,
				maybeSyncPtyOrder.RowCount_PtyTerminal,
			)
			someWorkspacePty.PtyProxy.TransitionMode_PostSnapshotToLive()
		} else if nil == maybeSyncPtyOrder && wasVisible {
			someWorkspacePty.PtyProxy.TransitionMode_LiveToPreSnapshot()
		} else if nil == maybeSyncPtyOrder && false == wasVisible {
		} else {
			_FMT.Println("invalid path: HandleSync_PtyPool__Coordinator")
		}
	}
}

func __emitSnapshot_SyncPty(
	onEmitSnapshot__ func(),
	websocketController *_WebsocketController_,
	originalId_WebsocketConnection uint64,
	targetId_PtyProxy uint32,
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) {
	Emit__PtyMessage_Egress(
		websocketController,
		originalId_WebsocketConnection,
		_SyncPtyStart__PtyMessage_Egress_{
			Id_PtyProxy:             targetId_PtyProxy,
			ColumnCount_PtyTerminal: columnCount_PtyTerminal,
			RowCount_PtyTerminal:    rowCount_PtyTerminal,
		},
	)
	onEmitSnapshot__()
	Emit__PtyMessage_Egress(
		websocketController,
		originalId_WebsocketConnection,
		_SyncPtyComplete__PtyMessage_Egress_{
			Id_PtyProxy: targetId_PtyProxy,
		},
	)
}

func (this *_WorkspaceController_) HandleExit_PtyProxy__Coordinator(
	id_PtyProxy uint32,
	exitOutcome_PtyProxy _ExitOutcome_PtyProxy_,
) {
	this.Mutex.Lock()
	targetWorkspacePty := this.PtyPool[id_PtyProxy]
	targetWorkspacePty.MaybeExitOutcome_PtyProxy = exitOutcome_PtyProxy
	this.Mutex.Unlock()
	targetWorkspacePty.PtyProxy.TransitionMode_ToExited()
	targetWorkspacePty.PtyProxy.PtyWriter.WorkerCancel()
	Emit__PtyMessage_Egress(
		this.WorkspaceNetwork.WebsocketController_Pty,
		this.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection,
		_PtyExit__PtyMessage_Egress_{
			Id_PtyProxy:          id_PtyProxy,
			ExitOutcome_PtyProxy: exitOutcome_PtyProxy,
		},
	)
}
