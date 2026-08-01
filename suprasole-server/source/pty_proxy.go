package source

import (
	"bytes"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"github.com/gitpod-io/xterm-go"
)

type PtyProxyMode int

const (
	PtyProxyMode_Spawning PtyProxyMode = iota
	PtyProxyMode_Running_Live
	PtyProxyMode_Running_PreSnapshot
	PtyProxyMode_Running_PostSnapshot
	PtyProxyMode_Exited
)

type PtyProxy struct {
	Pty_MasterFileDescriptor          *os.File
	Proxy_Id                          uint32
	Proxy_Mutex                       sync.Mutex
	Proxy_Mode                        PtyProxyMode
	Proxy_TerminalCommand             *exec.Cmd
	Proxy_TerminalState               *xterm.Terminal
	Proxy_PtyReader                   *PtyReader
	Proxy_PostSnapshotBuffer          *bytes.Buffer
	Proxy_OnOutput_Live               func(ptyProxy *PtyProxy, ptyOutputData []byte)
	Proxy_OnOutput_Snapshot           func(ptyProxy *PtyProxy, ptyOutputData []byte)
	Proxy_OnOutput_PostSnapshotBuffer func(ptyProxy *PtyProxy, ptyOutputData []byte)
	Proxy_OnExited_Eio_Success        func(ptyProxy *PtyProxy)
	Proxy_OnExited_Eio_Failure        func(ptyProxy *PtyProxy)
	Proxy_OnExited_Eio_Killed         func(ptyProxy *PtyProxy)
	Proxy_OnExited_Closed             func(ptyProxy *PtyProxy)
	Proxy_OnExited_SystemError        func(ptyProxy *PtyProxy, readerTerminalSignal error)
}

type NewPtyProxyApi struct {
	Proxy_Id                          uint32
	Proxy_ColumnCount                 int
	Proxy_RowCount                    int
	Command_ShellBinaryPath           string
	Proxy_EnvironmentVariables        []string
	Proxy_DirectoryPath               string
	TerminalState_ScrollbackLineCount int
	Reader_StagingBufferSize          int
	Proxy_PostSnapshotBufferSize      int
	Proxy_OnPtySpawned                func(ptyProxy *PtyProxy)
	Proxy_OnOutput_Live               func(ptyProxy *PtyProxy, ptyOutputData []byte)
	Proxy_OnOutput_Snapshot           func(ptyProxy *PtyProxy, ptyOutputData []byte)
	Proxy_OnOutput_PostSnapshotBuffer func(ptyProxy *PtyProxy, ptyOutputData []byte)
	Proxy_OnExited_Eio_Success        func(ptyProxy *PtyProxy)
	Proxy_OnExited_Eio_Failure        func(ptyProxy *PtyProxy)
	Proxy_OnExited_Eio_Killed         func(ptyProxy *PtyProxy)
	Proxy_OnExited_Closed             func(ptyProxy *PtyProxy)
	Proxy_OnExited_SystemError        func(ptyProxy *PtyProxy, readerTerminalSignal error)
}

