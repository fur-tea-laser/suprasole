package main

import (
	_BYTES "bytes"
	_FMT "fmt"
	_OS "os"
	_EXEC "os/exec"
	_SYNC "sync"
	_SYSCALL "syscall"

	_PTY "github.com/creack/pty"
	_XTERM "github.com/gitpod-io/xterm-go"
)

type PtyProxyMode int

const (
	SPAWNING__PtyProxyMode PtyProxyMode = iota
	RUNNING_LIVE__PtyProxyMode
	RUNNING_PRE_SNAPSHOT__PtyProxyMode
	RUNNING_POST_SNAPSHOT__PtyProxyMode
	EXITED__PtyProxyMode
)

type _PtyProxy_ struct {
	PtyMasterFileDescriptor     *_OS.File
	Id                          uint32
	Mutex                       _SYNC.Mutex
	ProxyMode                   PtyProxyMode
	PtyCommand                  *_EXEC.Cmd
	PtyTerminal                 *_XTERM.Terminal
	PtyReader                   *_PtyReader_
	PostSnapshotBuffer          *_BYTES.Buffer
	OnOutput_Live               func(ptyProxy *_PtyProxy_, ptyOutputData []byte)
	OnOutput_Snapshot           func(ptyProxy *_PtyProxy_, ptyOutputData []byte)
	OnOutput_PostSnapshotBuffer func(ptyProxy *_PtyProxy_, ptyOutputData []byte)
	OnExited_Eio_Success        func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Failure        func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Killed         func(ptyProxy *_PtyProxy_)
	OnExited_Closed             func(ptyProxy *_PtyProxy_)
	OnExited_SystemError        func(ptyProxy *_PtyProxy_, readerTerminalSignal error)
}

type _SpawnApi__PtyProxy_ struct {
	Id                              uint32
	ColumnCount_PtyTerminal         int
	RowCount_PtyTerminal            int
	ShellBinaryPath_PtyCommand      string
	DirectoryPath_PtyCommand        string
	EnvironmentVariables_PtyCommand []string
	StagingBufferSize_PtyReader     int
	ScrollbackLineCount_PtyTerminal int
	PostSnapshotBufferSize_PtyProxy int
	OnSpawned                       func(ptyProxy *_PtyProxy_)
	OnOutput_Live                   func(ptyProxy *_PtyProxy_, ptyOutputData []byte)
	OnOutput_Snapshot               func(ptyProxy *_PtyProxy_, ptyOutputData []byte)
	OnOutput_PostSnapshotBuffer     func(ptyProxy *_PtyProxy_, ptyOutputData []byte)
	OnExited_Eio_Success            func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Failure            func(ptyProxy *_PtyProxy_)
	OnExited_Eio_Killed             func(ptyProxy *_PtyProxy_)
	OnExited_Closed                 func(ptyProxy *_PtyProxy_)
	OnExited_SystemError            func(ptyProxy *_PtyProxy_, readerTerminalSignal error)
}

func Spawn__PtyProxy(api _SpawnApi__PtyProxy_) error {
	newPtyProxyResult := &_PtyProxy_{
		Id:                          api.Id,
		ProxyMode:                   SPAWNING__PtyProxyMode,
		OnOutput_Live:               api.OnOutput_Live,
		OnOutput_Snapshot:           api.OnOutput_Snapshot,
		OnOutput_PostSnapshotBuffer: api.OnOutput_PostSnapshotBuffer,
		OnExited_Eio_Success:        api.OnExited_Eio_Success,
		OnExited_Eio_Failure:        api.OnExited_Eio_Failure,
		OnExited_Eio_Killed:         api.OnExited_Eio_Killed,
		OnExited_Closed:             api.OnExited_Closed,
		OnExited_SystemError:        api.OnExited_SystemError,
		PtyMasterFileDescriptor:     nil,
		PtyCommand:                  nil,
		PtyTerminal:                 nil,
		PostSnapshotBuffer:          nil,
	}
	newPtyProxyResult.PtyReader = &_PtyReader_{
		UnflushedStagingBufferSliceSize: 0,
		OnTryFlush:                      newPtyProxyResult.HandleTryFlush,
		OnBlockingFlush:                 newPtyProxyResult.HandleBlockingFlush,
		OnExited_Closed:                 newPtyProxyResult.HandleExited_Closed,
		OnExited_Eio:                    newPtyProxyResult.HandleExited_Eio,
		OnExited_SystemError:            newPtyProxyResult.HandleExited_SystemError,
		PtyMasterFileDescriptor:         nil,
		StagingBuffer:                   nil,
	}
	newPtyProxyResult.PtyReader.StagingBuffer = make(
		[]byte,
		api.StagingBufferSize_PtyReader,
	)
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
	__PtyMasterFileDescriptor, ptyStartError := _PTY.Start(__PtyCommand_Proxy)
	if ptyStartError != nil {
		return ptyStartError
	}
	newPtyProxyResult.ProxyMode = RUNNING_LIVE__PtyProxyMode
	newPtyProxyResult.PtyCommand = __PtyCommand_Proxy
	newPtyProxyResult.PtyMasterFileDescriptor = __PtyMasterFileDescriptor
	newPtyProxyResult.PtyReader.PtyMasterFileDescriptor = __PtyMasterFileDescriptor
	api.OnSpawned(newPtyProxyResult)
	go newPtyProxyResult.PtyReader.RunWorker()
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
		var isWasPtyProxyModeRunningLive bool
		ptyProxy.PtyTerminal.Write(unflushedStagingBufferSlice)
		if RUNNING_LIVE__PtyProxyMode == ptyProxy.ProxyMode {
			isWasPtyProxyModeRunningLive = true
		} else if RUNNING_POST_SNAPSHOT__PtyProxyMode == ptyProxy.ProxyMode {
			ptyProxy.PostSnapshotBuffer.Write(unflushedStagingBufferSlice)
		} else if RUNNING_PRE_SNAPSHOT__PtyProxyMode == ptyProxy.ProxyMode {
		} else {
			// SPAWNING__PtyProxyMode == ptyProxy.ProxyMode
			// EXITED__PtyProxyMode == ptyProxy.ProxyMode
			_FMT.Println("invalid path: _PtyProxy_ __flushReaderStagingBufferSliceIfLockAcquired")
		}
		ptyProxy.Mutex.Unlock()
		if isWasPtyProxyModeRunningLive {
			ptyProxy.OnOutput_Live(
				ptyProxy,
				_BYTES.Clone(unflushedStagingBufferSlice),
			)
		}
		return true
	}
	return false
}

