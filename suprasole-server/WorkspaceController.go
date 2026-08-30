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

type _PtyProxyDefaults_ struct {
	ScrollbackLineCount_PtyTerminal       int
	StagingBufferSize_PtyReader           int
	PostSnapshotBufferSize_PtyProxy       int
	QueueBufferSize_InputOrder__PtyWriter int
}

type _WorkspaceController_ struct {
	Mutex                             _SYNC.Mutex
	WorkspaceNetwork                  *_WorkspaceNetwork_
	PtyPool                           map[uint32]*_WorkspacePty_
	NextId_PtyProxy                   uint32
	PtyProxyDefaults                  _PtyProxyDefaults_
	MessageDebouncer_ResizePtys       *_MessageDebouncer_ResizePtys_
	LifecycleCoordinator_WorkspacePty *_LifecycleCoordinator_WorkspacePty_
}

type _NewApi__WorkspaceController_ struct {
	HostPortAddress  string
	PtyProxyDefaults _PtyProxyDefaults_
}

func New__WorkspaceController(
	api _NewApi__WorkspaceController_,
) *_WorkspaceController_ {
	newWorkspaceControllerResult := &_WorkspaceController_{
		Mutex:                             _SYNC.Mutex{},
		PtyPool:                           make(map[uint32]*_WorkspacePty_),
		NextId_PtyProxy:                   1,
		PtyProxyDefaults:                  api.PtyProxyDefaults,
		MessageDebouncer_ResizePtys:       nil,
		LifecycleCoordinator_WorkspacePty: nil,
		WorkspaceNetwork:                  nil,
	}
	newWorkspaceControllerResult.MessageDebouncer_ResizePtys = New__MessageDebouncer_ResizePtys(_NewApi__MessageDebouncer_ResizePtys_{
		DebounceTimeout: 50 * _TIME.Millisecond,
		OnResizePtys:    newWorkspaceControllerResult.HandleResizePtys_Debouncer,
	})
	newWorkspaceControllerResult.LifecycleCoordinator_WorkspacePty = New__LifecycleCoordinator_WorkspacePty(_NewApi__LifecycleCoordinator_WorkspacePty_{
		OnConnect_PtyWebsocket:    newWorkspaceControllerResult.HandleConnect_Coordinator,
		OnDisconnect_PtyWebsocket: newWorkspaceControllerResult.HandleDisconnect_Coordinator,
		OnSync_PtyPool:            newWorkspaceControllerResult.HandleSync_Coordinator,
		OnExit_PtyProxy:           newWorkspaceControllerResult.HandleExitPty_Coordinator,
	})
	newWorkspaceControllerResult.WorkspaceNetwork = New__WorkspaceNetwork(_NewApi__WorkspaceNetwork_{
		HostPortAddress:                     api.HostPortAddress,
		OnConnected_PtyWebsocket:            newWorkspaceControllerResult.HandleConnected_PtyWebsocket,
		OnTakeoverConnected_PtyWebsocket:    newWorkspaceControllerResult.HandleTakeoverConnected_PtyWebsocket,
		OnDisconnected_PtyWebsocket:         newWorkspaceControllerResult.HandleDisconnected_PtyWebsocket,
		OnTakeoverDisconnected_PtyWebsocket: newWorkspaceControllerResult.HandleTakeoverDisconnected_PtyWebsocket,
		OnBinaryMessageFrame_PtyWebsocket:   newWorkspaceControllerResult.HandleBinaryMessageFrame_PtyWebsocket,
	})
	return newWorkspaceControllerResult
}

func (this *_WorkspaceController_) StartSession() error {
	go this.MessageDebouncer_ResizePtys.RunWorker()
	go this.LifecycleCoordinator_WorkspacePty.RunWorker()
	return this.WorkspaceNetwork.StartServer()
}

func (this *_WorkspaceController_) StopSession(
	shutdownContext _CONTEXT.Context,
) error {
	this.MessageDebouncer_ResizePtys.WorkerCancel()
	this.LifecycleCoordinator_WorkspacePty.WorkerCancel()
	return this.WorkspaceNetwork.StopServer(shutdownContext)
}

func (this *_WorkspaceController_) HandleConnected_PtyWebsocket(
	id_WebsocketConnection uint64,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _Connect__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection: id_WebsocketConnection,
	}
}

func (this *_WorkspaceController_) HandleTakeoverConnected_PtyWebsocket(
	id_WebsocketConnection uint64,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _Connect__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection: id_WebsocketConnection,
	}
}

