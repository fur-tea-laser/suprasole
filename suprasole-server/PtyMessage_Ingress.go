package main

type Code_PtyMessage_Ingress uint16

const (
	SPAWN_PTY__Code_PtyMessage_Ingress       Code_PtyMessage_Ingress = 0x0001
	RESIZE_PTYS__Code_PtyMessage_Ingress     Code_PtyMessage_Ingress = 0x0003
	TERMINATE_PTY__Code_PtyMessage_Ingress   Code_PtyMessage_Ingress = 0x0004
	REMOVE_PTY__Code_PtyMessage_Ingress      Code_PtyMessage_Ingress = 0x0006
	WRITE_PTY_INPUT__Code_PtyMessage_Ingress Code_PtyMessage_Ingress = 0x0007
	SYNC_WORKSPACE__Code_PtyMessage_Ingress  Code_PtyMessage_Ingress = 0x000a
)

type _PtyMessage_Ingress_ interface {
	Execute(workspaceController *_WorkspaceController_, id_WebsocketConnection uint64)
}

var DECODE_MESSAGE_MAP__PTY_MESSAGE_INGRESS = map[Code_PtyMessage_Ingress]func(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error){
	SPAWN_PTY__Code_PtyMessage_Ingress:       decodeMessage_SpawnPty,
	RESIZE_PTYS__Code_PtyMessage_Ingress:     decodeMessage_ResizePtys,
	TERMINATE_PTY__Code_PtyMessage_Ingress:   decodeMessage_TerminatePty,
	REMOVE_PTY__Code_PtyMessage_Ingress:      decodeMessage_RemovePty,
	WRITE_PTY_INPUT__Code_PtyMessage_Ingress: decodeMessage_WritePtyInput,
	SYNC_WORKSPACE__Code_PtyMessage_Ingress:  decodeMessage_SyncWorkspace,
}

