package main

import (
	_ERRORS "errors"
)

type Code_PtyMessage_Ingress uint16

const (
	SPAWN_PTY__Code_PtyMessage_Ingress       Code_PtyMessage_Ingress = 0x0001
	RESIZE_PTYS__Code_PtyMessage_Ingress     Code_PtyMessage_Ingress = 0x0003
	TERMINATE_PTY__Code_PtyMessage_Ingress   Code_PtyMessage_Ingress = 0x0004
	REMOVE_PTY__Code_PtyMessage_Ingress      Code_PtyMessage_Ingress = 0x0006
	WRITE_PTY_INPUT__Code_PtyMessage_Ingress Code_PtyMessage_Ingress = 0x0007
	SYNC_WORKSPACE__Code_PtyMessage_Ingress  Code_PtyMessage_Ingress = 0x000a
)

type Code_PtyProxyOption uint16

const (
	SCROLLBACK_LINE_COUNT__Code_PtyProxyOption         Code_PtyProxyOption = 0x0001
	STAGING_BUFFER_SIZE__Code_PtyProxyOption           Code_PtyProxyOption = 0x0002
	POST_SNAPSHOT_BUFFER_SIZE__Code_PtyProxyOption     Code_PtyProxyOption = 0x0003
	QUEUE_BUFFER_SIZE_INPUT_ORDER__Code_PtyProxyOption Code_PtyProxyOption = 0x0004
)

type _PtyMessage_Ingress_ interface {
	Execute(workspaceController *_WorkspaceController_, id_WebsocketConnection uint64)
}

var DECODE_MESSAGE_MAP__PTY_MESSAGE_INGRESS = map[Code_PtyMessage_Ingress]func(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error){
	SPAWN_PTY__Code_PtyMessage_Ingress:       decodeMessage_SpawnPty,
	RESIZE_PTYS__Code_PtyMessage_Ingress:     decodeMessage_ResizePtys,
	TERMINATE_PTY__Code_PtyMessage_Ingress:   decodeMessage_TerminatePty,
	REMOVE_PTY__Code_PtyMessage_Ingress:      decodeMessage_RemovePty,
	WRITE_PTY_INPUT__Code_PtyMessage_Ingress: decodeMessage_WritePtyInput,
	SYNC_WORKSPACE__Code_PtyMessage_Ingress:  decodeMessage_SyncWorkspace,
}

