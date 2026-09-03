package main

import (
	_FMT "fmt"
)

type Code_PtyMessage_Egress uint16

const (
	SPAWN_PTY_STATUS__Code_PtyMessage_Egress   Code_PtyMessage_Egress = 0x0002
	PTY_EXIT__Code_PtyMessage_Egress           Code_PtyMessage_Egress = 0x0005
	PTY_OUTPUT__Code_PtyMessage_Egress         Code_PtyMessage_Egress = 0x0008
	WORKSPACE_MANIFEST__Code_PtyMessage_Egress Code_PtyMessage_Egress = 0x0009
	SYNC_PTY_START__Code_PtyMessage_Egress     Code_PtyMessage_Egress = 0x000b
	SYNC_PTY_COMPLETE__Code_PtyMessage_Egress  Code_PtyMessage_Egress = 0x000c
)

type _PtyMessage_Egress_ interface {
	EncodeBinaryFrame() []byte
}

func Emit__PtyMessage_Egress(
	websocketController *_WebsocketController_,
	targetId_WebsocketConnection uint64,
	egressMessage _PtyMessage_Egress_,
) {
	_ = websocketController.WriteBinaryMessage(
		targetId_WebsocketConnection,
		egressMessage.EncodeBinaryFrame(),
	)
}

type _PtyBulletin_WorkspaceManifest_ struct {
	Id_PtyProxy               uint32
	MaybeExitOutcome_PtyProxy _ExitOutcome_PtyProxy_
	IsVisible_Client          bool
}

type _WorkspaceManifest_Message_ struct {
	PtyBulletins []_PtyBulletin_WorkspaceManifest_
}

func encodeStruct__ExitOutcome_PtyProxy(
	binaryEncoder *_BinaryEncoder_WebsocketMessage_,
	exitOutcome _ExitOutcome_PtyProxy_,
) {
	switch concreteOutcome := exitOutcome.(type) {
	case _Success__ExitOutcome_PtyProxy_:
		// ExitReason
		binaryEncoder.EncodeParameter_Uint8(uint8(SUCCESS__ExitReason_PtyProxy))
		// ExitCode
		binaryEncoder.EncodeParameter_Int32(0)
		// ExitSignal
		binaryEncoder.EncodeParameter_Int32(0)
	case _Failure__ExitOutcome_PtyProxy_:
		// ExitReason
		binaryEncoder.EncodeParameter_Uint8(uint8(FAILURE__ExitReason_PtyProxy))
		// ExitCode
		binaryEncoder.EncodeParameter_Int32(int32(concreteOutcome.ExitCode_PtyProcess))
		// ExitSignal
		binaryEncoder.EncodeParameter_Int32(0)
	case _Killed__ExitOutcome_PtyProxy_:
		// ExitReason
		binaryEncoder.EncodeParameter_Uint8(uint8(KILLED__ExitReason_PtyProxy))
		// ExitCode
		binaryEncoder.EncodeParameter_Int32(0)
		// ExitSignal
		binaryEncoder.EncodeParameter_Int32(int32(concreteOutcome.ExitSignal_PtyProcess))
	case _Closed__ExitOutcome_PtyProxy_:
		// ExitReason
		binaryEncoder.EncodeParameter_Uint8(uint8(CLOSED__ExitReason_PtyProxy))
		// ExitCode
		binaryEncoder.EncodeParameter_Int32(0)
		// ExitSignal
		binaryEncoder.EncodeParameter_Int32(0)
	case _SystemError__ExitOutcome_PtyProxy_:
		// ExitReason
		binaryEncoder.EncodeParameter_Uint8(uint8(SYSTEM_ERROR__ExitReason_PtyProxy))
		// ExitCode
		binaryEncoder.EncodeParameter_Int32(-1)
		// ExitSignal
		binaryEncoder.EncodeParameter_Int32(0)
	default:
		_FMT.Println("invalid path: encodeStruct__ExitOutcome_PtyProxy")
	}
}

