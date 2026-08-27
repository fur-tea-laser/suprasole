package main

type Code_PtyMessage_Egress uint16

const (
	SPAWN_PTY_STATUS__Code_PtyMessage_Egress    Code_PtyMessage_Egress = 0x0002
	PTY_EXIT__Code_PtyMessage_Egress            Code_PtyMessage_Egress = 0x0005
	PTY_OUTPUT__Code_PtyMessage_Egress          Code_PtyMessage_Egress = 0x0008
	RESYNC_PTY_START__Code_PtyMessage_Egress    Code_PtyMessage_Egress = 0x000c
	RESYNC_PTY_COMPLETE__Code_PtyMessage_Egress Code_PtyMessage_Egress = 0x000d
	RESYNC_PTY_FAILED__Code_PtyMessage_Egress   Code_PtyMessage_Egress = 0x000f
)

type _PtyMessage_Egress_ interface {
	Emit(websocketController *_WebsocketController_)
	compiletimemarker_PtyMessage_Egress()
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

func (this _SpawnPtyStatus_Message_) Emit(
	websocketController *_WebsocketController_,
) {
}

func (_SpawnPtyStatus_Message_) compiletimemarker_PtyMessage_Egress() {}

type _PtyExit_Message_ struct {
	Id_PtyProxy uint32
	ExitResult  _PtyExitResult_
}

func (this _PtyExit_Message_) Emit(
	websocketController *_WebsocketController_,
) {
}

func (_PtyExit_Message_) compiletimemarker_PtyMessage_Egress() {}

type _PtyOutput_Message_ struct {
	Id_PtyProxy uint32
	OutputData  []byte
}

func (this _PtyOutput_Message_) Emit(
	websocketController *_WebsocketController_,
) {
}

func (_PtyOutput_Message_) compiletimemarker_PtyMessage_Egress() {}

type _ResyncPtyStart_Message_ struct {
	Id_PtyProxy             uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

func (this _ResyncPtyStart_Message_) Emit(
	websocketController *_WebsocketController_,
) {
}

func (_ResyncPtyStart_Message_) compiletimemarker_PtyMessage_Egress() {}

type _ResyncPtyComplete_Message_ struct {
	Id_PtyProxy uint32
}

func (this _ResyncPtyComplete_Message_) Emit(
	websocketController *_WebsocketController_,
) {
}

func (_ResyncPtyComplete_Message_) compiletimemarker_PtyMessage_Egress() {}

type _ResyncPtyFailed_Message_ struct {
	Id_PtyProxy uint32
	ErrorCode   uint16
	Reason      string
}

func (this _ResyncPtyFailed_Message_) Emit(
	websocketController *_WebsocketController_,
) {
}

func (_ResyncPtyFailed_Message_) compiletimemarker_PtyMessage_Egress() {}
