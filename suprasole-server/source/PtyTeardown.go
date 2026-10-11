package main

import (
	_ERRORS "errors"
	_FMT "fmt"
	_OS "os"
	_STRCONV "strconv"
	_STRINGS "strings"
	_SYSCALL "syscall"
	_TIME "time"
)

const (
	TIMEOUT_DRAIN__PtyProxy              = 100 * _TIME.Millisecond
	TIMEOUT_UNRESPONSIVE__PtyProcess     = 1 * _TIME.Second
	TIMEOUT_UNINTERRUPTIBLE__PtyProcess  = 1 * _TIME.Second
	SIZE_QUEUE_BUFFER__Order_PtyTeardown = 3
	NUMBER_FIELD_STATE__Stat__Proc       = 3
	NUMBER_FIELD_EXIT_CODE__Stat__Proc   = 52
	PREFIX_LINE_TRACER_PID__Status__Proc = "TracerPid:"
	PATH_DIRECTORY_TASK__Self__Proc      = "/proc/self/task/"
	FORMAT_PATH_STAT__Proc               = "/proc/%d/stat"
	FORMAT_PATH_STATUS__Proc             = "/proc/%d/status"
)

type _PtyTeardown_ struct {
	OnExitOutcome__                    func(id_WorkspacePty uint32, exitOutcome _ExitOutcome_PtyProxy_)
	Id_WorkspacePty                    uint32
	QueueChannel__Order_PtyTeardown    chan _Order_PtyTeardown_
	Channel_FinalState                 <-chan struct{}
	PtyProcess                         *_PtyProcess_
	FileDescriptor_Master__Pty__shared *_OS.File
	Timer_Drain                        *_TIME.Timer
	Timer_Unresponsive                 *_TIME.Timer
	Timer_Uninterruptible              *_TIME.Timer
	Status_ReaderSide                  _Status_ReaderSide__PtyTeardown_
	Status_ProcessSide                 _Status_ProcessSide__PtyTeardown_
	Status_Master                      _Status_Master__PtyTeardown_
	Status_Cancellation                _Status_Cancellation__PtyTeardown_
	ExitSignal_PtyReader               error
	Report_Observation_latest          _TeardownReport_PtyProcess_
}

type _Status_ReaderSide__PtyTeardown_ uint8

const (
	PENDING__Status_ReaderSide__PtyTeardown  _Status_ReaderSide__PtyTeardown_ = 0
	RUNNING__Status_ReaderSide__PtyTeardown  _Status_ReaderSide__PtyTeardown_ = 1
	FINISHED__Status_ReaderSide__PtyTeardown _Status_ReaderSide__PtyTeardown_ = 2
)

type _Status_ProcessSide__PtyTeardown_ uint8

const (
	UNESCALATED__Status_ProcessSide__PtyTeardown     _Status_ProcessSide__PtyTeardown_ = 0
	AWAITING_HANGUP__Status_ProcessSide__PtyTeardown _Status_ProcessSide__PtyTeardown_ = 1
	AWAITING_KILL__Status_ProcessSide__PtyTeardown   _Status_ProcessSide__PtyTeardown_ = 2
	AWAITING_FINAL__Status_ProcessSide__PtyTeardown  _Status_ProcessSide__PtyTeardown_ = 3
	FINAL__Status_ProcessSide__PtyTeardown           _Status_ProcessSide__PtyTeardown_ = 4
	GIVEN_UP__Status_ProcessSide__PtyTeardown        _Status_ProcessSide__PtyTeardown_ = 5
)

type _Status_Master__PtyTeardown_ uint8

const (
	OPEN__Status_Master__PtyTeardown   _Status_Master__PtyTeardown_ = 0
	CLOSED__Status_Master__PtyTeardown _Status_Master__PtyTeardown_ = 1
)

type _Status_Cancellation__PtyTeardown_ uint8

const (
	NOT_CANCELED__Status_Cancellation__PtyTeardown _Status_Cancellation__PtyTeardown_ = 0
	CANCELED__Status_Cancellation__PtyTeardown     _Status_Cancellation__PtyTeardown_ = 1
)

type _Directive__PtyTeardown_ uint8

const (
	CONTINUE_WORKER__Directive__PtyTeardown _Directive__PtyTeardown_ = 0
	EXIT_WORKER__Directive__PtyTeardown     _Directive__PtyTeardown_ = 1
)

