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
	Mutex                          _SYNC.Mutex
	WorkspaceNetwork               *_WorkspaceNetwork_
	PtyPool                        map[uint32]*_WorkspacePty_
	NextId_PtyProxy                uint32
	PtyProxyDefaults               _PtyProxyDefaults_
	MessageDebouncer_ResizePtys    *_MessageDebouncer_ResizePtys_
	MessageCoalescer_SyncWorkspace *_MessageCoalescer_SyncWorkspace_
}

type _NewApi__WorkspaceController_ struct {
	HostPortAddress  string
	PtyProxyDefaults _PtyProxyDefaults_
}

func New__WorkspaceController(
	api _NewApi__WorkspaceController_,
) *_WorkspaceController_ {
	newWorkspaceControllerResult := &_WorkspaceController_{
		Mutex:                          _SYNC.Mutex{},
		PtyPool:                        make(map[uint32]*_WorkspacePty_),
		NextId_PtyProxy:                1,
		PtyProxyDefaults:               api.PtyProxyDefaults,
		MessageDebouncer_ResizePtys:    nil,
		MessageCoalescer_SyncWorkspace: nil,
		WorkspaceNetwork:               nil,
	}
	newWorkspaceControllerResult.MessageDebouncer_ResizePtys = New__MessageDebouncer_ResizePtys(_NewApi__MessageDebouncer_ResizePtys_{
		DebounceTimeout: 50 * _TIME.Millisecond,
		OnResizePtys:    newWorkspaceControllerResult.HandleResizePtys_Debouncer,
	})
	newWorkspaceControllerResult.MessageCoalescer_SyncWorkspace = New__MessageCoalescer_SyncWorkspace(_NewApi__MessageCoalescer_SyncWorkspace_{
		OnSyncWorkspace: newWorkspaceControllerResult.HandleSyncWorkspace_Coalescer,
	})
	newWorkspaceControllerResult.WorkspaceNetwork = New__WorkspaceNetwork(_NewApi__WorkspaceNetwork_{
		HostPortAddress:                     api.HostPortAddress,
		OnConnected_PtyWebsocket:            newWorkspaceControllerResult.HandleConnected_PtyWebsocket,
		OnTakeoverConnected_PtyWebsocket:    newWorkspaceControllerResult.HandleConnected_PtyWebsocket,
		OnDisconnected_PtyWebsocket:         newWorkspaceControllerResult.HandleDisconnected_PtyWebsocket,
		OnTakeoverDisconnected_PtyWebsocket: newWorkspaceControllerResult.HandleTakeoverDisconnected_PtyWebsocket,
		OnBinaryMessageFrame_PtyWebsocket:   newWorkspaceControllerResult.HandleBinaryMessageFrame_PtyWebsocket,
	})
	return newWorkspaceControllerResult
}

func (this *_WorkspaceController_) StartSession() error {
	go this.MessageDebouncer_ResizePtys.RunWorker()
	go this.MessageCoalescer_SyncWorkspace.RunWorker()
	return this.WorkspaceNetwork.StartServer()
}

func (this *_WorkspaceController_) StopSession(
	shutdownContext _CONTEXT.Context,
) error {
	this.MessageDebouncer_ResizePtys.WorkerCancel()
	this.MessageCoalescer_SyncWorkspace.WorkerCancel()
	return this.WorkspaceNetwork.StopServer(shutdownContext)
}

func (this *_WorkspaceController_) HandleConnected_PtyWebsocket() {
	this.Mutex.Lock()
	ptyBulletinsResult := make(
		[]_PtyBulletin_WorkspaceManifest_,
		0,
		len(this.PtyPool),
	)
	for idPtyProxy, workspacePty := range this.PtyPool {
		ptyBulletinsResult = append(
			ptyBulletinsResult,
			_PtyBulletin_WorkspaceManifest_{
				Id_PtyProxy:      idPtyProxy,
				IsVisible:        workspacePty.IsVisible,
				MaybeExitOutcome: workspacePty.MaybeExitOutcome,
			},
		)
	}
	this.Mutex.Unlock()
	_WorkspaceManifest_Message_{
		PtyBulletins: ptyBulletinsResult,
	}.Emit(this.WorkspaceNetwork.WebsocketController_Pty)
}

func (this *_WorkspaceController_) HandleDisconnected_PtyWebsocket(
	readMessageError error,
) {
}

func (this *_WorkspaceController_) HandleTakeoverDisconnected_PtyWebsocket() {
}

func (this *_WorkspaceController_) HandleBinaryMessageFrame_PtyWebsocket(
	binaryMessageFrame []byte,
) {
	__decodeBinaryMessageFrame(
		this,
		"pty websocket client",
		DECODE_MESSAGE_MAP__PTY_MESSAGE_INGRESS,
		binaryMessageFrame,
	)
}

func __decodeBinaryMessageFrame[
	MessageCodeType ~uint16,
	MessageType interface {
		Execute(workspaceController *_WorkspaceController_)
	},
](
	workspaceController *_WorkspaceController_,
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
	websocketMessage.Execute(workspaceController)
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
	}.Emit(this.WorkspaceNetwork.WebsocketController_Pty)
}

