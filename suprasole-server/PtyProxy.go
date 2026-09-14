package main

import (
	_BYTES "bytes"
	_CONTEXT "context"
	_OS "os"
	_EXEC "os/exec"
	_SYNC "sync"
	_SYSCALL "syscall"

	_PTY "github.com/creack/pty"
	_XTERM "github.com/gitpod-io/xterm-go"
)

type _Mode_PtyProxy_ int

const (
	SPAWNING__Mode_PtyProxy _Mode_PtyProxy_ = iota
	LIVE_RUNNING__Mode_PtyProxy
	PRE_SNAPSHOT__RUNNING___Mode_PtyProxy
	POST_SNAPSHOT__RUNNING___Mode_PtyProxy
	EXITED__Mode_PtyProxy
)

type _PtyProxy_ struct {
	OnOutput_Live__                  func(id_WorkspacePty uint32, outputData_PtyDevice []byte)
	OnOutput_Snapshot__              func(id_WorkspacePty uint32, outputData_PtyTerminal []byte)
	OnOutput_PostSnapshot__          func(id_WorkspacePty uint32, outputData_PostSnapshot []byte)
	OnExited_Eio_Success__           func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Failure__           func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Killed__            func(ptyProxy *_PtyProxy_)
	OnExited_Closed__                func(ptyProxy *_PtyProxy_)
	OnExited_SystemError__           func(ptyProxy *_PtyProxy_, exitSignal_PtyReader error)
	Id_WorkspacePty                  uint32
	Mutex                            _SYNC.Mutex
	Mode_current                     _Mode_PtyProxy_
	PtyCommand                       *_EXEC.Cmd
	FileDescriptor_Master__PtyDevice *_OS.File
	PtyTerminal                      *_XTERM.Terminal
	PtyReader                        *_PtyReader_
	PtyWriter                        *_PtyWriter_
	PostSnapshotBuffer               *_BYTES.Buffer
}

type _SpawnApi_PtyProxy_ struct {
	OnOutput_Live__PtyProxy__              func(id_WorkspacePty uint32, outputData_PtyDevice []byte)
	OnOutput_Snapshot__PtyProxy__          func(id_WorkspacePty uint32, outputData_PtyTerminal []byte)
	OnOutput_PostSnapshot__PtyProxy__      func(id_WorkspacePty uint32, outputData_PostSnapshot []byte)
	OnExited_Eio_Success__PtyProxy__       func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Failure__PtyProxy__       func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Killed__PtyProxy__        func(ptyProxy *_PtyProxy_)
	OnExited_Closed__PtyProxy__            func(ptyProxy *_PtyProxy_)
	OnExited_SystemError__PtyProxy__       func(ptyProxy *_PtyProxy_, exitSignal_PtyReader error)
	Id_WorkspacePty                        uint32
	ColumnCount_PtyTerminal                int
	RowCount_PtyTerminal                   int
	Path_ShellBinary__PtyCommand           string
	DirectoryPath_PtyCommand               string
	EnvironmentVariables_PtyCommand        []string
	Size_StagingBuffer__PtyReader          int
	Count_ScrollbackLine__PtyTerminal      int
	Size_PostSnapshotBuffer__PtyProxy      int
	Size_QueueBuffer__InputOrder_PtyWriter int
}

func Spawn_PtyProxy(
	api _SpawnApi_PtyProxy_,
) (*_PtyProxy_, error) {
	ptyCommand_PtyProxy_result := _EXEC.Command(api.Path_ShellBinary__PtyCommand)
	ptyCommand_PtyProxy_result.Env = api.EnvironmentVariables_PtyCommand
	ptyCommand_PtyProxy_result.Dir = api.DirectoryPath_PtyCommand
	fileDescriptor_master__PtyDevice, error_start__PtyCommand__maybe := _PTY.Start(ptyCommand_PtyProxy_result)
	if error_start__PtyCommand__maybe != nil {
		return nil, error_start__PtyCommand__maybe
	}
	ptyProxy_result := &_PtyProxy_{
		OnOutput_Live__:                  api.OnOutput_Live__PtyProxy__,
		OnOutput_Snapshot__:              api.OnOutput_Snapshot__PtyProxy__,
		OnOutput_PostSnapshot__:          api.OnOutput_PostSnapshot__PtyProxy__,
		OnExited_Eio_Success__:           api.OnExited_Eio_Success__PtyProxy__,
		OnExited_Eio_Failure__:           api.OnExited_Eio_Failure__PtyProxy__,
		OnExited_Eio_Killed__:            api.OnExited_Eio_Killed__PtyProxy__,
		OnExited_Closed__:                api.OnExited_Closed__PtyProxy__,
		OnExited_SystemError__:           api.OnExited_SystemError__PtyProxy__,
		Id_WorkspacePty:                  api.Id_WorkspacePty,
		Mode_current:                     SPAWNING__Mode_PtyProxy,
		PtyCommand:                       ptyCommand_PtyProxy_result,
		FileDescriptor_Master__PtyDevice: fileDescriptor_master__PtyDevice,
		PtyTerminal:                      nil,
		PtyReader:                        nil,
		PtyWriter:                        nil,
		PostSnapshotBuffer:               nil,
	}
	ptyProxy_result.PtyReader = &_PtyReader_{
		OnTryFlush__:                       ptyProxy_result.HandleTryFlush,
		OnBlockingFlush__:                  ptyProxy_result.HandleBlockingFlush,
		OnExited_Closed__:                  ptyProxy_result.HandleExited_Closed,
		OnExited_Eio__:                     ptyProxy_result.HandleExited_Eio,
		OnExited_SystemError__:             ptyProxy_result.HandleExited_SystemError,
		StagingBuffer:                      make([]byte, api.Size_StagingBuffer__PtyReader),
		Size_UnflushedSlice__StagingBuffer: 0,
		FileDescriptor_Master__PtyDevice:   fileDescriptor_master__PtyDevice,
	}
	workerContext_PtyWriter, workerCancel_PtyWriter := _CONTEXT.WithCancel(_CONTEXT.Background())
	ptyProxy_result.PtyWriter = &_PtyWriter_{
		QueueChannel_InputOrder:          make(chan _InputOrder_PtyWriter_, api.Size_QueueBuffer__InputOrder_PtyWriter),
		WorkerContext:                    workerContext_PtyWriter,
		WorkerCancel:                     workerCancel_PtyWriter,
		FileDescriptor_Master__PtyDevice: fileDescriptor_master__PtyDevice,
	}
	ptyProxy_result.PostSnapshotBuffer = _BYTES.NewBuffer(
		make([]byte, 0, api.Size_PostSnapshotBuffer__PtyProxy),
	)
	ptyProxy_result.PtyTerminal = _XTERM.New(
		_XTERM.WithCols(api.ColumnCount_PtyTerminal),
		_XTERM.WithRows(api.RowCount_PtyTerminal),
		_XTERM.WithScrollback(api.Count_ScrollbackLine__PtyTerminal),
	)
	return ptyProxy_result, nil
}

