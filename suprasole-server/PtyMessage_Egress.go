package main

import (
	_FMT "fmt"
)

type _Code__PtyMessage_Egress_ uint16

const (
	STATUS__SPAWN_PTY___Code__PtyMessage_Egress       _Code__PtyMessage_Egress_ = 0x0002
	PTY_EXIT___Code__PtyMessage_Egress                _Code__PtyMessage_Egress_ = 0x0005
	PTY_OUTPUT___Code__PtyMessage_Egress              _Code__PtyMessage_Egress_ = 0x0008
	WORKSPACE_MANIFEST___Code__PtyMessage_Egress      _Code__PtyMessage_Egress_ = 0x0009
	START_TASK__SYNC_PTY___Code__PtyMessage_Egress    _Code__PtyMessage_Egress_ = 0x000b
	COMPLETE_TASK__SYNC_PTY___Code__PtyMessage_Egress _Code__PtyMessage_Egress_ = 0x000c
)

type _PtyMessage_Egress_ interface {
	EncodePayload() []byte
}

func Emit__PtyMessage_Egress__WebsocketController_Pty(
	WebsocketController_Pty *_WebsocketController_,
	id_WebsocketConnection_expected uint64,
	egressMessage _PtyMessage_Egress_,
) {
	_ = WebsocketController_Pty.WritePayload_BinaryMessage(
		id_WebsocketConnection_expected,
		egressMessage.EncodePayload(),
	)
}

type _Bulletin_WorkspacePty_ struct {
	Id_PtyProxy                uint32
	ExitOutcome_PtyProxy_maybe _ExitOutcome_PtyProxy_
	Visibility_Client_current  _Visibility_Client_
}

type _WorkspaceManifest__PtyMessage_Egress_ struct {
	BulletinBatch_WorkspacePty []_Bulletin_WorkspacePty_
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
	binaryEncoder_WorkspaceManifest := Make__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_WorkspaceManifest.EncodeParameter_Uint16(uint16(WORKSPACE_MANIFEST___Code__PtyMessage_Egress))
	// BulletinBatch_WorkspacePty
	EncodeParameter__Slice16__BinaryEncoder_WebsocketMessage(
		&binaryEncoder_WorkspaceManifest,
		this.BulletinBatch_WorkspacePty,
		func(binaryEncoder_WorkspaceManifest *_BinaryEncoder_WebsocketMessage_, sliceIndex_current int, bulletin_WorkspacePty _Bulletin_WorkspacePty_) {
			// Id_PtyProxy
			binaryEncoder_WorkspaceManifest.EncodeParameter_Uint32(bulletin_WorkspacePty.Id_PtyProxy)
			// Visibility_Client_current
			binaryEncoder_WorkspaceManifest.EncodeParameter_Bool(VISIBLE__Visibility_Client == bulletin_WorkspacePty.Visibility_Client_current)
			if bulletin_WorkspacePty.ExitOutcome_PtyProxy_maybe != nil {
				// HasExitOutcome
				binaryEncoder_WorkspaceManifest.EncodeParameter_Bool(true)
				encodeStruct__ExitOutcome_PtyProxy(
					binaryEncoder_WorkspaceManifest,
					bulletin_WorkspacePty.ExitOutcome_PtyProxy_maybe,
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

type _Status_SpawnPty__PtyMessage_Egress_ struct {
	Status_SpawnPty _Status_SpawnPty_
	Id_PtyProxy     uint32
}

func (this _Status_SpawnPty__PtyMessage_Egress_) EncodePayload() []byte {
	binaryEncoder__Status_SpawnPty := Make__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder__Status_SpawnPty.EncodeParameter_Uint16(uint16(STATUS__SPAWN_PTY___Code__PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder__Status_SpawnPty.EncodeParameter_Uint32(this.Id_PtyProxy)
	// Status_SpawnPty
	binaryEncoder__Status_SpawnPty.EncodeParameter_Uint8(uint8(this.Status_SpawnPty))
	return binaryEncoder__Status_SpawnPty.Bytes()
}

type _PtyExit__PtyMessage_Egress_ struct {
	Id_PtyProxy          uint32
	ExitOutcome_PtyProxy _ExitOutcome_PtyProxy_
}

func (this _PtyExit__PtyMessage_Egress_) EncodePayload() []byte {
	binaryEncoder_PtyExit := Make__BinaryEncoder_WebsocketMessage()
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
	binaryEncoder_PtyOutput := Make__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder_PtyOutput.EncodeParameter_Uint16(uint16(PTY_OUTPUT___Code__PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder_PtyOutput.EncodeParameter_Uint32(this.Id_PtyProxy)
	// OutputData_PtyProxy
	binaryEncoder_PtyOutput.EncodeParameter_TrailingBytes(this.OutputData_PtyProxy)
	return binaryEncoder_PtyOutput.Bytes()
}

type _StartTask_SyncPty__PtyMessage_Egress_ struct {
	Id_PtyProxy uint32
}

func (this _StartTask_SyncPty__PtyMessage_Egress_) EncodePayload() []byte {
	binaryEncoder__StartTask_SyncPty := Make__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder__StartTask_SyncPty.EncodeParameter_Uint16(uint16(START_TASK__SYNC_PTY___Code__PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder__StartTask_SyncPty.EncodeParameter_Uint32(this.Id_PtyProxy)
	return binaryEncoder__StartTask_SyncPty.Bytes()
}

type _CompleteTask_SyncPty__PtyMessage_Egress_ struct {
	Id_PtyProxy uint32
}

func (this _CompleteTask_SyncPty__PtyMessage_Egress_) EncodePayload() []byte {
	binaryEncoder__CompleteTask_SyncPty := Make__BinaryEncoder_WebsocketMessage()
	// Opcode
	binaryEncoder__CompleteTask_SyncPty.EncodeParameter_Uint16(uint16(COMPLETE_TASK__SYNC_PTY___Code__PtyMessage_Egress))
	// Id_PtyProxy
	binaryEncoder__CompleteTask_SyncPty.EncodeParameter_Uint32(this.Id_PtyProxy)
	return binaryEncoder__CompleteTask_SyncPty.Bytes()
}
