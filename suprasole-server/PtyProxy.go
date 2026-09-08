package main

import (
	_BYTES "bytes"
	_CONTEXT "context"
	_FMT "fmt"
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
	OnOutput_Live__                  func(ptyProxy *_PtyProxy_, outputData_PtyDevice []byte)
	OnOutput_Snapshot__              func(ptyProxy *_PtyProxy_, outputData_PtyTerminal []byte)
	OnOutput_PostSnapshot__          func(ptyProxy *_PtyProxy_, outputData_PostSnapshot []byte)
	OnExited_Eio_Success__           func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Failure__           func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Killed__            func(ptyProxy *_PtyProxy_)
	OnExited_Closed__                func(ptyProxy *_PtyProxy_)
	OnExited_SystemError__           func(ptyProxy *_PtyProxy_, exitSignal_PtyReader error)
	FileDescriptor_Master__PtyDevice *_OS.File
	Id                               uint32
	Mutex                            _SYNC.Mutex
	Mode                             _Mode_PtyProxy_
	PtyCommand                       *_EXEC.Cmd
	PtyTerminal                      *_XTERM.Terminal
	PtyReader                        *_PtyReader_
	PtyWriter                        *_PtyWriter_
	PostSnapshotBuffer               *_BYTES.Buffer
}

type _SpawnApi__PtyProxy_ struct {
	OnSpawned_PtyProxy__                  func(ptyProxy *_PtyProxy_)
	OnOutput_Live__PtyProxy__             func(ptyProxy *_PtyProxy_, outputData_PtyDevice []byte)
	OnOutput_Snapshot__PtyProxy__         func(ptyProxy *_PtyProxy_, outputData_PtyTerminal []byte)
	OnOutput_PostSnapshot__PtyProxy__     func(ptyProxy *_PtyProxy_, outputData_PostSnapshot []byte)
	OnExited_Eio_Success__PtyProxy__      func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Failure__PtyProxy__      func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Killed__PtyProxy__       func(ptyProxy *_PtyProxy_)
	OnExited_Closed__PtyProxy__           func(ptyProxy *_PtyProxy_)
	OnExited_SystemError__PtyProxy__      func(ptyProxy *_PtyProxy_, exitSignal_PtyReader error)
	Id_PtyProxy                           uint32
	ColumnCount_PtyTerminal               int
	RowCount_PtyTerminal                  int
	ShellBinaryPath_PtyCommand            string
	DirectoryPath_PtyCommand              string
	EnvironmentVariables_PtyCommand       []string
	StagingBufferSize_PtyReader           int
	ScrollbackLineCount_PtyTerminal       int
	PostSnapshotBufferSize_PtyProxy       int
	QueueBufferSize_InputOrder__PtyWriter int
}

func Spawn__PtyProxy(
	api _SpawnApi__PtyProxy_,
) error {
	ptyProxy__new_result := &_PtyProxy_{
		OnOutput_Live__:                  api.OnOutput_Live__PtyProxy__,
		OnOutput_Snapshot__:              api.OnOutput_Snapshot__PtyProxy__,
		OnOutput_PostSnapshot__:          api.OnOutput_PostSnapshot__PtyProxy__,
		OnExited_Eio_Success__:           api.OnExited_Eio_Success__PtyProxy__,
		OnExited_Eio_Failure__:           api.OnExited_Eio_Failure__PtyProxy__,
		OnExited_Eio_Killed__:            api.OnExited_Eio_Killed__PtyProxy__,
		OnExited_Closed__:                api.OnExited_Closed__PtyProxy__,
		OnExited_SystemError__:           api.OnExited_SystemError__PtyProxy__,
		Mode:                             SPAWNING__Mode_PtyProxy,
		Id:                               api.Id_PtyProxy,
		FileDescriptor_Master__PtyDevice: nil,
		PtyCommand:                       nil,
		PtyTerminal:                      nil,
		PtyReader:                        nil,
		PtyWriter:                        nil,
		PostSnapshotBuffer:               nil,
	}
	ptyProxy__new_result.PtyReader = &_PtyReader_{
		OnTryFlush__:                       ptyProxy__new_result.HandleTryFlush,
		OnBlockingFlush__:                  ptyProxy__new_result.HandleBlockingFlush,
		OnExited_Closed__:                  ptyProxy__new_result.HandleExited_Closed,
		OnExited_Eio__:                     ptyProxy__new_result.HandleExited_Eio,
		OnExited_SystemError__:             ptyProxy__new_result.HandleExited_SystemError,
		StagingBuffer:                      make([]byte, api.StagingBufferSize_PtyReader),
		Size_UnflushedSlice__StagingBuffer: 0,
		FileDescriptor_Master__PtyDevice:   nil,
	}
	workerContext_PtyWriter, workerCancel_PtyWriter := _CONTEXT.WithCancel(_CONTEXT.Background())
	ptyProxy__new_result.PtyWriter = &_PtyWriter_{
		QueueChannel_InputOrder:          make(chan _InputOrder_PtyWriter_, api.QueueBufferSize_InputOrder__PtyWriter),
		WorkerContext:                    workerContext_PtyWriter,
		WorkerCancel:                     workerCancel_PtyWriter,
		FileDescriptor_Master__PtyDevice: nil,
	}
	ptyProxy__new_result.PostSnapshotBuffer = _BYTES.NewBuffer(
		make([]byte, 0, api.PostSnapshotBufferSize_PtyProxy),
	)
	ptyProxy__new_result.PtyTerminal = _XTERM.New(
		_XTERM.WithCols(api.ColumnCount_PtyTerminal),
		_XTERM.WithRows(api.RowCount_PtyTerminal),
		_XTERM.WithScrollback(api.ScrollbackLineCount_PtyTerminal),
	)
	ptyCommand_PtyProxy := _EXEC.Command(api.ShellBinaryPath_PtyCommand)
	ptyCommand_PtyProxy.Env = api.EnvironmentVariables_PtyCommand
	ptyCommand_PtyProxy.Dir = api.DirectoryPath_PtyCommand
	fileDescriptor_master__PtyDevice, error_start__PtyCommand__maybe := _PTY.Start(ptyCommand_PtyProxy)
	if error_start__PtyCommand__maybe != nil {
		return error_start__PtyCommand__maybe
	}
	ptyProxy__new_result.Mode = LIVE_RUNNING__Mode_PtyProxy
	ptyProxy__new_result.PtyCommand = ptyCommand_PtyProxy
	ptyProxy__new_result.FileDescriptor_Master__PtyDevice = fileDescriptor_master__PtyDevice
	ptyProxy__new_result.PtyReader.FileDescriptor_Master__PtyDevice = fileDescriptor_master__PtyDevice
	ptyProxy__new_result.PtyWriter.FileDescriptor_Master__PtyDevice = fileDescriptor_master__PtyDevice
	api.OnSpawned_PtyProxy__(ptyProxy__new_result)
	go ptyProxy__new_result.PtyReader.RunWorker()
	go ptyProxy__new_result.PtyWriter.RunWorker()
	return nil
}