func (this *_WorkspaceController_) HandleDisconnected_PtyWebsocket(
	id_WebsocketConnection uint64,
	readMessageError error,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _Disconnect__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection: id_WebsocketConnection,
	}
}

func (this *_WorkspaceController_) HandleTakeoverDisconnected_PtyWebsocket(
	id_WebsocketConnection uint64,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _Disconnect__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection: id_WebsocketConnection,
	}
}

func (this *_WorkspaceController_) HandleBinaryMessageFrame_PtyWebsocket(
	id_WebsocketConnection uint64,
	binaryMessageFrame []byte,
) {
	__decodeBinaryMessageFrame(
		this,
		id_WebsocketConnection,
		"pty websocket client",
		DECODE_MESSAGE_MAP__PTY_MESSAGE_INGRESS,
		binaryMessageFrame,
	)
}

func __decodeBinaryMessageFrame[
	MessageCodeType ~uint16,
	MessageType interface {
		Execute(workspaceController *_WorkspaceController_, id_WebsocketConnection uint64)
	},
](
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
	websocketContextLabel string,
	map_decodeWebsocketMessage map[MessageCodeType]func([]byte) (MessageType, error),
	binaryMessageFrame []byte,
) {
	if len(binaryMessageFrame) < 2 {
		_FMT.Printf(
			"%s message error: frame too short: %d bytes\n",
			websocketContextLabel,
			len(binaryMessageFrame),
		)
		return
	}
	messageCode := MessageCodeType(_BINARY.BigEndian.Uint16(binaryMessageFrame[0:2]))
	decodeWebsocketMessage := map_decodeWebsocketMessage[messageCode]
	if nil == decodeWebsocketMessage {
		_FMT.Printf(
			"%s message error: unrecognized message code: 0x%04x\n",
			websocketContextLabel,
			messageCode,
		)
		return
	}
	websocketMessage, decodeError_websocketMessage := decodeWebsocketMessage(binaryMessageFrame)
	if decodeError_websocketMessage != nil {
		_FMT.Printf(
			"%s message error: decode failure: %v\n",
			websocketContextLabel,
			decodeError_websocketMessage,
		)
		return
	}
	websocketMessage.Execute(
		workspaceController,
		id_WebsocketConnection,
	)
}

func (this *_WorkspaceController_) HandleSpawned_Pty(
	ptyProxy *_PtyProxy_,
) {
	workspacePty := &_WorkspacePty_{
		PtyProxy:         ptyProxy,
		IsVisible:        true,
		MaybeExitOutcome: nil,
	}
	this.Mutex.Lock()
	this.PtyPool[ptyProxy.Id] = workspacePty
	this.Mutex.Unlock()
	_SpawnPtyStatus_Message_{
		Id_PtyProxy: ptyProxy.Id,
		Status:      SUCCESS__Status_SpawnPty,
	}.Emit(
		this.WorkspaceNetwork.WebsocketController_Pty,
		this.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection,
	)
}

func (this *_WorkspaceController_) HandleSpawnFailed_Pty(
	id_PtyProxy uint32,
) {
	_SpawnPtyStatus_Message_{
		Id_PtyProxy: id_PtyProxy,
		Status:      FAILURE__Status_SpawnPty,
	}.Emit(
		this.WorkspaceNetwork.WebsocketController_Pty,
		this.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection,
	)
}

func (this *_WorkspaceController_) HandleOutput_Pty(
	ptyProxy *_PtyProxy_,
	outputData []byte,
) {
	_PtyOutput_Message_{
		Id_PtyProxy: ptyProxy.Id,
		OutputData:  outputData,
	}.Emit(
		this.WorkspaceNetwork.WebsocketController_Pty,
		this.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection,
	)
}

func (this *_WorkspaceController_) HandleExited_Eio_Success__Pty(
	ptyProxy *_PtyProxy_,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: ptyProxy.Id,
		ExitOutcome: _Success__ExitOutcome_PtyProxy_{},
	}
}

func (this *_WorkspaceController_) HandleExited_Eio_Failure__Pty(
	ptyProxy *_PtyProxy_,
) {
	processState := ptyProxy.PtyCommand.ProcessState
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: ptyProxy.Id,
		ExitOutcome: _Failure__ExitOutcome_PtyProxy_{
			ExitCode: processState.ExitCode(),
		},
	}
}

