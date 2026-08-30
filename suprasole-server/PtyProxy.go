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

type Mode_PtyProxy int

const (
	SPAWNING__Mode_PtyProxy Mode_PtyProxy = iota
	RUNNING_LIVE__Mode_PtyProxy
	RUNNING_PRE_SNAPSHOT__Mode_PtyProxy
	RUNNING_POST_SNAPSHOT__Mode_PtyProxy
	EXITED__Mode_PtyProxy
)

type _PtyProxy_ struct {
	MasterFileDescriptor_PtyDevice *_OS.File
	Id                             uint32
	Mutex                          _SYNC.Mutex
	Mode                           Mode_PtyProxy
	PtyCommand                     *_EXEC.Cmd
	PtyTerminal                    *_XTERM.Terminal
	PtyReader                      *_PtyReader_
	PtyWriter                      *_PtyWriter_
	PostSnapshotBuffer             *_BYTES.Buffer
	OnOutput_Live                  func(ptyProxy *_PtyProxy_, outputData_PtyDevice []byte)
	OnOutput_Snapshot              func(ptyProxy *_PtyProxy_, outputData_PtyTerminal []byte)
	OnOutput_PostSnapshotBuffer    func(ptyProxy *_PtyProxy_, outputData_PostSnapshotBuffer []byte)
	OnExited_Eio_Success           func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Failure           func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Killed            func(ptyProxy *_PtyProxy_)
	OnExited_Closed                func(ptyProxy *_PtyProxy_)
	OnExited_SystemError           func(ptyProxy *_PtyProxy_, exitSignal_PtyReader error)
}

type _SpawnApi__PtyProxy_ struct {
	Id                                    uint32
	ColumnCount_PtyTerminal               int
	RowCount_PtyTerminal                  int
	ShellBinaryPath_PtyCommand            string
	DirectoryPath_PtyCommand              string
	EnvironmentVariables_PtyCommand       []string
	StagingBufferSize_PtyReader           int
	ScrollbackLineCount_PtyTerminal       int
	PostSnapshotBufferSize_PtyProxy       int
	QueueBufferSize_InputOrder__PtyWriter int
	OnSpawned                             func(ptyProxy *_PtyProxy_)
	OnOutput_Live                         func(ptyProxy *_PtyProxy_, outputData_PtyDevice []byte)
	OnOutput_Snapshot                     func(ptyProxy *_PtyProxy_, outputData_PtyTerminal []byte)
	OnOutput_PostSnapshotBuffer           func(ptyProxy *_PtyProxy_, outputData_PostSnapshotBuffer []byte)
	OnExited_Eio_Success                  func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Failure                  func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Killed                   func(ptyProxy *_PtyProxy_)
	OnExited_Closed                       func(ptyProxy *_PtyProxy_)
	OnExited_SystemError                  func(ptyProxy *_PtyProxy_, exitSignal_PtyReader error)
}

