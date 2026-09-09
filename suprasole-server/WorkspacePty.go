package main

type _ExitReason_PtyProxy_ int

const (
	SUCCESS__ExitReason_PtyProxy _ExitReason_PtyProxy_ = iota
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
	ExitCode_PtyProcess int
}

func (_Failure__ExitOutcome_PtyProxy_) compiletimemarker_ExitOutcome_PtyProxy() {}

type _Killed__ExitOutcome_PtyProxy_ struct {
	ExitSignal_PtyProcess int
}

func (_Killed__ExitOutcome_PtyProxy_) compiletimemarker_ExitOutcome_PtyProxy() {}

type _Closed__ExitOutcome_PtyProxy_ struct{}

func (_Closed__ExitOutcome_PtyProxy_) compiletimemarker_ExitOutcome_PtyProxy() {}

type _SystemError__ExitOutcome_PtyProxy_ struct {
	SystemError_PtyDevice error
}

func (_SystemError__ExitOutcome_PtyProxy_) compiletimemarker_ExitOutcome_PtyProxy() {}

type _Visibility_Client_ uint8

const (
	NOT_VISIBLE__Visibility_Client _Visibility_Client_ = 0
	VISIBLE__Visibility_Client     _Visibility_Client_ = 1
)

type _WorkspacePty_ struct {
	PtyProxy                   *_PtyProxy_
	ExitOutcome_PtyProxy_maybe _ExitOutcome_PtyProxy_
	Visibility_Client_current  _Visibility_Client_
}
