package main

type Code_PtyMessage_Ingress uint16

const (
	SPAWN_PTY__Code_PtyMessage_Ingress            Code_PtyMessage_Ingress = 0x0001
	RESIZE_PTYS__Code_PtyMessage_Ingress          Code_PtyMessage_Ingress = 0x0003
	WRITE_PTY_INPUT__Code_PtyMessage_Ingress      Code_PtyMessage_Ingress = 0x0007
	TERMINATE_PTY__Code_PtyMessage_Ingress        Code_PtyMessage_Ingress = 0x0004
	REMOVE_PTY__Code_PtyMessage_Ingress           Code_PtyMessage_Ingress = 0x0006
	SET_PTY_VISIBILITIES__Code_PtyMessage_Ingress Code_PtyMessage_Ingress = 0x0009
	RESYNC_WORKSPACE__Code_PtyMessage_Ingress     Code_PtyMessage_Ingress = 0x000e
)

type _PtyMessage_Ingress_ interface {
	Execute(workspaceController *_WorkspaceController_)
	compiletimemarker_PtyMessage_Ingress()
}

var DECODE_MESSAGE_MAP__PTY_MESSAGE_INGRESS = map[Code_PtyMessage_Ingress]func(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error){
	SPAWN_PTY__Code_PtyMessage_Ingress:            decodeMessage_SpawnPty,
	RESIZE_PTYS__Code_PtyMessage_Ingress:          decodeMessage_ResizePtys,
	WRITE_PTY_INPUT__Code_PtyMessage_Ingress:      decodeMessage_WritePtyInput,
	TERMINATE_PTY__Code_PtyMessage_Ingress:        decodeMessage_TerminatePty,
	REMOVE_PTY__Code_PtyMessage_Ingress:           decodeMessage_RemovePty,
	SET_PTY_VISIBILITIES__Code_PtyMessage_Ingress: decodeMessage_SetPtyVisibilities,
	RESYNC_WORKSPACE__Code_PtyMessage_Ingress:     decodeMessage_ResyncWorkspace,
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

type _SpawnPty_Message_ struct {
	ColumnCount_PtyTerminal         int
	RowCount_PtyTerminal            int
	ShellBinaryPath_PtyCommand      string
	DirectoryPath_PtyCommand        string
	EnvironmentVariables_PtyCommand []string
	Options_PtyProxy                []_PtyProxyOption_
}

func (_SpawnPty_Message_) compiletimemarker_PtyMessage_Ingress() {}

func (this _SpawnPty_Message_) Execute(
	workspaceController *_WorkspaceController_,
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
			Id:                              newPtyId,
			ColumnCount_PtyTerminal:         this.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:            this.RowCount_PtyTerminal,
			ShellBinaryPath_PtyCommand:      this.ShellBinaryPath_PtyCommand,
			DirectoryPath_PtyCommand:        this.DirectoryPath_PtyCommand,
			EnvironmentVariables_PtyCommand: this.EnvironmentVariables_PtyCommand,
			ScrollbackLineCount_PtyTerminal: optionsResult_ptyProxy.ScrollbackLineCount_PtyTerminal,
			StagingBufferSize_PtyReader:     optionsResult_ptyProxy.StagingBufferSize_PtyReader,
			PostSnapshotBufferSize_PtyProxy: optionsResult_ptyProxy.PostSnapshotBufferSize_PtyProxy,
			OnSpawned:                       workspaceController.HandleSpawned_Pty,
			OnOutput_Live:                   workspaceController.HandleOutput_Pty,
			OnOutput_Snapshot:               workspaceController.HandleOutput_Pty,
			OnOutput_PostSnapshotBuffer:     workspaceController.HandleOutput_Pty,
			OnExited_Eio_Success:            workspaceController.HandleExited_Eio_Success__Pty,
			OnExited_Eio_Failure:            workspaceController.HandleExited_Eio_Failure__Pty,
			OnExited_Eio_Killed:             workspaceController.HandleExited_Eio_Killed__Pty,
			OnExited_Closed:                 workspaceController.HandleExited_Closed__Pty,
			OnExited_SystemError:            workspaceController.HandleExited_SystemError__Pty,
		})
		if ptyStartError != nil {
			workspaceController.HandleSpawnFailed_Pty(newPtyId)
		}
	}()
}

func decodeMessage_ResizePtys(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _ResizePtys_Entry_ struct {
	Id_PtyProxy             uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

type _ResizePtys_Message_ struct {
	Entries_PtyProxy []_ResizePtys_Entry_
}

func (_ResizePtys_Message_) compiletimemarker_PtyMessage_Ingress() {}

func (this _ResizePtys_Message_) Execute(
	workspaceController *_WorkspaceController_,
) {
	workspaceController.MessageDebouncer_ResizePtys.QueueChannel <- this
}

func decodeMessage_WritePtyInput(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _WritePtyInput_Message_ struct {
	Id_PtyProxy                       uint32
	InputData_PtyMasterFileDescriptor []byte
}

func (_WritePtyInput_Message_) compiletimemarker_PtyMessage_Ingress() {}

func (this _WritePtyInput_Message_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_TerminatePty(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _TerminatePty_Message_ struct {
	Id_PtyProxy uint32
	Signal      int
}

func (_TerminatePty_Message_) compiletimemarker_PtyMessage_Ingress() {}

func (this _TerminatePty_Message_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_RemovePty(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _RemovePty_Message_ struct {
	TargetPtyIds []uint32
}

func (_RemovePty_Message_) compiletimemarker_PtyMessage_Ingress() {}

func (this _RemovePty_Message_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_SetPtyVisibilities(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _SetPtyVisibilities_Message_ struct {
	VisiblePtyIds []uint32
}

func (_SetPtyVisibilities_Message_) compiletimemarker_PtyMessage_Ingress() {}

func (this _SetPtyVisibilities_Message_) Execute(
	workspaceController *_WorkspaceController_,
) {
}

func decodeMessage_ResyncWorkspace(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _ResyncWorkspace_Entry_ struct {
	Id_PtyProxy             uint32
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
	IsVisible_WorkspacePty  bool
}

type _ResyncWorkspace_Message_ struct {
	Entries []_ResyncWorkspace_Entry_
}

func (_ResyncWorkspace_Message_) compiletimemarker_PtyMessage_Ingress() {}

func (this _ResyncWorkspace_Message_) Execute(
	workspaceController *_WorkspaceController_,
) {
}