type _Order_PtyTeardown_ interface {
	Execute(PtyTeardown_forwarded *_PtyTeardown_)
}

type _Terminate__Order_PtyTeardown_ struct{}

type _Cancel__Order_PtyTeardown_ struct{}

type _ReaderStarted__Order_PtyTeardown_ struct{}

type _ReaderFinished__Order_PtyTeardown_ struct {
	ExitSignal error
}

type _Status_Observation__PtyProcess_ uint8

const (
	UNRESPONSIVE__Status_Observation__PtyProcess _Status_Observation__PtyProcess_ = 0
	TERMINATED__Status_Observation__PtyProcess   _Status_Observation__PtyProcess_ = 1
)

type _Status_Tracer__Proc_ uint8

const (
	NONE__Status_Tracer__Proc    _Status_Tracer__Proc_ = 0
	OWN__Status_Tracer__Proc     _Status_Tracer__Proc_ = 1
	FOREIGN__Status_Tracer__Proc _Status_Tracer__Proc_ = 2
)

type _Snapshot__Proc_ struct {
	StateLetter   string
	ExitCode      int
	TracerPid     int
	Status_Tracer _Status_Tracer__Proc_
}

func New_PtyTeardown(
	id_WorkspacePty uint32,
	onExitOutcome__ func(id_WorkspacePty uint32, exitOutcome _ExitOutcome_PtyProxy_),
	ptyProcess *_PtyProcess_,
	fileDescriptor_master__Pty__shared *_OS.File,
) *_PtyTeardown_ {
	ptyTeardown_result := &_PtyTeardown_{
		OnExitOutcome__:                    onExitOutcome__,
		Id_WorkspacePty:                    id_WorkspacePty,
		QueueChannel__Order_PtyTeardown:    make(chan _Order_PtyTeardown_, SIZE_QUEUE_BUFFER__Order_PtyTeardown),
		Channel_FinalState:                 ptyProcess.Channel_FinalState,
		PtyProcess:                         ptyProcess,
		FileDescriptor_Master__Pty__shared: fileDescriptor_master__Pty__shared,
		Timer_Drain:                        _TIME.NewTimer(TIMEOUT_DRAIN__PtyProxy),
		Timer_Unresponsive:                 _TIME.NewTimer(TIMEOUT_UNRESPONSIVE__PtyProcess),
		Timer_Uninterruptible:              _TIME.NewTimer(TIMEOUT_UNINTERRUPTIBLE__PtyProcess),
		Status_ReaderSide:                  PENDING__Status_ReaderSide__PtyTeardown,
		Status_ProcessSide:                 UNESCALATED__Status_ProcessSide__PtyTeardown,
		Status_Master:                      OPEN__Status_Master__PtyTeardown,
		Status_Cancellation:                NOT_CANCELED__Status_Cancellation__PtyTeardown,
		ExitSignal_PtyReader:               nil,
		Report_Observation_latest:          nil,
	}
	ptyTeardown_result.Timer_Drain.Stop()
	ptyTeardown_result.Timer_Unresponsive.Stop()
	ptyTeardown_result.Timer_Uninterruptible.Stop()
	return ptyTeardown_result
}

// HandleTerminate never blocks: the coordinator may still see the PTY Active after the outcome was emitted,
// when RunWorker has already returned. A full queue always holds a Terminate already — the other orders are
// at most two (ReaderStarted, ReaderFinished; Cancel excludes both) against SIZE_QUEUE_BUFFER__Order_PtyTeardown
// = 3 — and Terminate is idempotent, so dropping this one loses nothing.
func (This *_PtyTeardown_) HandleTerminate() {
	select {
	case This.QueueChannel__Order_PtyTeardown <- &_Terminate__Order_PtyTeardown_{}:
	default:
	}
}

func (This *_PtyTeardown_) HandleCancel() {
	This.QueueChannel__Order_PtyTeardown <- &_Cancel__Order_PtyTeardown_{}
}

func (This *_PtyTeardown_) HandleReaderStarted() {
	This.QueueChannel__Order_PtyTeardown <- &_ReaderStarted__Order_PtyTeardown_{}
}

func (This *_PtyTeardown_) HandleReaderFinished(
	exitSignal_PtyReader error,
) {
	This.QueueChannel__Order_PtyTeardown <- &_ReaderFinished__Order_PtyTeardown_{
		ExitSignal: exitSignal_PtyReader,
	}
}