func (this *_PtyProxy_) HandleTryFlush(
	UnflushedSlice_StagingBuffer__PtyReader []byte,
) bool {
	return __flushReaderStagingBufferSliceIfLockAcquired(
		this.Mutex.TryLock,
		this,
		UnflushedSlice_StagingBuffer__PtyReader,
	)
}

func (this *_PtyProxy_) HandleBlockingFlush(
	UnflushedSlice_StagingBuffer__PtyReader []byte,
) {
	__flushReaderStagingBufferSliceIfLockAcquired(
		this.LockAndReturnTrue,
		this,
		UnflushedSlice_StagingBuffer__PtyReader,
	)
}

func (this *_PtyProxy_) LockAndReturnTrue() bool {
	this.Mutex.Lock()
	return true
}

func __flushReaderStagingBufferSliceIfLockAcquired(
	maybeAcquirePtyProxyMutexLock__ func() bool,
	PtyProxy_this *_PtyProxy_,
	UnflushedSlice_StagingBuffer__PtyReader []byte,
) bool {
	if maybeAcquirePtyProxyMutexLock__() {
		mode_PtyProxy__captured := PtyProxy_this.Mode
		PtyProxy_this.PtyTerminal.Write(UnflushedSlice_StagingBuffer__PtyReader)
		if LIVE_RUNNING__Mode_PtyProxy == mode_PtyProxy__captured {
		} else if POST_SNAPSHOT__RUNNING___Mode_PtyProxy == mode_PtyProxy__captured {
			PtyProxy_this.PostSnapshotBuffer.Write(UnflushedSlice_StagingBuffer__PtyReader)
		} else if PRE_SNAPSHOT__RUNNING___Mode_PtyProxy == mode_PtyProxy__captured {
		} else {
			// SPAWNING__Mode_PtyProxy == mode_PtyProxy__captured
			// EXITED__Mode_PtyProxy == mode_PtyProxy__captured
			_FMT.Println("invalid path: _PtyProxy_ __flushReaderStagingBufferSliceIfLockAcquired")
		}
		PtyProxy_this.Mutex.Unlock()
		if LIVE_RUNNING__Mode_PtyProxy == mode_PtyProxy__captured {
			PtyProxy_this.OnOutput_Live__(
				PtyProxy_this,
				_BYTES.Clone(UnflushedSlice_StagingBuffer__PtyReader),
			)
		}
		return true
	}
	return false
}

func (this *_PtyProxy_) HandleExited_Closed(
	exitSignal_PtyReader error,
) {
	__executeExitedTeardown(
		this.HandleDispatchExited_Closed,
		this,
		exitSignal_PtyReader,
	)
}

func (this *_PtyProxy_) HandleDispatchExited_Closed(
	_ error,
) {
	this.OnExited_Closed__(this)
}