func decodeMessage_SpawnPty(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _PtyProxyOption_ interface {
	UpdateOptionsResult(optionsResult_ptyProxy *_PtyProxyDefaults_)
}

type _PtyProxyOption_ScrollbackLineCount_ struct {
	ScrollbackLineCount_PtyTerminal int
}

func (this _PtyProxyOption_ScrollbackLineCount_) UpdateOptionsResult(
	optionsResult_ptyProxy *_PtyProxyDefaults_,
) {
	optionsResult_ptyProxy.ScrollbackLineCount_PtyTerminal = this.ScrollbackLineCount_PtyTerminal
}

type _PtyProxyOption_StagingBufferSize_ struct {
	StagingBufferSize_PtyReader int
}

func (this _PtyProxyOption_StagingBufferSize_) UpdateOptionsResult(
	optionsResult_ptyProxy *_PtyProxyDefaults_,
) {
	optionsResult_ptyProxy.StagingBufferSize_PtyReader = this.StagingBufferSize_PtyReader
}

type _PtyProxyOption_PostSnapshotBufferSize_ struct {
	PostSnapshotBufferSize_PtyProxy int
}

func (this _PtyProxyOption_PostSnapshotBufferSize_) UpdateOptionsResult(
	optionsResult_ptyProxy *_PtyProxyDefaults_,
) {
	optionsResult_ptyProxy.PostSnapshotBufferSize_PtyProxy = this.PostSnapshotBufferSize_PtyProxy
}

type _PtyProxyOption_QueueBufferSize_InputOrder__PtyWriter_ struct {
	QueueBufferSize_InputOrder__PtyWriter int
}

func (this _PtyProxyOption_QueueBufferSize_InputOrder__PtyWriter_) UpdateOptionsResult(
	optionsResult_ptyProxy *_PtyProxyDefaults_,
) {
	optionsResult_ptyProxy.QueueBufferSize_InputOrder__PtyWriter = this.QueueBufferSize_InputOrder__PtyWriter
}

type _SpawnPty_Message_ struct {
	ColumnCount_PtyTerminal         int
	RowCount_PtyTerminal            int
	ShellBinaryPath_PtyCommand      string
	DirectoryPath_PtyCommand        string
	EnvironmentVariables_PtyCommand []string
	Options_PtyProxy                []_PtyProxyOption_
}

func (this _SpawnPty_Message_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
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
			Id:                                    newPtyId,
			ColumnCount_PtyTerminal:               this.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:                  this.RowCount_PtyTerminal,
			ShellBinaryPath_PtyCommand:            this.ShellBinaryPath_PtyCommand,
			DirectoryPath_PtyCommand:              this.DirectoryPath_PtyCommand,
			EnvironmentVariables_PtyCommand:       this.EnvironmentVariables_PtyCommand,
			ScrollbackLineCount_PtyTerminal:       optionsResult_ptyProxy.ScrollbackLineCount_PtyTerminal,
			StagingBufferSize_PtyReader:           optionsResult_ptyProxy.StagingBufferSize_PtyReader,
			PostSnapshotBufferSize_PtyProxy:       optionsResult_ptyProxy.PostSnapshotBufferSize_PtyProxy,
			QueueBufferSize_InputOrder__PtyWriter: optionsResult_ptyProxy.QueueBufferSize_InputOrder__PtyWriter,
			OnSpawned:                             workspaceController.HandleSpawned_Pty,
			OnOutput_Live:                         workspaceController.HandleOutput_Pty,
			OnOutput_Snapshot:                     workspaceController.HandleOutput_Pty,
			OnOutput_PostSnapshotBuffer:           workspaceController.HandleOutput_Pty,
			OnExited_Eio_Success:                  workspaceController.HandleExited_Eio_Success__Pty,
			OnExited_Eio_Failure:                  workspaceController.HandleExited_Eio_Failure__Pty,
			OnExited_Eio_Killed:                   workspaceController.HandleExited_Eio_Killed__Pty,
			OnExited_Closed:                       workspaceController.HandleExited_Closed__Pty,
			OnExited_SystemError:                  workspaceController.HandleExited_SystemError__Pty,
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

func (this _ResizePtys_Message_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.MessageDebouncer_ResizePtys.QueueChannel <- this
}

func decodeMessage_WritePtyInput(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _WritePtyInput_Message_ struct {
	Id_PtyProxy          uint32
	InputOrder_PtyWriter _InputOrder_PtyWriter_
}

func (this _WritePtyInput_Message_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.Mutex.Lock()
	targetWorkspacePty := workspaceController.PtyPool[this.Id_PtyProxy]
	workspaceController.Mutex.Unlock()
	if targetWorkspacePty != nil {
		targetWorkspacePty.PtyProxy.PtyWriter.QueueChannel_InputOrder <- this.InputOrder_PtyWriter
	}
}

func decodeMessage_TerminatePty(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _TerminatePty_Message_ struct {
	Id_PtyProxy               uint32
	TerminalSignal_PtyProcess int
}

func (this _TerminatePty_Message_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.Mutex.Lock()
	targetWorkspacePty := workspaceController.PtyPool[this.Id_PtyProxy]
	workspaceController.Mutex.Unlock()
	if targetWorkspacePty != nil {
		targetWorkspacePty.PtyProxy.Terminate(this.TerminalSignal_PtyProcess)
	}
}

func decodeMessage_RemovePty(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _RemovePty_Message_ struct {
	Ids_PtyPool []uint32
}

func (this _RemovePty_Message_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.Mutex.Lock()
	for _, someId_PtyProxy := range this.Ids_PtyPool {
		targetWorkspacePty := workspaceController.PtyPool[someId_PtyProxy]
		if targetWorkspacePty != nil && targetWorkspacePty.MaybeExitOutcome != nil {
			delete(
				workspaceController.PtyPool,
				someId_PtyProxy,
			)
		}
	}
	workspaceController.Mutex.Unlock()
}

func decodeMessage_SyncWorkspace(
	binaryMessageFrame []byte,
) (_PtyMessage_Ingress_, error) {
	return nil, nil
}

type _SyncPtyOrder_SyncWorkspace_ struct {
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

type _SyncWorkspace_Message_ struct {
	SyncPtyOrders map[uint32]*_SyncPtyOrder_SyncWorkspace_
}

func (this _SyncWorkspace_Message_) Execute(
	workspaceController *_WorkspaceController_,
	id_WebsocketConnection uint64,
) {
	workspaceController.LifecycleCoordinator_WorkspacePty.QueueChannel <- _Sync__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection: id_WebsocketConnection,
		Message_SyncWorkspace:  this,
	}
}
