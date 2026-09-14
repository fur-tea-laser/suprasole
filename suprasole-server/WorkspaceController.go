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
	Id_WorkspacePty_next              uint32
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
		Id_WorkspacePty_next:              0,
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
		OnConnect_PtyWebsocket__:     workspaceController_result.HandleConnect_PtyWebsocket__Coordinator,
		OnDisconnect_PtyWebsocket__:  workspaceController_result.HandleDisconnect_PtyWebsocket__Coordinator,
		OnSync_PtyPool__:             workspaceController_result.HandleSync_PtyPool__Coordinator,
		OnExit_PtyProxy__:            workspaceController_result.HandleExit_PtyProxy__Coordinator,
		OnSpawnPty__:                 workspaceController_result.HandleSpawnPty__Coordinator,
		OnStatus_SpawnPty__Success__: workspaceController_result.HandleStatus_SpawnPty__Success__Coordinator,
		OnStatus_SpawnPty__Failure__: workspaceController_result.HandleStatus_SpawnPty__Failure__Coordinator,
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
	message_decoded, error_decodePayloadToMessage__maybe := decodePayloadToMessage(payload_binaryMessage)
	if error_decodePayloadToMessage__maybe != nil {
		_FMT.Printf(
			"%s decode message error: %v\n",
			label_messageSource__ErrorLog__,
			error_decodePayloadToMessage__maybe,
		)
		return
	}
	message_decoded.Execute(
		WorkspaceController_this,
		id_WebsocketConnection_expected,
	)
}

func (This *_WorkspaceController_) HandleSpawnPty__Coordinator(
	id_WebsocketConnection_expected uint64,
	message_SpawnPty _SpawnPty__PtyMessage_Ingress_,
) {
	optionConfig_PtyProxy_result := This.OptionConfig_PtyProxy__default__
	for _, option_PtyProxy_current := range message_SpawnPty.OptionBatch_PtyProxy {
		option_PtyProxy_current.Update_OptionConfig(&optionConfig_PtyProxy_result)
	}
	This.Mutex.Lock()
	id_WorkspacePty_new := This.Id_WorkspacePty_next
	This.Id_WorkspacePty_next++
	This.PtyPool[id_WorkspacePty_new] = &_WorkspacePty_{
		Visibility__Client_connected__current: VISIBLE___Visibility__Client_connected,
		Id:                                    id_WorkspacePty_new,
		State_current: &_State_Spawning__WorkspacePty_{
			CancellationStatus:             NOT_CANCELED____CancellationStatus___State_Spawning__WorkspacePty,
			ResizeGeometry__deferred_maybe: nil,
		},
	}
	This.Mutex.Unlock()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		id_WebsocketConnection_expected,
		_Status_SpawnPty__PtyMessage_Egress_{
			Status_SpawnPty: SPAWNING__Status_SpawnPty,
			Id_PtyProxy:     id_WorkspacePty_new,
		},
	)
	go backgroundSpawn_PtyProxy__Coordinator(
		This.LifecycleCoordinator_WorkspacePty,
		_SpawnApi_PtyProxy_{
			OnOutput_Live__PtyProxy__:              This.HandleOutput_Pty,
			OnOutput_Snapshot__PtyProxy__:          This.HandleOutput_Pty,
			OnOutput_PostSnapshot__PtyProxy__:      This.HandleOutput_Pty,
			OnExited_Eio_Success__PtyProxy__:       This.HandleExited_Eio_Success__Pty,
			OnExited_Eio_Failure__PtyProxy__:       This.HandleExited_Eio_Failure__Pty,
			OnExited_Eio_Killed__PtyProxy__:        This.HandleExited_Eio_Killed__Pty,
			OnExited_Closed__PtyProxy__:            This.HandleExited_Closed__Pty,
			OnExited_SystemError__PtyProxy__:       This.HandleExited_SystemError__Pty,
			Id_WorkspacePty:                        id_WorkspacePty_new,
			ColumnCount_PtyTerminal:                message_SpawnPty.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:                   message_SpawnPty.RowCount_PtyTerminal,
			Path_ShellBinary__PtyCommand:           message_SpawnPty.Path_ShellBinary__PtyCommand,
			DirectoryPath_PtyCommand:               message_SpawnPty.DirectoryPath_PtyCommand,
			EnvironmentVariables_PtyCommand:        message_SpawnPty.EnvironmentVariables_PtyCommand,
			Size_StagingBuffer__PtyReader:          optionConfig_PtyProxy_result.Size_StagingBuffer__PtyReader,
			Count_ScrollbackLine__PtyTerminal:      optionConfig_PtyProxy_result.Count_ScrollbackLine__PtyTerminal,
			Size_PostSnapshotBuffer__PtyProxy:      optionConfig_PtyProxy_result.Size_PostSnapshotBuffer__PtyProxy,
			Size_QueueBuffer__InputOrder_PtyWriter: optionConfig_PtyProxy_result.Size_QueueBuffer__InputOrder_PtyWriter,
		},
	)
}

