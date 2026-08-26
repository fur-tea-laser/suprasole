package main

type MessageCode_PtyWebsocket uint16

const (
	SPAWN_PTY__MessageCode_PtyWebsocket           MessageCode_PtyWebsocket = 0x0001
	SPAWN_PTY_STATUS__MessageCode_PtyWebsocket    MessageCode_PtyWebsocket = 0x0002
	RESIZE_PTY__MessageCode_PtyWebsocket          MessageCode_PtyWebsocket = 0x0003
	TERMINATE_PTY__MessageCode_PtyWebsocket       MessageCode_PtyWebsocket = 0x0004
	PTY_EXIT__MessageCode_PtyWebsocket            MessageCode_PtyWebsocket = 0x0005
	REMOVE_PTY__MessageCode_PtyWebsocket          MessageCode_PtyWebsocket = 0x0006
	WRITE_PTY_INPUT__MessageCode_PtyWebsocket     MessageCode_PtyWebsocket = 0x0007
	PTY_OUTPUT__MessageCode_PtyWebsocket          MessageCode_PtyWebsocket = 0x0008
	SET_PTY_PRIORITIES__MessageCode_PtyWebsocket  MessageCode_PtyWebsocket = 0x0009
	RESYNC_PTY_START__MessageCode_PtyWebsocket    MessageCode_PtyWebsocket = 0x000c
	RESYNC_PTY_COMPLETE__MessageCode_PtyWebsocket MessageCode_PtyWebsocket = 0x000d
	RESYNC_WORKSPACE__MessageCode_PtyWebsocket    MessageCode_PtyWebsocket = 0x000e
	RESYNC_PTY_FAILED__MessageCode_PtyWebsocket   MessageCode_PtyWebsocket = 0x000f
)

type _PtyMessage_ interface {
	Execute(workspaceController *_WorkspaceController_)
	compiletimemarker_PtyMessage()
}

type _PtyMessageDecoder_ func(binaryMessageFrame []byte) (_PtyMessage_, error)

var DECODE_MESSAGE_MAP__PTY_WEBSOCKET = map[MessageCode_PtyWebsocket]_PtyMessageDecoder_{
	SPAWN_PTY__MessageCode_PtyWebsocket:          decodeMessage_SpawnPty,
	RESIZE_PTY__MessageCode_PtyWebsocket:         decodeMessage_ResizePty,
	TERMINATE_PTY__MessageCode_PtyWebsocket:      decodeMessage_TerminatePty,
	REMOVE_PTY__MessageCode_PtyWebsocket:         decodeMessage_RemovePty,
	WRITE_PTY_INPUT__MessageCode_PtyWebsocket:    decodeMessage_WritePtyInput,
	SET_PTY_PRIORITIES__MessageCode_PtyWebsocket: decodeMessage_SetPtyPriorities,
	RESYNC_WORKSPACE__MessageCode_PtyWebsocket:   decodeMessage_ResyncWorkspace,
}

func decodeMessage_SpawnPty(
	binaryMessageFrame []byte,
) (_PtyMessage_, error) {
	return nil, nil
}

type _PtyProxyOption_ interface {
	compiletimemarker_PtyProxyOption()
}

type _PtyProxyOption_ScrollbackLineCount_ struct {
	ScrollbackLineCount_PtyTerminal int
}

func (_PtyProxyOption_ScrollbackLineCount_) compiletimemarker_PtyProxyOption() {}

type _PtyProxyOption_StagingBufferSize_ struct {
	StagingBufferSize_PtyReader int
}

func (_PtyProxyOption_StagingBufferSize_) compiletimemarker_PtyProxyOption() {}

type _PtyProxyOption_PostSnapshotBufferSize_ struct {
	PostSnapshotBufferSize_PtyProxy int
}

func (_PtyProxyOption_PostSnapshotBufferSize_) compiletimemarker_PtyProxyOption() {}

type _SpawnPtyMessage_ struct {
	ColumnCount_PtyTerminal         int
	RowCount_PtyTerminal            int
	ShellBinaryPath_PtyCommand      string
	DirectoryPath_PtyCommand        string
	EnvironmentVariables_PtyCommand []string
	Options_PtyProxy                []_PtyProxyOption_
}

func (_SpawnPtyMessage_) compiletimemarker_PtyMessage() {}

func (this _SpawnPtyMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_ResizePty(
	binaryMessageFrame []byte,
) (_PtyMessage_, error) {
	return nil, nil
}

type _ResizePtyMessage_ struct {
	TargetPtyId             uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

func (_ResizePtyMessage_) compiletimemarker_PtyMessage() {}

func (this _ResizePtyMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_TerminatePty(
	binaryMessageFrame []byte,
) (_PtyMessage_, error) {
	return nil, nil
}

type _TerminatePtyMessage_ struct {
	TargetPtyId uint32
	Signal      int
}

func (_TerminatePtyMessage_) compiletimemarker_PtyMessage() {}

func (this _TerminatePtyMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_RemovePty(
	binaryMessageFrame []byte,
) (_PtyMessage_, error) {
	return nil, nil
}

type _RemovePtyMessage_ struct {
	TargetPtyIds []uint32
}

func (_RemovePtyMessage_) compiletimemarker_PtyMessage() {}

func (this _RemovePtyMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_WritePtyInput(
	binaryMessageFrame []byte,
) (_PtyMessage_, error) {
	return nil, nil
}

type _WritePtyInputMessage_ struct {
	TargetPtyId uint32
	StdinData   []byte
}

func (_WritePtyInputMessage_) compiletimemarker_PtyMessage() {}

func (this _WritePtyInputMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_SetPtyPriorities(
	binaryMessageFrame []byte,
) (_PtyMessage_, error) {
	return nil, nil
}

type _SetPtyPrioritiesMessage_ struct {
	ActivePtyIds []uint32
}

func (_SetPtyPrioritiesMessage_) compiletimemarker_PtyMessage() {}

func (this _SetPtyPrioritiesMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_ResyncWorkspace(
	binaryMessageFrame []byte,
) (_PtyMessage_, error) {
	return nil, nil
}

type _ResyncWorkspaceTerminalDescriptor_ struct {
	PtyId                   uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
	IsActive                bool
}

type _ResyncWorkspaceMessage_ struct {
	Terminals []_ResyncWorkspaceTerminalDescriptor_
}

func (_ResyncWorkspaceMessage_) compiletimemarker_PtyMessage() {}

func (this _ResyncWorkspaceMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}
