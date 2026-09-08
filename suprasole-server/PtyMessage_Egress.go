package main

import (
	_FMT "fmt"
)

type _Code__PtyMessage_Egress_ uint16

const (
	SPAWN_PTY_STATUS___Code__PtyMessage_Egress   _Code__PtyMessage_Egress_ = 0x0002
	PTY_EXIT___Code__PtyMessage_Egress           _Code__PtyMessage_Egress_ = 0x0005
	PTY_OUTPUT___Code__PtyMessage_Egress         _Code__PtyMessage_Egress_ = 0x0008
	WORKSPACE_MANIFEST___Code__PtyMessage_Egress _Code__PtyMessage_Egress_ = 0x0009
	START__SYNC_PTY___Code__PtyMessage_Egress    _Code__PtyMessage_Egress_ = 0x000b
	COMPLETE__SYNC_PTY___Code__PtyMessage_Egress _Code__PtyMessage_Egress_ = 0x000c
)

type _PtyMessage_Egress_ interface {
	EncodePayload() []byte
}

func Emit__PtyMessage_Egress__WebsocketController_Pty(
	WebsocketController_Pty *_WebsocketController_,
	expectedId_WebsocketConnection uint64,
	egressMessage _PtyMessage_Egress_,
) {
	_ = WebsocketController_Pty.WritePayload_BinaryMessage(
		expectedId_WebsocketConnection,
		egressMessage.EncodePayload(),
	)
}

type _PtyBulletin_WorkspaceManifest_ struct {
	Id_PtyProxy               uint32
	MaybeExitOutcome_PtyProxy _ExitOutcome_PtyProxy_
	IsVisible_Client          bool
}

type _WorkspaceManifest__PtyMessage_Egress_ struct {
	PtyBulletins []_PtyBulletin_WorkspaceManifest_
}

func encodeStruct__ExitOutcome_PtyProxy(
	binaryEncoder *_BinaryEncoder_WebsocketMessage_,
	exitOutcome_PtyProxy _ExitOutcome_PtyProxy_,
) {
	switch theExitOutcome := exitOutcome_PtyProxy.(type) {
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
		binaryEncoder.EncodeParameter_Int32(int32(theExitOutcome.ExitCode_PtyProcess))
		// ExitSignal
		binaryEncoder.EncodeParameter_Int32(0)
	case _Killed__ExitOutcome_PtyProxy_:
		// ExitReason
		binaryEncoder.EncodeParameter_Uint8(uint8(KILLED__ExitReason_PtyProxy))
		// ExitCode
		binaryEncoder.EncodeParameter_Int32(0)
		// ExitSignal
		binaryEncoder.EncodeParameter_Int32(int32(theExitOutcome.ExitSignal_PtyProcess))
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

func (this _WorkspaceManifest__PtyMessage_Egress_) EncodePayload() []byte {
	binaryEncoder_WorkspaceManifest := New__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_WorkspaceManifest.EncodeParameter_Uint16(uint16(WORKSPACE_MANIFEST___Code__PtyMessage_Egress))
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

type _Status_SpawnPty_ byte

const (
	SUCCESS__Status_SpawnPty _Status_SpawnPty_ = 0x00
	FAILURE__Status_SpawnPty _Status_SpawnPty_ = 0x01
)

type _SpawnPtyStatus__PtyMessage_Egress_ struct {
	Status_SpawnPty _Status_SpawnPty_
	Id_PtyProxy     uint32
}

func (this _SpawnPtyStatus__PtyMessage_Egress_) EncodePayload() []byte {
	binaryEncoder_SpawnPtyStatus := New__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_SpawnPtyStatus.EncodeParameter_Uint16(uint16(SPAWN_PTY_STATUS___Code__PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder_SpawnPtyStatus.EncodeParameter_Uint32(this.Id_PtyProxy)
	// Status_SpawnPty
	binaryEncoder_SpawnPtyStatus.EncodeParameter_Uint8(uint8(this.Status_SpawnPty))
	return binaryEncoder_SpawnPtyStatus.Bytes()
}

type _PtyExit__PtyMessage_Egress_ struct {
	Id_PtyProxy          uint32
	ExitOutcome_PtyProxy _ExitOutcome_PtyProxy_
}

func (this _PtyExit__PtyMessage_Egress_) EncodePayload() []byte {
	binaryEncoder_PtyExit := New__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_PtyExit.EncodeParameter_Uint16(uint16(PTY_EXIT___Code__PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder_PtyExit.EncodeParameter_Uint32(this.Id_PtyProxy)
	// ExitOutcome_PtyProxy
	encodeStruct__ExitOutcome_PtyProxy(
		&binaryEncoder_PtyExit,
		this.ExitOutcome_PtyProxy,
	)
	return binaryEncoder_PtyExit.Bytes()
}

type _PtyOutput__PtyMessage_Egress_ struct {
	Id_PtyProxy         uint32
	OutputData_PtyProxy []byte
}

func (this _PtyOutput__PtyMessage_Egress_) EncodePayload() []byte {
	binaryEncoder_PtyOutput := New__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_PtyOutput.EncodeParameter_Uint16(uint16(PTY_OUTPUT___Code__PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder_PtyOutput.EncodeParameter_Uint32(this.Id_PtyProxy)
	// OutputData_PtyProxy
	binaryEncoder_PtyOutput.EncodeParameter_TrailingBytes(this.OutputData_PtyProxy)
	return binaryEncoder_PtyOutput.Bytes()
}

type _Start_SyncPty__PtyMessage_Egress_ struct {
	Id_PtyProxy             uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

func (this _Start_SyncPty__PtyMessage_Egress_) EncodePayload() []byte {
	binaryEncoder_Start_SyncPty := New__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_Start_SyncPty.EncodeParameter_Uint16(uint16(START__SYNC_PTY___Code__PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder_Start_SyncPty.EncodeParameter_Uint32(this.Id_PtyProxy)
	// ColumnCount_PtyTerminal
	binaryEncoder_Start_SyncPty.EncodeParameter_Uint16(uint16(this.ColumnCount_PtyTerminal))
	// RowCount_PtyTerminal
	binaryEncoder_Start_SyncPty.EncodeParameter_Uint16(uint16(this.RowCount_PtyTerminal))
	return binaryEncoder_Start_SyncPty.Bytes()
}

type _Complete_SyncPty__PtyMessage_Egress_ struct {
	Id_PtyProxy uint32
}

func (this _Complete_SyncPty__PtyMessage_Egress_) EncodePayload() []byte {
	binaryEncoder_Complete_SyncPty := New__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_Complete_SyncPty.EncodeParameter_Uint16(uint16(COMPLETE__SYNC_PTY___Code__PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder_Complete_SyncPty.EncodeParameter_Uint32(this.Id_PtyProxy)
	return binaryEncoder_Complete_SyncPty.Bytes()
}
