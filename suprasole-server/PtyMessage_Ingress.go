package main

import (
	_ERRORS "errors"
)

var MAP__DECODE_PAYLOAD___PTY_MESSAGE__INGRESS = map[_Code__PtyMessage_Ingress_]func(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error){
	SPAWN_PTY___Code__PtyMessage_Ingress:       decodePayload_SpawnPty,
	RESIZE_PTYS___Code__PtyMessage_Ingress:     decodePayload_ResizePtys,
	TERMINATE_PTY___Code__PtyMessage_Ingress:   decodePayload_TerminatePty,
	REMOVE_PTY___Code__PtyMessage_Ingress:      decodePayload_RemovePty,
	WRITE_PTY_INPUT___Code__PtyMessage_Ingress: decodePayload_WritePtyInput,
	SYNC_WORKSPACE___Code__PtyMessage_Ingress:  decodePayload_SyncWorkspace,
}

type _Code__PtyMessage_Ingress_ uint16

const (
	SPAWN_PTY___Code__PtyMessage_Ingress       _Code__PtyMessage_Ingress_ = 0x0001
	RESIZE_PTYS___Code__PtyMessage_Ingress     _Code__PtyMessage_Ingress_ = 0x0003
	TERMINATE_PTY___Code__PtyMessage_Ingress   _Code__PtyMessage_Ingress_ = 0x0004
	REMOVE_PTY___Code__PtyMessage_Ingress      _Code__PtyMessage_Ingress_ = 0x0006
	WRITE_PTY_INPUT___Code__PtyMessage_Ingress _Code__PtyMessage_Ingress_ = 0x0007
	SYNC_WORKSPACE___Code__PtyMessage_Ingress  _Code__PtyMessage_Ingress_ = 0x000a
)

type _PtyMessage_Ingress_ interface {
	Execute(
		workspaceController *_WorkspaceController_,
		id_WebsocketConnection uint64,
	)
}

func decodePayload_SpawnPty(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_SpawnPty := New__BinaryDecoder_WebsocketMessage(
		_NewApi__BinaryDecoder_WebsocketMessage_{
			Buffer__Payload_BinaryMessage: payload_binaryMessage,
			Cursor_Buffer:                 2,
			Label_MessageStruct:           "SpawnPty",
		},
	)
	columnCount_PtyTerminal := int(binaryDecoder_SpawnPty.DecodeParameter_Uint16("ColumnCount_PtyTerminal"))
	rowCount_PtyTerminal := int(binaryDecoder_SpawnPty.DecodeParameter_Uint16("RowCount_PtyTerminal"))
	shellBinaryPath_PtyCommand := binaryDecoder_SpawnPty.DecodeParameter_String16("ShellBinaryPath_PtyCommand")
	directoryPath_PtyCommand := binaryDecoder_SpawnPty.DecodeParameter_String16("DirectoryPath_PtyCommand")
	environmentVariables_PtyCommand := binaryDecoder_SpawnPty.DecodeParameter__Slice16_String16("EnvironmentVariables_PtyCommand")
	options_PtyProxy := DecodeParameter__Slice16__BinaryDecoder_WebsocketMessage(
		&binaryDecoder_SpawnPty,
		"Options_PtyProxy",
		func(binaryDecoder_SpawnPty *_BinaryDecoder_WebsocketMessage_, currentSliceIndex int) _Option_PtyProxy_ {
			optionCode := _Code__Option_PtyProxy_(binaryDecoder_SpawnPty.DecodeParameter_Uint16("OptionCode"))
			optionValue := int(binaryDecoder_SpawnPty.DecodeParameter_Uint32("OptionValue"))
			switch optionCode {
			case SCROLLBACK_LINE_COUNT___Code__Option_PtyProxy:
				return _ScrollbackLineCount__Option_PtyProxy_{
					ScrollbackLineCount_PtyTerminal: optionValue,
				}
			case STAGING_BUFFER_SIZE___Code__Option_PtyProxy:
				return _StagingBufferSize__Option_PtyProxy_{
					StagingBufferSize_PtyReader: optionValue,
				}
			case POST_SNAPSHOT_BUFFER_SIZE___Code__Option_PtyProxy:
				return _PostSnapshotBufferSize__Option_PtyProxy_{
					PostSnapshotBufferSize_PtyProxy: optionValue,
				}
			case QUEUE_BUFFER_SIZE_INPUT_ORDER___Code__Option_PtyProxy:
				return _QueueBufferSize_InputOrder__PtyWriter___Option_PtyProxy_{
					QueueBufferSize_InputOrder__PtyWriter: optionValue,
				}
			default:
				binaryDecoder_SpawnPty.MaybeError_Earliest = _ERRORS.New("unrecognized pty proxy option code")
				return nil
			}
		},
	)
	binaryDecoder_SpawnPty.AssertEndOfPayload()
	if binaryDecoder_SpawnPty.MaybeError_Earliest != nil {
		return nil, binaryDecoder_SpawnPty.MaybeError_Earliest
	}
	return _SpawnPty__PtyMessage_Ingress_{
		ColumnCount_PtyTerminal:         columnCount_PtyTerminal,
		RowCount_PtyTerminal:            rowCount_PtyTerminal,
		ShellBinaryPath_PtyCommand:      shellBinaryPath_PtyCommand,
		DirectoryPath_PtyCommand:        directoryPath_PtyCommand,
		EnvironmentVariables_PtyCommand: environmentVariables_PtyCommand,
		Options_PtyProxy:                options_PtyProxy,
	}, nil
}

