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

type _SpawnPtyStatusMessage_ struct {
	PtyId  uint32
	Status Status_SpawnPty
}

func (this _SpawnPtyStatusMessage_) Emit(
	websocketController *_WebsocketController_,
) {
}

func (_SpawnPtyStatusMessage_) compiletimemarker_PtyMessage_Egress() {}

type _PtyExitMessage_ struct {
	PtyId      uint32
	ExitResult _PtyExitResult_
}

func (this _PtyExitMessage_) Emit(
	websocketController *_WebsocketController_,
) {
}

func (_PtyExitMessage_) compiletimemarker_PtyMessage_Egress() {}

type _PtyOutputMessage_ struct {
	PtyId      uint32
	OutputData []byte
}

func (this _PtyOutputMessage_) Emit(
	websocketController *_WebsocketController_,
) {
}

func (_PtyOutputMessage_) compiletimemarker_PtyMessage_Egress() {}

type _ResyncPtyStartMessage_ struct {
	PtyId                   uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

func (this _ResyncPtyStartMessage_) Emit(
	websocketController *_WebsocketController_,
) {
}

func (_ResyncPtyStartMessage_) compiletimemarker_PtyMessage_Egress() {}

type _ResyncPtyCompleteMessage_ struct {
	PtyId uint32
}

func (this _ResyncPtyCompleteMessage_) Emit(
	websocketController *_WebsocketController_,
) {
}

func (_ResyncPtyCompleteMessage_) compiletimemarker_PtyMessage_Egress() {}

type _ResyncPtyFailedMessage_ struct {
	PtyId     uint32
	ErrorCode uint16
	Reason    string
}

func (this _ResyncPtyFailedMessage_) Emit(
	websocketController *_WebsocketController_,
) {
}

func (_ResyncPtyFailedMessage_) compiletimemarker_PtyMessage_Egress() {}
