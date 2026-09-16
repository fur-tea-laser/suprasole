package main

import (
	_BYTES "bytes"
	_CONTEXT "context"
	_MAPS "maps"
	_EXEC "os/exec"
	_SYSCALL "syscall"

	_PTY "github.com/creack/pty"
	_XTERM "github.com/gitpod-io/xterm-go"
)

func (this *_LifecycleCoordinator_WorkspacePty_) RunWorker() {
	for {
		select {
		case <-this.WorkerContext.Done():
			return
		case order_leading := <-this.QueueChannel_WorkspaceOrder:
			orderBatch_pending := this.Drain__QueueChannel_WorkspaceOrder(order_leading)
			orderBatch_reconciled := this.Reconcile__orderBatch_pending(orderBatch_pending)
			for _, order__reconciled_current := range orderBatch_reconciled {
				order__reconciled_current.Execute(this)
			}
		}
	}
}

func (this *_LifecycleCoordinator_WorkspacePty_) Drain__QueueChannel_WorkspaceOrder(
	order_leading _WorkspaceOrder_LifecycleCoordinator_,
) []_WorkspaceOrder_LifecycleCoordinator_ {
	orderBatch__pending_result := []_WorkspaceOrder_LifecycleCoordinator_{order_leading}
	for {
		select {
		case order__pending_next := <-this.QueueChannel_WorkspaceOrder:
			orderBatch__pending_result = append(
				orderBatch__pending_result,
				order__pending_next,
			)
		default:
			return orderBatch__pending_result
		}
	}
}

func (this *_LifecycleCoordinator_WorkspacePty_) Reconcile__orderBatch_pending(
	orderBatch_pending []_WorkspaceOrder_LifecycleCoordinator_,
) []_WorkspaceOrder_LifecycleCoordinator_ {
	var orderBatch_ExitPty_result []_WorkspaceOrder_LifecycleCoordinator_
	var orderBatch_SpawnPty_result []_WorkspaceOrder_LifecycleCoordinator_
	var orderBatch_Session_result []_WorkspaceOrder_LifecycleCoordinator_
	var orderBatch__Status_SpawnPty__result []_WorkspaceOrder_LifecycleCoordinator_
	for _, order__pending_current := range orderBatch_pending {
		switch order__pending_current_the := order__pending_current.(type) {
		case _ExitPty__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch_ExitPty_result = append(
				orderBatch_ExitPty_result,
				order__pending_current_the,
			)
		case _SpawnPty__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch_SpawnPty_result = append(
				orderBatch_SpawnPty_result,
				order__pending_current_the,
			)
		case _Disconnect__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch_Session_result = []_WorkspaceOrder_LifecycleCoordinator_{
				order__pending_current_the,
			}
		case _Connect__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch_Session_result = upsertTail_Order__orderBatch_Session(
				orderBatch_Session_result,
				order__pending_current_the,
			)
		case _Sync__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch_Session_result = upsertTail_Order__orderBatch_Session(
				orderBatch_Session_result,
				order__pending_current_the,
			)
		case _Status_SpawnPty__Success__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch__Status_SpawnPty__result = append(
				orderBatch__Status_SpawnPty__result,
				order__pending_current_the,
			)
		case _Status_SpawnPty__Failure__WorkspaceOrder_LifecycleCoordinator_:
			orderBatch__Status_SpawnPty__result = append(
				orderBatch__Status_SpawnPty__result,
				order__pending_current_the,
			)
		default:
			panic("invalid path: _LifecycleCoordinator_WorkspacePty_ Reconcile__orderBatch_pending")
		}
	}
	return append(
		orderBatch_ExitPty_result,
		append(
			orderBatch_SpawnPty_result,
			append(
				orderBatch_Session_result,
				orderBatch__Status_SpawnPty__result...,
			)...,
		)...,
	)
}

func upsertTail_Order__orderBatch_Session[
	__Order__ _WorkspaceOrder_LifecycleCoordinator_,
](
	orderBatch_Session_result []_WorkspaceOrder_LifecycleCoordinator_,
	order__pending_current_the __Order__,
) []_WorkspaceOrder_LifecycleCoordinator_ {
	if 0 == len(orderBatch_Session_result) {
		return append(
			orderBatch_Session_result,
			order__pending_current_the,
		)
	}
	switch orderBatch_Session_result[len(orderBatch_Session_result)-1].(type) {
	case __Order__:
		orderBatch_Session_result[len(orderBatch_Session_result)-1] = order__pending_current_the
		return orderBatch_Session_result
	default:
		return append(
			orderBatch_Session_result,
			order__pending_current_the,
		)
	}
}