func Spawn__PtyProxy(api _SpawnApi__PtyProxy_) error {
	newPtyProxyResult := &_PtyProxy_{
		MasterFileDescriptor_PtyDevice: nil,
		Id:                             api.Id,
		Mode:                           SPAWNING__Mode_PtyProxy,
		PtyCommand:                     nil,
		PtyTerminal:                    nil,
		PtyReader:                      nil,
		PtyWriter:                      nil,
		PostSnapshotBuffer:             nil,
		OnOutput_Live:                  api.OnOutput_Live,
		OnOutput_Snapshot:              api.OnOutput_Snapshot,
		OnOutput_PostSnapshotBuffer:    api.OnOutput_PostSnapshotBuffer,
		OnExited_Eio_Success:           api.OnExited_Eio_Success,
		OnExited_Eio_Failure:           api.OnExited_Eio_Failure,
		OnExited_Eio_Killed:            api.OnExited_Eio_Killed,
		OnExited_Closed:                api.OnExited_Closed,
		OnExited_SystemError:           api.OnExited_SystemError,
	}
	newPtyProxyResult.PtyReader = &_PtyReader_{
		MasterFileDescriptor_PtyDevice:  nil,
		StagingBuffer:                   nil,
		UnflushedStagingBufferSliceSize: 0,
		OnTryFlush:                      newPtyProxyResult.HandleTryFlush,
		OnBlockingFlush:                 newPtyProxyResult.HandleBlockingFlush,
		OnExited_Closed:                 newPtyProxyResult.HandleExited_Closed,
		OnExited_Eio:                    newPtyProxyResult.HandleExited_Eio,
		OnExited_SystemError:            newPtyProxyResult.HandleExited_SystemError,
	}
	newPtyProxyResult.PtyReader.StagingBuffer = make(
		[]byte,
		api.StagingBufferSize_PtyReader,
	)
	__WorkerContext_PtyWriter, __WorkerCancel_PtyWriter := _CONTEXT.WithCancel(_CONTEXT.Background())
	newPtyProxyResult.PtyWriter = &_PtyWriter_{
		MasterFileDescriptor_PtyDevice: nil,
		QueueChannel_InputOrder: make(
			chan _InputOrder_PtyWriter_,
			api.QueueBufferSize_InputOrder__PtyWriter,
		),
		WorkerContext: __WorkerContext_PtyWriter,
		WorkerCancel:  __WorkerCancel_PtyWriter,
	}
	newPtyProxyResult.PostSnapshotBuffer = _BYTES.NewBuffer(
		make(
			[]byte,
			0,
			api.PostSnapshotBufferSize_PtyProxy,
		),
	)
	newPtyProxyResult.PtyTerminal = _XTERM.New(
		_XTERM.WithCols(api.ColumnCount_PtyTerminal),
		_XTERM.WithRows(api.RowCount_PtyTerminal),
		_XTERM.WithScrollback(api.ScrollbackLineCount_PtyTerminal),
	)
	__PtyCommand_Proxy := _EXEC.Command(api.ShellBinaryPath_PtyCommand)
	__PtyCommand_Proxy.Env = api.EnvironmentVariables_PtyCommand
	__PtyCommand_Proxy.Dir = api.DirectoryPath_PtyCommand
	__MasterFileDescriptor_PtyDevice, ptyStartError := _PTY.Start(__PtyCommand_Proxy)
	if ptyStartError != nil {
		return ptyStartError
	}
	newPtyProxyResult.Mode = RUNNING_LIVE__Mode_PtyProxy
	newPtyProxyResult.PtyCommand = __PtyCommand_Proxy
	newPtyProxyResult.MasterFileDescriptor_PtyDevice = __MasterFileDescriptor_PtyDevice
	newPtyProxyResult.PtyReader.MasterFileDescriptor_PtyDevice = __MasterFileDescriptor_PtyDevice
	newPtyProxyResult.PtyWriter.MasterFileDescriptor_PtyDevice = __MasterFileDescriptor_PtyDevice
	api.OnSpawned(newPtyProxyResult)
	go newPtyProxyResult.PtyReader.RunWorker()
	go newPtyProxyResult.PtyWriter.RunWorker()
	return nil
}

func (this *_PtyProxy_) HandleTryFlush(unflushedStagingBufferSlice []byte) bool {
	return __flushReaderStagingBufferSliceIfLockAcquired(
		this.Mutex.TryLock,
		this,
		unflushedStagingBufferSlice,
	)
}

func (this *_PtyProxy_) HandleBlockingFlush(unflushedStagingBufferSlice []byte) {
	__flushReaderStagingBufferSliceIfLockAcquired(
		this.LockAndReturnTrue,
		this,
		unflushedStagingBufferSlice,
	)
}

func (this *_PtyProxy_) LockAndReturnTrue() bool {
	this.Mutex.Lock()
	return true
}

func __flushReaderStagingBufferSliceIfLockAcquired(
	maybeAcquirePtyProxyMutexLock func() bool,
	ptyProxy *_PtyProxy_,
	unflushedStagingBufferSlice []byte,
) bool {
	if maybeAcquirePtyProxyMutexLock() {
		var isWasPtyModeRunningLive bool
		ptyProxy.PtyTerminal.Write(unflushedStagingBufferSlice)
		if RUNNING_LIVE__Mode_PtyProxy == ptyProxy.Mode {
			isWasPtyModeRunningLive = true
		} else if RUNNING_POST_SNAPSHOT__Mode_PtyProxy == ptyProxy.Mode {
			ptyProxy.PostSnapshotBuffer.Write(unflushedStagingBufferSlice)
		} else if RUNNING_PRE_SNAPSHOT__Mode_PtyProxy == ptyProxy.Mode {
		} else {
			// SPAWNING__Mode_PtyProxy == ptyProxy.Mode
			// EXITED__Mode_PtyProxy == ptyProxy.Mode
			_FMT.Println("invalid path: _PtyProxy_ __flushReaderStagingBufferSliceIfLockAcquired")
		}
		ptyProxy.Mutex.Unlock()
		if isWasPtyModeRunningLive {
			ptyProxy.OnOutput_Live(
				ptyProxy,
				_BYTES.Clone(unflushedStagingBufferSlice),
			)
		}
		return true
	}
	return false
}

func (this *_PtyProxy_) HandleExited_Closed(exitSignal_PtyReader error) {
	__executeExitedTeardownPipeline(
		this.HandleDispatchExitedHandler_Closed,
		this,
		exitSignal_PtyReader,
	)
}