func backgroundSpawn_PtyProxy__Coordinator(
	LifecycleCoordinator_WorkspacePty *_LifecycleCoordinator_WorkspacePty_,
	spawnApi_PtyProxy _SpawnApi_PtyProxy_,
) {
	ptyProxy_dormant, error_start__PtyCommand__maybe := Spawn_PtyProxy(spawnApi_PtyProxy)
	if error_start__PtyCommand__maybe != nil {
		LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Status_SpawnPty__Failure__WorkspaceOrder_LifecycleCoordinator_{
			Id_WorkspacePty_failed:  spawnApi_PtyProxy.Id_WorkspacePty,
			Error_Start__PtyCommand: error_start__PtyCommand__maybe,
		}
		return
	}
	LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Status_SpawnPty__Success__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_spawned: spawnApi_PtyProxy.Id_WorkspacePty,
		PtyProxy_dormant:        ptyProxy_dormant,
	}
}

func (This *_WorkspaceController_) HandleStatus_SpawnPty__Success__Coordinator(
	id_WorkspacePty_spawned uint32,
	ptyProxy_dormant *_PtyProxy_,
) {
	This.Mutex.Lock()
	WorkspacePty_target := This.PtyPool[id_WorkspacePty_spawned]
	State__WorkspacePty_target__spawning := WorkspacePty_target.State_current.(*_State_Spawning__WorkspacePty_)
	State__WorkspacePty_target__spawning.Mutex.Lock()
	cancellationStatus_captured := State__WorkspacePty_target__spawning.CancellationStatus
	resizeGeometry__deferred_maybe := State__WorkspacePty_target__spawning.ResizeGeometry__deferred_maybe
	State__WorkspacePty_target__spawning.Mutex.Unlock()
	if CANCELED____CancellationStatus___State_Spawning__WorkspacePty == cancellationStatus_captured {
		delete(This.PtyPool, id_WorkspacePty_spawned)
	}
	visibility__Client_connected__current := WorkspacePty_target.Visibility__Client_connected__current
	This.Mutex.Unlock()
	if CANCELED____CancellationStatus___State_Spawning__WorkspacePty == cancellationStatus_captured {
		ptyProxy_dormant.Terminate_PtyProcess(int(_SYSCALL.SIGTERM))
		Emit__PtyMessage_Egress__WebsocketController_Pty(
			This.WorkspaceNetwork.WebsocketController_Pty,
			This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_current,
			_Status_SpawnPty__PtyMessage_Egress_{
				Status_SpawnPty: CANCELED__Status_SpawnPty,
				Id_PtyProxy:     id_WorkspacePty_spawned,
			},
		)
		return
	}
	if resizeGeometry__deferred_maybe != nil {
		_ = ptyProxy_dormant.Resize(
			resizeGeometry__deferred_maybe.ColumnCount_PtyTerminal,
			resizeGeometry__deferred_maybe.RowCount_PtyTerminal,
		)
	}
	This.Mutex.Lock()
	if CONNECTED__Status_WebsocketConnection == This.WorkspaceNetwork.WebsocketController_Pty.Status_WebsocketConnection_current && VISIBLE___Visibility__Client_connected == visibility__Client_connected__current {
		ptyProxy_dormant.Mode_current = LIVE_RUNNING__Mode_PtyProxy
	} else {
		ptyProxy_dormant.Mode_current = PRE_SNAPSHOT__RUNNING___Mode_PtyProxy
	}
	WorkspacePty_target.State_current = &_State_Active__WorkspacePty_{
		PtyProxy: ptyProxy_dormant,
	}
	This.Mutex.Unlock()
	go ptyProxy_dormant.PtyReader.RunWorker()
	go ptyProxy_dormant.PtyWriter.RunWorker()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_current,
		_Status_SpawnPty__PtyMessage_Egress_{
			Status_SpawnPty: SUCCESS__Status_SpawnPty,
			Id_PtyProxy:     id_WorkspacePty_spawned,
		},
	)
}

func (This *_WorkspaceController_) HandleStatus_SpawnPty__Failure__Coordinator(
	id_WorkspacePty_failed uint32,
	error_start__PtyCommand error,
) {
	This.Mutex.Lock()
	WorkspacePty_target := This.PtyPool[id_WorkspacePty_failed]
	delete(This.PtyPool, id_WorkspacePty_failed)
	State__WorkspacePty_target__spawning := WorkspacePty_target.State_current.(*_State_Spawning__WorkspacePty_)
	This.Mutex.Unlock()
	State__WorkspacePty_target__spawning.Mutex.Lock()
	cancellationStatus_captured := State__WorkspacePty_target__spawning.CancellationStatus
	State__WorkspacePty_target__spawning.Mutex.Unlock()
	switch cancellationStatus_captured {
	case CANCELED____CancellationStatus___State_Spawning__WorkspacePty:
		Emit__PtyMessage_Egress__WebsocketController_Pty(
			This.WorkspaceNetwork.WebsocketController_Pty,
			This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_current,
			_Status_SpawnPty__PtyMessage_Egress_{
				Status_SpawnPty: CANCELED__Status_SpawnPty,
				Id_PtyProxy:     id_WorkspacePty_failed,
			},
		)
	case NOT_CANCELED____CancellationStatus___State_Spawning__WorkspacePty:
		Emit__PtyMessage_Egress__WebsocketController_Pty(
			This.WorkspaceNetwork.WebsocketController_Pty,
			This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_current,
			_Status_SpawnPty__PtyMessage_Egress_{
				Status_SpawnPty: FAILURE__Status_SpawnPty,
				Id_PtyProxy:     id_WorkspacePty_failed,
			},
		)
	default:
		panic("invalid path: _WorkspaceController_ HandleStatus_SpawnPty__Failure__Coordinator")
	}
}