func decodeMessage_SpawnPty(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_SpawnPty := New__BinaryDecoder_WebsocketMessage(
		_NewApi__BinaryDecoder_WebsocketMessage_{
			Buffer_BinaryMessageFrame: binaryMessageFrame,
			Cursor_Buffer:             2,
			Label_MessageStruct:       "SpawnPty",
		},
	)
	__ColumnCount_PtyTerminal := int(binaryDecoder_SpawnPty.DecodeParameter_Uint16("ColumnCount_PtyTerminal"))
	__RowCount_PtyTerminal := int(binaryDecoder_SpawnPty.DecodeParameter_Uint16("RowCount_PtyTerminal"))
	__ShellBinaryPath_PtyCommand := binaryDecoder_SpawnPty.DecodeParameter_String16("ShellBinaryPath_PtyCommand")
	__DirectoryPath_PtyCommand := binaryDecoder_SpawnPty.DecodeParameter_String16("DirectoryPath_PtyCommand")
	__EnvironmentVariables_PtyCommand := binaryDecoder_SpawnPty.DecodeParameter__Slice16_String16("EnvironmentVariables_PtyCommand")
	__Options_PtyProxy := DecodeParameter__Slice16__BinaryDecoder_WebsocketMessage(
		&binaryDecoder_SpawnPty,
		"Options_PtyProxy",
		func(binaryDecoder_SpawnPty *_BinaryDecoder_WebsocketMessage_, currentSliceIndex int) _PtyProxyOption_ {
			optionCode := Code_PtyProxyOption(binaryDecoder_SpawnPty.DecodeParameter_Uint16("OptionCode"))
			optionValue := int(binaryDecoder_SpawnPty.DecodeParameter_Uint32("OptionValue"))
			switch optionCode {
			case SCROLLBACK_LINE_COUNT__Code_PtyProxyOption:
				return _PtyProxyOption_ScrollbackLineCount_{
					ScrollbackLineCount_PtyTerminal: optionValue,
				}
			case STAGING_BUFFER_SIZE__Code_PtyProxyOption:
				return _PtyProxyOption_StagingBufferSize_{
					StagingBufferSize_PtyReader: optionValue,
				}
			case POST_SNAPSHOT_BUFFER_SIZE__Code_PtyProxyOption:
				return _PtyProxyOption_PostSnapshotBufferSize_{
					PostSnapshotBufferSize_PtyProxy: optionValue,
				}
			case QUEUE_BUFFER_SIZE_INPUT_ORDER__Code_PtyProxyOption:
				return _PtyProxyOption_QueueBufferSize_InputOrder__PtyWriter_{
					QueueBufferSize_InputOrder__PtyWriter: optionValue,
				}
			default:
				binaryDecoder_SpawnPty.MaybeError_Earliest = _ERRORS.New("unrecognized pty proxy option code")
				return nil
			}
		},
	)
	binaryDecoder_SpawnPty.AssertEndOfFrame()
	if binaryDecoder_SpawnPty.MaybeError_Earliest != nil {
		return nil, binaryDecoder_SpawnPty.MaybeError_Earliest
	}
	return _SpawnPty_Message_{
		ColumnCount_PtyTerminal:         __ColumnCount_PtyTerminal,
		RowCount_PtyTerminal:            __RowCount_PtyTerminal,
		ShellBinaryPath_PtyCommand:      __ShellBinaryPath_PtyCommand,
		DirectoryPath_PtyCommand:        __DirectoryPath_PtyCommand,
		EnvironmentVariables_PtyCommand: __EnvironmentVariables_PtyCommand,
		Options_PtyProxy:                __Options_PtyProxy,
	}, nil
}

type _PtyProxyOption_ interface {
	UpdateOptionsResult(optionsResult_ptyProxy *_PtyProxyDefaults_)
}

type _PtyProxyOption_ScrollbackLineCount_ struct {
	ScrollbackLineCount_PtyTerminal int
}

func (this _PtyProxyOption_ScrollbackLineCount_) UpdateOptionsResult(
	optionsResult_ptyProxy *_PtyProxyDefaults_,
) {
	optionsResult_ptyProxy.ScrollbackLineCount_PtyTerminal = this.ScrollbackLineCount_PtyTerminal
}

type _PtyProxyOption_StagingBufferSize_ struct {
	StagingBufferSize_PtyReader int
}

func (this _PtyProxyOption_StagingBufferSize_) UpdateOptionsResult(
	optionsResult_ptyProxy *_PtyProxyDefaults_,
) {
	optionsResult_ptyProxy.StagingBufferSize_PtyReader = this.StagingBufferSize_PtyReader
}

type _PtyProxyOption_PostSnapshotBufferSize_ struct {
	PostSnapshotBufferSize_PtyProxy int
}

func (this _PtyProxyOption_PostSnapshotBufferSize_) UpdateOptionsResult(
	optionsResult_ptyProxy *_PtyProxyDefaults_,
) {
	optionsResult_ptyProxy.PostSnapshotBufferSize_PtyProxy = this.PostSnapshotBufferSize_PtyProxy
}

type _PtyProxyOption_QueueBufferSize_InputOrder__PtyWriter_ struct {
	QueueBufferSize_InputOrder__PtyWriter int
}

func (this _PtyProxyOption_QueueBufferSize_InputOrder__PtyWriter_) UpdateOptionsResult(
	optionsResult_ptyProxy *_PtyProxyDefaults_,
) {
	optionsResult_ptyProxy.QueueBufferSize_InputOrder__PtyWriter = this.QueueBufferSize_InputOrder__PtyWriter
}

