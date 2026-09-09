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

type _WorkspaceController_ struct {
	OptionConfig_PtyProxy__default__  _OptionConfig_PtyProxy_
	Mutex                             _SYNC.Mutex
	PtyPool                           map[uint32]*_WorkspacePty_
	Id_PtyProxy_next                  uint32
	WorkspaceNetwork                  *_WorkspaceNetwork_
	MessageDebouncer__Batch_ResizePty *_MessageDebouncer__Batch_ResizePty_
	LifecycleCoordinator_WorkspacePty *_LifecycleCoordinator_WorkspacePty_
}

type _MakeApi_WorkspaceController_ struct {
	Address_HttpServer__             string
	OptionConfig_PtyProxy__default__ _OptionConfig_PtyProxy_
}

func Make_WorkspaceController(
	api _MakeApi_WorkspaceController_,
) *_WorkspaceController_ {
	workspaceController_result := &_WorkspaceController_{
		OptionConfig_PtyProxy__default__:  api.OptionConfig_PtyProxy__default__,
		Mutex:                             _SYNC.Mutex{},
		PtyPool:                           make(map[uint32]*_WorkspacePty_),
		Id_PtyProxy_next:                  0,
		WorkspaceNetwork:                  nil,
		MessageDebouncer__Batch_ResizePty: nil,
		LifecycleCoordinator_WorkspacePty: nil,
	}
	workspaceController_result.WorkspaceNetwork = Make_WorkspaceNetwork(_MakeApi_WorkspaceNetwork_{
		Address_HttpServer__:                    api.Address_HttpServer__,
		OnConnected_PtyWebsocket__:              workspaceController_result.HandleConnected_PtyWebsocket,
		OnConnected_Takeover__PtyWebsocket__:    workspaceController_result.HandleConnected_Takeover__PtyWebsocket,
		OnDisconnected_PtyWebsocket__:           workspaceController_result.HandleDisconnected_PtyWebsocket,
		OnDisconnected_Takeover__PtyWebsocket__: workspaceController_result.HandleDisconnected_Takeover__PtyWebsocket,
		OnPayload_BinaryMessage__PtyWebsocket__: workspaceController_result.HandlePayload_BinaryMessage__PtyWebsocket,
	})
	workspaceController_result.MessageDebouncer__Batch_ResizePty = Make__MessageDebouncer__Batch_ResizePty(_MakeApi__MessageDebouncer__Batch_ResizePty_{
		DebounceTimeout__:               50 * _TIME.Millisecond,
		OnFlush__OrderBatch_ResizePty__: workspaceController_result.HandleBatch_ResizePty__Debouncer,
	})
	workspaceController_result.LifecycleCoordinator_WorkspacePty = Make__LifecycleCoordinator_WorkspacePty(_MakeApi__LifecycleCoordinator_WorkspacePty_{
		OnConnect_PtyWebsocket__:    workspaceController_result.HandleConnect_PtyWebsocket__Coordinator,
		OnDisconnect_PtyWebsocket__: workspaceController_result.HandleDisconnect_PtyWebsocket__Coordinator,
		OnSync_PtyPool__:            workspaceController_result.HandleSync_PtyPool__Coordinator,
		OnExit_PtyProxy__:           workspaceController_result.HandleExit_PtyProxy__Coordinator,
	})
	return workspaceController_result
}

func (This *_WorkspaceController_) StartSession() error {
	go This.MessageDebouncer__Batch_ResizePty.RunWorker()
	go This.LifecycleCoordinator_WorkspacePty.RunWorker()
	return This.WorkspaceNetwork.Start()
}

func (This *_WorkspaceController_) StopSession(
	context_shutdownDeadline__HttpServer _CONTEXT.Context,
) error {
	This.MessageDebouncer__Batch_ResizePty.WorkerCancel()
	This.LifecycleCoordinator_WorkspacePty.WorkerCancel()
	return This.WorkspaceNetwork.Stop(context_shutdownDeadline__HttpServer)
}

func (This *_WorkspaceController_) HandleConnected_PtyWebsocket(
	id_WebsocketConnection_new uint64,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Connect__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: id_WebsocketConnection_new,
	}
}

func (This *_WorkspaceController_) HandleConnected_Takeover__PtyWebsocket(
	id_WebsocketConnection_new uint64,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Connect__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: id_WebsocketConnection_new,
	}
}

