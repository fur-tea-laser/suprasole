package main

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
	Emit(websocketController *_WebsocketController_)
}

type _PtyBulletin_WorkspaceManifest_ struct {
	Id_PtyProxy      uint32
	IsVisible        bool
	MaybeExitOutcome _ExitOutcome_PtyProxy_
}

type _WorkspaceManifest_Message_ struct {
	PtyBulletins []_PtyBulletin_WorkspaceManifest_
}

func (this _WorkspaceManifest_Message_) Emit(
	websocketController *_WebsocketController_,
) {
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

type _PtyExit_Message_ struct {
	Id_PtyProxy uint32
	ExitOutcome _ExitOutcome_PtyProxy_
}

func (this _PtyExit_Message_) Emit(
	websocketController *_WebsocketController_,
) {
}

type _PtyOutput_Message_ struct {
	Id_PtyProxy uint32
	OutputData  []byte
}

func (this _PtyOutput_Message_) Emit(
	websocketController *_WebsocketController_,
) {
}

type _SyncPtyStart_Message_ struct {
	Id_PtyProxy             uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

func (this _SyncPtyStart_Message_) Emit(
	websocketController *_WebsocketController_,
) {
}

type _SyncPtyComplete_Message_ struct {
	Id_PtyProxy uint32
}

func (this _SyncPtyComplete_Message_) Emit(
	websocketController *_WebsocketController_,
) {
}
