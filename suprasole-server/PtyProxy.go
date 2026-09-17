package main

import (
	_BYTES "bytes"
	_OS "os"
	_EXEC "os/exec"
	_SYNC "sync"

	_XTERM "github.com/gitpod-io/xterm-go"
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
	Mode_state                       _Mode_PtyProxy_
	PtyCommand                       *_EXEC.Cmd
	FileDescriptor_Master__PtyDevice *_OS.File
	PtyTerminal                      *_XTERM.Terminal
	PtyReader                        *_PtyReader_
	PtyWriter                        *_PtyWriter_
	PostSnapshotBuffer               *_BYTES.Buffer
}

type _Mode_PtyProxy_ int

const (
	SPAWNING__Mode_PtyProxy _Mode_PtyProxy_ = iota
	LIVE_RUNNING__Mode_PtyProxy
	PRE_SNAPSHOT__RUNNING___Mode_PtyProxy
	POST_SNAPSHOT__RUNNING___Mode_PtyProxy
	EXITED__Mode_PtyProxy
)
