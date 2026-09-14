package main

import (
	_FMT "fmt"
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

func (this *_State_Spawning__WorkspacePty_) HandleWriteInput_Ingress(inputOrder_PtyWriter _InputOrder_PtyWriter_) {
	_FMT.Println("invalid path: _State_Spawning__WorkspacePty_ HandleWriteInput_Ingress")
}

func (this *_State_Spawning__WorkspacePty_) HandleTerminate_Ingress(terminalSignal int) {
	this.Mutex.Lock()
	this.CancellationStatus = CANCELED____CancellationStatus___State_Spawning__WorkspacePty
	this.Mutex.Unlock()
}

func (this *_State_Spawning__WorkspacePty_) HandleRemove_Ingress(ptyPool map[uint32]*_WorkspacePty_, id_WorkspacePty uint32) {
	// Valid invocation: Client requested removal, but spawning sessions must remain in PtyPool until the OS spawn worker completes.
	// No action required: Session is retained in PtyPool; coordinator will purge upon spawn failure or cancellation.
}

func (this *_State_Spawning__WorkspacePty_) HandleResize_Debouncer(
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) {
	this.Mutex.Lock()
	this.ResizeGeometry__deferred_maybe = &_ResizeGeometry___State_Spawning__WorkspacePty_{
		ColumnCount_PtyTerminal: columnCount_PtyTerminal,
		RowCount_PtyTerminal:    rowCount_PtyTerminal,
	}
	this.Mutex.Unlock()
}

func (this *_State_Spawning__WorkspacePty_) HandleDisconnect_Coordinator() {
}

func (this *_State_Spawning__WorkspacePty_) Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = SPAWNING__Status_WorkspacePty
}

func (this *_State_Spawning__WorkspacePty_) HandleSync_Coordinator(
	order_SyncPty_maybe *_Order_SyncPty_,
	_ _Visibility__Client_connected_,
	_ *_WebsocketController_,
	_ uint64,
	_ uint32,
) {
	if order_SyncPty_maybe != nil {
		this.Mutex.Lock()
		this.ResizeGeometry__deferred_maybe = &_ResizeGeometry___State_Spawning__WorkspacePty_{
			ColumnCount_PtyTerminal: order_SyncPty_maybe.ColumnCount_PtyTerminal,
			RowCount_PtyTerminal:    order_SyncPty_maybe.RowCount_PtyTerminal,
		}
		this.Mutex.Unlock()
	}
}

type _State_Active__WorkspacePty_ struct {
	PtyProxy *_PtyProxy_
}

func (this *_State_Active__WorkspacePty_) HandleWriteInput_Ingress(inputOrder_PtyWriter _InputOrder_PtyWriter_) {
	select {
	case <-this.PtyProxy.PtyWriter.WorkerContext.Done():
	default:
		select {
		case this.PtyProxy.PtyWriter.QueueChannel_InputOrder <- inputOrder_PtyWriter:
		default:
		}
	}
}

func (this *_State_Active__WorkspacePty_) HandleTerminate_Ingress(terminalSignal int) {
	this.PtyProxy.Terminate_PtyProcess(terminalSignal)
}

func (this *_State_Active__WorkspacePty_) HandleRemove_Ingress(ptyPool map[uint32]*_WorkspacePty_, id_WorkspacePty uint32) {
	// Valid invocation: Client requested removal, but active running sessions cannot be purged.
	// No action required: Running sessions remain protected in PtyPool until process termination.
}

func (this *_State_Active__WorkspacePty_) HandleResize_Debouncer(
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) {
	_ = this.PtyProxy.Resize(
		columnCount_PtyTerminal,
		rowCount_PtyTerminal,
	)
}

func (this *_State_Active__WorkspacePty_) HandleDisconnect_Coordinator() {
	switch this.PtyProxy.Mode_current {
	case LIVE_RUNNING__Mode_PtyProxy:
		this.PtyProxy.TransitionMode_LiveToPreSnapshot()
	case POST_SNAPSHOT__RUNNING___Mode_PtyProxy:
		this.PtyProxy.TransitionMode_PostSnapshotToPreSnapshot()
	}
}

func (this *_State_Active__WorkspacePty_) Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = ACTIVE__Status_WorkspacePty
}