type _Code__Option_PtyProxy_ uint16

const (
	SCROLLBACK_LINE_COUNT___Code__Option_PtyProxy         _Code__Option_PtyProxy_ = 0x0001
	STAGING_BUFFER_SIZE___Code__Option_PtyProxy           _Code__Option_PtyProxy_ = 0x0002
	POST_SNAPSHOT_BUFFER_SIZE___Code__Option_PtyProxy     _Code__Option_PtyProxy_ = 0x0003
	QUEUE_BUFFER_SIZE_INPUT_ORDER___Code__Option_PtyProxy _Code__Option_PtyProxy_ = 0x0004
)

type _Option_PtyProxy_ interface {
	UpdateOptionsResult(
		optionsResult_ptyProxy *_Defaults_PtyProxy_,
	)
}

type _ScrollbackLineCount__Option_PtyProxy_ struct {
	ScrollbackLineCount_PtyTerminal int
}

func (this _ScrollbackLineCount__Option_PtyProxy_) UpdateOptionsResult(
	optionsResult_ptyProxy *_Defaults_PtyProxy_,
) {
	optionsResult_ptyProxy.ScrollbackLineCount_PtyTerminal__ = this.ScrollbackLineCount_PtyTerminal
}

type _StagingBufferSize__Option_PtyProxy_ struct {
	StagingBufferSize_PtyReader int
}

func (this _StagingBufferSize__Option_PtyProxy_) UpdateOptionsResult(
	optionsResult_ptyProxy *_Defaults_PtyProxy_,
) {
	optionsResult_ptyProxy.StagingBufferSize_PtyReader__ = this.StagingBufferSize_PtyReader
}

type _PostSnapshotBufferSize__Option_PtyProxy_ struct {
	PostSnapshotBufferSize_PtyProxy int
}

func (this _PostSnapshotBufferSize__Option_PtyProxy_) UpdateOptionsResult(
	optionsResult_ptyProxy *_Defaults_PtyProxy_,
) {
	optionsResult_ptyProxy.PostSnapshotBufferSize_PtyProxy__ = this.PostSnapshotBufferSize_PtyProxy
}

type _QueueBufferSize_InputOrder__PtyWriter___Option_PtyProxy_ struct {
	QueueBufferSize_InputOrder__PtyWriter int
}

func (this _QueueBufferSize_InputOrder__PtyWriter___Option_PtyProxy_) UpdateOptionsResult(
	optionsResult_ptyProxy *_Defaults_PtyProxy_,
) {
	optionsResult_ptyProxy.QueueBufferSize_InputOrder__PtyWriter__ = this.QueueBufferSize_InputOrder__PtyWriter
}