type _SpawnPty_Message_ struct {
	ColumnCount_PtyTerminal         int
	RowCount_PtyTerminal            int
	ShellBinaryPath_PtyCommand      string
	DirectoryPath_PtyCommand        string
	EnvironmentVariables_PtyCommand []string
	Options_PtyProxy                []_PtyProxyOption_
}

func (this _SpawnPty_Message_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	optionsResult_ptyProxy := workspaceController.PtyProxyDefaults
	for _, option_ptyProxy := range this.Options_PtyProxy {
		option_ptyProxy.UpdateOptionsResult(&optionsResult_ptyProxy)
	}
	workspaceController.Mutex.Lock()
	newPtyId := workspaceController.NextId_PtyProxy
	workspaceController.NextId_PtyProxy++
	workspaceController.Mutex.Unlock()
	go func() {
		ptyStartError := Spawn__PtyProxy(_SpawnApi__PtyProxy_{
			Id:                                    newPtyId,
			ColumnCount_PtyTerminal:               this.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:                  this.RowCount_PtyTerminal,
			ShellBinaryPath_PtyCommand:            this.ShellBinaryPath_PtyCommand,
			DirectoryPath_PtyCommand:              this.DirectoryPath_PtyCommand,
			EnvironmentVariables_PtyCommand:       this.EnvironmentVariables_PtyCommand,
			ScrollbackLineCount_PtyTerminal:       optionsResult_ptyProxy.ScrollbackLineCount_PtyTerminal,
			StagingBufferSize_PtyReader:           optionsResult_ptyProxy.StagingBufferSize_PtyReader,
			PostSnapshotBufferSize_PtyProxy:       optionsResult_ptyProxy.PostSnapshotBufferSize_PtyProxy,
			QueueBufferSize_InputOrder__PtyWriter: optionsResult_ptyProxy.QueueBufferSize_InputOrder__PtyWriter,
			OnSpawned:                             workspaceController.HandleSpawned_Pty,
			OnOutput_Live:                         workspaceController.HandleOutput_Pty,
			OnOutput_Snapshot:                     workspaceController.HandleOutput_Pty,
			OnOutput_PostSnapshotBuffer:           workspaceController.HandleOutput_Pty,
			OnExited_Eio_Success:                  workspaceController.HandleExited_Eio_Success__Pty,
			OnExited_Eio_Failure:                  workspaceController.HandleExited_Eio_Failure__Pty,
			OnExited_Eio_Killed:                   workspaceController.HandleExited_Eio_Killed__Pty,
			OnExited_Closed:                       workspaceController.HandleExited_Closed__Pty,
			OnExited_SystemError:                  workspaceController.HandleExited_SystemError__Pty,
		})
		if ptyStartError != nil {
			workspaceController.HandleSpawnFailed_Pty(newPtyId)
		}
	}()
}

func decodeMessage_ResizePtys(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_ResizePtys := New__BinaryDecoder_WebsocketMessage(
		_NewApi__BinaryDecoder_WebsocketMessage_{
			Buffer_BinaryMessageFrame: binaryMessageFrame,
			Cursor_Buffer:             2,
			Label_MessageStruct:       "ResizePtys",
		},
	)
	__Entries_PtyProxy := DecodeParameter__Slice16__BinaryDecoder_WebsocketMessage(
		&binaryDecoder_ResizePtys,
		"Entries_PtyProxy",
		func(binaryDecoder_ResizePtys *_BinaryDecoder_WebsocketMessage_, currentSliceIndex int) _ResizePtys_Entry_ {
			__Id_PtyProxy := binaryDecoder_ResizePtys.DecodeParameter_Uint32("Id_PtyProxy")
			__ColumnCount_PtyTerminal := int(binaryDecoder_ResizePtys.DecodeParameter_Uint16("ColumnCount_PtyTerminal"))
			__RowCount_PtyTerminal := int(binaryDecoder_ResizePtys.DecodeParameter_Uint16("RowCount_PtyTerminal"))
			return _ResizePtys_Entry_{
				Id_PtyProxy:             __Id_PtyProxy,
				ColumnCount_PtyTerminal: __ColumnCount_PtyTerminal,
				RowCount_PtyTerminal:    __RowCount_PtyTerminal,
			}
		},
	)
	binaryDecoder_ResizePtys.AssertEndOfFrame()
	if binaryDecoder_ResizePtys.MaybeError_Earliest != nil {
		return nil, binaryDecoder_ResizePtys.MaybeError_Earliest
	}
	return _ResizePtys_Message_{
		Entries_PtyProxy: __Entries_PtyProxy,
	}, nil
}

