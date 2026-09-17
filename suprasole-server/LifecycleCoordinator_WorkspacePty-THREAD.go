package main

import (
	_BYTES "bytes"
	_CONTEXT "context"
	_MAPS "maps"
	_EXEC "os/exec"
	_SLICES "slices"
	_SYSCALL "syscall"

	_PTY "github.com/creack/pty"
	_XTERM "github.com/gitpod-io/xterm-go"
)

func (this *_LifecycleCoordinator_WorkspacePty_) RunWorker() {
	var orderBatch__pending_reconciled_state []_WorkspaceOrder_LifecycleCoordinator_
	for {
		select {
		case <-this.WorkerContext.Done():
			return
		case order_leading := <-this.QueueChannel_WorkspaceOrder:
			orderBatch__pending_reconciled_state = append(
				orderBatch__pending_reconciled_state,
				order_leading,
			)
		}
		for len(orderBatch__pending_reconciled_state) > 0 {
			select {
			case <-this.WorkerContext.Done():
				return
			case order_next := <-this.QueueChannel_WorkspaceOrder:
				insert_WorkspaceOrder___orderBatch__pending_reconciled_state(
					&orderBatch__pending_reconciled_state,
					order_next,
				)
			default:
				order_active := orderBatch__pending_reconciled_state[0]
				orderBatch__pending_reconciled_state = orderBatch__pending_reconciled_state[1:]
				order_active.Execute(this)
			}
		}
	}
}

func insert_WorkspaceOrder___orderBatch__pending_reconciled_state(
	orderBatch__pending_reconciled_state *[]_WorkspaceOrder_LifecycleCoordinator_,
	order_next _WorkspaceOrder_LifecycleCoordinator_,
) {
	rank__order_next := reconciliationRank_WorkspaceOrder(order_next)
	for index__subjectOrder_current, subjectOrder_current := range *orderBatch__pending_reconciled_state {
		if reconciliationRank_WorkspaceOrder(subjectOrder_current) > rank__order_next {
			*orderBatch__pending_reconciled_state = _SLICES.Insert(
				*orderBatch__pending_reconciled_state,
				index__subjectOrder_current,
				order_next,
			)
			return
		}
	}
	*orderBatch__pending_reconciled_state = append(
		*orderBatch__pending_reconciled_state,
		order_next,
	)
}