func (this *_WorkspaceController_) HandleSpawnFailed_Pty(
	ptyId uint32,
) {
	_SpawnPtyStatus_Message_{
		Id_PtyProxy: ptyId,
		Status:      FAILURE__Status_SpawnPty,
	}.Emit(this.WorkspaceNetwork.WebsocketController_Pty)
}

func (this *_WorkspaceController_) HandleOutput_Pty(
	ptyProxy *_PtyProxy_,
	outputData []byte,
) {
	_PtyOutput_Message_{
		Id_PtyProxy: ptyProxy.Id,
		OutputData:  outputData,
	}.Emit(this.WorkspaceNetwork.WebsocketController_Pty)
}

func (this *_WorkspaceController_) HandleExited_Eio_Success__Pty(
	ptyProxy *_PtyProxy_,
) {
	this.HandleProcessExit(
		ptyProxy.Id,
		_Success__ExitOutcome_PtyProxy_{},
	)
}

func (this *_WorkspaceController_) HandleExited_Eio_Failure__Pty(
	ptyProxy *_PtyProxy_,
) {
	processState := ptyProxy.PtyCommand.ProcessState
	this.HandleProcessExit(
		ptyProxy.Id,
		_Failure__ExitOutcome_PtyProxy_{
			ExitCode: processState.ExitCode(),
		},
	)
}

func (this *_WorkspaceController_) HandleExited_Eio_Killed__Pty(
	ptyProxy *_PtyProxy_,
) {
	processWaitStatus := ptyProxy.PtyCommand.ProcessState.Sys().(_SYSCALL.WaitStatus)
	this.HandleProcessExit(
		ptyProxy.Id,
		_Killed__ExitOutcome_PtyProxy_{
			ExitSignal: int(processWaitStatus.Signal()),
		},
	)
}

func (this *_WorkspaceController_) HandleExited_Closed__Pty(
	ptyProxy *_PtyProxy_,
) {
	this.HandleProcessExit(
		ptyProxy.Id,
		_Closed__ExitOutcome_PtyProxy_{},
	)
}

func (this *_WorkspaceController_) HandleExited_SystemError__Pty(
	ptyProxy *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	this.HandleProcessExit(
		ptyProxy.Id,
		_SystemError__ExitOutcome_PtyProxy_{
			SystemError: exitSignal_PtyReader,
		},
	)
}

func (this *_WorkspaceController_) HandleProcessExit(
	ptyId uint32,
	exitOutcome _ExitOutcome_PtyProxy_,
) {
	this.Mutex.Lock()
	targetWorkspacePty := this.PtyPool[ptyId]
	if targetWorkspacePty != nil {
		targetWorkspacePty.MaybeExitOutcome = exitOutcome
	}
	this.Mutex.Unlock()
	if nil == targetWorkspacePty {
		return
	}
	_PtyExit_Message_{
		Id_PtyProxy: ptyId,
		ExitOutcome: exitOutcome,
	}.Emit(this.WorkspaceNetwork.WebsocketController_Pty)
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

func (this *_WorkspaceController_) HandleSyncWorkspace_Coalescer(
	syncWorkspaceMessage _SyncWorkspace_Message_,
) {
	this.Mutex.Lock()
	ptyPoolClone := _MAPS.Clone(this.PtyPool)
	this.Mutex.Unlock()
	for idPtyProxy, targetWorkspacePty := range ptyPoolClone {
		someSyncPtyOrder := syncWorkspaceMessage.SyncPtyOrders[idPtyProxy]
		this.Mutex.Lock()
		isWasVisible := targetWorkspacePty.IsVisible
		if someSyncPtyOrder != nil {
			targetWorkspacePty.IsVisible = true
		} else {
			targetWorkspacePty.IsVisible = false
		}
		this.Mutex.Unlock()
		if someSyncPtyOrder != nil && isWasVisible {
			_ = targetWorkspacePty.PtyProxy.Resize(
				someSyncPtyOrder.ColumnCount_PtyTerminal,
				someSyncPtyOrder.RowCount_PtyTerminal,
			)
		} else if someSyncPtyOrder != nil && false == isWasVisible {
			_ = targetWorkspacePty.PtyProxy.Resize(
				someSyncPtyOrder.ColumnCount_PtyTerminal,
				someSyncPtyOrder.RowCount_PtyTerminal,
			)
			_SyncPtyStart_Message_{
				Id_PtyProxy:             targetWorkspacePty.PtyProxy.Id,
				ColumnCount_PtyTerminal: someSyncPtyOrder.ColumnCount_PtyTerminal,
				RowCount_PtyTerminal:    someSyncPtyOrder.RowCount_PtyTerminal,
			}.Emit(this.WorkspaceNetwork.WebsocketController_Pty)
			targetWorkspacePty.PtyProxy.TransitionMode_PreToPostSnapshot()
			_SyncPtyComplete_Message_{
				Id_PtyProxy: targetWorkspacePty.PtyProxy.Id,
			}.Emit(this.WorkspaceNetwork.WebsocketController_Pty)
			targetWorkspacePty.PtyProxy.TransitionMode_PostSnapshotToLive()
		} else if nil == someSyncPtyOrder && isWasVisible {
			targetWorkspacePty.PtyProxy.TransitionMode_LiveToPreSnapshot()
		} else if nil == someSyncPtyOrder && false == isWasVisible {
		} else {
			_FMT.Println("invalid path: HandleSyncWorkspace_Coalescer")
		}
	}
}