type _ResizePtys_Entry_ struct {
	Id_PtyProxy             uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

type _ResizePtys_Message_ struct {
	Entries_PtyProxy []_ResizePtys_Entry_
}

func (this _ResizePtys_Message_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.MessageDebouncer_ResizePtys.QueueChannel <- this
}

func decodeMessage_WritePtyInput(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_WritePtyInput := New__BinaryDecoder_WebsocketMessage(
		_NewApi__BinaryDecoder_WebsocketMessage_{
			Buffer_BinaryMessageFrame: binaryMessageFrame,
			Cursor_Buffer:             2,
			Label_MessageStruct:       "WritePtyInput",
		},
	)
	__Id_PtyProxy := binaryDecoder_WritePtyInput.DecodeParameter_Uint32("Id_PtyProxy")
	__RawInputBytes := binaryDecoder_WritePtyInput.DecodeParameter_TrailingBytes("RawInputBytes")
	binaryDecoder_WritePtyInput.AssertEndOfFrame()
	if binaryDecoder_WritePtyInput.MaybeError_Earliest != nil {
		return nil, binaryDecoder_WritePtyInput.MaybeError_Earliest
	}
	return _WritePtyInput_Message_{
		Id_PtyProxy: __Id_PtyProxy,
		InputOrder_PtyWriter: &_Passthrough__InputOrder_PtyWriter_{
			InputData_PtyMaster: __RawInputBytes,
		},
	}, nil
}

type _WritePtyInput_Message_ struct {
	Id_PtyProxy          uint32
	InputOrder_PtyWriter _InputOrder_PtyWriter_
}

func (this _WritePtyInput_Message_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.Mutex.Lock()
	targetWorkspacePty := workspaceController.PtyPool[this.Id_PtyProxy]
	workspaceController.Mutex.Unlock()
	if targetWorkspacePty != nil {
		targetWorkspacePty.PtyProxy.PtyWriter.QueueChannel_InputOrder <- this.InputOrder_PtyWriter
	}
}

func decodeMessage_TerminatePty(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_TerminatePty := New__BinaryDecoder_WebsocketMessage(
		_NewApi__BinaryDecoder_WebsocketMessage_{
			Buffer_BinaryMessageFrame: binaryMessageFrame,
			Cursor_Buffer:             2,
			Label_MessageStruct:       "TerminatePty",
		},
	)
	__Id_PtyProxy := binaryDecoder_TerminatePty.DecodeParameter_Uint32("Id_PtyProxy")
	__TerminalSignal_PtyProcess := int(binaryDecoder_TerminatePty.DecodeParameter_Int32("TerminalSignal_PtyProcess"))
	binaryDecoder_TerminatePty.AssertEndOfFrame()
	if binaryDecoder_TerminatePty.MaybeError_Earliest != nil {
		return nil, binaryDecoder_TerminatePty.MaybeError_Earliest
	}
	return _TerminatePty_Message_{
		Id_PtyProxy:               __Id_PtyProxy,
		TerminalSignal_PtyProcess: __TerminalSignal_PtyProcess,
	}, nil
}

type _TerminatePty_Message_ struct {
	Id_PtyProxy               uint32
	TerminalSignal_PtyProcess int
}

func (this _TerminatePty_Message_) Execute(
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

func decodeMessage_RemovePty(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_RemovePty := New__BinaryDecoder_WebsocketMessage(
		_NewApi__BinaryDecoder_WebsocketMessage_{
			Buffer_BinaryMessageFrame: binaryMessageFrame,
			Cursor_Buffer:             2,
			Label_MessageStruct:       "RemovePty",
		},
	)
	__Ids_PtyPool := binaryDecoder_RemovePty.DecodeParameter__Slice16_Uint32("Ids_PtyPool")
	binaryDecoder_RemovePty.AssertEndOfFrame()
	if binaryDecoder_RemovePty.MaybeError_Earliest != nil {
		return nil, binaryDecoder_RemovePty.MaybeError_Earliest
	}
	return _RemovePty_Message_{
		Ids_PtyPool: __Ids_PtyPool,
	}, nil
}

type _RemovePty_Message_ struct {
	Ids_PtyPool []uint32
}

func (this _RemovePty_Message_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.Mutex.Lock()
	for _, someId_PtyProxy := range this.Ids_PtyPool {
		targetWorkspacePty := workspaceController.PtyPool[someId_PtyProxy]
		if targetWorkspacePty != nil && targetWorkspacePty.MaybeExitOutcome != nil {
			delete(
				workspaceController.PtyPool,
				someId_PtyProxy,
			)
		}
	}
	workspaceController.Mutex.Unlock()
}

func decodeMessage_SyncWorkspace(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_SyncWorkspace := New__BinaryDecoder_WebsocketMessage(
		_NewApi__BinaryDecoder_WebsocketMessage_{
			Buffer_BinaryMessageFrame: binaryMessageFrame,
			Cursor_Buffer:             2,
			Label_MessageStruct:       "SyncWorkspace",
		},
	)
	__SyncPtyOrders := DecodeParameter__Map16__BinaryDecoder_WebsocketMessage(
		&binaryDecoder_SyncWorkspace,
		"SyncPtyOrders",
		func(binaryDecoder_SyncWorkspace *_BinaryDecoder_WebsocketMessage_, currentMapIndex int) (uint32, *_SyncPtyOrder_SyncWorkspace_) {
			__Id_PtyProxy := binaryDecoder_SyncWorkspace.DecodeParameter_Uint32("Id_PtyProxy")
			__ColumnCount_PtyTerminal := int(binaryDecoder_SyncWorkspace.DecodeParameter_Uint16("ColumnCount_PtyTerminal"))
			__RowCount_PtyTerminal := int(binaryDecoder_SyncWorkspace.DecodeParameter_Uint16("RowCount_PtyTerminal"))
			return __Id_PtyProxy, &_SyncPtyOrder_SyncWorkspace_{
				ColumnCount_PtyTerminal: __ColumnCount_PtyTerminal,
				RowCount_PtyTerminal:    __RowCount_PtyTerminal,
			}
		},
	)
	binaryDecoder_SyncWorkspace.AssertEndOfFrame()
	if binaryDecoder_SyncWorkspace.MaybeError_Earliest != nil {
		return nil, binaryDecoder_SyncWorkspace.MaybeError_Earliest
	}
	return _SyncWorkspace_Message_{
		SyncPtyOrders: __SyncPtyOrders,
	}, nil
}

type _SyncPtyOrder_SyncWorkspace_ struct {
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

type _SyncWorkspace_Message_ struct {
	SyncPtyOrders map[uint32]*_SyncPtyOrder_SyncWorkspace_
}

func (this _SyncWorkspace_Message_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.LifecycleCoordinator_WorkspacePty.QueueChannel <- _Sync__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection: id_WebsocketConnection,
		Message_SyncWorkspace:  this,
	}
}