func (this *_PtyProxy_) HandleExited_Eio(
	exitSignal_PtyReader error,
) {
	__executeExitedTeardown(
		this.HandleDispatchExited_Eio,
		this,
		exitSignal_PtyReader,
	)
}

func (this *_PtyProxy_) HandleDispatchExited_Eio(
	_ error,
) {
	waitStatus_PtyProcess := this.PtyCommand.ProcessState.Sys().(_SYSCALL.WaitStatus)
	if waitStatus_PtyProcess.Signaled() {
		this.OnExited_Eio_Killed__(this)
	} else if waitStatus_PtyProcess.Exited() && 0 == waitStatus_PtyProcess.ExitStatus() {
		this.OnExited_Eio_Success__(this)
	} else if waitStatus_PtyProcess.Exited() {
		this.OnExited_Eio_Failure__(this)
	} else {
		_FMT.Println("invalid path: _PtyProxy_ HandleDispatchExited_Eio")
	}
}

func (this *_PtyProxy_) HandleExited_SystemError(
	exitSignal_PtyReader error,
) {
	__executeExitedTeardown(
		this.HandleDispatchExited_SystemError,
		this,
		exitSignal_PtyReader,
	)
}

func (this *_PtyProxy_) HandleDispatchExited_SystemError(
	exitSignal_PtyReader error,
) {
	this.OnExited_SystemError__(
		this,
		exitSignal_PtyReader,
	)
}

func __executeExitedTeardown(
	onDispatchExited__ func(exitSignal_PtyReader error),
	PtyProxy_this *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	_ = PtyProxy_this.FileDescriptor_Master__PtyDevice.Close()
	_ = PtyProxy_this.PtyCommand.Wait()
	onDispatchExited__(exitSignal_PtyReader)
}

func (this *_PtyProxy_) TransitionMode_ToExited() {
	this.Mode = EXITED__Mode_PtyProxy
}

func (this *_PtyProxy_) TransitionMode_LiveToPreSnapshot() {
	this.Mutex.Lock()
	this.Mode = PRE_SNAPSHOT__RUNNING___Mode_PtyProxy
	this.Mutex.Unlock()
}

func (this *_PtyProxy_) TransitionMode_PostSnapshotToPreSnapshot() {
	this.Mutex.Lock()
	this.Mode = PRE_SNAPSHOT__RUNNING___Mode_PtyProxy
	this.Mutex.Unlock()
}

func (this *_PtyProxy_) TransitionMode_PreToPostSnapshot() {
	this.Mutex.Lock()
	this.PostSnapshotBuffer.Reset()
	this.Mode = POST_SNAPSHOT__RUNNING___Mode_PtyProxy
	serializeAddon := _XTERM.NewSerializeAddon(this.PtyTerminal)
	outputData_PtyTerminal := serializeAddon.Serialize(nil)
	this.Mutex.Unlock()
	if len(outputData_PtyTerminal) > 0 {
		this.OnOutput_Snapshot__(
			this,
			outputData_PtyTerminal,
		)
	}
}

func (this *_PtyProxy_) TransitionMode_PostSnapshotToLive() {
	this.Mutex.Lock()
	outputData_PostSnapshot := _BYTES.Clone(this.PostSnapshotBuffer.Bytes())
	this.Mode = LIVE_RUNNING__Mode_PtyProxy
	this.Mutex.Unlock()
	if len(outputData_PostSnapshot) > 0 {
		this.OnOutput_PostSnapshot__(
			this,
			outputData_PostSnapshot,
		)
	}
}

func (this *_PtyProxy_) EmitSnapshot_Exited() {
	serializeAddon := _XTERM.NewSerializeAddon(this.PtyTerminal)
	outputData_PtyTerminal := serializeAddon.Serialize(nil)
	if len(outputData_PtyTerminal) > 0 {
		this.OnOutput_Snapshot__(
			this,
			outputData_PtyTerminal,
		)
	}
}

func (this *_PtyProxy_) Resize(
	columnCount_PtyTerminal__next int,
	rowCount_PtyTerminal__next int,
) error {
	this.Mutex.Lock()
	this.PtyTerminal.Resize(
		columnCount_PtyTerminal__next,
		rowCount_PtyTerminal__next,
	)
	this.Mutex.Unlock()
	return _PTY.Setsize(
		this.FileDescriptor_Master__PtyDevice,
		&_PTY.Winsize{
			Rows: uint16(rowCount_PtyTerminal__next),
			Cols: uint16(columnCount_PtyTerminal__next),
		},
	)
}

func (this *_PtyProxy_) Terminate_PtyProcess(
	terminalSignal_PtyProcess int,
) {
	syscallSignal_PtyProcess := _SYSCALL.Signal(terminalSignal_PtyProcess)
	error_kill__PtyProcess__maybe := _SYSCALL.Kill(
		-this.PtyCommand.Process.Pid,
		syscallSignal_PtyProcess,
	)
	if error_kill__PtyProcess__maybe != nil {
		_ = this.PtyCommand.Process.Signal(syscallSignal_PtyProcess)
	}
}
