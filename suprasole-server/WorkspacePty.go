package main

type ExitReason_PtyProxy int

const (
	SUCCESS__ExitReason_PtyProxy ExitReason_PtyProxy = iota
	FAILURE__ExitReason_PtyProxy
	KILLED__ExitReason_PtyProxy
	CLOSED__ExitReason_PtyProxy
	SYSTEM_ERROR__ExitReason_PtyProxy
)

type _ExitOutcome_PtyProxy_ interface {
	compiletimemarker_ExitOutcome_PtyProxy()
}

type _Success__ExitOutcome_PtyProxy_ struct{}

func (_Success__ExitOutcome_PtyProxy_) compiletimemarker_ExitOutcome_PtyProxy() {}

type _Failure__ExitOutcome_PtyProxy_ struct {
	ExitCode int
}

func (_Failure__ExitOutcome_PtyProxy_) compiletimemarker_ExitOutcome_PtyProxy() {}

type _Killed__ExitOutcome_PtyProxy_ struct {
	ExitSignal int
}

func (_Killed__ExitOutcome_PtyProxy_) compiletimemarker_ExitOutcome_PtyProxy() {}

type _Closed__ExitOutcome_PtyProxy_ struct{}

func (_Closed__ExitOutcome_PtyProxy_) compiletimemarker_ExitOutcome_PtyProxy() {}

type _SystemError__ExitOutcome_PtyProxy_ struct {
	SystemError error
}

func (_SystemError__ExitOutcome_PtyProxy_) compiletimemarker_ExitOutcome_PtyProxy() {}

type _WorkspacePty_ struct {
	PtyProxy         *_PtyProxy_
	IsVisible        bool
	MaybeExitOutcome _ExitOutcome_PtyProxy_
}
