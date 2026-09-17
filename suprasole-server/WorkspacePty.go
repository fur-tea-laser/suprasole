package main

type _WorkspacePty_ struct {
	Id                                  uint32
	PtyResizer                          *_PtyResizer_
	Visibility__Client_connected__state _Visibility__Client_connected_
	State_state                         _State_WorkspacePty_
}

type _Visibility__Client_connected_ uint8

const (
	DISCONNECTED_UNKNOWN___Visibility__Client_connected _Visibility__Client_connected_ = 0
	VISIBLE___Visibility__Client_connected              _Visibility__Client_connected_ = 1
	NOT_VISIBLE___Visibility__Client_connected          _Visibility__Client_connected_ = 2
)

type _State_WorkspacePty_ interface {
	HandleWriteInput_Ingress(inputOrder_PtyWriter _InputOrder_PtyWriter_)
	HandleDisconnect_Coordinator()
	Update_ManifestBulletin(bulletin_result *_Bulletin_WorkspacePty_)
}

type _State_Spawning__WorkspacePty_ struct {
	CancellationStatus _CancellationStatus___State_Spawning__WorkspacePty_
}

type _CancellationStatus___State_Spawning__WorkspacePty_ uint8

const (
	NOT_CANCELED____CancellationStatus___State_Spawning__WorkspacePty _CancellationStatus___State_Spawning__WorkspacePty_ = 0
	CANCELED____CancellationStatus___State_Spawning__WorkspacePty     _CancellationStatus___State_Spawning__WorkspacePty_ = 1
)

type _State_Active__WorkspacePty_ struct {
	PtyProxy *_PtyProxy_
}

type _State_Exited__WorkspacePty_ struct {
	PtyProxy    *_PtyProxy_
	ExitOutcome _ExitOutcome_PtyProxy_
}

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