func (this *_PtyProxy_) HandleDispatchExitedHandler_Closed(_ error) {
	this.OnExited_Closed(this)
}

func (this *_PtyProxy_) HandleExited_Eio(exitSignal_PtyReader error) {
	__executeExitedTeardownPipeline(
		this.HandleDispatchExitedHandler_Eio,
		this,
		exitSignal_PtyReader,
	)
}

func (this *_PtyProxy_) HandleDispatchExitedHandler_Eio(_ error) {
	processState := this.PtyCommand.ProcessState
	processWaitStatus := processState.Sys().(_SYSCALL.WaitStatus)
	if processWaitStatus.Signaled() {
		this.OnExited_Eio_Killed(this)
	} else if processState.Success() {
		this.OnExited_Eio_Success(this)
	} else if processWaitStatus.Exited() {
		this.OnExited_Eio_Failure(this)
	} else {
		_FMT.Println("invalid path: _PtyProxy_ HandleDispatchExitedHandler_Eio")
	}
}

func (this *_PtyProxy_) HandleExited_SystemError(exitSignal_PtyReader error) {
	__executeExitedTeardownPipeline(
		this.HandleDispatchExitedHandler_SystemError,
		this,
		exitSignal_PtyReader,
	)
}

func (this *_PtyProxy_) HandleDispatchExitedHandler_SystemError(exitSignal_PtyReader error) {
	this.OnExited_SystemError(
		this,
		exitSignal_PtyReader,
	)
}

func __executeExitedTeardownPipeline(
	onDispatchExitedHandler func(exitSignal_PtyReader error),
	ptyProxy *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	ptyProxy.Mutex.Lock()
	ptyProxy.Mode = EXITED__Mode_PtyProxy
	ptyProxy.Mutex.Unlock()
	ptyProxy.PtyWriter.WorkerCancel()
	_ = ptyProxy.MasterFileDescriptor_PtyDevice.Close()
	_ = ptyProxy.PtyCommand.Wait()
	onDispatchExitedHandler(exitSignal_PtyReader)
}

func (this *_PtyProxy_) TransitionMode_LiveToPreSnapshot() {
	this.Mutex.Lock()
	this.Mode = RUNNING_PRE_SNAPSHOT__Mode_PtyProxy
	this.Mutex.Unlock()
}

func (this *_PtyProxy_) TransitionMode_PostSnapshotToPreSnapshot() {
	this.Mutex.Lock()
	this.Mode = RUNNING_PRE_SNAPSHOT__Mode_PtyProxy
	this.Mutex.Unlock()
}

func (this *_PtyProxy_) TransitionMode_PreToPostSnapshot() {
	var snapshotBytes []byte
	this.Mutex.Lock()
	this.PostSnapshotBuffer.Reset()
	this.Mode = RUNNING_POST_SNAPSHOT__Mode_PtyProxy
	serializeAddon := _XTERM.NewSerializeAddon(this.PtyTerminal)
	snapshotBytes = serializeAddon.Serialize(nil)
	this.Mutex.Unlock()
	if len(snapshotBytes) > 0 {
		this.OnOutput_Snapshot(
			this,
			snapshotBytes,
		)
	}
}

func (this *_PtyProxy_) TransitionMode_PostSnapshotToLive() {
	var clonedPostSnapshotBuffer []byte
	this.Mutex.Lock()
	clonedPostSnapshotBuffer = _BYTES.Clone(this.PostSnapshotBuffer.Bytes())
	this.Mode = RUNNING_LIVE__Mode_PtyProxy
	this.Mutex.Unlock()
	if len(clonedPostSnapshotBuffer) > 0 {
		this.OnOutput_PostSnapshotBuffer(
			this,
			clonedPostSnapshotBuffer,
		)
	}
}

func (this *_PtyProxy_) EmitSnapshot_Exited() {
	var snapshotBytes []byte
	this.Mutex.Lock()
	serializeAddon := _XTERM.NewSerializeAddon(this.PtyTerminal)
	snapshotBytes = serializeAddon.Serialize(nil)
	this.Mutex.Unlock()
	if len(snapshotBytes) > 0 {
		this.OnOutput_Snapshot(
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
		this.MasterFileDescriptor_PtyDevice,
		&_PTY.Winsize{
			Rows: uint16(nextRowCount_PtyTerminal),
			Cols: uint16(nextColumnCount_PtyTerminal),
		},
	)
}

func (this *_PtyProxy_) Terminate(terminalSignal_ptyProcess int) {
	syscallSignal := _SYSCALL.Signal(terminalSignal_ptyProcess)
	killError := _SYSCALL.Kill(
		-this.PtyCommand.Process.Pid,
		syscallSignal,
	)
	if killError != nil {
		_ = this.PtyCommand.Process.Signal(syscallSignal)
	}
}