type _SpawnPty__PtyMessage_Ingress_ struct {
	ColumnCount_PtyTerminal         int
	RowCount_PtyTerminal            int
	ShellBinaryPath_PtyCommand      string
	DirectoryPath_PtyCommand        string
	EnvironmentVariables_PtyCommand []string
	Options_PtyProxy                []_Option_PtyProxy_
}

func (this _SpawnPty__PtyMessage_Ingress_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	optionsResult_ptyProxy := workspaceController.Defaults_PtyProxy__
	for _, option_ptyProxy := range this.Options_PtyProxy {
		option_ptyProxy.UpdateOptionsResult(&optionsResult_ptyProxy)
	}
	workspaceController.Mutex.Lock()
	newId_PtyProxy := workspaceController.NextId_PtyProxy
	workspaceController.NextId_PtyProxy++
	workspaceController.Mutex.Unlock()
	go func() {
		startError_PtyCommand := Spawn__PtyProxy(_SpawnApi__PtyProxy_{
			OnSpawned_PtyProxy__:                  workspaceController.HandleSpawned_Pty,
			OnOutput_Live__PtyProxy__:             workspaceController.HandleOutput_Pty,
			OnOutput_Snapshot__PtyProxy__:         workspaceController.HandleOutput_Pty,
			OnOutput_PostSnapshot__PtyProxy__:     workspaceController.HandleOutput_Pty,
			OnExited_Eio_Success__PtyProxy__:      workspaceController.HandleExited_Eio_Success__Pty,
			OnExited_Eio_Failure__PtyProxy__:      workspaceController.HandleExited_Eio_Failure__Pty,
			OnExited_Eio_Killed__PtyProxy__:       workspaceController.HandleExited_Eio_Killed__Pty,
			OnExited_Closed__PtyProxy__:           workspaceController.HandleExited_Closed__Pty,
			OnExited_SystemError__PtyProxy__:      workspaceController.HandleExited_SystemError__Pty,
			Id_PtyProxy:                           newId_PtyProxy,
			ColumnCount_PtyTerminal:               this.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:                  this.RowCount_PtyTerminal,
			ShellBinaryPath_PtyCommand:            this.ShellBinaryPath_PtyCommand,
			DirectoryPath_PtyCommand:              this.DirectoryPath_PtyCommand,
			EnvironmentVariables_PtyCommand:       this.EnvironmentVariables_PtyCommand,
			ScrollbackLineCount_PtyTerminal:       optionsResult_ptyProxy.ScrollbackLineCount_PtyTerminal__,
			StagingBufferSize_PtyReader:           optionsResult_ptyProxy.StagingBufferSize_PtyReader__,
			PostSnapshotBufferSize_PtyProxy:       optionsResult_ptyProxy.PostSnapshotBufferSize_PtyProxy__,
			QueueBufferSize_InputOrder__PtyWriter: optionsResult_ptyProxy.QueueBufferSize_InputOrder__PtyWriter__,
		})
		if startError_PtyCommand != nil {
			workspaceController.HandleSpawnFailed_Pty(newId_PtyProxy)
		}
	}()
}

func decodePayload_ResizePtys(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_ResizePtys := New__BinaryDecoder_WebsocketMessage(
		_NewApi__BinaryDecoder_WebsocketMessage_{
			Buffer__Payload_BinaryMessage: payload_binaryMessage,
			Cursor_Buffer:                 2,
			Label_MessageStruct:           "ResizePtys",
		},
	)
	resizePtyOrders := DecodeParameter__Slice16__BinaryDecoder_WebsocketMessage(
		&binaryDecoder_ResizePtys,
		"ResizePtyOrders",
		func(binaryDecoder_ResizePtys *_BinaryDecoder_WebsocketMessage_, currentSliceIndex int) _ResizePtyOrder_ResizePtys_ {
			id_PtyProxy := binaryDecoder_ResizePtys.DecodeParameter_Uint32("Id_PtyProxy")
			columnCount_PtyTerminal := int(binaryDecoder_ResizePtys.DecodeParameter_Uint16("ColumnCount_PtyTerminal"))
			rowCount_PtyTerminal := int(binaryDecoder_ResizePtys.DecodeParameter_Uint16("RowCount_PtyTerminal"))
			return _ResizePtyOrder_ResizePtys_{
				Id_PtyProxy:             id_PtyProxy,
				ColumnCount_PtyTerminal: columnCount_PtyTerminal,
				RowCount_PtyTerminal:    rowCount_PtyTerminal,
			}
		},
	)
	binaryDecoder_ResizePtys.AssertEndOfPayload()
	if binaryDecoder_ResizePtys.MaybeError_Earliest != nil {
		return nil, binaryDecoder_ResizePtys.MaybeError_Earliest
	}
	return _ResizePtys__PtyMessage_Ingress_{
		ResizePtyOrders: resizePtyOrders,
	}, nil
}