func (This *_WorkspaceController_) HandleOutput_Pty(
	id_WorkspacePty uint32,
	outputData_PtyProxy []byte,
) {
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_current,
		_PtyOutput__PtyMessage_Egress_{
			Id_PtyProxy:         id_WorkspacePty,
			OutputData_PtyProxy: outputData_PtyProxy,
		},
	)
}

func (This *_WorkspaceController_) HandleExited_Eio_Success__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy:   _Success__ExitOutcome_PtyProxy_{},
	}
}

func (This *_WorkspaceController_) HandleExited_Eio_Failure__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
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
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy: _Killed__ExitOutcome_PtyProxy_{
			ExitSignal_PtyProcess: int(waitStatus_PtyProcess.Signal()),
		},
	}
}

func (This *_WorkspaceController_) HandleExited_Closed__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy:   _Closed__ExitOutcome_PtyProxy_{},
	}
}

func (This *_WorkspaceController_) HandleExited_SystemError__Pty(
	PtyProxy_exited *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
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
		WorkspacePty_target := This.PtyPool[order_ResizePty__pending_some.Id_WorkspacePty]
		This.Mutex.Unlock()
		if WorkspacePty_target != nil {
			WorkspacePty_target.State_current.HandleResize_Debouncer(
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
		bulletin_WorkspacePty_result := _Bulletin_WorkspacePty_{
			Id_WorkspacePty:              WorkspacePty_some.Id,
			Visibility__Client_connected: WorkspacePty_some.Visibility__Client_connected__current,
			Status_WorkspacePty:          0, // nil
			ExitOutcome_PtyProxy_maybe:   nil,
		}
		WorkspacePty_some.State_current.Update_ManifestBulletin(&bulletin_WorkspacePty_result)
		bulletinBatch_WorkspacePty_result = append(
			bulletinBatch_WorkspacePty_result,
			bulletin_WorkspacePty_result,
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
	for _, WorkspacePty_some := range ptyPool_cloned {
		WorkspacePty_some.Visibility__Client_connected__current = DISCONNECTED_UNKNOWN___Visibility__Client_connected
	}
	This.Mutex.Unlock()
	for _, WorkspacePty_some := range ptyPool_cloned {
		WorkspacePty_some.State_current.HandleDisconnect_Coordinator()
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
		order_SyncPty_maybe := message__Batch_SyncPty.OrderBatch_SyncPty[WorkspacePty_some.Id]
		This.Mutex.Lock()
		visibility__Client_connected__previous := WorkspacePty_some.Visibility__Client_connected__current
		if order_SyncPty_maybe != nil {
			WorkspacePty_some.Visibility__Client_connected__current = VISIBLE___Visibility__Client_connected
		} else if nil == order_SyncPty_maybe {
			WorkspacePty_some.Visibility__Client_connected__current = NOT_VISIBLE___Visibility__Client_connected
		}
		State__WorkspacePty_some__current := WorkspacePty_some.State_current
		This.Mutex.Unlock()
		State__WorkspacePty_some__current.HandleSync_Coordinator(
			order_SyncPty_maybe,
			visibility__Client_connected__previous,
			This.WorkspaceNetwork.WebsocketController_Pty,
			id_WebsocketConnection_expected,
			WorkspacePty_some.Id,
		)
	}
}

func (This *_WorkspaceController_) HandleExit_PtyProxy__Coordinator(
	id_WorkspacePty_exited uint32,
	exitOutcome_PtyProxy _ExitOutcome_PtyProxy_,
) {
	This.Mutex.Lock()
	WorkspacePty_target := This.PtyPool[id_WorkspacePty_exited]
	State__WorkspacePty_target__active := WorkspacePty_target.State_current.(*_State_Active__WorkspacePty_)
	This.Mutex.Unlock()
	State__WorkspacePty_target__active.PtyProxy.TransitionMode_ToExited()
	State__WorkspacePty_target__active.PtyProxy.PtyWriter.WorkerCancel()
	This.Mutex.Lock()
	WorkspacePty_target.State_current = &_State_Exited__WorkspacePty_{
		PtyProxy:    State__WorkspacePty_target__active.PtyProxy,
		ExitOutcome: exitOutcome_PtyProxy,
	}
	This.Mutex.Unlock()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_current,
		_PtyExit__PtyMessage_Egress_{
			Id_PtyProxy:          id_WorkspacePty_exited,
			ExitOutcome_PtyProxy: exitOutcome_PtyProxy,
		},
	)
}
