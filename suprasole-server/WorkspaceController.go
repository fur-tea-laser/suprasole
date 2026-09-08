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
	MessageDebouncer__Batch_ResizePty *_MessageDebouncer__Batch_ResizePty_
	LifecycleCoordinator_WorkspacePty *_LifecycleCoordinator_WorkspacePty_
}

type _NewApi__WorkspaceController_ struct {
	Address_HttpServer__ string
	Defaults_PtyProxy__  _Defaults_PtyProxy_
}

func New__WorkspaceController(
	api _NewApi__WorkspaceController_,
) *_WorkspaceController_ {
	workspaceController_new_result := &_WorkspaceController_{
		Defaults_PtyProxy__:               api.Defaults_PtyProxy__,
		Mutex:                             _SYNC.Mutex{},
		PtyPool:                           make(map[uint32]*_WorkspacePty_),
		NextId_PtyProxy:                   0,
		WorkspaceNetwork:                  nil,
		MessageDebouncer__Batch_ResizePty: nil,
		LifecycleCoordinator_WorkspacePty: nil,
	}
	workspaceController_new_result.WorkspaceNetwork = New__WorkspaceNetwork(_NewApi__WorkspaceNetwork_{
		Address_HttpServer__:                    api.Address_HttpServer__,
		OnConnected_PtyWebsocket__:              workspaceController_new_result.HandleConnected_PtyWebsocket,
		OnConnected_Takeover__PtyWebsocket__:    workspaceController_new_result.HandleConnected_Takeover__PtyWebsocket,
		OnDisconnected_PtyWebsocket__:           workspaceController_new_result.HandleDisconnected_PtyWebsocket,
		OnDisconnected_Takeover__PtyWebsocket__: workspaceController_new_result.HandleDisconnected_Takeover__PtyWebsocket,
		OnPayload_BinaryMessage__PtyWebsocket__: workspaceController_new_result.HandlePayload_BinaryMessage__PtyWebsocket,
	})
	workspaceController_new_result.MessageDebouncer__Batch_ResizePty = New__MessageDebouncer__Batch_ResizePty(_NewApi__MessageDebouncer__Batch_ResizePty_{
		DebounceTimeout:   50 * _TIME.Millisecond,
		OnBatch_ResizePty: workspaceController_new_result.HandleBatch_ResizePty__Debouncer,
	})
	workspaceController_new_result.LifecycleCoordinator_WorkspacePty = New__LifecycleCoordinator_WorkspacePty(_NewApi__LifecycleCoordinator_WorkspacePty_{
		OnConnect_PtyWebsocket__:    workspaceController_new_result.HandleConnect_PtyWebsocket__Coordinator,
		OnDisconnect_PtyWebsocket__: workspaceController_new_result.HandleDisconnect_PtyWebsocket__Coordinator,
		OnSync_PtyPool__:            workspaceController_new_result.HandleSync_PtyPool__Coordinator,
		OnExit_PtyProxy__:           workspaceController_new_result.HandleExit_PtyProxy__Coordinator,
	})
	return workspaceController_new_result
}

func (this *_WorkspaceController_) StartSession() error {
	go this.MessageDebouncer__Batch_ResizePty.RunWorker()
	go this.LifecycleCoordinator_WorkspacePty.RunWorker()
	return this.WorkspaceNetwork.Start()
}

func (this *_WorkspaceController_) StopSession(
	context_shutdownDeadline__HttpServer _CONTEXT.Context,
) error {
	this.MessageDebouncer__Batch_ResizePty.WorkerCancel()
	this.LifecycleCoordinator_WorkspacePty.WorkerCancel()
	return this.WorkspaceNetwork.Stop(context_shutdownDeadline__HttpServer)
}

func (this *_WorkspaceController_) HandleConnected_PtyWebsocket(
	id_WebsocketConnection__new uint64,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Connect__WorkspaceOrder_LifecycleCoordinator_{
		ExpectedId_WebsocketConnection: id_WebsocketConnection__new,
	}
}

func (this *_WorkspaceController_) HandleConnected_Takeover__PtyWebsocket(
	id_WebsocketConnection__new uint64,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Connect__WorkspaceOrder_LifecycleCoordinator_{
		ExpectedId_WebsocketConnection: id_WebsocketConnection__new,
	}
}

func (this *_WorkspaceController_) HandleDisconnected_PtyWebsocket() {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Disconnect__WorkspaceOrder_LifecycleCoordinator_{}
}

func (this *_WorkspaceController_) HandleDisconnected_Takeover__PtyWebsocket() {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Disconnect__WorkspaceOrder_LifecycleCoordinator_{}
}

