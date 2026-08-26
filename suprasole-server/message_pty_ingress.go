package main

type Code_PtyMessage_Ingress uint16

const (
	SPAWN_PTY__Code_PtyMessage_Ingress          Code_PtyMessage_Ingress = 0x0001
	RESIZE_PTY__Code_PtyMessage_Ingress         Code_PtyMessage_Ingress = 0x0003
	TERMINATE_PTY__Code_PtyMessage_Ingress      Code_PtyMessage_Ingress = 0x0004
	REMOVE_PTY__Code_PtyMessage_Ingress         Code_PtyMessage_Ingress = 0x0006
	WRITE_PTY_INPUT__Code_PtyMessage_Ingress    Code_PtyMessage_Ingress = 0x0007
	SET_PTY_PRIORITIES__Code_PtyMessage_Ingress Code_PtyMessage_Ingress = 0x0009
	RESYNC_WORKSPACE__Code_PtyMessage_Ingress   Code_PtyMessage_Ingress = 0x000e
)

type _PtyMessage_Ingress_ interface {
	Execute(workspaceController *_WorkspaceController_)
	compiletimemarker_PtyMessage_Ingress()
}

var DECODE_MESSAGE_MAP__PTY_MESSAGE_INGRESS = map[Code_PtyMessage_Ingress]func(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error){
	SPAWN_PTY__Code_PtyMessage_Ingress:          decodeMessage_SpawnPty,
	RESIZE_PTY__Code_PtyMessage_Ingress:         decodeMessage_ResizePty,
	TERMINATE_PTY__Code_PtyMessage_Ingress:      decodeMessage_TerminatePty,
	REMOVE_PTY__Code_PtyMessage_Ingress:         decodeMessage_RemovePty,
	WRITE_PTY_INPUT__Code_PtyMessage_Ingress:    decodeMessage_WritePtyInput,
	SET_PTY_PRIORITIES__Code_PtyMessage_Ingress: decodeMessage_SetPtyPriorities,
	RESYNC_WORKSPACE__Code_PtyMessage_Ingress:   decodeMessage_ResyncWorkspace,
}