func (This *_WorkspaceController_) HandleDisconnected_PtyWebsocket() {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Disconnect__WorkspaceOrder_LifecycleCoordinator_{}
}

func (This *_WorkspaceController_) HandleDisconnected_Takeover__PtyWebsocket() {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Disconnect__WorkspaceOrder_LifecycleCoordinator_{}
}

func (This *_WorkspaceController_) HandlePayload_BinaryMessage__PtyWebsocket(
	id_WebsocketConnection_expected uint64,
	payload_binaryMessage []byte,
) {
	__decodeWebsocketPayload_binaryMessage(
		MAP__DECODE_PAYLOAD___PTY_MESSAGE__INGRESS,
		"pty websocket client",
		This,
		id_WebsocketConnection_expected,
		payload_binaryMessage,
	)
}

func __decodeWebsocketPayload_binaryMessage[
	__Code__Message_Ingress__ ~uint16,
	__Message_Ingress__ interface {
		Execute(
			WorkspaceController_forwarded *_WorkspaceController_,
			id_WebsocketConnection_expected uint64,
		)
	},
](
	map_decodePayloadToMessage__ map[__Code__Message_Ingress__]func(payload_binaryMessage []byte) (__Message_Ingress__, error),
	label_messageSource__ErrorLog__ string,
	WorkspaceController_this *_WorkspaceController_,
	id_WebsocketConnection_expected uint64,
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
	code_messagePayload := __Code__Message_Ingress__(_BINARY.BigEndian.Uint16(payload_binaryMessage[:2]))
	decodePayloadToMessage := map_decodePayloadToMessage__[code_messagePayload]
	if nil == decodePayloadToMessage {
		_FMT.Printf(
			"%s decode message error: unrecognized message opcode: 0x%04x\n",
			label_messageSource__ErrorLog__,
			code_messagePayload,
		)
		return
	}
	ingressMessage_decoded, error_decodePayloadToMessage__maybe := decodePayloadToMessage(payload_binaryMessage)
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
		id_WebsocketConnection_expected,
	)
}

func (This *_WorkspaceController_) HandleSpawned_Pty(
	ptyProxy_spawned *_PtyProxy_,
) {
	workspacePty_new := &_WorkspacePty_{
		Visibility_Client_current:  VISIBLE__Visibility_Client,
		PtyProxy:                   ptyProxy_spawned,
		ExitOutcome_PtyProxy_maybe: nil,
	}
	This.Mutex.Lock()
	This.PtyPool[ptyProxy_spawned.Id] = workspacePty_new
	This.Mutex.Unlock()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_current,
		_Status_SpawnPty__PtyMessage_Egress_{
			Status_SpawnPty: SUCCESS__Status_SpawnPty,
			Id_PtyProxy:     ptyProxy_spawned.Id,
		},
	)
}

func (This *_WorkspaceController_) HandleSpawnFailed_Pty(
	id_PtyProxy_failed uint32,
) {
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_current,
		_Status_SpawnPty__PtyMessage_Egress_{
			Status_SpawnPty: FAILURE__Status_SpawnPty,
			Id_PtyProxy:     id_PtyProxy_failed,
		},
	)
}

func (This *_WorkspaceController_) HandleOutput_Pty(
	PtyProxy_source *_PtyProxy_,
	outputData_PtyProxy []byte,
) {
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_current,
		_PtyOutput__PtyMessage_Egress_{
			Id_PtyProxy:         PtyProxy_source.Id,
			OutputData_PtyProxy: outputData_PtyProxy,
		},
	)
}

func (This *_WorkspaceController_) HandleExited_Eio_Success__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy:          PtyProxy_exited.Id,
		ExitOutcome_PtyProxy: _Success__ExitOutcome_PtyProxy_{},
	}
}

func (This *_WorkspaceController_) HandleExited_Eio_Failure__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: PtyProxy_exited.Id,
		ExitOutcome_PtyProxy: _Failure__ExitOutcome_PtyProxy_{
			ExitCode_PtyProcess: PtyProxy_exited.PtyCommand.ProcessState.ExitCode(),
		},
	}
}