func (this *_State_Active__WorkspacePty_) HandleSync_Coordinator(
	order_SyncPty_maybe *_Order_SyncPty_,
	visibility__Client_connected__previous _Visibility__Client_connected_,
	WebsocketController_Pty *_WebsocketController_,
	id_WebsocketConnection_expected uint64,
	id_WorkspacePty uint32,
) {
	if order_SyncPty_maybe != nil {
		_ = this.PtyProxy.Resize(
			order_SyncPty_maybe.ColumnCount_PtyTerminal,
			order_SyncPty_maybe.RowCount_PtyTerminal,
		)
	}
	if order_SyncPty_maybe != nil && visibility__Client_connected__previous != VISIBLE___Visibility__Client_connected {
		__emitSnapshot_SyncPty__WebsocketController_Pty(
			this.PtyProxy.TransitionMode_PreToPostSnapshot,
			WebsocketController_Pty,
			id_WebsocketConnection_expected,
			id_WorkspacePty,
		)
		this.PtyProxy.TransitionMode_PostSnapshotToLive()
	} else if nil == order_SyncPty_maybe && VISIBLE___Visibility__Client_connected == visibility__Client_connected__previous {
		this.PtyProxy.TransitionMode_LiveToPreSnapshot()
	}
}

type _State_Exited__WorkspacePty_ struct {
	PtyProxy    *_PtyProxy_
	ExitOutcome _ExitOutcome_PtyProxy_
}

func (this *_State_Exited__WorkspacePty_) HandleWriteInput_Ingress(inputOrder_PtyWriter _InputOrder_PtyWriter_) {
	// Valid invocation: Client keystrokes transmitted prior to receiving process exit status cross in-flight across the network.
	// No action required: The process has already exited and the input writer is closed. In-flight input is safely dropped.
}

func (this *_State_Exited__WorkspacePty_) HandleTerminate_Ingress(terminalSignal int) {
	// Valid invocation: A client termination request crosses in-flight across the network with natural process exit on the server.
	// No action required: The process has already exited and its exit outcome is finalized.
}

func (this *_State_Exited__WorkspacePty_) HandleRemove_Ingress(ptyPool map[uint32]*_WorkspacePty_, id_WorkspacePty uint32) {
	delete(ptyPool, id_WorkspacePty)
}

func (this *_State_Exited__WorkspacePty_) HandleResize_Debouncer(
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) {
	_ = this.PtyProxy.Resize(
		columnCount_PtyTerminal,
		rowCount_PtyTerminal,
	)
}

func (this *_State_Exited__WorkspacePty_) HandleDisconnect_Coordinator() {
}

func (this *_State_Exited__WorkspacePty_) Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_) {
	bulletin_result.Status_WorkspacePty = EXITED__Status_WorkspacePty
	bulletin_result.ExitOutcome_PtyProxy_maybe = this.ExitOutcome
}

func (this *_State_Exited__WorkspacePty_) HandleSync_Coordinator(
	order_SyncPty_maybe *_Order_SyncPty_,
	visibility__Client_connected__previous _Visibility__Client_connected_,
	WebsocketController_Pty *_WebsocketController_,
	id_WebsocketConnection_expected uint64,
	id_WorkspacePty uint32,
) {
	if order_SyncPty_maybe != nil {
		_ = this.PtyProxy.Resize(
			order_SyncPty_maybe.ColumnCount_PtyTerminal,
			order_SyncPty_maybe.RowCount_PtyTerminal,
		)
	}
	if order_SyncPty_maybe != nil && visibility__Client_connected__previous != VISIBLE___Visibility__Client_connected {
		__emitSnapshot_SyncPty__WebsocketController_Pty(
			this.PtyProxy.EmitSnapshot_Exited,
			WebsocketController_Pty,
			id_WebsocketConnection_expected,
			id_WorkspacePty,
		)
	}
}

type _WorkspacePty_ struct {
	Id                                    uint32
	Visibility__Client_connected__current _Visibility__Client_connected_
	State_current                         _State_WorkspacePty_
}

func __emitSnapshot_SyncPty__WebsocketController_Pty(
	onEmitSnapshot__ func(),
	WebsocketController_Pty *_WebsocketController_,
	id_WebsocketConnection_expected uint64,
	id_WorkspacePty_target uint32,
) {
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		WebsocketController_Pty,
		id_WebsocketConnection_expected,
		_TaskStart_SyncPty__PtyMessage_Egress_{
			Id_PtyProxy: id_WorkspacePty_target,
		},
	)
	onEmitSnapshot__()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		WebsocketController_Pty,
		id_WebsocketConnection_expected,
		_TaskComplete_SyncPty__PtyMessage_Egress_{
			Id_PtyProxy: id_WorkspacePty_target,
		},
	)
}