func (this _SpawnPty__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnSpawnPty__(
		this.Id_WebsocketConnection_expected,
		this.Message_SpawnPty,
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
		Visibility__Client_connected__state: VISIBLE___Visibility__Client_connected,
		Id:                                  id_WorkspacePty_new,
		State_state: &_State_Spawning__WorkspacePty_{
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

type _SpawnApi_PtyProxy_ struct {
	OnOutput_Live__PtyProxy__              func(id_WorkspacePty uint32, outputData_PtyDevice []byte)
	OnOutput_Snapshot__PtyProxy__          func(id_WorkspacePty uint32, outputData_PtyTerminal []byte)
	OnOutput_PostSnapshot__PtyProxy__      func(id_WorkspacePty uint32, outputData_PostSnapshot []byte)
	OnExited_Eio_Success__PtyProxy__       func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Failure__PtyProxy__       func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Killed__PtyProxy__        func(ptyProxy *_PtyProxy_)
	OnExited_Closed__PtyProxy__            func(ptyProxy *_PtyProxy_)
	OnExited_SystemError__PtyProxy__       func(ptyProxy *_PtyProxy_, exitSignal_PtyReader error)
	Id_WorkspacePty                        uint32
	ColumnCount_PtyTerminal                int
	RowCount_PtyTerminal                   int
	Path_ShellBinary__PtyCommand           string
	DirectoryPath_PtyCommand               string
	EnvironmentVariables_PtyCommand        []string
	Size_StagingBuffer__PtyReader          int
	Count_ScrollbackLine__PtyTerminal      int
	Size_PostSnapshotBuffer__PtyProxy      int
	Size_QueueBuffer__InputOrder_PtyWriter int
}

func Spawn_PtyProxy(
	api _SpawnApi_PtyProxy_,
) (*_PtyProxy_, error) {
	ptyCommand_PtyProxy_result := _EXEC.Command(api.Path_ShellBinary__PtyCommand)
	ptyCommand_PtyProxy_result.Env = api.EnvironmentVariables_PtyCommand
	ptyCommand_PtyProxy_result.Dir = api.DirectoryPath_PtyCommand
	fileDescriptor_master__PtyDevice, error_start__PtyCommand__maybe := _PTY.Start(ptyCommand_PtyProxy_result)
	if error_start__PtyCommand__maybe != nil {
		return nil, error_start__PtyCommand__maybe
	}
	ptyProxy_result := &_PtyProxy_{
		OnOutput_Live__:                  api.OnOutput_Live__PtyProxy__,
		OnOutput_Snapshot__:              api.OnOutput_Snapshot__PtyProxy__,
		OnOutput_PostSnapshot__:          api.OnOutput_PostSnapshot__PtyProxy__,
		OnExited_Eio_Success__:           api.OnExited_Eio_Success__PtyProxy__,
		OnExited_Eio_Failure__:           api.OnExited_Eio_Failure__PtyProxy__,
		OnExited_Eio_Killed__:            api.OnExited_Eio_Killed__PtyProxy__,
		OnExited_Closed__:                api.OnExited_Closed__PtyProxy__,
		OnExited_SystemError__:           api.OnExited_SystemError__PtyProxy__,
		Id_WorkspacePty:                  api.Id_WorkspacePty,
		Mode_state:                       SPAWNING__Mode_PtyProxy,
		PtyCommand:                       ptyCommand_PtyProxy_result,
		FileDescriptor_Master__PtyDevice: fileDescriptor_master__PtyDevice,
		PtyTerminal:                      nil,
		PtyReader:                        nil,
		PtyWriter:                        nil,
		PostSnapshotBuffer:               nil,
	}
	ptyProxy_result.PtyReader = &_PtyReader_{
		OnTryFlush__:                       ptyProxy_result.HandleTryFlush,
		OnBlockingFlush__:                  ptyProxy_result.HandleBlockingFlush,
		OnExited_Closed__:                  ptyProxy_result.HandleExited_Closed,
		OnExited_Eio__:                     ptyProxy_result.HandleExited_Eio,
		OnExited_SystemError__:             ptyProxy_result.HandleExited_SystemError,
		StagingBuffer:                      make([]byte, api.Size_StagingBuffer__PtyReader),
		Size_UnflushedSlice__StagingBuffer: 0,
		FileDescriptor_Master__PtyDevice:   fileDescriptor_master__PtyDevice,
	}
	workerContext_PtyWriter, workerCancel_PtyWriter := _CONTEXT.WithCancel(_CONTEXT.Background())
	ptyProxy_result.PtyWriter = &_PtyWriter_{
		QueueChannel_InputOrder:          make(chan _InputOrder_PtyWriter_, api.Size_QueueBuffer__InputOrder_PtyWriter),
		WorkerContext:                    workerContext_PtyWriter,
		WorkerCancel:                     workerCancel_PtyWriter,
		FileDescriptor_Master__PtyDevice: fileDescriptor_master__PtyDevice,
	}
	ptyProxy_result.PostSnapshotBuffer = _BYTES.NewBuffer(
		make([]byte, 0, api.Size_PostSnapshotBuffer__PtyProxy),
	)
	ptyProxy_result.PtyTerminal = _XTERM.New(
		_XTERM.WithCols(api.ColumnCount_PtyTerminal),
		_XTERM.WithRows(api.RowCount_PtyTerminal),
		_XTERM.WithScrollback(api.Count_ScrollbackLine__PtyTerminal),
	)
	return ptyProxy_result, nil
}

func (this _Status_SpawnPty__Success__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnStatus_SpawnPty__Success__(
		this.Id_WorkspacePty_spawned,
		this.PtyProxy_quiescent,
	)
}

func (This *_WorkspaceController_) HandleStatus_SpawnPty__Success__Coordinator(
	id_WorkspacePty_spawned uint32,
	ptyProxy_quiescent *_PtyProxy_,
) {
	This.Mutex.Lock()
	WorkspacePty_target := This.PtyPool[id_WorkspacePty_spawned]
	State__WorkspacePty_target__spawning := WorkspacePty_target.State_state.(*_State_Spawning__WorkspacePty_)
	State__WorkspacePty_target__spawning.Mutex.Lock()
	cancellationStatus_captured := State__WorkspacePty_target__spawning.CancellationStatus
	resizeGeometry__deferred_maybe := State__WorkspacePty_target__spawning.ResizeGeometry__deferred_maybe
	State__WorkspacePty_target__spawning.Mutex.Unlock()
	if CANCELED____CancellationStatus___State_Spawning__WorkspacePty == cancellationStatus_captured {
		delete(This.PtyPool, id_WorkspacePty_spawned)
	}
	visibility__Client_connected__current := WorkspacePty_target.Visibility__Client_connected__state
	This.Mutex.Unlock()
	if CANCELED____CancellationStatus___State_Spawning__WorkspacePty == cancellationStatus_captured {
		ptyProxy_quiescent.Terminate_PtyProcess(int(_SYSCALL.SIGTERM))
		Emit__PtyMessage_Egress__WebsocketController_Pty(
			This.WorkspaceNetwork.WebsocketController_Pty,
			This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_state,
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
	This.WorkspaceNetwork.WebsocketController_Pty.Mutex.Lock()
	status_WebsocketConnection_captured := This.WorkspaceNetwork.WebsocketController_Pty.Status_WebsocketConnection_state
	This.WorkspaceNetwork.WebsocketController_Pty.Mutex.Unlock()
	This.Mutex.Lock()
	if CONNECTED__Status_WebsocketConnection == status_WebsocketConnection_captured && VISIBLE___Visibility__Client_connected == visibility__Client_connected__current {
		ptyProxy_quiescent.Mode_state = LIVE_RUNNING__Mode_PtyProxy
	} else {
		ptyProxy_quiescent.Mode_state = PRE_SNAPSHOT__RUNNING___Mode_PtyProxy
	}
	WorkspacePty_target.State_state = &_State_Active__WorkspacePty_{
		PtyProxy: ptyProxy_quiescent,
	}
	This.Mutex.Unlock()
	go ptyProxy_quiescent.PtyReader.RunWorker()
	go ptyProxy_quiescent.PtyWriter.RunWorker()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_state,
		_Status_SpawnPty__PtyMessage_Egress_{
			Status_SpawnPty: SUCCESS__Status_SpawnPty,
			Id_PtyProxy:     id_WorkspacePty_spawned,
		},
	)
}

func (this _Status_SpawnPty__Failure__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnStatus_SpawnPty__Failure__(
		this.Id_WorkspacePty_failed,
		this.Error_Start__PtyCommand,
	)
}

func (This *_WorkspaceController_) HandleStatus_SpawnPty__Failure__Coordinator(
	id_WorkspacePty_failed uint32,
	error_start__PtyCommand error,
) {
	This.Mutex.Lock()
	WorkspacePty_target := This.PtyPool[id_WorkspacePty_failed]
	delete(This.PtyPool, id_WorkspacePty_failed)
	State__WorkspacePty_target__spawning := WorkspacePty_target.State_state.(*_State_Spawning__WorkspacePty_)
	This.Mutex.Unlock()
	State__WorkspacePty_target__spawning.Mutex.Lock()
	cancellationStatus_captured := State__WorkspacePty_target__spawning.CancellationStatus
	State__WorkspacePty_target__spawning.Mutex.Unlock()
	switch cancellationStatus_captured {
	case CANCELED____CancellationStatus___State_Spawning__WorkspacePty:
		Emit__PtyMessage_Egress__WebsocketController_Pty(
			This.WorkspaceNetwork.WebsocketController_Pty,
			This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_state,
			_Status_SpawnPty__PtyMessage_Egress_{
				Status_SpawnPty: CANCELED__Status_SpawnPty,
				Id_PtyProxy:     id_WorkspacePty_failed,
			},
		)
	case NOT_CANCELED____CancellationStatus___State_Spawning__WorkspacePty:
		Emit__PtyMessage_Egress__WebsocketController_Pty(
			This.WorkspaceNetwork.WebsocketController_Pty,
			This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_state,
			_Status_SpawnPty__PtyMessage_Egress_{
				Status_SpawnPty: FAILURE__Status_SpawnPty,
				Id_PtyProxy:     id_WorkspacePty_failed,
			},
		)
	default:
		panic("invalid path: _WorkspaceController_ HandleStatus_SpawnPty__Failure__Coordinator")
	}
}

func (this _Connect__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnConnect_PtyWebsocket__(this.Id_WebsocketConnection_expected)
}

func (This *_WorkspaceController_) HandleConnect_PtyWebsocket__Coordinator(
	id_WebsocketConnection_expected uint64,
) {
	This.Mutex.Lock()
	bulletinBatch_WorkspacePty_result := make([]_Bulletin_WorkspacePty_, 0, len(This.PtyPool))
	for _, WorkspacePty_some := range This.PtyPool {
		bulletin_WorkspacePty_result := _Bulletin_WorkspacePty_{
			Id_WorkspacePty:              WorkspacePty_some.Id,
			Visibility__Client_connected: WorkspacePty_some.Visibility__Client_connected__state,
			Status_WorkspacePty:          0, // nil
			ExitOutcome_PtyProxy_maybe:   nil,
		}
		WorkspacePty_some.State_state.Update_ManifestBulletin(&bulletin_WorkspacePty_result)
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

func (This *_State_Spawning__WorkspacePty_) Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = SPAWNING__Status_WorkspacePty
}

func (this *_State_Active__WorkspacePty_) Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = ACTIVE__Status_WorkspacePty
}

func (this *_State_Exited__WorkspacePty_) Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = EXITED__Status_WorkspacePty
	bulletin_result.ExitOutcome_PtyProxy_maybe = this.ExitOutcome
}

func (this _Disconnect__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnDisconnect_PtyWebsocket__()
}

func (This *_WorkspaceController_) HandleDisconnect_PtyWebsocket__Coordinator() {
	This.Mutex.Lock()
	ptyPool_cloned := _MAPS.Clone(This.PtyPool)
	for _, WorkspacePty_some := range ptyPool_cloned {
		WorkspacePty_some.Visibility__Client_connected__state = DISCONNECTED_UNKNOWN___Visibility__Client_connected
	}
	This.Mutex.Unlock()
	for _, WorkspacePty_some := range ptyPool_cloned {
		WorkspacePty_some.State_state.HandleDisconnect_Coordinator()
	}
}

func (This *_State_Spawning__WorkspacePty_) HandleDisconnect_Coordinator() {
}

func (this *_State_Active__WorkspacePty_) HandleDisconnect_Coordinator() {
	switch this.PtyProxy.Mode_state {
	case LIVE_RUNNING__Mode_PtyProxy:
		this.PtyProxy.TransitionMode_LiveToPreSnapshot()
	case POST_SNAPSHOT__RUNNING___Mode_PtyProxy:
		this.PtyProxy.TransitionMode_PostSnapshotToPreSnapshot()
	}
}

func (this *_State_Exited__WorkspacePty_) HandleDisconnect_Coordinator() {
}

func (This *_PtyProxy_) TransitionMode_LiveToPreSnapshot() {
	This.Mutex.Lock()
	This.Mode_state = PRE_SNAPSHOT__RUNNING___Mode_PtyProxy
	This.Mutex.Unlock()
}

func (This *_PtyProxy_) TransitionMode_PostSnapshotToPreSnapshot() {
	This.Mutex.Lock()
	This.Mode_state = PRE_SNAPSHOT__RUNNING___Mode_PtyProxy
	This.Mutex.Unlock()
}

func (this _Sync__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnSync_PtyPool__(
		this.Id_WebsocketConnection_expected,
		this.Message__Batch_SyncPty,
	)
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
		visibility__Client_connected__previous := WorkspacePty_some.Visibility__Client_connected__state
		if order_SyncPty_maybe != nil {
			WorkspacePty_some.Visibility__Client_connected__state = VISIBLE___Visibility__Client_connected
		} else if nil == order_SyncPty_maybe {
			WorkspacePty_some.Visibility__Client_connected__state = NOT_VISIBLE___Visibility__Client_connected
		}
		State__WorkspacePty_some__current := WorkspacePty_some.State_state
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

func (This *_State_Spawning__WorkspacePty_) HandleSync_Coordinator(
	order_SyncPty_maybe *_Order_SyncPty_,
	_ _Visibility__Client_connected_,
	_ *_WebsocketController_,
	_ uint64,
	_ uint32,
) {
	if order_SyncPty_maybe != nil {
		This.Mutex.Lock()
		This.ResizeGeometry__deferred_maybe = &_ResizeGeometry___State_Spawning__WorkspacePty_{
			ColumnCount_PtyTerminal: order_SyncPty_maybe.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:    order_SyncPty_maybe.RowCount_PtyTerminal,
		}
		This.Mutex.Unlock()
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

func (This *_PtyProxy_) TransitionMode_PreToPostSnapshot() {
	This.Mutex.Lock()
	This.PostSnapshotBuffer.Reset()
	This.Mode_state = POST_SNAPSHOT__RUNNING___Mode_PtyProxy
	serializeAddon := _XTERM.NewSerializeAddon(This.PtyTerminal)
	outputData_PtyTerminal := serializeAddon.Serialize(nil)
	This.Mutex.Unlock()
	if len(outputData_PtyTerminal) > 0 {
		This.OnOutput_Snapshot__(
			This.Id_WorkspacePty,
			outputData_PtyTerminal,
		)
	}
}

func (This *_PtyProxy_) TransitionMode_PostSnapshotToLive() {
	This.Mutex.Lock()
	outputData_PostSnapshot := _BYTES.Clone(This.PostSnapshotBuffer.Bytes())
	This.Mode_state = LIVE_RUNNING__Mode_PtyProxy
	This.Mutex.Unlock()
	if len(outputData_PostSnapshot) > 0 {
		This.OnOutput_PostSnapshot__(
			This.Id_WorkspacePty,
			outputData_PostSnapshot,
		)
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

func (This *_PtyProxy_) EmitSnapshot_Exited() {
	This.Mutex.Lock()
	serializeAddon := _XTERM.NewSerializeAddon(This.PtyTerminal)
	outputData_PtyTerminal := serializeAddon.Serialize(nil)
	This.Mutex.Unlock()
	if len(outputData_PtyTerminal) > 0 {
		This.OnOutput_Snapshot__(
			This.Id_WorkspacePty,
			outputData_PtyTerminal,
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

func (this _ExitPty__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnExit_PtyProxy__(
		this.Id_WorkspacePty_exited,
		this.ExitOutcome_PtyProxy,
	)
}

func (This *_WorkspaceController_) HandleExit_PtyProxy__Coordinator(
	id_WorkspacePty_exited uint32,
	exitOutcome_PtyProxy _ExitOutcome_PtyProxy_,
) {
	This.Mutex.Lock()
	WorkspacePty_target := This.PtyPool[id_WorkspacePty_exited]
	State__WorkspacePty_target__active := WorkspacePty_target.State_state.(*_State_Active__WorkspacePty_)
	This.Mutex.Unlock()
	State__WorkspacePty_target__active.PtyProxy.TransitionMode_ToExited()
	State__WorkspacePty_target__active.PtyProxy.PtyWriter.WorkerCancel()
	This.Mutex.Lock()
	WorkspacePty_target.State_state = &_State_Exited__WorkspacePty_{
		PtyProxy:    State__WorkspacePty_target__active.PtyProxy,
		ExitOutcome: exitOutcome_PtyProxy,
	}
	This.Mutex.Unlock()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_state,
		_PtyExit__PtyMessage_Egress_{
			Id_PtyProxy:          id_WorkspacePty_exited,
			ExitOutcome_PtyProxy: exitOutcome_PtyProxy,
		},
	)
}

func (This *_PtyProxy_) TransitionMode_ToExited() {
	This.Mode_state = EXITED__Mode_PtyProxy
}