type _ResizePtyOrder_ResizePtys_ struct {
	Id_PtyProxy             uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

type _ResizePtys__PtyMessage_Ingress_ struct {
	ResizePtyOrders []_ResizePtyOrder_ResizePtys_
}

func (this _ResizePtys__PtyMessage_Ingress_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.MessageDebouncer_ResizePtys.QueueChannel <- this
}

func decodePayload_WritePtyInput(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_WritePtyInput := New__BinaryDecoder_WebsocketMessage(
		_NewApi__BinaryDecoder_WebsocketMessage_{
			Buffer__Payload_BinaryMessage: payload_binaryMessage,
			Cursor_Buffer:                 2,
			Label_MessageStruct:           "WritePtyInput",
		},
	)
	id_PtyProxy := binaryDecoder_WritePtyInput.DecodeParameter_Uint32("Id_PtyProxy")
	rawInputBytes := binaryDecoder_WritePtyInput.DecodeParameter_TrailingBytes("RawInputBytes")
	binaryDecoder_WritePtyInput.AssertEndOfPayload()
	if binaryDecoder_WritePtyInput.MaybeError_Earliest != nil {
		return nil, binaryDecoder_WritePtyInput.MaybeError_Earliest
	}
	return _WritePtyInput__PtyMessage_Ingress_{
		Id_PtyProxy: id_PtyProxy,
		InputOrder_PtyWriter: &_Passthrough__InputOrder_PtyWriter_{
			InputData_PtyDevice: rawInputBytes,
		},
	}, nil
}

type _WritePtyInput__PtyMessage_Ingress_ struct {
	Id_PtyProxy          uint32
	InputOrder_PtyWriter _InputOrder_PtyWriter_
}

func (this _WritePtyInput__PtyMessage_Ingress_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.Mutex.Lock()
	targetWorkspacePty := workspaceController.PtyPool[this.Id_PtyProxy]
	workspaceController.Mutex.Unlock()
	if targetWorkspacePty != nil {
		select {
		case <-targetWorkspacePty.PtyProxy.PtyWriter.WorkerContext.Done():
		default:
			select {
			case targetWorkspacePty.PtyProxy.PtyWriter.QueueChannel_InputOrder <- this.InputOrder_PtyWriter:
			default:
			}
		}
	}
}

func decodePayload_TerminatePty(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_TerminatePty := New__BinaryDecoder_WebsocketMessage(
		_NewApi__BinaryDecoder_WebsocketMessage_{
			Buffer__Payload_BinaryMessage: payload_binaryMessage,
			Cursor_Buffer:                 2,
			Label_MessageStruct:           "TerminatePty",
		},
	)
	id_PtyProxy := binaryDecoder_TerminatePty.DecodeParameter_Uint32("Id_PtyProxy")
	terminalSignal_PtyProcess := int(binaryDecoder_TerminatePty.DecodeParameter_Int32("TerminalSignal_PtyProcess"))
	binaryDecoder_TerminatePty.AssertEndOfPayload()
	if binaryDecoder_TerminatePty.MaybeError_Earliest != nil {
		return nil, binaryDecoder_TerminatePty.MaybeError_Earliest
	}
	return _TerminatePty__PtyMessage_Ingress_{
		Id_PtyProxy:               id_PtyProxy,
		TerminalSignal_PtyProcess: terminalSignal_PtyProcess,
	}, nil
}

type _TerminatePty__PtyMessage_Ingress_ struct {
	Id_PtyProxy               uint32
	TerminalSignal_PtyProcess int
}

func (this _TerminatePty__PtyMessage_Ingress_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.Mutex.Lock()
	targetWorkspacePty := workspaceController.PtyPool[this.Id_PtyProxy]
	workspaceController.Mutex.Unlock()
	if targetWorkspacePty != nil {
		targetWorkspacePty.PtyProxy.Terminate(this.TerminalSignal_PtyProcess)
	}
}

func decodePayload_RemovePty(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_RemovePty := New__BinaryDecoder_WebsocketMessage(
		_NewApi__BinaryDecoder_WebsocketMessage_{
			Buffer__Payload_BinaryMessage: payload_binaryMessage,
			Cursor_Buffer:                 2,
			Label_MessageStruct:           "RemovePty",
		},
	)
	ids_PtyPool := binaryDecoder_RemovePty.DecodeParameter__Slice16_Uint32("Ids_PtyPool")
	binaryDecoder_RemovePty.AssertEndOfPayload()
	if binaryDecoder_RemovePty.MaybeError_Earliest != nil {
		return nil, binaryDecoder_RemovePty.MaybeError_Earliest
	}
	return _RemovePty__PtyMessage_Ingress_{
		Ids_PtyPool: ids_PtyPool,
	}, nil
}

type _RemovePty__PtyMessage_Ingress_ struct {
	Ids_PtyPool []uint32
}

func (this _RemovePty__PtyMessage_Ingress_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.Mutex.Lock()
	for _, someId_PtyProxy := range this.Ids_PtyPool {
		targetWorkspacePty := workspaceController.PtyPool[someId_PtyProxy]
		if targetWorkspacePty != nil && targetWorkspacePty.MaybeExitOutcome_PtyProxy != nil {
			delete(
				workspaceController.PtyPool,
				someId_PtyProxy,
			)
		}
	}
	workspaceController.Mutex.Unlock()
}

func decodePayload_SyncWorkspace(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_SyncWorkspace := New__BinaryDecoder_WebsocketMessage(
		_NewApi__BinaryDecoder_WebsocketMessage_{
			Buffer__Payload_BinaryMessage: payload_binaryMessage,
			Cursor_Buffer:                 2,
			Label_MessageStruct:           "SyncWorkspace",
		},
	)
	syncPtyOrders := DecodeParameter__Map16__BinaryDecoder_WebsocketMessage(
		&binaryDecoder_SyncWorkspace,
		"SyncPtyOrders",
		func(binaryDecoder_SyncWorkspace *_BinaryDecoder_WebsocketMessage_, currentMapIndex int) (uint32, *_SyncPtyOrder_SyncWorkspace_) {
			id_PtyProxy := binaryDecoder_SyncWorkspace.DecodeParameter_Uint32("Id_PtyProxy")
			columnCount_PtyTerminal := int(binaryDecoder_SyncWorkspace.DecodeParameter_Uint16("ColumnCount_PtyTerminal"))
			rowCount_PtyTerminal := int(binaryDecoder_SyncWorkspace.DecodeParameter_Uint16("RowCount_PtyTerminal"))
			return id_PtyProxy, &_SyncPtyOrder_SyncWorkspace_{
				ColumnCount_PtyTerminal: columnCount_PtyTerminal,
				RowCount_PtyTerminal:    rowCount_PtyTerminal,
			}
		},
	)
	binaryDecoder_SyncWorkspace.AssertEndOfPayload()
	if binaryDecoder_SyncWorkspace.MaybeError_Earliest != nil {
		return nil, binaryDecoder_SyncWorkspace.MaybeError_Earliest
	}
	return _SyncWorkspace__PtyMessage_Ingress_{
		SyncPtyOrders: syncPtyOrders,
	}, nil
}

type _SyncPtyOrder_SyncWorkspace_ struct {
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

type _SyncWorkspace__PtyMessage_Ingress_ struct {
	SyncPtyOrders map[uint32]*_SyncPtyOrder_SyncWorkspace_
}

func (this _SyncWorkspace__PtyMessage_Ingress_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.LifecycleCoordinator_WorkspacePty.QueueChannel <- _Sync__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection: id_WebsocketConnection,
		Message_SyncWorkspace:  this,
	}
}