func decodeMessage_SpawnPty(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _PtyProxyOption_ interface {
	UpdateOptionsResult(optionsResult_ptyProxy *_PtyProxyDefaults_)
	compiletimemarker_PtyProxyOption()
}

type _PtyProxyOption_ScrollbackLineCount_ struct {
	ScrollbackLineCount_PtyTerminal int
}

func (this _PtyProxyOption_ScrollbackLineCount_) UpdateOptionsResult(
	optionsResult_ptyProxy *_PtyProxyDefaults_,
) {
	optionsResult_ptyProxy.ScrollbackLineCount_PtyTerminal = this.ScrollbackLineCount_PtyTerminal
}

func (_PtyProxyOption_ScrollbackLineCount_) compiletimemarker_PtyProxyOption() {}

type _PtyProxyOption_StagingBufferSize_ struct {
	StagingBufferSize_PtyReader int
}

func (this _PtyProxyOption_StagingBufferSize_) UpdateOptionsResult(
	optionsResult_ptyProxy *_PtyProxyDefaults_,
) {
	optionsResult_ptyProxy.StagingBufferSize_PtyReader = this.StagingBufferSize_PtyReader
}

func (_PtyProxyOption_StagingBufferSize_) compiletimemarker_PtyProxyOption() {}

type _PtyProxyOption_PostSnapshotBufferSize_ struct {
	PostSnapshotBufferSize_PtyProxy int
}

func (this _PtyProxyOption_PostSnapshotBufferSize_) UpdateOptionsResult(
	optionsResult_ptyProxy *_PtyProxyDefaults_,
) {
	optionsResult_ptyProxy.PostSnapshotBufferSize_PtyProxy = this.PostSnapshotBufferSize_PtyProxy
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

func (_SpawnPtyMessage_) compiletimemarker_PtyMessage_Ingress() {}

func (this _SpawnPtyMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
	optionsResult_ptyProxy := workspaceController.PtyProxyDefaults
	for _, option_ptyProxy := range this.Options_PtyProxy {
		option_ptyProxy.UpdateOptionsResult(&optionsResult_ptyProxy)
	}
	workspaceController.Mutex.Lock()
	newPtyId := workspaceController.NextPtyId
	workspaceController.NextPtyId++
	workspaceController.Mutex.Unlock()
	go func() {
		_, ptyStartError := NewPtyProxy(_NewPtyProxyApi_{
			Id:                              newPtyId,
			ColumnCount_PtyTerminal:         this.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:            this.RowCount_PtyTerminal,
			ShellBinaryPath_PtyCommand:      this.ShellBinaryPath_PtyCommand,
			DirectoryPath_PtyCommand:        this.DirectoryPath_PtyCommand,
			EnvironmentVariables_PtyCommand: this.EnvironmentVariables_PtyCommand,
			ScrollbackLineCount_PtyTerminal: optionsResult_ptyProxy.ScrollbackLineCount_PtyTerminal,
			StagingBufferSize_PtyReader:     optionsResult_ptyProxy.StagingBufferSize_PtyReader,
			PostSnapshotBufferSize_PtyProxy: optionsResult_ptyProxy.PostSnapshotBufferSize_PtyProxy,
			OnPtySpawned:                    workspaceController.HandlePtySpawned,
			OnOutput_Live:                   workspaceController.HandlePtyOutput,
			OnOutput_Snapshot:               workspaceController.HandlePtyOutput,
			OnOutput_PostSnapshotBuffer:     workspaceController.HandlePtyOutput,
			OnExited_Eio_Success:            workspaceController.HandlePtyExited_Eio_Success,
			OnExited_Eio_Failure:            workspaceController.HandlePtyExited_Eio_Failure,
			OnExited_Eio_Killed:             workspaceController.HandlePtyExited_Eio_Killed,
			OnExited_Closed:                 workspaceController.HandlePtyExited_Closed,
			OnExited_SystemError:            workspaceController.HandlePtyExited_SystemError,
		})
		if ptyStartError != nil {
			workspaceController.HandlePtySpawnFailed(newPtyId)
		}
	}()
}

func decodeMessage_ResizePty(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _ResizePtyMessage_ struct {
	TargetPtyId             uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

func (_ResizePtyMessage_) compiletimemarker_PtyMessage_Ingress() {}

func (this _ResizePtyMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_TerminatePty(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _TerminatePtyMessage_ struct {
	TargetPtyId uint32
	Signal      int
}

func (_TerminatePtyMessage_) compiletimemarker_PtyMessage_Ingress() {}

func (this _TerminatePtyMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_RemovePty(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _RemovePtyMessage_ struct {
	TargetPtyIds []uint32
}

func (_RemovePtyMessage_) compiletimemarker_PtyMessage_Ingress() {}

func (this _RemovePtyMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_WritePtyInput(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _WritePtyInputMessage_ struct {
	TargetPtyId uint32
	StdinData   []byte
}

func (_WritePtyInputMessage_) compiletimemarker_PtyMessage_Ingress() {}

func (this _WritePtyInputMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_SetPtyPriorities(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _SetPtyPrioritiesMessage_ struct {
	ActivePtyIds []uint32
}

func (_SetPtyPrioritiesMessage_) compiletimemarker_PtyMessage_Ingress() {}

func (this _SetPtyPrioritiesMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_ResyncWorkspace(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _ResyncWorkspaceTerminalDescriptor_ struct {
	PtyId                   uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
	IsActive_WorkspacePty   bool
}

type _ResyncWorkspaceMessage_ struct {
	Terminals []_ResyncWorkspaceTerminalDescriptor_
}

func (_ResyncWorkspaceMessage_) compiletimemarker_PtyMessage_Ingress() {}

func (this _ResyncWorkspaceMessage_) Execute(
	workspaceController *_WorkspaceController_,
) {
}