func (this *_WorkspaceController_) HandleExited_Eio_Killed__Pty(
	ptyProxy *_PtyProxy_,
) {
	processWaitStatus := ptyProxy.PtyCommand.ProcessState.Sys().(_SYSCALL.WaitStatus)
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: ptyProxy.Id,
		ExitOutcome: _Killed__ExitOutcome_PtyProxy_{
			ExitSignal: int(processWaitStatus.Signal()),
		},
	}
}

func (this *_WorkspaceController_) HandleExited_Closed__Pty(
	ptyProxy *_PtyProxy_,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: ptyProxy.Id,
		ExitOutcome: _Closed__ExitOutcome_PtyProxy_{},
	}
}

func (this *_WorkspaceController_) HandleExited_SystemError__Pty(
	ptyProxy *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: ptyProxy.Id,
		ExitOutcome: _SystemError__ExitOutcome_PtyProxy_{
			SystemError: exitSignal_PtyReader,
		},
	}
}

func (this *_WorkspaceController_) HandleResizePtys_Debouncer(
	pendingEntries_ResizePtys map[uint32]_ResizePtys_Entry_,
) {
	for _, somePendingEntry := range pendingEntries_ResizePtys {
		this.Mutex.Lock()
		targetWorkspacePty := this.PtyPool[somePendingEntry.Id_PtyProxy]
		this.Mutex.Unlock()
		if targetWorkspacePty != nil {
			_ = targetWorkspacePty.PtyProxy.Resize(
				somePendingEntry.ColumnCount_PtyTerminal,
				somePendingEntry.RowCount_PtyTerminal,
			)
		}
	}
}

func (this *_WorkspaceController_) HandleConnect_Coordinator(
	id_WebsocketConnection uint64,
) {
	this.Mutex.Lock()
	ptyBulletinsResult := make(
		[]_PtyBulletin_WorkspaceManifest_,
		0,
		len(this.PtyPool),
	)
	for id_PtyProxy, workspacePty := range this.PtyPool {
		ptyBulletinsResult = append(
			ptyBulletinsResult,
			_PtyBulletin_WorkspaceManifest_{
				Id_PtyProxy:      id_PtyProxy,
				IsVisible:        workspacePty.IsVisible,
				MaybeExitOutcome: workspacePty.MaybeExitOutcome,
			},
		)
	}
	this.Mutex.Unlock()
	_WorkspaceManifest_Message_{
		PtyBulletins: ptyBulletinsResult,
	}.Emit(
		this.WorkspaceNetwork.WebsocketController_Pty,
		id_WebsocketConnection,
	)
}

func (this *_WorkspaceController_) HandleDisconnect_Coordinator(
	id_WebsocketConnection uint64,
) {
	this.Mutex.Lock()
	ptyPoolClone := _MAPS.Clone(this.PtyPool)
	this.Mutex.Unlock()
	for _, workspacePty := range ptyPoolClone {
		workspacePty.PtyProxy.Mutex.Lock()
		mode_PtyProxy := workspacePty.PtyProxy.Mode
		workspacePty.PtyProxy.Mutex.Unlock()
		switch mode_PtyProxy {
		case RUNNING_LIVE__Mode_PtyProxy:
			workspacePty.PtyProxy.TransitionMode_LiveToPreSnapshot()
		case RUNNING_POST_SNAPSHOT__Mode_PtyProxy:
			workspacePty.PtyProxy.TransitionMode_PostSnapshotToPreSnapshot()
		case RUNNING_PRE_SNAPSHOT__Mode_PtyProxy:
		case EXITED__Mode_PtyProxy:
		default:
			// SPAWNING__Mode_PtyProxy == mode_PtyProxy
			_FMT.Println("invalid path: HandleDisconnect_Coordinator")
		}
	}
}

func __emitSnapshot_SyncPty(
	onEmitSnapshot func(),
	websocketController *_WebsocketController_,
	id_WebsocketConnection uint64,
	id_PtyProxy uint32,
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) {
	_SyncPtyStart_Message_{
		Id_PtyProxy:             id_PtyProxy,
		ColumnCount_PtyTerminal: columnCount_PtyTerminal,
		RowCount_PtyTerminal:    rowCount_PtyTerminal,
	}.Emit(
		websocketController,
		id_WebsocketConnection,
	)
	onEmitSnapshot()
	_SyncPtyComplete_Message_{
		Id_PtyProxy: id_PtyProxy,
	}.Emit(
		websocketController,
		id_WebsocketConnection,
	)
}

