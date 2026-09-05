package main

import (
	_CONTEXT "context"
	_FLAG "flag"
	_FMT "fmt"
	_OS "os"
	_SIGNAL "os/signal"
	_SYSCALL "syscall"
	_TIME "time"
)

func main() {
	hostPortAddressFlag := _FLAG.String(
		"address",
		"127.0.0.1:8000",
		"Host and port address to listen on",
	)
	_FLAG.Parse()
	workspaceController := New__WorkspaceController(_NewApi__WorkspaceController_{
		Address_HttpServer__: *hostPortAddressFlag,
		Defaults_PtyProxy__: _Defaults_PtyProxy_{
			ScrollbackLineCount_PtyTerminal__:       10000,
			StagingBufferSize_PtyReader__:           16 * 1024,
			PostSnapshotBufferSize_PtyProxy__:       64 * 1024,
			QueueBufferSize_InputOrder__PtyWriter__: 1024,
		},
	})
	startSessionError := workspaceController.StartSession()
	if startSessionError != nil {
		_FMT.Fprintf(
			_OS.Stderr,
			"Error starting session: %v\n",
			startSessionError,
		)
		_OS.Exit(1)
	}
	_FMT.Printf(
		"Suprasole server listening on %s\n",
		*hostPortAddressFlag,
	)
	shutdownSignalChannel := make(
		chan _OS.Signal,
		1,
	)
	_SIGNAL.Notify(
		shutdownSignalChannel,
		_OS.Interrupt,
		_SYSCALL.SIGTERM,
	)
	<-shutdownSignalChannel
	_FMT.Println("Shutting down suprasole server...")
	context_shutdownDeadline__HttpServer, cancelShutdownDeadline_HttpServer := _CONTEXT.WithTimeout(
		_CONTEXT.Background(),
		5*_TIME.Second,
	)
	_ = workspaceController.StopSession(context_shutdownDeadline__HttpServer)
	cancelShutdownDeadline_HttpServer()
}