func (This *_WorkspaceController_) HandleExited_Eio_Killed__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	waitStatus_PtyProcess := PtyProxy_exited.PtyCommand.ProcessState.Sys().(_SYSCALL.WaitStatus)
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: PtyProxy_exited.Id,
		ExitOutcome_PtyProxy: _Killed__ExitOutcome_PtyProxy_{
			ExitSignal_PtyProcess: int(waitStatus_PtyProcess.Signal()),
		},
	}
}

func (This *_WorkspaceController_) HandleExited_Closed__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy:          PtyProxy_exited.Id,
		ExitOutcome_PtyProxy: _Closed__ExitOutcome_PtyProxy_{},
	}
}

func (This *_WorkspaceController_) HandleExited_SystemError__Pty(
	PtyProxy_exited *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_PtyProxy: PtyProxy_exited.Id,
		ExitOutcome_PtyProxy: _SystemError__ExitOutcome_PtyProxy_{
			SystemError_PtyDevice: exitSignal_PtyReader,
		},
	}
}

func (This *_WorkspaceController_) HandleBatch_ResizePty__Debouncer(
	orderBatch_ResizePty_pending map[uint32]_Order_ResizePty_,
) {
	for _, order_ResizePty__pending_some := range orderBatch_ResizePty_pending {
		This.Mutex.Lock()
		WorkspacePty_target := This.PtyPool[order_ResizePty__pending_some.Id_PtyProxy]
		This.Mutex.Unlock()
		if WorkspacePty_target != nil {
			_ = WorkspacePty_target.PtyProxy.Resize(
				order_ResizePty__pending_some.ColumnCount_PtyTerminal,
				order_ResizePty__pending_some.RowCount_PtyTerminal,
			)
		}
	}
}

func (This *_WorkspaceController_) HandleConnect_PtyWebsocket__Coordinator(
	id_WebsocketConnection_expected uint64,
) {
	This.Mutex.Lock()
	bulletinBatch_WorkspacePty_result := make([]_Bulletin_WorkspacePty_, 0, len(This.PtyPool))
	for _, WorkspacePty_some := range This.PtyPool {
		bulletinBatch_WorkspacePty_result = append(
			bulletinBatch_WorkspacePty_result,
			_Bulletin_WorkspacePty_{
				Id_PtyProxy:                WorkspacePty_some.PtyProxy.Id,
				ExitOutcome_PtyProxy_maybe: WorkspacePty_some.ExitOutcome_PtyProxy_maybe,
				Visibility_Client_current:  WorkspacePty_some.Visibility_Client_current,
			},
		)
	}
	This.Mutex.Unlock()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		id_WebsocketConnection_expected,
		_WorkspaceManifest__PtyMessage_Egress_{
			BulletinBatch_WorkspacePty: bulletinBatch_WorkspacePty_result,
		},
	)
}

func (This *_WorkspaceController_) HandleDisconnect_PtyWebsocket__Coordinator() {
	This.Mutex.Lock()
	ptyPool_cloned := _MAPS.Clone(This.PtyPool)
	This.Mutex.Unlock()
	for _, WorkspacePty_some := range ptyPool_cloned {
		switch WorkspacePty_some.PtyProxy.Mode_current {
		case LIVE_RUNNING__Mode_PtyProxy:
			WorkspacePty_some.PtyProxy.TransitionMode_LiveToPreSnapshot()
		case POST_SNAPSHOT__RUNNING___Mode_PtyProxy:
			WorkspacePty_some.PtyProxy.TransitionMode_PostSnapshotToPreSnapshot()
		case PRE_SNAPSHOT__RUNNING___Mode_PtyProxy:
		case EXITED__Mode_PtyProxy:
		default:
			// SPAWNING__Mode_PtyProxy == WorkspacePty_some.PtyProxy.Mode_current
			_FMT.Println("invalid path: _WorkspaceController_ HandleDisconnect_PtyWebsocket__Coordinator")
		}
	}
}