func (this *_Alive__State_PtyProcess_) Observe_Unresponsive() (_Status_Observation__PtyProcess_, _TeardownReport_PtyProcess_) {
	return UNRESPONSIVE__Status_Observation__PtyProcess, observe__Pid_Unresponsive(this.Pid)
}

func (this *_Zombie_Withheld__State_PtyProcess_) Observe_Unresponsive() (_Status_Observation__PtyProcess_, _TeardownReport_PtyProcess_) {
	return UNRESPONSIVE__Status_Observation__PtyProcess, observe__Pid_Unresponsive(this.Pid)
}

func (this *_Zombie__State_PtyProcess_) Observe_Unresponsive() (_Status_Observation__PtyProcess_, _TeardownReport_PtyProcess_) {
	return TERMINATED__Status_Observation__PtyProcess, nil
}

func (this *_Reaped__State_PtyProcess_) Observe_Unresponsive() (_Status_Observation__PtyProcess_, _TeardownReport_PtyProcess_) {
	return TERMINATED__Status_Observation__PtyProcess, nil
}

func (this *_SystemError__State_PtyProcess_) Observe_Unresponsive() (_Status_Observation__PtyProcess_, _TeardownReport_PtyProcess_) {
	return TERMINATED__Status_Observation__PtyProcess, nil
}

func observe__Pid_Unresponsive(
	pid_PtyProcess int,
) _TeardownReport_PtyProcess_ {
	snapshot_Proc, error_read__Proc__maybe := read__Snapshot_Proc(pid_PtyProcess)
	if error_read__Proc__maybe != nil {
		return _SystemError__TeardownReport_PtyProcess_{
			SystemError_Wait: error_read__Proc__maybe,
		}
	}
	return classify__Snapshot_Proc(snapshot_Proc)
}

func classify__Snapshot_Proc(
	snapshot_Proc _Snapshot__Proc_,
) _TeardownReport_PtyProcess_ {
	if "D" == snapshot_Proc.StateLetter {
		return _Unresponsive_Uninterruptible__TeardownReport_PtyProcess_{}
	} else if FOREIGN__Status_Tracer__Proc == snapshot_Proc.Status_Tracer {
		return _Unresponsive_Debugged__TeardownReport_PtyProcess_{
			TracerPid: snapshot_Proc.TracerPid,
		}
	} else if "T" == snapshot_Proc.StateLetter || "t" == snapshot_Proc.StateLetter {
		return _Unresponsive_Stopped__TeardownReport_PtyProcess_{
			StopSignal_Teardown: _SYSCALL.Signal(snapshot_Proc.ExitCode),
		}
	} else if "R" == snapshot_Proc.StateLetter || "S" == snapshot_Proc.StateLetter || "Z" == snapshot_Proc.StateLetter {
		return _Unresponsive_Rogue__TeardownReport_PtyProcess_{}
	} else {
		return _SystemError__TeardownReport_PtyProcess_{
			SystemError_Wait: _FMT.Errorf(
				"unresolved process exit state: unrecognized process state %q",
				snapshot_Proc.StateLetter,
			),
		}
	}
}

func read__Snapshot_Proc(
	pid_PtyProcess int,
) (_Snapshot__Proc_, error) {
	statBytes, error_read__Stat__maybe := _OS.ReadFile(
		_FMT.Sprintf(
			FORMAT_PATH_STAT__Proc,
			pid_PtyProcess,
		),
	)
	if error_read__Stat__maybe != nil {
		return _Snapshot__Proc_{}, error_read__Stat__maybe
	}
	// proc(5) numbers the fields from 1. The command (field 2) may itself contain spaces and ")", so the fields are
	// split after its last ")", where field 3, the state, begins.
	stat := string(statBytes)
	index_commandEnd := _STRINGS.LastIndexByte(
		stat,
		')',
	)
	fields_fromState := _STRINGS.Fields(stat[index_commandEnd+1:])
	if -1 == index_commandEnd || len(fields_fromState) <= NUMBER_FIELD_EXIT_CODE__Stat__Proc-NUMBER_FIELD_STATE__Stat__Proc {
		return _Snapshot__Proc_{}, _FMT.Errorf(
			"unresolved process exit state: malformed /proc/%d/stat",
			pid_PtyProcess,
		)
	}
	stateLetter := fields_fromState[0]
	exitCode, error_parse__ExitCode__maybe := _STRCONV.Atoi(fields_fromState[NUMBER_FIELD_EXIT_CODE__Stat__Proc-NUMBER_FIELD_STATE__Stat__Proc])
	if error_parse__ExitCode__maybe != nil {
		return _Snapshot__Proc_{}, error_parse__ExitCode__maybe
	}
	statusBytes, error_read__Status__maybe := _OS.ReadFile(
		_FMT.Sprintf(
			FORMAT_PATH_STATUS__Proc,
			pid_PtyProcess,
		),
	)
	if error_read__Status__maybe != nil {
		return _Snapshot__Proc_{}, error_read__Status__maybe
	}
	tracerPid, error_parse__TracerPid__maybe := parse__TracerPid_Status(string(statusBytes))
	if error_parse__TracerPid__maybe != nil {
		return _Snapshot__Proc_{}, error_parse__TracerPid__maybe
	}
	snapshot_Proc_result := _Snapshot__Proc_{
		StateLetter:   stateLetter,
		ExitCode:      exitCode,
		TracerPid:     tracerPid,
		Status_Tracer: resolve__Status_Tracer(tracerPid),
	}
	return snapshot_Proc_result, nil
}