func (this _WorkspaceManifest_Message_) EncodeBinaryFrame() []byte {
	binaryEncoder_WorkspaceManifest := New__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_WorkspaceManifest.EncodeParameter_Uint16(uint16(WORKSPACE_MANIFEST__Code_PtyMessage_Egress))
	// PtyBulletins
	EncodeParameter__Slice16__BinaryEncoder_WebsocketMessage(
		&binaryEncoder_WorkspaceManifest,
		this.PtyBulletins,
		func(binaryEncoder_WorkspaceManifest *_BinaryEncoder_WebsocketMessage_, currentSliceIndex int, ptyBulletin _PtyBulletin_WorkspaceManifest_) {
			// Id_PtyProxy
			binaryEncoder_WorkspaceManifest.EncodeParameter_Uint32(ptyBulletin.Id_PtyProxy)
			// IsVisible_Client
			binaryEncoder_WorkspaceManifest.EncodeParameter_Bool(ptyBulletin.IsVisible_Client)
			if ptyBulletin.MaybeExitOutcome_PtyProxy != nil {
				// HasExitOutcome
				binaryEncoder_WorkspaceManifest.EncodeParameter_Bool(true)
				encodeStruct__ExitOutcome_PtyProxy(
					binaryEncoder_WorkspaceManifest,
					ptyBulletin.MaybeExitOutcome_PtyProxy,
				)
			} else {
				// HasExitOutcome
				binaryEncoder_WorkspaceManifest.EncodeParameter_Bool(false)
			}
		},
	)
	return binaryEncoder_WorkspaceManifest.Bytes()
}

type Status_SpawnPty byte

const (
	SUCCESS__Status_SpawnPty Status_SpawnPty = 0x00
	FAILURE__Status_SpawnPty Status_SpawnPty = 0x01
)

type _SpawnPtyStatus_Message_ struct {
	Id_PtyProxy uint32
	Status      Status_SpawnPty
}

func (this _SpawnPtyStatus_Message_) EncodeBinaryFrame() []byte {
	binaryEncoder_SpawnPtyStatus := New__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_SpawnPtyStatus.EncodeParameter_Uint16(uint16(SPAWN_PTY_STATUS__Code_PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder_SpawnPtyStatus.EncodeParameter_Uint32(this.Id_PtyProxy)
	// Status
	binaryEncoder_SpawnPtyStatus.EncodeParameter_Uint8(uint8(this.Status))
	return binaryEncoder_SpawnPtyStatus.Bytes()
}

type _PtyExit_Message_ struct {
	Id_PtyProxy uint32
	ExitOutcome _ExitOutcome_PtyProxy_
}

func (this _PtyExit_Message_) EncodeBinaryFrame() []byte {
	binaryEncoder_PtyExit := New__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_PtyExit.EncodeParameter_Uint16(uint16(PTY_EXIT__Code_PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder_PtyExit.EncodeParameter_Uint32(this.Id_PtyProxy)
	// ExitOutcome
	encodeStruct__ExitOutcome_PtyProxy(
		&binaryEncoder_PtyExit,
		this.ExitOutcome,
	)
	return binaryEncoder_PtyExit.Bytes()
}

type _PtyOutput_Message_ struct {
	Id_PtyProxy uint32
	OutputData  []byte
}

func (this _PtyOutput_Message_) EncodeBinaryFrame() []byte {
	binaryEncoder_PtyOutput := New__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_PtyOutput.EncodeParameter_Uint16(uint16(PTY_OUTPUT__Code_PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder_PtyOutput.EncodeParameter_Uint32(this.Id_PtyProxy)
	// OutputData
	binaryEncoder_PtyOutput.EncodeParameter_TrailingBytes(this.OutputData)
	return binaryEncoder_PtyOutput.Bytes()
}

type _SyncPtyStart_Message_ struct {
	Id_PtyProxy             uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

func (this _SyncPtyStart_Message_) EncodeBinaryFrame() []byte {
	binaryEncoder_SyncPtyStart := New__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_SyncPtyStart.EncodeParameter_Uint16(uint16(SYNC_PTY_START__Code_PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder_SyncPtyStart.EncodeParameter_Uint32(this.Id_PtyProxy)
	// ColumnCount_PtyTerminal
	binaryEncoder_SyncPtyStart.EncodeParameter_Uint16(uint16(this.ColumnCount_PtyTerminal))
	// RowCount_PtyTerminal
	binaryEncoder_SyncPtyStart.EncodeParameter_Uint16(uint16(this.RowCount_PtyTerminal))
	return binaryEncoder_SyncPtyStart.Bytes()
}

type _SyncPtyComplete_Message_ struct {
	Id_PtyProxy uint32
}

func (this _SyncPtyComplete_Message_) EncodeBinaryFrame() []byte {
	binaryEncoder_SyncPtyComplete := New__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_SyncPtyComplete.EncodeParameter_Uint16(uint16(SYNC_PTY_COMPLETE__Code_PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder_SyncPtyComplete.EncodeParameter_Uint32(this.Id_PtyProxy)
	return binaryEncoder_SyncPtyComplete.Bytes()
}