func (this *_WorkspaceController_) HandleSync_Coordinator(
	originalId_WebsocketConnection uint64,
	message_SyncWorkspace _SyncWorkspace_Message_,
) {
	this.Mutex.Lock()
	ptyPoolClone := _MAPS.Clone(this.PtyPool)
	this.Mutex.Unlock()
	for id_PtyProxy, targetWorkspacePty := range ptyPoolClone {
		maybeSyncPtyOrder_targetWorkspacePty := message_SyncWorkspace.SyncPtyOrders[id_PtyProxy]
		this.Mutex.Lock()
		isWasVisible_targetWorkspacePty := targetWorkspacePty.IsVisible
		isWasExited_targetWorkspacePty := targetWorkspacePty.MaybeExitOutcome != nil
		if maybeSyncPtyOrder_targetWorkspacePty != nil {
			targetWorkspacePty.IsVisible = true
		} else {
			targetWorkspacePty.IsVisible = false
		}
		this.Mutex.Unlock()
		if maybeSyncPtyOrder_targetWorkspacePty != nil && isWasVisible_targetWorkspacePty {
			_ = targetWorkspacePty.PtyProxy.Resize(
				maybeSyncPtyOrder_targetWorkspacePty.ColumnCount_PtyTerminal,
				maybeSyncPtyOrder_targetWorkspacePty.RowCount_PtyTerminal,
			)
		} else if maybeSyncPtyOrder_targetWorkspacePty != nil && false == isWasVisible_targetWorkspacePty && isWasExited_targetWorkspacePty {
			_ = targetWorkspacePty.PtyProxy.Resize(
				maybeSyncPtyOrder_targetWorkspacePty.ColumnCount_PtyTerminal,
				maybeSyncPtyOrder_targetWorkspacePty.RowCount_PtyTerminal,
			)
			__emitSnapshot_SyncPty(
				targetWorkspacePty.PtyProxy.EmitSnapshot_Exited,
				this.WorkspaceNetwork.WebsocketController_Pty,
				originalId_WebsocketConnection,
				targetWorkspacePty.PtyProxy.Id,
				maybeSyncPtyOrder_targetWorkspacePty.ColumnCount_PtyTerminal,
				maybeSyncPtyOrder_targetWorkspacePty.RowCount_PtyTerminal,
			)
		} else if maybeSyncPtyOrder_targetWorkspacePty != nil && false == isWasVisible_targetWorkspacePty && false == isWasExited_targetWorkspacePty {
			_ = targetWorkspacePty.PtyProxy.Resize(
				maybeSyncPtyOrder_targetWorkspacePty.ColumnCount_PtyTerminal,
				maybeSyncPtyOrder_targetWorkspacePty.RowCount_PtyTerminal,
			)
			__emitSnapshot_SyncPty(
				targetWorkspacePty.PtyProxy.TransitionMode_PreToPostSnapshot,
				this.WorkspaceNetwork.WebsocketController_Pty,
				originalId_WebsocketConnection,
				targetWorkspacePty.PtyProxy.Id,
				maybeSyncPtyOrder_targetWorkspacePty.ColumnCount_PtyTerminal,
				maybeSyncPtyOrder_targetWorkspacePty.RowCount_PtyTerminal,
			)
			targetWorkspacePty.PtyProxy.TransitionMode_PostSnapshotToLive()
		} else if nil == maybeSyncPtyOrder_targetWorkspacePty && isWasVisible_targetWorkspacePty {
			targetWorkspacePty.PtyProxy.TransitionMode_LiveToPreSnapshot()
		} else if nil == maybeSyncPtyOrder_targetWorkspacePty && false == isWasVisible_targetWorkspacePty {
		} else {
			_FMT.Println("invalid path: HandleSync_Coordinator")
		}
	}
}

func (this *_WorkspaceController_) HandleExitPty_Coordinator(
	id_PtyProxy uint32,
	exitOutcome _ExitOutcome_PtyProxy_,
) {
	this.Mutex.Lock()
	this.PtyPool[id_PtyProxy].MaybeExitOutcome = exitOutcome
	this.Mutex.Unlock()
	_PtyExit_Message_{
		Id_PtyProxy: id_PtyProxy,
		ExitOutcome: exitOutcome,
	}.Emit(
		this.WorkspaceNetwork.WebsocketController_Pty,
		this.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection,
	)
}
