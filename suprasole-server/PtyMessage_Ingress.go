package main

import (
	_ERRORS "errors"
)

type _PtyMessage_Ingress_ interface {
	Execute(
		WorkspaceController_forwarded *_WorkspaceController_,
		id_WebsocketConnection_expected uint64,
	)
}

type _Code__PtyMessage_Ingress_ uint16

const (
	SPAWN_PTY___Code__PtyMessage_Ingress         _Code__PtyMessage_Ingress_ = 0x0001
	BATCH__RESIZE_PTY___Code__PtyMessage_Ingress _Code__PtyMessage_Ingress_ = 0x0003
	TERMINATE_PTY___Code__PtyMessage_Ingress     _Code__PtyMessage_Ingress_ = 0x0004
	BATCH__REMOVE_PTY___Code__PtyMessage_Ingress _Code__PtyMessage_Ingress_ = 0x0006
	WRITE_INPUT__PTY___Code__PtyMessage_Ingress  _Code__PtyMessage_Ingress_ = 0x0007
	BATCH__SYNC_PTY___Code__PtyMessage_Ingress   _Code__PtyMessage_Ingress_ = 0x000a
)

var MAP__DECODE_PAYLOAD___PTY_MESSAGE__INGRESS = map[_Code__PtyMessage_Ingress_]func(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error){
	SPAWN_PTY___Code__PtyMessage_Ingress:         decodePayload_SpawnPty,
	BATCH__RESIZE_PTY___Code__PtyMessage_Ingress: decodePayload__Batch_ResizePty,
	TERMINATE_PTY___Code__PtyMessage_Ingress:     decodePayload_TerminatePty,
	BATCH__REMOVE_PTY___Code__PtyMessage_Ingress: decodePayload__Batch_RemovePty,
	WRITE_INPUT__PTY___Code__PtyMessage_Ingress:  decodePayload__WriteInput_Pty,
	BATCH__SYNC_PTY___Code__PtyMessage_Ingress:   decodePayload__Batch_SyncPty,
}

type _SpawnPty__PtyMessage_Ingress_ struct {
	ColumnCount_PtyTerminal         int
	RowCount_PtyTerminal            int
	Path_ShellBinary__PtyCommand    string
	DirectoryPath_PtyCommand        string
	EnvironmentVariables_PtyCommand []string
	OptionBatch_PtyProxy            []_Option_PtyProxy_
}

type _Option_PtyProxy_ interface {
	Update_OptionConfig(
		optionConfig_PtyProxy_result *_OptionConfig_PtyProxy_,
	)
}

type _OptionConfig_PtyProxy_ struct {
	Count_ScrollbackLine__PtyTerminal      int
	Size_StagingBuffer__PtyReader          int
	Size_PostSnapshotBuffer__PtyProxy      int
	Size_QueueBuffer__InputOrder_PtyWriter int
}

type _Code__Option_PtyProxy_ uint16

const (
	COUNT__SCROLLBACK_LINE___Code__Option_PtyProxy          _Code__Option_PtyProxy_ = 0x0001
	SIZE__STAGING_BUFFER___Code__Option_PtyProxy            _Code__Option_PtyProxy_ = 0x0002
	SIZE__POST_SNAPSHOT_BUFFER___Code__Option_PtyProxy      _Code__Option_PtyProxy_ = 0x0003
	SIZE__QUEUE_BUFFER__INPUT_ORDER___Code__Option_PtyProxy _Code__Option_PtyProxy_ = 0x0004
)

type _Count_ScrollbackLine__Option_PtyProxy_ struct {
	Count_ScrollbackLine__PtyTerminal int
}

type _Size_StagingBuffer__Option_PtyProxy_ struct {
	Size_StagingBuffer__PtyReader int
}

type _Size_PostSnapshotBuffer__Option_PtyProxy_ struct {
	Size_PostSnapshotBuffer__PtyProxy int
}

type _Size_QueueBuffer__InputOrder_PtyWriter___Option_PtyProxy_ struct {
	Size_QueueBuffer__InputOrder_PtyWriter int
}