func (this *_WorkspaceController_) HandlePayload_BinaryMessage__PtyWebsocket(
	id_WebsocketConnection__expected uint64,
	payload_binaryMessage []byte,
) {
	__decodeWebsocketPayload_binaryMessage(
		MAP__DECODE_PAYLOAD___PTY_MESSAGE__INGRESS,
		"pty websocket client",
		this,
		id_WebsocketConnection__expected,
		payload_binaryMessage,
	)
}

func __decodeWebsocketPayload_binaryMessage[
	__Code__Message_Ingress__ ~uint16,
	__Message_Ingress__ interface {
		Execute(
			workspaceController *_WorkspaceController_,
			id_WebsocketConnection__expected uint64,
		)
	},
](
	map_decodePayloadToMessage__ map[__Code__Message_Ingress__]func(payload_binaryMessage []byte) (__Message_Ingress__, error),
	label_messageSource__ErrorLog__ string,
	WorkspaceController_this *_WorkspaceController_,
	id_WebsocketConnection__expected uint64,
	payload_binaryMessage []byte,
) {
	if len(payload_binaryMessage) < 2 {
		_FMT.Printf(
			"%s message error: payload too short: %d bytes\n",
			label_messageSource__ErrorLog__,
			len(payload_binaryMessage),
		)
		return
	}
	messageCode := __Code__Message_Ingress__(_BINARY.BigEndian.Uint16(payload_binaryMessage[0:2]))
	decodePayloadToMessage__ := map_decodePayloadToMessage__[messageCode]
	if nil == decodePayloadToMessage__ {
		_FMT.Printf(
			"%s message error: unrecognized message code: 0x%04x\n",
			label_messageSource__ErrorLog__,
			messageCode,
		)
		return
	}
	ingressMessage_decoded, error_decodePayloadToMessage__maybe := decodePayloadToMessage__(payload_binaryMessage)
	if error_decodePayloadToMessage__maybe != nil {
		_FMT.Printf(
			"%s decode message error: %v\n",
			label_messageSource__ErrorLog__,
			error_decodePayloadToMessage__maybe,
		)
		return
	}
	ingressMessage_decoded.Execute(
		WorkspaceController_this,
		id_WebsocketConnection__expected,
	)
}

func (this *_WorkspaceController_) HandleSpawned_Pty(
	ptyProxy_spawned *_PtyProxy_,
) {
	workspacePty_new := &_WorkspacePty_{
		Visibility_Client:           VISIBLE__Visibility_Client,
		PtyProxy:                    ptyProxy_spawned,
		ExitOutcome_PtyProxy__maybe: nil,
	}
	this.Mutex.Lock()
	this.PtyPool[ptyProxy_spawned.Id] = workspacePty_new
	this.Mutex.Unlock()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		this.WorkspaceNetwork.WebsocketController_Pty,
		this.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection,
		_SpawnPtyStatus__PtyMessage_Egress_{
			Status_SpawnPty: SUCCESS__Status_SpawnPty,
			Id_PtyProxy:     ptyProxy_spawned.Id,
		},
	)
}

func (this *_WorkspaceController_) HandleSpawnFailed_Pty(
	id_PtyProxy__failed uint32,
) {
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		this.WorkspaceNetwork.WebsocketController_Pty,
		this.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection,
		_SpawnPtyStatus__PtyMessage_Egress_{
			Status_SpawnPty: FAILURE__Status_SpawnPty,
			Id_PtyProxy:     id_PtyProxy__failed,
		},
	)
}

func (this *_WorkspaceController_) HandleOutput_Pty(
	PtyProxy_source *_PtyProxy_,
	outputData_PtyProxy []byte,
) {
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		this.WorkspaceNetwork.WebsocketController_Pty,
		this.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection,
		_PtyOutput__PtyMessage_Egress_{
			Id_PtyProxy:         PtyProxy_source.Id,
			OutputData_PtyProxy: outputData_PtyProxy,
		},
	)
}

func (this *_WorkspaceController_) HandleExited_Eio_Success__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy:          PtyProxy_exited.Id,
		ExitOutcome_PtyProxy: _Success__ExitOutcome_PtyProxy_{},
	}
}

func (this *_WorkspaceController_) HandleExited_Eio_Failure__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: PtyProxy_exited.Id,
		ExitOutcome_PtyProxy: _Failure__ExitOutcome_PtyProxy_{
			ExitCode_PtyProcess: PtyProxy_exited.PtyCommand.ProcessState.ExitCode(),
		},
	}
}

