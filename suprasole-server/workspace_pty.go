package main

type PtyExitReason int

const (
	SUCCESS__PtyExitReason PtyExitReason = iota
	FAILURE__PtyExitReason
	KILLED__PtyExitReason
	CLOSED__PtyExitReason
	SYSTEM_ERROR__PtyExitReason
)

type _PtyExitResult_ interface {
	compiletimemarker_PtyExitResult()
}

type _PtyExitResult_Success_ struct{}

func (_PtyExitResult_Success_) compiletimemarker_PtyExitResult() {}

type _PtyExitResult_Failure_ struct {
	ExitCode int
}

func (_PtyExitResult_Failure_) compiletimemarker_PtyExitResult() {}

type _PtyExitResult_Killed_ struct {
	ExitSignal int
}

func (_PtyExitResult_Killed_) compiletimemarker_PtyExitResult() {}

type _PtyExitResult_Closed_ struct{}

func (_PtyExitResult_Closed_) compiletimemarker_PtyExitResult() {}

type _PtyExitResult_SystemError_ struct {
	SystemError error
}

func (_PtyExitResult_SystemError_) compiletimemarker_PtyExitResult() {}

type _WorkspacePty_ struct {
	Id              uint32
	PtyProxy        *_PtyProxy_
	IsActive        bool
	MaybeExitResult _PtyExitResult_
}