func reconciliationRank_WorkspaceOrder(
	order _WorkspaceOrder_LifecycleCoordinator_,
) int {
	switch order.(type) {
	case _ExitPty__WorkspaceOrder_LifecycleCoordinator_:
		return 0
	case _TerminatePty__WorkspaceOrder_LifecycleCoordinator_:
		return 1
	case _SpawnPty__WorkspaceOrder_LifecycleCoordinator_:
		return 2
	case _Disconnect__WorkspaceOrder_LifecycleCoordinator_,
		_Connect__WorkspaceOrder_LifecycleCoordinator_,
		_SyncVisibility__WorkspaceOrder_LifecycleCoordinator_:
		return 3
	case _EmitSnapshot_PtyProxy__WorkspaceOrder_LifecycleCoordinator_:
		return 4
	case _Status_SpawnPty__Success__WorkspaceOrder_LifecycleCoordinator_,
		_Status_SpawnPty__Failure__WorkspaceOrder_LifecycleCoordinator_:
		return 5
	default:
		panic("invalid path: reconciliationRank_WorkspaceOrder")
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
	workerContext_PtyResizer, workerCancel_PtyResizer := _CONTEXT.WithCancel(_CONTEXT.Background())
	This.Mutex.Lock()
	id_WorkspacePty_new := This.Id_WorkspacePty_next
	This.Id_WorkspacePty_next++
	WorkspacePty_new := &_WorkspacePty_{
		Id: id_WorkspacePty_new,
		PtyResizer: &_PtyResizer_{
			QueueChannel__Order_PtyResizer: make(chan _Order_PtyResizer_, 16),
			BindChannel__PtyProxy_spawned:  make(chan *_PtyProxy_, 1),
			WorkerContext:                 workerContext_PtyResizer,
			WorkerCancel:                  workerCancel_PtyResizer,
		},
		Visibility__Client_connected__state: VISIBLE___Visibility__Client_connected,
		State_state: &_State_Spawning__WorkspacePty_{
			CancellationStatus: NOT_CANCELED____CancellationStatus___State_Spawning__WorkspacePty,
		},
	}
	This.PtyPool[id_WorkspacePty_new] = WorkspacePty_new
	go WorkspacePty_new.PtyResizer.RunWorker()
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
		id_WebsocketConnection_expected,
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

func (this _TerminatePty__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnTerminatePty__(
		this.Id_WebsocketConnection_expected,
		this.Id_WorkspacePty,
		this.TerminalSignal_PtyProcess,
	)
}

func (This *_WorkspaceController_) HandleTerminatePty__Coordinator(
	id_WebsocketConnection_expected uint64,
	id_WorkspacePty uint32,
	terminalSignal_PtyProcess int,
) {
	var PtyProxy__WorkspacePty_target__active_maybe *_PtyProxy_
	This.Mutex.Lock()
	if WorkspacePty_target := This.PtyPool[id_WorkspacePty]; WorkspacePty_target != nil {
		switch State__WorkspacePty_target__the := WorkspacePty_target.State_state.(type) {
		case *_State_Spawning__WorkspacePty_:
			State__WorkspacePty_target__the.CancellationStatus = CANCELED____CancellationStatus___State_Spawning__WorkspacePty
		case *_State_Active__WorkspacePty_:
			PtyProxy__WorkspacePty_target__active_maybe = State__WorkspacePty_target__the.PtyProxy
		case *_State_Exited__WorkspacePty_:
			// Valid invocation: A client termination request crosses in-flight across the network with natural process exit on the server.
			// No action required: The process has already exited and its exit outcome is finalized.
		default:
			panic("invalid path: _WorkspaceController_ HandleTerminatePty__Coordinator")
		}
	}
	This.Mutex.Unlock()
	if PtyProxy__WorkspacePty_target__active_maybe != nil {
		go PtyProxy__WorkspacePty_target__active_maybe.Terminate_PtyProcess(terminalSignal_PtyProcess)
	}
}

func (This *_PtyProxy_) Terminate_PtyProcess(
	terminalSignal_PtyProcess int,
) {
	syscallSignal_PtyProcess := _SYSCALL.Signal(terminalSignal_PtyProcess)
	error_kill__PtyProcess__maybe := _SYSCALL.Kill(
		-This.PtyCommand.Process.Pid,
		syscallSignal_PtyProcess,
	)
	if error_kill__PtyProcess__maybe != nil {
		_ = This.PtyCommand.Process.Signal(syscallSignal_PtyProcess)
	}
}

func (This *_PtyProxy_) TerminateCancelled_PtyProxy() {
	This.Terminate_PtyProcess(
		int(_SYSCALL.SIGTERM),
	)
	_ = This.FileDescriptor_Master__PtyDevice.Close()
	_ = This.PtyCommand.Wait()
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
	id_WebsocketConnection_expected uint64,
	spawnApi_PtyProxy _SpawnApi_PtyProxy_,
) {
	ptyProxy_quiescent, error_start__PtyCommand__maybe := Spawn_PtyProxy(spawnApi_PtyProxy)
	if error_start__PtyCommand__maybe != nil {
		LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Status_SpawnPty__Failure__WorkspaceOrder_LifecycleCoordinator_{
			Id_WebsocketConnection_expected: id_WebsocketConnection_expected,
			Id_WorkspacePty_failed:          spawnApi_PtyProxy.Id_WorkspacePty,
			Error_Start__PtyCommand:         error_start__PtyCommand__maybe,
		}
		return
	}
	LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Status_SpawnPty__Success__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: id_WebsocketConnection_expected,
		Id_WorkspacePty_spawned:         spawnApi_PtyProxy.Id_WorkspacePty,
		PtyProxy_quiescent:              ptyProxy_quiescent,
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
	fileDescriptor_master__PtyDevice, error_start__PtyCommand__maybe := _PTY.StartWithSize(
		ptyCommand_PtyProxy_result,
		&_PTY.Winsize{
			Rows: uint16(api.RowCount_PtyTerminal),
			Cols: uint16(api.ColumnCount_PtyTerminal),
		},
	)
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
		this.Id_WebsocketConnection_expected,
		this.Id_WorkspacePty_spawned,
		this.PtyProxy_quiescent,
	)
}

func (This *_WorkspaceController_) HandleStatus_SpawnPty__Success__Coordinator(
	id_WebsocketConnection_expected uint64,
	id_WorkspacePty_spawned uint32,
	ptyProxy_quiescent *_PtyProxy_,
) {
	This.Mutex.Lock()
	WorkspacePty_target := This.PtyPool[id_WorkspacePty_spawned]
	State__WorkspacePty_target__spawning := WorkspacePty_target.State_state.(*_State_Spawning__WorkspacePty_)
	cancellationStatus_captured := State__WorkspacePty_target__spawning.CancellationStatus
	if CANCELED____CancellationStatus___State_Spawning__WorkspacePty == cancellationStatus_captured {
		delete(This.PtyPool, id_WorkspacePty_spawned)
	}
	visibility__Client_connected__current := WorkspacePty_target.Visibility__Client_connected__state
	This.Mutex.Unlock()
	if CANCELED____CancellationStatus___State_Spawning__WorkspacePty == cancellationStatus_captured {
		WorkspacePty_target.PtyResizer.WorkerCancel()
		go ptyProxy_quiescent.TerminateCancelled_PtyProxy()
		Emit__PtyMessage_Egress__WebsocketController_Pty(
			This.WorkspaceNetwork.WebsocketController_Pty,
			id_WebsocketConnection_expected,
			_Status_SpawnPty__PtyMessage_Egress_{
				Status_SpawnPty: CANCELED__Status_SpawnPty,
				Id_PtyProxy:     id_WorkspacePty_spawned,
			},
		)
		return
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
	WorkspacePty_target.PtyResizer.BindChannel__PtyProxy_spawned <- ptyProxy_quiescent
	go ptyProxy_quiescent.PtyWriter.RunWorker()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		id_WebsocketConnection_expected,
		_Status_SpawnPty__PtyMessage_Egress_{
			Status_SpawnPty: SUCCESS__Status_SpawnPty,
			Id_PtyProxy:     id_WorkspacePty_spawned,
		},
	)
	go ptyProxy_quiescent.PtyReader.RunWorker()
}

func (this _Status_SpawnPty__Failure__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnStatus_SpawnPty__Failure__(
		this.Id_WebsocketConnection_expected,
		this.Id_WorkspacePty_failed,
		this.Error_Start__PtyCommand,
	)
}

func (This *_WorkspaceController_) HandleStatus_SpawnPty__Failure__Coordinator(
	id_WebsocketConnection_expected uint64,
	id_WorkspacePty_failed uint32,
	error_start__PtyCommand error,
) {
	This.Mutex.Lock()
	WorkspacePty_target := This.PtyPool[id_WorkspacePty_failed]
	delete(This.PtyPool, id_WorkspacePty_failed)
	State__WorkspacePty_target__spawning := WorkspacePty_target.State_state.(*_State_Spawning__WorkspacePty_)
	cancellationStatus_captured := State__WorkspacePty_target__spawning.CancellationStatus
	This.Mutex.Unlock()
	WorkspacePty_target.PtyResizer.WorkerCancel()
	switch cancellationStatus_captured {
	case CANCELED____CancellationStatus___State_Spawning__WorkspacePty:
		Emit__PtyMessage_Egress__WebsocketController_Pty(
			This.WorkspaceNetwork.WebsocketController_Pty,
			id_WebsocketConnection_expected,
			_Status_SpawnPty__PtyMessage_Egress_{
				Status_SpawnPty: CANCELED__Status_SpawnPty,
				Id_PtyProxy:     id_WorkspacePty_failed,
			},
		)
	case NOT_CANCELED____CancellationStatus___State_Spawning__WorkspacePty:
		Emit__PtyMessage_Egress__WebsocketController_Pty(
			This.WorkspaceNetwork.WebsocketController_Pty,
			id_WebsocketConnection_expected,
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

func (this _SyncVisibility__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnSyncVisibility__(
		this.Id_WebsocketConnection_expected,
		this.TargetBatch_pending,
	)
}

func (This *_WorkspaceController_) HandleSyncVisibility__Coordinator(
	id_WebsocketConnection_expected uint64,
	targetBatch_pending map[uint32]*_Target__GeometryUpdate_PtyProxy_,
) {
	This.WorkspaceNetwork.WebsocketController_Pty.Mutex.Lock()
	status_WebsocketConnection_captured := This.WorkspaceNetwork.WebsocketController_Pty.Status_WebsocketConnection_state
	id_WebsocketConnection_captured := This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_state
	This.WorkspaceNetwork.WebsocketController_Pty.Mutex.Unlock()

	if CONNECTED__Status_WebsocketConnection != status_WebsocketConnection_captured ||
		id_WebsocketConnection_captured != id_WebsocketConnection_expected {
		return
	}

	var ptyProxyList_transitionToPreSnapshot []*_PtyProxy_
	This.Mutex.Lock()
	for _, WorkspacePty_some := range This.PtyPool {
		if _, isVisibleInSync := targetBatch_pending[WorkspacePty_some.Id]; isVisibleInSync {
			WorkspacePty_some.Visibility__Client_connected__state = VISIBLE___Visibility__Client_connected
		} else {
			if VISIBLE___Visibility__Client_connected == WorkspacePty_some.Visibility__Client_connected__state {
				WorkspacePty_some.Visibility__Client_connected__state = NOT_VISIBLE___Visibility__Client_connected
				if State__WorkspacePty_active_maybe, ok := WorkspacePty_some.State_state.(*_State_Active__WorkspacePty_); ok {
					ptyProxyList_transitionToPreSnapshot = append(
						ptyProxyList_transitionToPreSnapshot,
						State__WorkspacePty_active_maybe.PtyProxy,
					)
				}
			}
		}
	}
	This.Mutex.Unlock()

	for _, ptyProxy_some := range ptyProxyList_transitionToPreSnapshot {
		ptyProxy_some.TransitionMode_LiveToPreSnapshot()
	}
}

func (this _EmitSnapshot_PtyProxy__WorkspaceOrder_LifecycleCoordinator_) Execute(
	lifecycleCoordinator *_LifecycleCoordinator_WorkspacePty_,
) {
	lifecycleCoordinator.OnEmitSnapshot_PtyProxy__(
		this.Id_WebsocketConnection_expected,
		this.Id_WorkspacePty,
	)
}

func (This *_WorkspaceController_) HandleEmitSnapshot_PtyProxy__Coordinator(
	id_WebsocketConnection_expected uint64,
	id_WorkspacePty uint32,
) {
	var visibility__Client_connected__current _Visibility__Client_connected_
	var State__WorkspacePty_target__current _State_WorkspacePty_
	This.Mutex.Lock()
	if WorkspacePty_target := This.PtyPool[id_WorkspacePty]; WorkspacePty_target != nil {
		visibility__Client_connected__current = WorkspacePty_target.Visibility__Client_connected__state
		State__WorkspacePty_target__current = WorkspacePty_target.State_state
	}
	This.Mutex.Unlock()

	if nil == State__WorkspacePty_target__current || VISIBLE___Visibility__Client_connected != visibility__Client_connected__current {
		return
	}
	switch State__WorkspacePty_target__the := State__WorkspacePty_target__current.(type) {
	case *_State_Active__WorkspacePty_:
		__emitSnapshot_SyncPty__WebsocketController_Pty(
			State__WorkspacePty_target__the.PtyProxy.TransitionMode_PreToPostSnapshot,
			This.WorkspaceNetwork.WebsocketController_Pty,
			id_WebsocketConnection_expected,
			id_WorkspacePty,
		)
		State__WorkspacePty_target__the.PtyProxy.TransitionMode_PostSnapshotToLive()
	case *_State_Exited__WorkspacePty_:
		__emitSnapshot_SyncPty__WebsocketController_Pty(
			State__WorkspacePty_target__the.PtyProxy.EmitSnapshot_Exited,
			This.WorkspaceNetwork.WebsocketController_Pty,
			id_WebsocketConnection_expected,
			id_WorkspacePty,
		)
	case *_State_Spawning__WorkspacePty_:
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
	This.WorkspaceNetwork.WebsocketController_Pty.Mutex.Lock()
	id_WebsocketConnection_captured := This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_state
	This.WorkspaceNetwork.WebsocketController_Pty.Mutex.Unlock()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		id_WebsocketConnection_captured,
		_PtyExit__PtyMessage_Egress_{
			Id_PtyProxy:          id_WorkspacePty_exited,
			ExitOutcome_PtyProxy: exitOutcome_PtyProxy,
		},
	)
}

func (This *_PtyProxy_) TransitionMode_ToExited() {
	This.Mode_state = EXITED__Mode_PtyProxy
}
