package main

import (
	_MAPS "maps"
	_SYSCALL "syscall"
)

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

func (this _Count_ScrollbackLine__Option_PtyProxy_) Update_OptionConfig(
	optionConfig_PtyProxy_result *_OptionConfig_PtyProxy_,
) {
	optionConfig_PtyProxy_result.Count_ScrollbackLine__PtyTerminal = this.Count_ScrollbackLine__PtyTerminal
}

func (this _Size_StagingBuffer__Option_PtyProxy_) Update_OptionConfig(
	optionConfig_PtyProxy_result *_OptionConfig_PtyProxy_,
) {
	optionConfig_PtyProxy_result.Size_StagingBuffer__PtyReader = this.Size_StagingBuffer__PtyReader
}

func (this _Size_PostSnapshotBuffer__Option_PtyProxy_) Update_OptionConfig(
	optionConfig_PtyProxy_result *_OptionConfig_PtyProxy_,
) {
	optionConfig_PtyProxy_result.Size_PostSnapshotBuffer__PtyProxy = this.Size_PostSnapshotBuffer__PtyProxy
}

func (this _Size_QueueBuffer__InputOrder_PtyWriter___Option_PtyProxy_) Update_OptionConfig(
	optionConfig_PtyProxy_result *_OptionConfig_PtyProxy_,
) {
	optionConfig_PtyProxy_result.Size_QueueBuffer__InputOrder_PtyWriter = this.Size_QueueBuffer__InputOrder_PtyWriter
}

func backgroundSpawn_PtyProxy__Coordinator(
	LifecycleCoordinator_WorkspacePty *_LifecycleCoordinator_WorkspacePty_,
	spawnApi_PtyProxy _SpawnApi_PtyProxy_,
) {
	ptyProxy_quiescent, error_start__PtyCommand__maybe := Spawn_PtyProxy(spawnApi_PtyProxy)
	if error_start__PtyCommand__maybe != nil {
		LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Status_SpawnPty__Failure__WorkspaceOrder_LifecycleCoordinator_{
			Id_WorkspacePty_failed:  spawnApi_PtyProxy.Id_WorkspacePty,
			Error_Start__PtyCommand: error_start__PtyCommand__maybe,
		}
		return
	}
	LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Status_SpawnPty__Success__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_spawned: spawnApi_PtyProxy.Id_WorkspacePty,
		PtyProxy_quiescent:      ptyProxy_quiescent,
	}
}

func (This *_WorkspaceController_) HandleStatus_SpawnPty__Success__Coordinator(
	id_WorkspacePty_spawned uint32,
	ptyProxy_quiescent *_PtyProxy_,
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
		ptyProxy_quiescent.Terminate_PtyProcess(int(_SYSCALL.SIGTERM))
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
		_ = ptyProxy_quiescent.Resize(
			resizeGeometry__deferred_maybe.ColumnCount_PtyTerminal,
			resizeGeometry__deferred_maybe.RowCount_PtyTerminal,
		)
	}
	This.Mutex.Lock()
	if CONNECTED__Status_WebsocketConnection == This.WorkspaceNetwork.WebsocketController_Pty.Status_WebsocketConnection_current && VISIBLE___Visibility__Client_connected == visibility__Client_connected__current {
		ptyProxy_quiescent.Mode_current = LIVE_RUNNING__Mode_PtyProxy
	} else {
		ptyProxy_quiescent.Mode_current = PRE_SNAPSHOT__RUNNING___Mode_PtyProxy
	}
	WorkspacePty_target.State_current = &_State_Active__WorkspacePty_{
		PtyProxy: ptyProxy_quiescent,
	}
	This.Mutex.Unlock()
	go ptyProxy_quiescent.PtyReader.RunWorker()
	go ptyProxy_quiescent.PtyWriter.RunWorker()
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

func (this *_State_Spawning__WorkspacePty_) Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = SPAWNING__Status_WorkspacePty
}

func (this *_State_Active__WorkspacePty_) Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = ACTIVE__Status_WorkspacePty
}

func (this *_State_Exited__WorkspacePty_) Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = EXITED__Status_WorkspacePty
	bulletin_result.ExitOutcome_PtyProxy_maybe = this.ExitOutcome
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

func (this *_State_Spawning__WorkspacePty_) HandleDisconnect_Coordinator() {
}

