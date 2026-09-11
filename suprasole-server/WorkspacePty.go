package main

import (
	_FMT "fmt"
)

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

type _Status_WorkspacePty_ uint8

const (
	SPAWNING__Status_WorkspacePty _Status_WorkspacePty_ = 0x00
	ACTIVE__Status_WorkspacePty   _Status_WorkspacePty_ = 0x01
	EXITED__Status_WorkspacePty   _Status_WorkspacePty_ = 0x02
)

type _State__WorkspacePty_ interface {
	HandleWriteInput_Ingress(order _InputOrder_PtyWriter_)
	HandleTerminate_Ingress(terminalSignal int)
	HandleRemove_Ingress(ptyPool map[uint32]*_WorkspacePty_, id_WorkspacePty uint32)
	HandleResize_Debouncer(columnCount int, rowCount int)
	HandleDisconnect_Coordinator()
	HandlePopulateBulletin_Manifest(bulletin_result *_Bulletin_WorkspacePty_)
}

type _DeferredResize__SpawningState_WorkspacePty_ struct {
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

type _CancellationStatus_SpawningState_ uint8

const (
	NOT_CANCELED__CancellationStatus_SpawningState _CancellationStatus_SpawningState_ = 0
	CANCELED__CancellationStatus_SpawningState     _CancellationStatus_SpawningState_ = 1
)

type _SpawningState__WorkspacePty_ struct {
	CancellationStatus   _CancellationStatus_SpawningState_
	DeferredResize_maybe *_DeferredResize__SpawningState_WorkspacePty_
}

func (this *_SpawningState__WorkspacePty_) HandleWriteInput_Ingress(order _InputOrder_PtyWriter_) {
	_FMT.Println("invalid path: _SpawningState__WorkspacePty_ HandleWriteInput_Ingress")
}

func (this *_SpawningState__WorkspacePty_) HandleTerminate_Ingress(terminalSignal int) {
	this.CancellationStatus = CANCELED__CancellationStatus_SpawningState
}

func (this *_SpawningState__WorkspacePty_) HandleRemove_Ingress(ptyPool map[uint32]*_WorkspacePty_, id_WorkspacePty uint32) {
	// Valid invocation: Client requested removal, but spawning sessions must remain in PtyPool until the OS spawn worker completes.
	// No action required: Session is retained in PtyPool; coordinator will purge upon spawn failure or cancellation.
}

func (this *_SpawningState__WorkspacePty_) HandleResize_Debouncer(columnCount int, rowCount int) {
	this.DeferredResize_maybe = &_DeferredResize__SpawningState_WorkspacePty_{
		ColumnCount_PtyTerminal: columnCount,
		RowCount_PtyTerminal:    rowCount,
	}
}

func (this *_SpawningState__WorkspacePty_) HandleDisconnect_Coordinator() {
	// Valid invocation: The coordinator broadcasts disconnect events across all sessions in PtyPool when the websocket disconnects.
	// No action required: Process spawning is in flight and no PtyProxy exists yet. Spawn completion independently derives PRE_SNAPSHOT mode if disconnected.
}

func (this *_SpawningState__WorkspacePty_) HandlePopulateBulletin_Manifest(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = SPAWNING__Status_WorkspacePty
}

type _ActiveState__WorkspacePty_ struct {
	PtyProxy *_PtyProxy_
}

func (this *_ActiveState__WorkspacePty_) HandleWriteInput_Ingress(order _InputOrder_PtyWriter_) {
	select {
	case <-this.PtyProxy.PtyWriter.WorkerContext.Done():
	default:
		select {
		case this.PtyProxy.PtyWriter.QueueChannel_InputOrder <- order:
		default:
		}
	}
}

func (this *_ActiveState__WorkspacePty_) HandleTerminate_Ingress(terminalSignal int) {
	this.PtyProxy.Terminate_PtyProcess(terminalSignal)
}

func (this *_ActiveState__WorkspacePty_) HandleRemove_Ingress(ptyPool map[uint32]*_WorkspacePty_, id_WorkspacePty uint32) {
	// Valid invocation: Client requested removal, but active running sessions cannot be purged.
	// No action required: Running sessions remain protected in PtyPool until process termination.
}

func (this *_ActiveState__WorkspacePty_) HandleResize_Debouncer(columnCount int, rowCount int) {
	_ = this.PtyProxy.Resize(
		columnCount,
		rowCount,
	)
}

func (this *_ActiveState__WorkspacePty_) HandleDisconnect_Coordinator() {
	switch this.PtyProxy.Mode_current {
	case LIVE_RUNNING__Mode_PtyProxy:
		this.PtyProxy.TransitionMode_LiveToPreSnapshot()
	case POST_SNAPSHOT__RUNNING___Mode_PtyProxy:
		this.PtyProxy.TransitionMode_PostSnapshotToPreSnapshot()
	}
}

func (this *_ActiveState__WorkspacePty_) HandlePopulateBulletin_Manifest(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = ACTIVE__Status_WorkspacePty
}

type _ExitedState__WorkspacePty_ struct {
	PtyProxy    *_PtyProxy_
	ExitOutcome _ExitOutcome_PtyProxy_
}

func (this *_ExitedState__WorkspacePty_) HandleWriteInput_Ingress(order _InputOrder_PtyWriter_) {
	// Valid invocation: Client keystrokes transmitted prior to receiving process exit status cross in-flight across the network.
	// No action required: The process has already exited and the input writer is closed. In-flight input is safely dropped.
}

func (this *_ExitedState__WorkspacePty_) HandleTerminate_Ingress(terminalSignal int) {
	// Valid invocation: A client termination request crosses in-flight across the network with natural process exit on the server.
	// No action required: The process has already exited and its exit outcome is finalized.
}

func (this *_ExitedState__WorkspacePty_) HandleRemove_Ingress(ptyPool map[uint32]*_WorkspacePty_, id_WorkspacePty uint32) {
	delete(ptyPool, id_WorkspacePty)
}

func (this *_ExitedState__WorkspacePty_) HandleResize_Debouncer(columnCount int, rowCount int) {
	_ = this.PtyProxy.Resize(
		columnCount,
		rowCount,
	)
}

func (this *_ExitedState__WorkspacePty_) HandleDisconnect_Coordinator() {
	// Valid invocation: The coordinator broadcasts disconnect events across all sessions in PtyPool when the websocket disconnects.
	// No action required: The session has already exited and live streaming is inactive.
}

func (this *_ExitedState__WorkspacePty_) HandlePopulateBulletin_Manifest(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = EXITED__Status_WorkspacePty
	bulletin_result.ExitOutcome_PtyProxy_maybe = this.ExitOutcome
}

type _WorkspacePty_ struct {
	Id                        uint32
	Visibility_Client_current _Visibility_Client_
	State_current             _State__WorkspacePty_
}