func (This *_WorkspaceController_) HandleSync_PtyPool__Coordinator(
	id_WebsocketConnection_expected uint64,
	message__Batch_SyncPty _Batch_SyncPty__PtyMessage_Ingress_,
) {
	This.Mutex.Lock()
	ptyPool_cloned := _MAPS.Clone(This.PtyPool)
	This.Mutex.Unlock()
	for _, WorkspacePty_some := range ptyPool_cloned {
		order_SyncPty_maybe := message__Batch_SyncPty.OrderBatch_SyncPty[WorkspacePty_some.PtyProxy.Id]
		This.Mutex.Lock()
		visibility_Client_captured := WorkspacePty_some.Visibility_Client_current
		if order_SyncPty_maybe != nil {
			WorkspacePty_some.Visibility_Client_current = VISIBLE__Visibility_Client
		} else {
			WorkspacePty_some.Visibility_Client_current = NOT_VISIBLE__Visibility_Client
		}
		This.Mutex.Unlock()
		if order_SyncPty_maybe != nil && VISIBLE__Visibility_Client == visibility_Client_captured {
			_ = WorkspacePty_some.PtyProxy.Resize(
				order_SyncPty_maybe.ColumnCount_PtyTerminal,
				order_SyncPty_maybe.RowCount_PtyTerminal,
			)
		} else if order_SyncPty_maybe != nil && NOT_VISIBLE__Visibility_Client == visibility_Client_captured && WorkspacePty_some.ExitOutcome_PtyProxy_maybe != nil {
			_ = WorkspacePty_some.PtyProxy.Resize(
				order_SyncPty_maybe.ColumnCount_PtyTerminal,
				order_SyncPty_maybe.RowCount_PtyTerminal,
			)
			__emitSnapshot_SyncPty__WebsocketController_Pty(
				WorkspacePty_some.PtyProxy.EmitSnapshot_Exited,
				This.WorkspaceNetwork.WebsocketController_Pty,
				id_WebsocketConnection_expected,
				WorkspacePty_some.PtyProxy.Id,
			)
		} else if order_SyncPty_maybe != nil && NOT_VISIBLE__Visibility_Client == visibility_Client_captured && nil == WorkspacePty_some.ExitOutcome_PtyProxy_maybe {
			_ = WorkspacePty_some.PtyProxy.Resize(
				order_SyncPty_maybe.ColumnCount_PtyTerminal,
				order_SyncPty_maybe.RowCount_PtyTerminal,
			)
			__emitSnapshot_SyncPty__WebsocketController_Pty(
				WorkspacePty_some.PtyProxy.TransitionMode_PreToPostSnapshot,
				This.WorkspaceNetwork.WebsocketController_Pty,
				id_WebsocketConnection_expected,
				WorkspacePty_some.PtyProxy.Id,
			)
			WorkspacePty_some.PtyProxy.TransitionMode_PostSnapshotToLive()
		} else if nil == order_SyncPty_maybe && VISIBLE__Visibility_Client == visibility_Client_captured {
			WorkspacePty_some.PtyProxy.TransitionMode_LiveToPreSnapshot()
		} else if nil == order_SyncPty_maybe && NOT_VISIBLE__Visibility_Client == visibility_Client_captured {
		} else {
			_FMT.Println("invalid path: _WorkspaceController_ HandleSync_PtyPool__Coordinator")
		}
	}
}

func __emitSnapshot_SyncPty__WebsocketController_Pty(
	onEmitSnapshot__ func(),
	WebsocketController_Pty *_WebsocketController_,
	id_WebsocketConnection_expected uint64,
	id_PtyProxy_target uint32,
) {
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		WebsocketController_Pty,
		id_WebsocketConnection_expected,
		_StartTask_SyncPty__PtyMessage_Egress_{
			Id_PtyProxy: id_PtyProxy_target,
		},
	)
	onEmitSnapshot__()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		WebsocketController_Pty,
		id_WebsocketConnection_expected,
		_CompleteTask_SyncPty__PtyMessage_Egress_{
			Id_PtyProxy: id_PtyProxy_target,
		},
	)
}

func (This *_WorkspaceController_) HandleExit_PtyProxy__Coordinator(
	id_PtyProxy_exited uint32,
	exitOutcome_PtyProxy _ExitOutcome_PtyProxy_,
) {
	This.Mutex.Lock()
	WorkspacePty_target := This.PtyPool[id_PtyProxy_exited]
	WorkspacePty_target.ExitOutcome_PtyProxy_maybe = exitOutcome_PtyProxy
	This.Mutex.Unlock()
	WorkspacePty_target.PtyProxy.TransitionMode_ToExited()
	WorkspacePty_target.PtyProxy.PtyWriter.WorkerCancel()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_current,
		_PtyExit__PtyMessage_Egress_{
			Id_PtyProxy:          id_PtyProxy_exited,
			ExitOutcome_PtyProxy: exitOutcome_PtyProxy,
		},
	)
}
