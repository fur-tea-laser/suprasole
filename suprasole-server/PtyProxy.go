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
	RUNNING_LIVE__Mode_PtyProxy
	RUNNING__PRE_SNAPSHOT___Mode_PtyProxy
	RUNNING__POST_SNAPSHOT___Mode_PtyProxy
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
	newPtyProxyResult := &_PtyProxy_{
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
	newPtyProxyResult.PtyReader = &_PtyReader_{
		OnTryFlush__:                     newPtyProxyResult.HandleTryFlush,
		OnBlockingFlush__:                newPtyProxyResult.HandleBlockingFlush,
		OnExited_Closed__:                newPtyProxyResult.HandleExited_Closed,
		OnExited_Eio__:                   newPtyProxyResult.HandleExited_Eio,
		OnExited_SystemError__:           newPtyProxyResult.HandleExited_SystemError,
		FileDescriptor_Master__PtyDevice: nil,
		StagingBuffer:                    make([]byte, api.StagingBufferSize_PtyReader),
		UnflushedSliceSize_StagingBuffer: 0,
	}
	workerContext_PtyWriter, workerCancel_PtyWriter := _CONTEXT.WithCancel(_CONTEXT.Background())
	newPtyProxyResult.PtyWriter = &_PtyWriter_{
		QueueChannel_InputOrder:          make(chan _InputOrder_PtyWriter_, api.QueueBufferSize_InputOrder__PtyWriter),
		WorkerContext:                    workerContext_PtyWriter,
		WorkerCancel:                     workerCancel_PtyWriter,
		FileDescriptor_Master__PtyDevice: nil,
	}
	newPtyProxyResult.PostSnapshotBuffer = _BYTES.NewBuffer(
		make([]byte, 0, api.PostSnapshotBufferSize_PtyProxy),
	)
	newPtyProxyResult.PtyTerminal = _XTERM.New(
		_XTERM.WithCols(api.ColumnCount_PtyTerminal),
		_XTERM.WithRows(api.RowCount_PtyTerminal),
		_XTERM.WithScrollback(api.ScrollbackLineCount_PtyTerminal),
	)
	ptyCommand_PtyProxy := _EXEC.Command(api.ShellBinaryPath_PtyCommand)
	ptyCommand_PtyProxy.Env = api.EnvironmentVariables_PtyCommand
	ptyCommand_PtyProxy.Dir = api.DirectoryPath_PtyCommand
	fileDescriptor_master__PtyDevice, startError_PtyCommand := _PTY.Start(ptyCommand_PtyProxy)
	if startError_PtyCommand != nil {
		return startError_PtyCommand
	}
	newPtyProxyResult.Mode = RUNNING_LIVE__Mode_PtyProxy
	newPtyProxyResult.PtyCommand = ptyCommand_PtyProxy
	newPtyProxyResult.FileDescriptor_Master__PtyDevice = fileDescriptor_master__PtyDevice
	newPtyProxyResult.PtyReader.FileDescriptor_Master__PtyDevice = fileDescriptor_master__PtyDevice
	newPtyProxyResult.PtyWriter.FileDescriptor_Master__PtyDevice = fileDescriptor_master__PtyDevice
	api.OnSpawned_PtyProxy__(newPtyProxyResult)
	go newPtyProxyResult.PtyReader.RunWorker()
	go newPtyProxyResult.PtyWriter.RunWorker()
	return nil
}

func (this *_PtyProxy_) HandleTryFlush(
	unflushedSlice_StagingBuffer []byte,
) bool {
	return __flushReaderStagingBufferSliceIfLockAcquired(
		this.Mutex.TryLock,
		this,
		unflushedSlice_StagingBuffer,
	)
}

func (this *_PtyProxy_) HandleBlockingFlush(
	unflushedSlice_StagingBuffer []byte,
) {
	__flushReaderStagingBufferSliceIfLockAcquired(
		this.LockAndReturnTrue,
		this,
		unflushedSlice_StagingBuffer,
	)
}

func (this *_PtyProxy_) LockAndReturnTrue() bool {
	this.Mutex.Lock()
	return true
}