func NewPtyProxy(api NewPtyProxyApi) (*PtyProxy, error) {
	newPtyProxyResult := &PtyProxy{
		Pty_MasterFileDescriptor:          nil,
		Proxy_Id:                          api.Proxy_Id,
		Proxy_Mode:                        PtyProxyMode_Spawning,
		Proxy_TerminalCommand:             nil,
		Proxy_TerminalState:               nil,
		Proxy_PostSnapshotBuffer:          nil,
		Proxy_OnOutput_Live:               api.Proxy_OnOutput_Live,
		Proxy_OnOutput_Snapshot:           api.Proxy_OnOutput_Snapshot,
		Proxy_OnOutput_PostSnapshotBuffer: api.Proxy_OnOutput_PostSnapshotBuffer,
		Proxy_OnExited_Eio_Success:        api.Proxy_OnExited_Eio_Success,
		Proxy_OnExited_Eio_Failure:        api.Proxy_OnExited_Eio_Failure,
		Proxy_OnExited_Eio_Killed:         api.Proxy_OnExited_Eio_Killed,
		Proxy_OnExited_Closed:             api.Proxy_OnExited_Closed,
		Proxy_OnExited_SystemError:        api.Proxy_OnExited_SystemError,
	}
	newPtyProxyResult.Proxy_PtyReader = &PtyReader{
		Pty_MasterFileDescriptor:               nil,
		Reader_StagingBuffer:                   nil,
		Reader_UnflushedStagingBufferSliceSize: 0,
		Reader_OnTryFlush:                      newPtyProxyResult.Proxy_HandleTryFlush,
		Reader_OnBlockingFlush:                 newPtyProxyResult.Proxy_HandleBlockingFlush,
		Reader_OnExited_Closed:                 newPtyProxyResult.Proxy_HandleExited_Closed,
		Reader_OnExited_Eio:                    newPtyProxyResult.Proxy_HandleExited_Eio,
		Reader_OnExited_SystemError:            newPtyProxyResult.Proxy_HandleExited_SystemError,
	}
	newPtyProxyResult.Proxy_PtyReader.Reader_StagingBuffer = make(
		[]byte,
		api.Reader_StagingBufferSize,
	)
	newPtyProxyResult.Proxy_PostSnapshotBuffer = bytes.NewBuffer(
		make(
			[]byte,
			0,
			api.Proxy_PostSnapshotBufferSize,
		),
	)
	newPtyProxyResult.Proxy_TerminalState = xterm.New(
		xterm.WithCols(api.Proxy_ColumnCount),
		xterm.WithRows(api.Proxy_RowCount),
		xterm.WithScrollback(api.TerminalState_ScrollbackLineCount),
	)
	__Proxy_TerminalCommand := exec.Command(api.Command_ShellBinaryPath)
	__Proxy_TerminalCommand.Env = api.Proxy_EnvironmentVariables
	__Proxy_TerminalCommand.Dir = api.Proxy_DirectoryPath
	__Pty_MasterFileDescriptor, ptyStartError := pty.Start(__Proxy_TerminalCommand)
	if ptyStartError != nil {
		return nil, ptyStartError
	}
	newPtyProxyResult.Proxy_TerminalCommand = __Proxy_TerminalCommand
	newPtyProxyResult.Pty_MasterFileDescriptor = __Pty_MasterFileDescriptor
	newPtyProxyResult.Proxy_PtyReader.Pty_MasterFileDescriptor = __Pty_MasterFileDescriptor
	api.Proxy_OnPtySpawned(newPtyProxyResult)
	go newPtyProxyResult.Proxy_PtyReader.Reader_StartReading()
	return newPtyProxyResult, nil
}

func (thisPtyProxy *PtyProxy) Proxy_HandleTryFlush(unflushedStagingBufferSlice []byte) bool {
	return __flushReaderStagingBufferSliceIfLockAcquired(
		thisPtyProxy.Proxy_Mutex.TryLock,
		thisPtyProxy,
		unflushedStagingBufferSlice,
	)
}

func (thisPtyProxy *PtyProxy) Proxy_HandleBlockingFlush(unflushedStagingBufferSlice []byte) {
	__flushReaderStagingBufferSliceIfLockAcquired(
		thisPtyProxy.Proxy_LockAndReturnTrue,
		thisPtyProxy,
		unflushedStagingBufferSlice,
	)
}

func (thisPtyProxy *PtyProxy) Proxy_LockAndReturnTrue() bool {
	thisPtyProxy.Proxy_Mutex.Lock()
	return true
}

func __flushReaderStagingBufferSliceIfLockAcquired(
	maybeAcquirePtyProxyMutexLock func() bool,
	ptyProxy *PtyProxy,
	unflushedStagingBufferSlice []byte,
) bool {
	if maybeAcquirePtyProxyMutexLock() {
		var isWasPtyProxyModeRunningLive bool
		ptyProxy.Proxy_TerminalState.Write(unflushedStagingBufferSlice)
		if ptyProxy.Proxy_Mode == PtyProxyMode_Running_Live {
			isWasPtyProxyModeRunningLive = true
		} else if ptyProxy.Proxy_Mode == PtyProxyMode_Running_PostSnapshot {
			ptyProxy.Proxy_PostSnapshotBuffer.Write(unflushedStagingBufferSlice)
		}
		ptyProxy.Proxy_Mutex.Unlock()
		if isWasPtyProxyModeRunningLive {
			ptyProxy.Proxy_OnOutput_Live(
				ptyProxy,
				bytes.Clone(unflushedStagingBufferSlice),
			)
		}
		return true
	}
	return false
}

func (thisPtyProxy *PtyProxy) Proxy_HandleExited_Closed(readerTerminalSignal error) {
	__executeExitedTeardownPipeline(
		thisPtyProxy.Proxy_HandleDispatchExitedHandler_Closed,
		thisPtyProxy,
		readerTerminalSignal,
	)
}

func (thisPtyProxy *PtyProxy) Proxy_HandleDispatchExitedHandler_Closed(_ error) {
	thisPtyProxy.Proxy_OnExited_Closed(thisPtyProxy)
}