func (this *_State_Active__WorkspacePty_) HandleDisconnect_Coordinator() {
	switch this.PtyProxy.Mode_current {
	case LIVE_RUNNING__Mode_PtyProxy:
		this.PtyProxy.TransitionMode_LiveToPreSnapshot()
	case POST_SNAPSHOT__RUNNING___Mode_PtyProxy:
		this.PtyProxy.TransitionMode_PostSnapshotToPreSnapshot()
	}
}

func (this *_State_Exited__WorkspacePty_) HandleDisconnect_Coordinator() {
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

func (this *_State_Spawning__WorkspacePty_) HandleSync_Coordinator(
	order_SyncPty_maybe *_Order_SyncPty_,
	_ _Visibility__Client_connected_,
	_ *_WebsocketController_,
	_ uint64,
	_ uint32,
) {
	if order_SyncPty_maybe != nil {
		this.Mutex.Lock()
		this.ResizeGeometry__deferred_maybe = &_ResizeGeometry___State_Spawning__WorkspacePty_{
			ColumnCount_PtyTerminal: order_SyncPty_maybe.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:    order_SyncPty_maybe.RowCount_PtyTerminal,
		}
		this.Mutex.Unlock()
	}
}

func (this *_State_Active__WorkspacePty_) HandleSync_Coordinator(
	order_SyncPty_maybe *_Order_SyncPty_,
	visibility__Client_connected__previous _Visibility__Client_connected_,
	WebsocketController_Pty *_WebsocketController_,
	id_WebsocketConnection_expected uint64,
	id_WorkspacePty uint32,
) {
	if order_SyncPty_maybe != nil {
		_ = this.PtyProxy.Resize(
			order_SyncPty_maybe.ColumnCount_PtyTerminal,
			order_SyncPty_maybe.RowCount_PtyTerminal,
		)
	}
	if order_SyncPty_maybe != nil && visibility__Client_connected__previous != VISIBLE___Visibility__Client_connected {
		__emitSnapshot_SyncPty__WebsocketController_Pty(
			this.PtyProxy.TransitionMode_PreToPostSnapshot,
			WebsocketController_Pty,
			id_WebsocketConnection_expected,
			id_WorkspacePty,
		)
		this.PtyProxy.TransitionMode_PostSnapshotToLive()
	} else if nil == order_SyncPty_maybe && VISIBLE___Visibility__Client_connected == visibility__Client_connected__previous {
		this.PtyProxy.TransitionMode_LiveToPreSnapshot()
	}
}

func (this *_State_Exited__WorkspacePty_) HandleSync_Coordinator(
	order_SyncPty_maybe *_Order_SyncPty_,
	visibility__Client_connected__previous _Visibility__Client_connected_,
	WebsocketController_Pty *_WebsocketController_,
	id_WebsocketConnection_expected uint64,
	id_WorkspacePty uint32,
) {
	if order_SyncPty_maybe != nil {
		_ = this.PtyProxy.Resize(
			order_SyncPty_maybe.ColumnCount_PtyTerminal,
			order_SyncPty_maybe.RowCount_PtyTerminal,
		)
	}
	if order_SyncPty_maybe != nil && visibility__Client_connected__previous != VISIBLE___Visibility__Client_connected {
		__emitSnapshot_SyncPty__WebsocketController_Pty(
			this.PtyProxy.EmitSnapshot_Exited,
			WebsocketController_Pty,
			id_WebsocketConnection_expected,
			id_WorkspacePty,
		)
	}
}

func __emitSnapshot_SyncPty__WebsocketController_Pty(
	onEmitSnapshot__ func(),
	WebsocketController_Pty *_WebsocketController_,
	id_WebsocketConnection_expected uint64,
	id_WorkspacePty_target uint32,
) {
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		WebsocketController_Pty,
		id_WebsocketConnection_expected,
		_TaskStart_SyncPty__PtyMessage_Egress_{
			Id_PtyProxy: id_WorkspacePty_target,
		},
	)
	onEmitSnapshot__()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		WebsocketController_Pty,
		id_WebsocketConnection_expected,
		_TaskComplete_SyncPty__PtyMessage_Egress_{
			Id_PtyProxy: id_WorkspacePty_target,
		},
	)
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