func __flushReaderStagingBufferSliceIfLockAcquired(
	maybeAcquirePtyProxyMutexLock__ func() bool,
	ptyProxy *_PtyProxy_,
	unflushedSlice_StagingBuffer []byte,
) bool {
	if maybeAcquirePtyProxyMutexLock__() {
		var isWasRunningLive__Mode_PtyProxy bool
		ptyProxy.PtyTerminal.Write(unflushedSlice_StagingBuffer)
		if RUNNING_LIVE__Mode_PtyProxy == ptyProxy.Mode {
			isWasRunningLive__Mode_PtyProxy = true
		} else if RUNNING__POST_SNAPSHOT___Mode_PtyProxy == ptyProxy.Mode {
			ptyProxy.PostSnapshotBuffer.Write(unflushedSlice_StagingBuffer)
		} else if RUNNING__PRE_SNAPSHOT___Mode_PtyProxy == ptyProxy.Mode {
		} else {
			// SPAWNING__Mode_PtyProxy == ptyProxy.Mode
			// EXITED__Mode_PtyProxy == ptyProxy.Mode
			_FMT.Println("invalid path: _PtyProxy_ __flushReaderStagingBufferSliceIfLockAcquired")
		}
		ptyProxy.Mutex.Unlock()
		if isWasRunningLive__Mode_PtyProxy {
			ptyProxy.OnOutput_Live__(
				ptyProxy,
				_BYTES.Clone(unflushedSlice_StagingBuffer),
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
	processState := this.PtyCommand.ProcessState
	processWaitStatus := processState.Sys().(_SYSCALL.WaitStatus)
	if processWaitStatus.Signaled() {
		this.OnExited_Eio_Killed__(this)
	} else if processState.Success() {
		this.OnExited_Eio_Success__(this)
	} else if processWaitStatus.Exited() {
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
	ptyProxy *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	_ = ptyProxy.FileDescriptor_Master__PtyDevice.Close()
	_ = ptyProxy.PtyCommand.Wait()
	onDispatchExited__(exitSignal_PtyReader)
}

func (this *_PtyProxy_) TransitionMode_ToExited() {
	this.Mode = EXITED__Mode_PtyProxy
}

func (this *_PtyProxy_) TransitionMode_LiveToPreSnapshot() {
	this.Mutex.Lock()
	this.Mode = RUNNING__PRE_SNAPSHOT___Mode_PtyProxy
	this.Mutex.Unlock()
}

func (this *_PtyProxy_) TransitionMode_PostSnapshotToPreSnapshot() {
	this.Mutex.Lock()
	this.Mode = RUNNING__PRE_SNAPSHOT___Mode_PtyProxy
	this.Mutex.Unlock()
}

func (this *_PtyProxy_) TransitionMode_PreToPostSnapshot() {
	var snapshotBytes []byte
	this.Mutex.Lock()
	this.PostSnapshotBuffer.Reset()
	this.Mode = RUNNING__POST_SNAPSHOT___Mode_PtyProxy
	serializeAddon := _XTERM.NewSerializeAddon(this.PtyTerminal)
	snapshotBytes = serializeAddon.Serialize(nil)
	this.Mutex.Unlock()
	if len(snapshotBytes) > 0 {
		this.OnOutput_Snapshot__(
			this,
			snapshotBytes,
		)
	}
}

func (this *_PtyProxy_) TransitionMode_PostSnapshotToLive() {
	var clonedBytes_PostSnapshotBuffer []byte
	this.Mutex.Lock()
	clonedBytes_PostSnapshotBuffer = _BYTES.Clone(this.PostSnapshotBuffer.Bytes())
	this.Mode = RUNNING_LIVE__Mode_PtyProxy
	this.Mutex.Unlock()
	if len(clonedBytes_PostSnapshotBuffer) > 0 {
		this.OnOutput_PostSnapshot__(
			this,
			clonedBytes_PostSnapshotBuffer,
		)
	}
}

func (this *_PtyProxy_) EmitSnapshot_Exited() {
	var snapshotBytes []byte
	serializeAddon := _XTERM.NewSerializeAddon(this.PtyTerminal)
	snapshotBytes = serializeAddon.Serialize(nil)
	if len(snapshotBytes) > 0 {
		this.OnOutput_Snapshot__(
			this,
			snapshotBytes,
		)
	}
}

func (this *_PtyProxy_) Resize(
	nextColumnCount_PtyTerminal int,
	nextRowCount_PtyTerminal int,
) error {
	this.Mutex.Lock()
	this.PtyTerminal.Resize(
		nextColumnCount_PtyTerminal,
		nextRowCount_PtyTerminal,
	)
	this.Mutex.Unlock()
	return _PTY.Setsize(
		this.FileDescriptor_Master__PtyDevice,
		&_PTY.Winsize{
			Rows: uint16(nextRowCount_PtyTerminal),
			Cols: uint16(nextColumnCount_PtyTerminal),
		},
	)
}

func (this *_PtyProxy_) Terminate(
	terminalSignal_ptyProcess int,
) {
	syscallSignal := _SYSCALL.Signal(terminalSignal_ptyProcess)
	killError := _SYSCALL.Kill(
		-this.PtyCommand.Process.Pid,
		syscallSignal,
	)
	if killError != nil {
		_ = this.PtyCommand.Process.Signal(syscallSignal)
	}
}