func (This *_PtyProxy_) TransitionMode_ToExited() {
	This.Mode_current = EXITED__Mode_PtyProxy
}

func (This *_PtyProxy_) TransitionMode_LiveToPreSnapshot() {
	This.Mutex.Lock()
	This.Mode_current = PRE_SNAPSHOT__RUNNING___Mode_PtyProxy
	This.Mutex.Unlock()
}

func (This *_PtyProxy_) TransitionMode_PostSnapshotToPreSnapshot() {
	This.Mutex.Lock()
	This.Mode_current = PRE_SNAPSHOT__RUNNING___Mode_PtyProxy
	This.Mutex.Unlock()
}

func (This *_PtyProxy_) TransitionMode_PreToPostSnapshot() {
	This.Mutex.Lock()
	This.PostSnapshotBuffer.Reset()
	This.Mode_current = POST_SNAPSHOT__RUNNING___Mode_PtyProxy
	serializeAddon := _XTERM.NewSerializeAddon(This.PtyTerminal)
	outputData_PtyTerminal := serializeAddon.Serialize(nil)
	This.Mutex.Unlock()
	if len(outputData_PtyTerminal) > 0 {
		This.OnOutput_Snapshot__(
			This.Id_WorkspacePty,
			outputData_PtyTerminal,
		)
	}
}

func (This *_PtyProxy_) TransitionMode_PostSnapshotToLive() {
	This.Mutex.Lock()
	outputData_PostSnapshot := _BYTES.Clone(This.PostSnapshotBuffer.Bytes())
	This.Mode_current = LIVE_RUNNING__Mode_PtyProxy
	This.Mutex.Unlock()
	if len(outputData_PostSnapshot) > 0 {
		This.OnOutput_PostSnapshot__(
			This.Id_WorkspacePty,
			outputData_PostSnapshot,
		)
	}
}

func (This *_PtyProxy_) EmitSnapshot_Exited() {
	serializeAddon := _XTERM.NewSerializeAddon(This.PtyTerminal)
	outputData_PtyTerminal := serializeAddon.Serialize(nil)
	if len(outputData_PtyTerminal) > 0 {
		This.OnOutput_Snapshot__(
			This.Id_WorkspacePty,
			outputData_PtyTerminal,
		)
	}
}

func (This *_PtyProxy_) Resize(
	columnCount_PtyTerminal_next int,
	rowCount_PtyTerminal_next int,
) error {
	This.Mutex.Lock()
	This.PtyTerminal.Resize(
		columnCount_PtyTerminal_next,
		rowCount_PtyTerminal_next,
	)
	This.Mutex.Unlock()
	return _PTY.Setsize(
		This.FileDescriptor_Master__PtyDevice,
		&_PTY.Winsize{
			Rows: uint16(rowCount_PtyTerminal_next),
			Cols: uint16(columnCount_PtyTerminal_next),
		},
	)
}

func (This *_PtyProxy_) Terminate_PtyProcess(
	terminalSignal_PtyProcess int,
) {
	syscallSignal_PtyProcess := _SYSCALL.Signal(terminalSignal_PtyProcess)
	error_kill__PtyProcess__maybe := _SYSCALL.Kill(
		-This.PtyCommand.Process.Pid,
		syscallSignal_PtyProcess,
	)
	if error_kill__PtyProcess__maybe != nil {
		_ = This.PtyCommand.Process.Signal(syscallSignal_PtyProcess)
	}
}