func (this *_WorkspaceController_) HandleExited_Eio_Killed__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	waitStatus_PtyProcess := PtyProxy_exited.PtyCommand.ProcessState.Sys().(_SYSCALL.WaitStatus)
	this.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: PtyProxy_exited.Id,
		ExitOutcome_PtyProxy: _Killed__ExitOutcome_PtyProxy_{
			ExitSignal_PtyProcess: int(waitStatus_PtyProcess.Signal()),
		},
	}
}

func (this *_WorkspaceController_) HandleExited_Closed__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy:          PtyProxy_exited.Id,
		ExitOutcome_PtyProxy: _Closed__ExitOutcome_PtyProxy_{},
	}
}

func (this *_WorkspaceController_) HandleExited_SystemError__Pty(
	PtyProxy_exited *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	this.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: PtyProxy_exited.Id,
		ExitOutcome_PtyProxy: _SystemError__ExitOutcome_PtyProxy_{
			SystemError_PtyDevice: exitSignal_PtyReader,
		},
	}
}

func (this *_WorkspaceController_) HandleBatch_ResizePty__Debouncer(
	orderBatch_ResizePty__pending map[uint32]_Order_ResizePty_,
) {
	for _, order_ResizePty__pending_some := range orderBatch_ResizePty__pending {
		this.Mutex.Lock()
		WorkspacePty_target := this.PtyPool[order_ResizePty__pending_some.Id_PtyProxy]
		this.Mutex.Unlock()
		if WorkspacePty_target != nil {
			_ = WorkspacePty_target.PtyProxy.Resize(
				order_ResizePty__pending_some.ColumnCount_PtyTerminal,
				order_ResizePty__pending_some.RowCount_PtyTerminal,
			)
		}
	}
}

func (this *_WorkspaceController_) HandleConnect_PtyWebsocket__Coordinator(
	id_WebsocketConnection__expected uint64,
) {
	this.Mutex.Lock()
	bulletinBatch_WorkspacePty__result := make([]_Bulletin_WorkspacePty_, 0, len(this.PtyPool))
	for _, WorkspacePty_some := range this.PtyPool {
		bulletinBatch_WorkspacePty__result = append(
			bulletinBatch_WorkspacePty__result,
			_Bulletin_WorkspacePty_{
				Id_PtyProxy:                 WorkspacePty_some.PtyProxy.Id,
				ExitOutcome_PtyProxy__maybe: WorkspacePty_some.ExitOutcome_PtyProxy__maybe,
				Visibility_Client:           WorkspacePty_some.Visibility_Client,
			},
		)
	}
	this.Mutex.Unlock()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		this.WorkspaceNetwork.WebsocketController_Pty,
		id_WebsocketConnection__expected,
		_WorkspaceManifest__PtyMessage_Egress_{
			BulletinBatch_WorkspacePty: bulletinBatch_WorkspacePty__result,
		},
	)
}

func (this *_WorkspaceController_) HandleDisconnect_PtyWebsocket__Coordinator() {
	this.Mutex.Lock()
	ptyPool_cloned := _MAPS.Clone(this.PtyPool)
	this.Mutex.Unlock()
	for _, WorkspacePty_some := range ptyPool_cloned {
		switch WorkspacePty_some.PtyProxy.Mode {
		case LIVE_RUNNING__Mode_PtyProxy:
			WorkspacePty_some.PtyProxy.TransitionMode_LiveToPreSnapshot()
		case POST_SNAPSHOT__RUNNING___Mode_PtyProxy:
			WorkspacePty_some.PtyProxy.TransitionMode_PostSnapshotToPreSnapshot()
		case PRE_SNAPSHOT__RUNNING___Mode_PtyProxy:
		case EXITED__Mode_PtyProxy:
		default:
			// SPAWNING__Mode_PtyProxy == WorkspacePty_some.PtyProxy.Mode
			_FMT.Println("invalid path: HandleDisconnect_PtyWebsocket__Coordinator")
		}
	}
}