func (this *_PtyProxy_) HandleExited_Closed(readerTerminalSignal error) {
	__executeExitedTeardownPipeline(
		this.HandleDispatchExitedHandler_Closed,
		this,
		readerTerminalSignal,
	)
}

func (this *_PtyProxy_) HandleDispatchExitedHandler_Closed(_ error) {
	this.OnExited_Closed(this)
}

func (this *_PtyProxy_) HandleExited_Eio(readerTerminalSignal error) {
	__executeExitedTeardownPipeline(
		this.HandleDispatchExitedHandler_Eio,
		this,
		readerTerminalSignal,
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

func (this *_PtyProxy_) HandleExited_SystemError(readerTerminalSignal error) {
	__executeExitedTeardownPipeline(
		this.HandleDispatchExitedHandler_SystemError,
		this,
		readerTerminalSignal,
	)
}

func (this *_PtyProxy_) HandleDispatchExitedHandler_SystemError(readerTerminalSignal error) {
	this.OnExited_SystemError(
		this,
		readerTerminalSignal,
	)
}

func __executeExitedTeardownPipeline(
	onDispatchExitedHandler func(readerTerminalSignal error),
	ptyProxy *_PtyProxy_,
	readerTerminalSignal error,
) {
	ptyProxy.Mutex.Lock()
	ptyProxy.ProxyMode = EXITED__PtyProxyMode
	ptyProxy.Mutex.Unlock()
	_ = ptyProxy.PtyMasterFileDescriptor.Close()
	_ = ptyProxy.PtyCommand.Wait()
	onDispatchExitedHandler(readerTerminalSignal)
}

func (this *_PtyProxy_) TransitionMode_LiveToPreSnapshot() {
	this.Mutex.Lock()
	if this.ProxyMode != EXITED__PtyProxyMode {
		this.ProxyMode = RUNNING_PRE_SNAPSHOT__PtyProxyMode
	}
	this.Mutex.Unlock()
}

func (this *_PtyProxy_) TransitionMode_PreToPostSnapshot() {
	this.Mutex.Lock()
	this.PostSnapshotBuffer.Reset()
	if this.ProxyMode != EXITED__PtyProxyMode {
		this.ProxyMode = RUNNING_POST_SNAPSHOT__PtyProxyMode
	}
	serializeAddon := _XTERM.NewSerializeAddon(this.PtyTerminal)
	snapshotBytes := serializeAddon.Serialize(nil)
	this.Mutex.Unlock()
	this.OnOutput_Snapshot(
		this,
		snapshotBytes,
	)
}

func (this *_PtyProxy_) TransitionMode_PostSnapshotToLive() {
	this.Mutex.Lock()
	clonedPostSnapshotBuffer := _BYTES.Clone(this.PostSnapshotBuffer.Bytes())
	if this.ProxyMode != EXITED__PtyProxyMode {
		this.ProxyMode = RUNNING_LIVE__PtyProxyMode
	}
	this.Mutex.Unlock()
	if len(clonedPostSnapshotBuffer) > 0 {
		this.OnOutput_PostSnapshotBuffer(
			this,
			clonedPostSnapshotBuffer,
		)
	}
}

func (this *_PtyProxy_) Resize(
	nextColumnCount int,
	nextRowCount int,
) error {
	this.Mutex.Lock()
	this.PtyTerminal.Resize(
		nextColumnCount,
		nextRowCount,
	)
	this.Mutex.Unlock()
	return _PTY.Setsize(
		this.PtyMasterFileDescriptor,
		&_PTY.Winsize{
			Rows: uint16(nextRowCount),
			Cols: uint16(nextColumnCount),
		},
	)
}
