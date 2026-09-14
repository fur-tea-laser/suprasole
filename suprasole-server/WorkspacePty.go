package main

import (
	_SYNC "sync"
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

type _Status_WorkspacePty_ uint8

const (
	SPAWNING__Status_WorkspacePty _Status_WorkspacePty_ = 0x00
	ACTIVE__Status_WorkspacePty   _Status_WorkspacePty_ = 0x01
	EXITED__Status_WorkspacePty   _Status_WorkspacePty_ = 0x02
)

type _Visibility__Client_connected_ uint8

const (
	DISCONNECTED_UNKNOWN___Visibility__Client_connected _Visibility__Client_connected_ = 0
	VISIBLE___Visibility__Client_connected              _Visibility__Client_connected_ = 1
	NOT_VISIBLE___Visibility__Client_connected          _Visibility__Client_connected_ = 2
)

type _State_WorkspacePty_ interface {
	HandleWriteInput_Ingress(inputOrder_PtyWriter _InputOrder_PtyWriter_)
	HandleTerminate_Ingress(terminalSignal int)
	HandleRemove_Ingress(ptyPool map[uint32]*_WorkspacePty_, id_WorkspacePty uint32)
	HandleResize_Debouncer(columnCount_PtyTerminal int, rowCount_PtyTerminal int)
	HandleDisconnect_Coordinator()
	Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_)
	HandleSync_Coordinator(order_SyncPty_maybe *_Order_SyncPty_, visibility__Client_connected__previous _Visibility__Client_connected_, WebsocketController_Pty *_WebsocketController_, id_WebsocketConnection_expected uint64, id_WorkspacePty uint32)
}

type _ResizeGeometry___State_Spawning__WorkspacePty_ struct {
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

type _CancellationStatus___State_Spawning__WorkspacePty_ uint8

const (
	NOT_CANCELED____CancellationStatus___State_Spawning__WorkspacePty _CancellationStatus___State_Spawning__WorkspacePty_ = 0
	CANCELED____CancellationStatus___State_Spawning__WorkspacePty     _CancellationStatus___State_Spawning__WorkspacePty_ = 1
)

type _State_Spawning__WorkspacePty_ struct {
	Mutex                          _SYNC.Mutex
	CancellationStatus             _CancellationStatus___State_Spawning__WorkspacePty_
	ResizeGeometry__deferred_maybe *_ResizeGeometry___State_Spawning__WorkspacePty_
}

func (this *_State_Spawning__WorkspacePty_) Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = SPAWNING__Status_WorkspacePty
}

type _State_Active__WorkspacePty_ struct {
	PtyProxy *_PtyProxy_
}

func (this *_State_Active__WorkspacePty_) Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = ACTIVE__Status_WorkspacePty
}

type _State_Exited__WorkspacePty_ struct {
	PtyProxy    *_PtyProxy_
	ExitOutcome _ExitOutcome_PtyProxy_
}

func (this *_State_Exited__WorkspacePty_) Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = EXITED__Status_WorkspacePty
	bulletin_result.ExitOutcome_PtyProxy_maybe = this.ExitOutcome
}

type _WorkspacePty_ struct {
	Id                                    uint32
	Visibility__Client_connected__current _Visibility__Client_connected_
	State_current                         _State_WorkspacePty_
}