func (this *_WorkspaceController_) HandleSync_PtyPool__Coordinator(
	id_WebsocketConnection__expected uint64,
	message_SyncWorkspace _SyncWorkspace__PtyMessage_Ingress_,
) {
	this.Mutex.Lock()
	ptyPool_cloned := _MAPS.Clone(this.PtyPool)
	this.Mutex.Unlock()
	for _, WorkspacePty_some := range ptyPool_cloned {
		order_SyncPty__maybe := message_SyncWorkspace.SyncPtyOrders[WorkspacePty_some.PtyProxy.Id]
		this.Mutex.Lock()
		visibility_Client__captured := WorkspacePty_some.Visibility_Client
		if order_SyncPty__maybe != nil {
			WorkspacePty_some.Visibility_Client = VISIBLE__Visibility_Client
		} else {
			WorkspacePty_some.Visibility_Client = NOT_VISIBLE__Visibility_Client
		}
		this.Mutex.Unlock()
		if order_SyncPty__maybe != nil && VISIBLE__Visibility_Client == visibility_Client__captured {
			_ = WorkspacePty_some.PtyProxy.Resize(
				order_SyncPty__maybe.ColumnCount_PtyTerminal,
				order_SyncPty__maybe.RowCount_PtyTerminal,
			)
		} else if order_SyncPty__maybe != nil && NOT_VISIBLE__Visibility_Client == visibility_Client__captured && WorkspacePty_some.ExitOutcome_PtyProxy__maybe != nil {
			_ = WorkspacePty_some.PtyProxy.Resize(
				order_SyncPty__maybe.ColumnCount_PtyTerminal,
				order_SyncPty__maybe.RowCount_PtyTerminal,
			)
			__emitSnapshot_SyncPty__WebsocketController_Pty(
				WorkspacePty_some.PtyProxy.EmitSnapshot_Exited,
				this.WorkspaceNetwork.WebsocketController_Pty,
				id_WebsocketConnection__expected,
				WorkspacePty_some.PtyProxy.Id,
				order_SyncPty__maybe.ColumnCount_PtyTerminal,
				order_SyncPty__maybe.RowCount_PtyTerminal,
			)
		} else if order_SyncPty__maybe != nil && NOT_VISIBLE__Visibility_Client == visibility_Client__captured && nil == WorkspacePty_some.ExitOutcome_PtyProxy__maybe {
			_ = WorkspacePty_some.PtyProxy.Resize(
				order_SyncPty__maybe.ColumnCount_PtyTerminal,
				order_SyncPty__maybe.RowCount_PtyTerminal,
			)
			__emitSnapshot_SyncPty__WebsocketController_Pty(
				WorkspacePty_some.PtyProxy.TransitionMode_PreToPostSnapshot,
				this.WorkspaceNetwork.WebsocketController_Pty,
				id_WebsocketConnection__expected,
				WorkspacePty_some.PtyProxy.Id,
				order_SyncPty__maybe.ColumnCount_PtyTerminal,
				order_SyncPty__maybe.RowCount_PtyTerminal,
			)
			WorkspacePty_some.PtyProxy.TransitionMode_PostSnapshotToLive()
		} else if nil == order_SyncPty__maybe && VISIBLE__Visibility_Client == visibility_Client__captured {
			WorkspacePty_some.PtyProxy.TransitionMode_LiveToPreSnapshot()
		} else if nil == order_SyncPty__maybe && NOT_VISIBLE__Visibility_Client == visibility_Client__captured {
		} else {
			_FMT.Println("invalid path: HandleSync_PtyPool__Coordinator")
		}
	}
}

func __emitSnapshot_SyncPty__WebsocketController_Pty(
	onEmitSnapshot__ func(),
	WebsocketController_Pty *_WebsocketController_,
	id_WebsocketConnection__expected uint64,
	id_PtyProxy__target uint32,
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) {
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		WebsocketController_Pty,
		id_WebsocketConnection__expected,
		_Start_SyncPty__PtyMessage_Egress_{
			Id_PtyProxy:             id_PtyProxy__target,
			ColumnCount_PtyTerminal: columnCount_PtyTerminal,
			RowCount_PtyTerminal:    rowCount_PtyTerminal,
		},
	)
	onEmitSnapshot__()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		WebsocketController_Pty,
		id_WebsocketConnection__expected,
		_Complete_SyncPty__PtyMessage_Egress_{
			Id_PtyProxy: id_PtyProxy__target,
		},
	)
}

func (this *_WorkspaceController_) HandleExit_PtyProxy__Coordinator(
	id_PtyProxy__exited uint32,
	exitOutcome_PtyProxy _ExitOutcome_PtyProxy_,
) {
	this.Mutex.Lock()
	WorkspacePty_target := this.PtyPool[id_PtyProxy__exited]
	WorkspacePty_target.ExitOutcome_PtyProxy__maybe = exitOutcome_PtyProxy
	this.Mutex.Unlock()
	WorkspacePty_target.PtyProxy.TransitionMode_ToExited()
	WorkspacePty_target.PtyProxy.PtyWriter.WorkerCancel()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		this.WorkspaceNetwork.WebsocketController_Pty,
		this.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection,
		_PtyExit__PtyMessage_Egress_{
			Id_PtyProxy:          id_PtyProxy__exited,
			ExitOutcome_PtyProxy: exitOutcome_PtyProxy,
		},
	)
}