func decodePayload_SpawnPty(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_SpawnPty := Make__BinaryDecoder_WebsocketMessage(
		_MakeApi__BinaryDecoder_WebsocketMessage_{
			Label_MessageStruct__:         "SpawnPty",
			Cursor_Buffer:                 2,
			Buffer__Payload_BinaryMessage: payload_binaryMessage,
		},
	)
	columnCount_PtyTerminal := int(binaryDecoder_SpawnPty.DecodeParameter_Uint16("ColumnCount_PtyTerminal"))
	rowCount_PtyTerminal := int(binaryDecoder_SpawnPty.DecodeParameter_Uint16("RowCount_PtyTerminal"))
	path_ShellBinary__PtyCommand := binaryDecoder_SpawnPty.DecodeParameter_String16("Path_ShellBinary__PtyCommand")
	directoryPath_PtyCommand := binaryDecoder_SpawnPty.DecodeParameter_String16("DirectoryPath_PtyCommand")
	environmentVariables_PtyCommand := binaryDecoder_SpawnPty.DecodeParameter__Slice16_String16("EnvironmentVariables_PtyCommand")
	optionBatch_PtyProxy := DecodeParameter__Slice16__BinaryDecoder_WebsocketMessage(
		&binaryDecoder_SpawnPty,
		"OptionBatch_PtyProxy",
		func(binaryDecoder_SpawnPty *_BinaryDecoder_WebsocketMessage_, sliceIndex_current int) _Option_PtyProxy_ {
			optionCode_PtyProxy := _Code__Option_PtyProxy_(binaryDecoder_SpawnPty.DecodeParameter_Uint16("OptionCode_PtyProxy"))
			optionValue_PtyProxy := int(binaryDecoder_SpawnPty.DecodeParameter_Uint32("OptionValue_PtyProxy"))
			switch optionCode_PtyProxy {
			case COUNT__SCROLLBACK_LINE___Code__Option_PtyProxy:
				return _Count_ScrollbackLine__Option_PtyProxy_{
					Count_ScrollbackLine__PtyTerminal: optionValue_PtyProxy,
				}
			case SIZE__STAGING_BUFFER___Code__Option_PtyProxy:
				return _Size_StagingBuffer__Option_PtyProxy_{
					Size_StagingBuffer__PtyReader: optionValue_PtyProxy,
				}
			case SIZE__POST_SNAPSHOT_BUFFER___Code__Option_PtyProxy:
				return _Size_PostSnapshotBuffer__Option_PtyProxy_{
					Size_PostSnapshotBuffer__PtyProxy: optionValue_PtyProxy,
				}
			case SIZE__QUEUE_BUFFER__INPUT_ORDER___Code__Option_PtyProxy:
				return _Size_QueueBuffer__InputOrder_PtyWriter___Option_PtyProxy_{
					Size_QueueBuffer__InputOrder_PtyWriter: optionValue_PtyProxy,
				}
			default:
				binaryDecoder_SpawnPty.Error_Earliest_maybe = _ERRORS.New("unrecognized pty proxy option code")
				return nil
			}
		},
	)
	binaryDecoder_SpawnPty.AssertEndOfPayload()
	if binaryDecoder_SpawnPty.Error_Earliest_maybe != nil {
		return nil, binaryDecoder_SpawnPty.Error_Earliest_maybe
	}
	return _SpawnPty__PtyMessage_Ingress_{
		ColumnCount_PtyTerminal:         columnCount_PtyTerminal,
		RowCount_PtyTerminal:            rowCount_PtyTerminal,
		Path_ShellBinary__PtyCommand:    path_ShellBinary__PtyCommand,
		DirectoryPath_PtyCommand:        directoryPath_PtyCommand,
		EnvironmentVariables_PtyCommand: environmentVariables_PtyCommand,
		OptionBatch_PtyProxy:            optionBatch_PtyProxy,
	}, nil
}

type _Batch_ResizePty__PtyMessage_Ingress_ struct {
	OrderBatch_ResizePty []_Order_ResizePty_
}