func (thisPtyProxy *PtyProxy) Proxy_HandleExited_Eio(readerTerminalSignal error) {
	__executeExitedTeardownPipeline(
		thisPtyProxy.Proxy_HandleDispatchExitedHandler_Eio,
		thisPtyProxy,
		readerTerminalSignal,
	)
}

func (thisPtyProxy *PtyProxy) Proxy_HandleDispatchExitedHandler_Eio(_ error) {
	processState := thisPtyProxy.Proxy_TerminalCommand.ProcessState
	processWaitStatus := processState.Sys().(syscall.WaitStatus)
	if processWaitStatus.Signaled() {
		thisPtyProxy.Proxy_OnExited_Eio_Killed(thisPtyProxy)
	} else if processState.Success() {
		thisPtyProxy.Proxy_OnExited_Eio_Success(thisPtyProxy)
	} else {
		thisPtyProxy.Proxy_OnExited_Eio_Failure(thisPtyProxy)
	}
}

func (thisPtyProxy *PtyProxy) Proxy_HandleExited_SystemError(readerTerminalSignal error) {
	__executeExitedTeardownPipeline(
		thisPtyProxy.Proxy_HandleDispatchExitedHandler_SystemError,
		thisPtyProxy,
		readerTerminalSignal,
	)
}

func (thisPtyProxy *PtyProxy) Proxy_HandleDispatchExitedHandler_SystemError(readerTerminalSignal error) {
	thisPtyProxy.Proxy_OnExited_SystemError(
		thisPtyProxy,
		readerTerminalSignal,
	)
}

func __executeExitedTeardownPipeline(
	onDispatchExitedHandler func(readerTerminalSignal error),
	ptyProxy *PtyProxy,
	readerTerminalSignal error,
) {
	ptyProxy.Proxy_Mutex.Lock()
	ptyProxy.Proxy_Mode = PtyProxyMode_Exited
	ptyProxy.Proxy_Mutex.Unlock()
	ptyProxy.Pty_MasterFileDescriptor.Close()
	ptyProxy.Proxy_TerminalCommand.Wait()
	onDispatchExitedHandler(readerTerminalSignal)
}

func (thisPtyProxy *PtyProxy) Proxy_TransitionMode_LiveToPreSnapshot() {
	thisPtyProxy.Proxy_Mutex.Lock()
	thisPtyProxy.Proxy_Mode = PtyProxyMode_Running_PreSnapshot
	thisPtyProxy.Proxy_Mutex.Unlock()
}

func (thisPtyProxy *PtyProxy) Proxy_TransitionMode_PreToPostSnapshot() {
	thisPtyProxy.Proxy_Mutex.Lock()
	thisPtyProxy.Proxy_PostSnapshotBuffer.Reset()
	thisPtyProxy.Proxy_Mode = PtyProxyMode_Running_PostSnapshot
	serializeAddon := xterm.NewSerializeAddon(thisPtyProxy.Proxy_TerminalState)
	snapshotBytes := serializeAddon.Serialize(nil)
	thisPtyProxy.Proxy_Mutex.Unlock()
	thisPtyProxy.Proxy_OnOutput_Snapshot(
		thisPtyProxy,
		snapshotBytes,
	)
}

func (thisPtyProxy *PtyProxy) Proxy_TransitionMode_PostSnapshotToLive() {
	thisPtyProxy.Proxy_Mutex.Lock()
	clonedPostSnapshotBuffer := bytes.Clone(thisPtyProxy.Proxy_PostSnapshotBuffer.Bytes())
	thisPtyProxy.Proxy_Mode = PtyProxyMode_Running_Live
	thisPtyProxy.Proxy_Mutex.Unlock()
	if len(clonedPostSnapshotBuffer) > 0 {
		thisPtyProxy.Proxy_OnOutput_PostSnapshotBuffer(
			thisPtyProxy,
			clonedPostSnapshotBuffer,
		)
	}
}

func (thisPtyProxy *PtyProxy) Proxy_Resize(
	nextColumnCount int,
	nextRowCount int,
) error {
	nextWinsize := &pty.Winsize{
		Rows: uint16(nextRowCount),
		Cols: uint16(nextColumnCount),
	}
	ptySetSizeError := pty.Setsize(
		thisPtyProxy.Pty_MasterFileDescriptor,
		nextWinsize,
	)
	if ptySetSizeError != nil {
		return ptySetSizeError
	}
	thisPtyProxy.Proxy_Mutex.Lock()
	thisPtyProxy.Proxy_TerminalState.Resize(
		nextColumnCount,
		nextRowCount,
	)
	thisPtyProxy.Proxy_Mutex.Unlock()
	return nil
}
