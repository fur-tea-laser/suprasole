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
	cliOption__Address_HttpServer := _FLAG.String(
		"Address_HttpServer",
		"127.0.0.1:8000",
		"Network address (host:port) for the HTTP server to listen on",
	)
	_FLAG.Parse()
	workspaceController := Make_WorkspaceController(_MakeApi_WorkspaceController_{
		Address_HttpServer__: *cliOption__Address_HttpServer,
		OptionConfig_PtyProxy__default__: _OptionConfig_PtyProxy_{
			Count_ScrollbackLine__PtyTerminal:      10000,
			Size_StagingBuffer__PtyReader:          16 * 1024,
			Size_PostSnapshotBuffer__PtyProxy:      64 * 1024,
			Size_QueueBuffer__InputOrder_PtyWriter: 1024,
		},
	})
	error_startSession_maybe := workspaceController.StartSession()
	if error_startSession_maybe != nil {
		_FMT.Fprintf(
			_OS.Stderr,
			"Error starting session: %v\n",
			error_startSession_maybe,
		)
		_OS.Exit(1)
	}
	_FMT.Printf(
		"Suprasole server listening on %s\n",
		*cliOption__Address_HttpServer,
	)
	channel_shutdownSignal := make(chan _OS.Signal, 1)
	_SIGNAL.Notify(
		channel_shutdownSignal,
		_OS.Interrupt,
		_SYSCALL.SIGTERM,
	)
	<-channel_shutdownSignal
	_FMT.Println("Shutting down suprasole server...")
	context_shutdownDeadline__HttpServer, cancel_shutdownDeadline__HttpServer := _CONTEXT.WithTimeout(
		_CONTEXT.Background(),
		5*_TIME.Second,
	)
	_ = workspaceController.StopSession(context_shutdownDeadline__HttpServer)
	cancel_shutdownDeadline__HttpServer()
}