type _Order_ResizePty_ struct {
	Id_WorkspacePty         uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

func decodePayload__Batch_ResizePty(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder__Batch_ResizePty := Make__BinaryDecoder_WebsocketMessage(
		_MakeApi__BinaryDecoder_WebsocketMessage_{
			Label_MessageStruct__:         "Batch_ResizePty",
			Cursor_Buffer:                 2,
			Buffer__Payload_BinaryMessage: payload_binaryMessage,
		},
	)
	orderBatch_ResizePty := DecodeParameter__Slice16__BinaryDecoder_WebsocketMessage(
		&binaryDecoder__Batch_ResizePty,
		"OrderBatch_ResizePty",
		func(binaryDecoder__Batch_ResizePty *_BinaryDecoder_WebsocketMessage_, sliceIndex_current int) _Order_ResizePty_ {
			id_WorkspacePty := binaryDecoder__Batch_ResizePty.DecodeParameter_Uint32("Id_WorkspacePty")
			columnCount_PtyTerminal := int(binaryDecoder__Batch_ResizePty.DecodeParameter_Uint16("ColumnCount_PtyTerminal"))
			rowCount_PtyTerminal := int(binaryDecoder__Batch_ResizePty.DecodeParameter_Uint16("RowCount_PtyTerminal"))
			return _Order_ResizePty_{
				Id_WorkspacePty:         id_WorkspacePty,
				ColumnCount_PtyTerminal: columnCount_PtyTerminal,
				RowCount_PtyTerminal:    rowCount_PtyTerminal,
			}
		},
	)
	binaryDecoder__Batch_ResizePty.AssertEndOfPayload()
	if binaryDecoder__Batch_ResizePty.Error_Earliest_maybe != nil {
		return nil, binaryDecoder__Batch_ResizePty.Error_Earliest_maybe
	}
	return _Batch_ResizePty__PtyMessage_Ingress_{
		OrderBatch_ResizePty: orderBatch_ResizePty,
	}, nil
}

type _TerminatePty__PtyMessage_Ingress_ struct {
	Id_WorkspacePty           uint32
	TerminalSignal_PtyProcess int
}

func decodePayload_TerminatePty(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_TerminatePty := Make__BinaryDecoder_WebsocketMessage(
		_MakeApi__BinaryDecoder_WebsocketMessage_{
			Label_MessageStruct__:         "TerminatePty",
			Cursor_Buffer:                 2,
			Buffer__Payload_BinaryMessage: payload_binaryMessage,
		},
	)
	id_WorkspacePty := binaryDecoder_TerminatePty.DecodeParameter_Uint32("Id_WorkspacePty")
	terminalSignal_PtyProcess := int(binaryDecoder_TerminatePty.DecodeParameter_Int32("TerminalSignal_PtyProcess"))
	binaryDecoder_TerminatePty.AssertEndOfPayload()
	if binaryDecoder_TerminatePty.Error_Earliest_maybe != nil {
		return nil, binaryDecoder_TerminatePty.Error_Earliest_maybe
	}
	return _TerminatePty__PtyMessage_Ingress_{
		Id_WorkspacePty:           id_WorkspacePty,
		TerminalSignal_PtyProcess: terminalSignal_PtyProcess,
	}, nil
}

type _Batch_RemovePty__PtyMessage_Ingress_ struct {
	OrderBatch_RemovePty []_Order_RemovePty_
}

type _Order_RemovePty_ struct {
	Id_WorkspacePty uint32
}

func decodePayload__Batch_RemovePty(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder__Batch_RemovePty := Make__BinaryDecoder_WebsocketMessage(
		_MakeApi__BinaryDecoder_WebsocketMessage_{
			Label_MessageStruct__:         "Batch_RemovePty",
			Cursor_Buffer:                 2,
			Buffer__Payload_BinaryMessage: payload_binaryMessage,
		},
	)
	orderBatch_RemovePty := DecodeParameter__Slice16__BinaryDecoder_WebsocketMessage(
		&binaryDecoder__Batch_RemovePty,
		"OrderBatch_RemovePty",
		func(binaryDecoder__Batch_RemovePty *_BinaryDecoder_WebsocketMessage_, sliceIndex_current int) _Order_RemovePty_ {
			id_WorkspacePty := binaryDecoder__Batch_RemovePty.DecodeParameter_Uint32("Id_WorkspacePty")
			return _Order_RemovePty_{
				Id_WorkspacePty: id_WorkspacePty,
			}
		},
	)
	binaryDecoder__Batch_RemovePty.AssertEndOfPayload()
	if binaryDecoder__Batch_RemovePty.Error_Earliest_maybe != nil {
		return nil, binaryDecoder__Batch_RemovePty.Error_Earliest_maybe
	}
	return _Batch_RemovePty__PtyMessage_Ingress_{
		OrderBatch_RemovePty: orderBatch_RemovePty,
	}, nil
}

type _WriteInput_Pty__PtyMessage_Ingress_ struct {
	Id_WorkspacePty      uint32
	InputOrder_PtyWriter _InputOrder_PtyWriter_
}

func decodePayload__WriteInput_Pty(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder_WriteInput_Pty := Make__BinaryDecoder_WebsocketMessage(
		_MakeApi__BinaryDecoder_WebsocketMessage_{
			Label_MessageStruct__:         "WriteInput_Pty",
			Cursor_Buffer:                 2,
			Buffer__Payload_BinaryMessage: payload_binaryMessage,
		},
	)
	id_WorkspacePty := binaryDecoder_WriteInput_Pty.DecodeParameter_Uint32("Id_WorkspacePty")
	inputData_PtyDevice := binaryDecoder_WriteInput_Pty.DecodeParameter_TrailingBytes("InputData_PtyDevice")
	binaryDecoder_WriteInput_Pty.AssertEndOfPayload()
	if binaryDecoder_WriteInput_Pty.Error_Earliest_maybe != nil {
		return nil, binaryDecoder_WriteInput_Pty.Error_Earliest_maybe
	}
	return _WriteInput_Pty__PtyMessage_Ingress_{
		Id_WorkspacePty: id_WorkspacePty,
		InputOrder_PtyWriter: &_Passthrough__InputOrder_PtyWriter_{
			InputData_PtyDevice: inputData_PtyDevice,
		},
	}, nil
}

type _Batch_SyncPty__PtyMessage_Ingress_ struct {
	OrderBatch_SyncPty map[uint32]*_Order_SyncPty_
}

type _Order_SyncPty_ struct {
	Id_WorkspacePty         uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

func decodePayload__Batch_SyncPty(
	payload_binaryMessage []byte,
) (_PtyMessage_Ingress_, error) {
	binaryDecoder__Batch_SyncPty := Make__BinaryDecoder_WebsocketMessage(
		_MakeApi__BinaryDecoder_WebsocketMessage_{
			Label_MessageStruct__:         "Batch_SyncPty",
			Cursor_Buffer:                 2,
			Buffer__Payload_BinaryMessage: payload_binaryMessage,
		},
	)
	orderBatch_SyncPty := DecodeParameter__Map16__BinaryDecoder_WebsocketMessage(
		&binaryDecoder__Batch_SyncPty,
		"OrderBatch_SyncPty",
		func(binaryDecoder__Batch_SyncPty *_BinaryDecoder_WebsocketMessage_, currentMapIndex int) (uint32, *_Order_SyncPty_) {
			id_WorkspacePty := binaryDecoder__Batch_SyncPty.DecodeParameter_Uint32("Id_WorkspacePty")
			columnCount_PtyTerminal := int(binaryDecoder__Batch_SyncPty.DecodeParameter_Uint16("ColumnCount_PtyTerminal"))
			rowCount_PtyTerminal := int(binaryDecoder__Batch_SyncPty.DecodeParameter_Uint16("RowCount_PtyTerminal"))
			return id_WorkspacePty, &_Order_SyncPty_{
				Id_WorkspacePty:         id_WorkspacePty,
				ColumnCount_PtyTerminal: columnCount_PtyTerminal,
				RowCount_PtyTerminal:    rowCount_PtyTerminal,
			}
		},
	)
	binaryDecoder__Batch_SyncPty.AssertEndOfPayload()
	if binaryDecoder__Batch_SyncPty.Error_Earliest_maybe != nil {
		return nil, binaryDecoder__Batch_SyncPty.Error_Earliest_maybe
	}
	return _Batch_SyncPty__PtyMessage_Ingress_{
		OrderBatch_SyncPty: orderBatch_SyncPty,
	}, nil
}