func parse__TracerPid_Status(
	status string,
) (int, error) {
	for _, line_Status := range _STRINGS.Split(status, "\n") {
		text_TracerPid, isTracerPid := _STRINGS.CutPrefix(
			line_Status,
			PREFIX_LINE_TRACER_PID__Status__Proc,
		)
		if isTracerPid {
			return _STRCONV.Atoi(_STRINGS.TrimSpace(text_TracerPid))
		}
	}
	return 0, _ERRORS.New("unresolved process exit state: /proc status lacks TracerPid")
}

// TracerPid names the tracer *thread* — under PTRACE_TRACEME, the OS thread that forked, which in Go need not be
// the main thread (issue §6-AI) — so self-tracing is recognized by membership in suprasole's own thread group.
func resolve__Status_Tracer(
	tracerPid int,
) _Status_Tracer__Proc_ {
	if 0 == tracerPid {
		return NONE__Status_Tracer__Proc
	}
	_, error_stat__Task__maybe := _OS.Stat(PATH_DIRECTORY_TASK__Self__Proc + _STRCONV.Itoa(tracerPid))
	if nil == error_stat__Task__maybe {
		return OWN__Status_Tracer__Proc
	} else {
		return FOREIGN__Status_Tracer__Proc
	}
}

type _TeardownReport_PtyProcess_ interface {
	Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_
}

type _SystemError__TeardownReport_PtyProcess_ struct {
	SystemError_Wait error
}

func (this _SystemError__TeardownReport_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return _SystemError_Process__ExitOutcome_PtyProxy_{
		SystemError_PtyProcess: this.SystemError_Wait,
	}
}

type _Unresponsive_Uninterruptible__TeardownReport_PtyProcess_ struct{}

func (_Unresponsive_Uninterruptible__TeardownReport_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return _SystemError_Process__ExitOutcome_PtyProxy_{
		SystemError_PtyProcess: _ERRORS.New("unresolved process exit state: process unkillable"),
	}
}

type _Unresponsive_Stopped__TeardownReport_PtyProcess_ struct {
	StopSignal_Teardown _SYSCALL.Signal
}

func (this _Unresponsive_Stopped__TeardownReport_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return _Stopped__ExitOutcome_PtyProxy_{
		StopSignal_PtyProcess: int(this.StopSignal_Teardown),
	}
}

type _Unresponsive_Rogue__TeardownReport_PtyProcess_ struct{}

func (_Unresponsive_Rogue__TeardownReport_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return _SystemError_Process__ExitOutcome_PtyProxy_{
		SystemError_PtyProcess: _ERRORS.New("unresolved process exit state: process unresponsive to hangup"),
	}
}

type _Unresponsive_Debugged__TeardownReport_PtyProcess_ struct {
	TracerPid int
}

func (this _Unresponsive_Debugged__TeardownReport_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return _SystemError_Process__ExitOutcome_PtyProxy_{
		SystemError_PtyProcess: _FMT.Errorf(
			"unresolved process exit state: process held by debugger (TracerPid %d)",
			this.TracerPid,
		),
	}
}

func (This *_WorkspaceController_) HandleExitOutcome_Pty(
	id_WorkspacePty uint32,
	exitOutcome_result _ExitOutcome_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: id_WorkspacePty,
		ExitOutcome_PtyProxy:   exitOutcome_result,
	}
}
