//go:build tests_overlay

package main

import (
	_BYTES "bytes"
	_ERRORS "errors"
	_FMT "fmt"
	_OS "os"
	_PATHFILEPATH "path/filepath"
	_RUNTIME "runtime"
	_STRCONV "strconv"
	_STRINGS "strings"
	_SYNC "sync"
	_SYSCALL "syscall"
	_TESTING "testing"
	_TIME "time"
)

const (
	DEADLINE__PtyTeardown_test         = 10 * _TIME.Second
	GRACE_NO_OUTCOME__PtyTeardown_test = 300 * _TIME.Millisecond
	ID_WORKSPACE_PTY__PtyTeardown_test = 7
)

// _Harness__PtyTeardown_test_ stands in for the coordinator: it spawns through Spawn_PtyProxy (which starts
// _PtyProcess_ and _PtyTeardown_), collects live output, and receives the outcome _PtyTeardown_ emits.
type _Harness__PtyTeardown_test_ struct {
	PtyProxy            *_PtyProxy_
	Channel_ExitOutcome chan _ExitOutcome_PtyProxy_
	Mutex               _SYNC.Mutex
	OutputData          []byte
	Status_Ended        _Status_Ended__Harness_test_
}

type _Status_Ended__Harness_test_ uint8

const (
	PENDING__Status_Ended__Harness_test   _Status_Ended__Harness_test_ = 0
	ACTIVATED__Status_Ended__Harness_test _Status_Ended__Harness_test_ = 1
	CANCELED__Status_Ended__Harness_test  _Status_Ended__Harness_test_ = 2
)

// §3E, §6-W: a natural exit — the PTY process's termination and the reader's EIO, in either order — emits one
// outcome from Reaped, after the dispatcher's final flush, and the master ends closed.
func TestPtyTeardown_NaturalExitFlushesOutputThenReportsTermination(t *_TESTING.T) {
	harness := spawn__PtyTeardown_test(
		t,
		"/bin/sh",
	)
	harness.Activate()
	harness.Write("printf 'teardown-%s\\n' marker; exit 3\n")
	harness.ExpectOutcome(
		t,
		_Failure__ExitOutcome_PtyProxy_{
			ExitCode_PtyProcess: 3,
		},
	)
	if !_BYTES.Contains(harness.Output(), []byte("teardown-marker")) {
		t.Fatalf(
			"outcome emitted before the final flush: output %q lacks the marker",
			harness.Output(),
		)
	}
	harness.ExpectMasterClosed(t)
}

// §3E: once RunWorker has returned, a Terminate crossing the outcome in flight must not block the coordinator.
// The queue fills to SIZE_QUEUE_BUFFER__Order_PtyTeardown, and later orders are dropped.
func TestPtyTeardown_TerminateAfterOutcomeNeverBlocks(t *_TESTING.T) {
	harness := spawn__PtyTeardown_test(
		t,
		"/bin/sh",
	)
	harness.Activate()
	harness.Write("exit 0\n")
	harness.ExpectOutcome(
		t,
		_Success__ExitOutcome_PtyProxy_{},
	)
	expectReturned__PtyTeardown_test(
		t,
		harness.PtyProxy.PtyTeardown,
	)
	for range 2 * SIZE_QUEUE_BUFFER__Order_PtyTeardown {
		harness.PtyProxy.PtyTeardown.HandleTerminate()
	}
}

// §3E, §8.1, §9D: a process that keeps the slave open after the PTY process exits withholds EIO. The drain timer
// unplugs after TIMEOUT_DRAIN__PtyProxy, the outcome still comes from Reaped, and the lingering process survives
// (unplug only, no group kill).
func TestPtyTeardown_LingeringSlaveHolderIsUnpluggedAfterDrain(t *_TESTING.T) {
	path_lingeringPid := _PATHFILEPATH.Join(
		t.TempDir(),
		"lingering.pid",
	)
	harness := spawn__PtyTeardown_test(
		t,
		"/bin/sh",
	)
	harness.Activate()
	harness.Write("(trap '' HUP; exec sleep 5) & echo $! > " + path_lingeringPid + "; exit 5\n")
	expectFinal__PtyTeardown_test(
		t,
		harness.PtyProxy.PtyProcess,
	)
	instant_final := _TIME.Now()
	harness.ExpectOutcome(
		t,
		_Failure__ExitOutcome_PtyProxy_{
			ExitCode_PtyProcess: 5,
		},
	)
	duration_drain := _TIME.Since(instant_final)
	if duration_drain < TIMEOUT_DRAIN__PtyProxy/2 {
		t.Fatalf(
			"outcome %v after the process side ended, want the drain (%v): EIO should have been withheld",
			duration_drain,
			TIMEOUT_DRAIN__PtyProxy,
		)
	}
	harness.ExpectMasterClosed(t)
	lingeringPidText, error_read__LingeringPid__maybe := _OS.ReadFile(path_lingeringPid)
	if error_read__LingeringPid__maybe != nil {
		t.Fatalf(
			"read lingering pid: %v",
			error_read__LingeringPid__maybe,
		)
	}
	lingeringPid, _ := _STRCONV.Atoi(_STRINGS.TrimSpace(string(lingeringPidText)))
	stateLetter_lingering := readStateLetter__Proc_test(lingeringPid)
	if "gone" == stateLetter_lingering || "Z" == stateLetter_lingering {
		t.Fatalf(
			"lingering process %d is %s after the unplug, want it alive (unplug only, issue §8.1)",
			lingeringPid,
			stateLetter_lingering,
		)
	}
}

// §3D, §3E: Terminate sends SIGHUP + SIGCONT through the pidfd; an idle interactive shell dies of the hangup with
// no escalation.
func TestPtyTeardown_TerminateHangsUpIdleShell(t *_TESTING.T) {
	harness := spawn__PtyTeardown_test(
		t,
		"/bin/sh",
	)
	harness.Activate()
	instant_terminate := _TIME.Now()
	harness.PtyProxy.PtyTeardown.HandleTerminate()
	harness.ExpectOutcome(
		t,
		_Killed__ExitOutcome_PtyProxy_{
			ExitSignal_PtyProcess: int(_SYSCALL.SIGHUP),
		},
	)
	if _TIME.Since(instant_terminate) >= TIMEOUT_UNRESPONSIVE__PtyProcess {
		t.Fatalf(
			"hangup took %v, want it before the TIMEOUT_UNRESPONSIVE__PtyProcess escalation",
			_TIME.Since(instant_terminate),
		)
	}
	harness.ExpectMasterClosed(t)
}

// §3E, §5B.6: a PTY process ignoring SIGHUP outlives the hangup. After TIMEOUT_UNRESPONSIVE__PtyProcess, the
// teardown unplugs, observes, and kills the group, so the outcome is the SIGKILL death. Repeated Terminates are
// idempotent. The shell execs sleep: a shell still reading the terminal would end on the unplug itself (dash exits
// 2 on the failed read), which is a true outcome too, but not the escalation under test.
func TestPtyTeardown_TerminateEscalatesToGroupKillWhenHangupIgnored(t *_TESTING.T) {
	harness := spawn__PtyTeardown_test(
		t,
		"/bin/sh",
	)
	harness.Activate()
	harness.Write("trap '' HUP TERM; printf 'trap-%s\\n' set; exec sleep 30\n")
	harness.AwaitOutput(
		t,
		"trap-set",
	)
	instant_terminate := _TIME.Now()
	for range 5 {
		harness.PtyProxy.PtyTeardown.HandleTerminate()
	}
	harness.ExpectOutcome(
		t,
		_Killed__ExitOutcome_PtyProxy_{
			ExitSignal_PtyProcess: int(_SYSCALL.SIGKILL),
		},
	)
	if _TIME.Since(instant_terminate) < TIMEOUT_UNRESPONSIVE__PtyProcess {
		t.Fatalf(
			"escalated after %v, want TIMEOUT_UNRESPONSIVE__PtyProcess (%v) first",
			_TIME.Since(instant_terminate),
			TIMEOUT_UNRESPONSIVE__PtyProcess,
		)
	}
}

// §3E, §9E (rogue): the PTY process closes every slave descriptor and keeps running, ignoring SIGHUP. EIO ends
// the reader first; the teardown closes the master, waits TIMEOUT_UNRESPONSIVE__PtyProcess, then kills the group.
func TestPtyTeardown_ReaderFinishedFirstEscalatesRogueProcess(t *_TESTING.T) {
	harness := spawn__PtyTeardown_test(
		t,
		writeScript__PtyTeardown_test(
			t,
			"trap '' HUP\nexec sleep 30 </dev/null >/dev/null 2>&1\n",
		),
	)
	instant_activate := _TIME.Now()
	harness.Activate()
	harness.ExpectOutcome(
		t,
		_Killed__ExitOutcome_PtyProxy_{
			ExitSignal_PtyProcess: int(_SYSCALL.SIGKILL),
		},
	)
	if _TIME.Since(instant_activate) < TIMEOUT_UNRESPONSIVE__PtyProcess {
		t.Fatalf(
			"escalated after %v, want TIMEOUT_UNRESPONSIVE__PtyProcess (%v) first",
			_TIME.Since(instant_activate),
			TIMEOUT_UNRESPONSIVE__PtyProcess,
		)
	}
}

// §3E: a reader error other than EIO / os.ErrClosed is the outcome, once the process side ends too (§3F). Here
// the reader side is driven by hand; the master close it triggers hangs up the shell.
func TestPtyTeardown_ReaderErrorIsDescriptorSystemError(t *_TESTING.T) {
	harness := spawn__PtyTeardown_test(
		t,
		"/bin/sh",
	)
	harness.Status_Ended = ACTIVATED__Status_Ended__Harness_test
	error_injected := _ERRORS.New("injected read failure")
	harness.PtyProxy.PtyTeardown.HandleReaderStarted()
	harness.PtyProxy.PtyTeardown.HandleReaderFinished(error_injected)
	exitOutcome := harness.ReceiveOutcome(t)
	exitOutcome_descriptor, isDescriptor := exitOutcome.(_SystemError_Descriptor__ExitOutcome_PtyProxy_)
	if !isDescriptor || exitOutcome_descriptor.SystemError_PtyDescriptor != error_injected {
		t.Fatalf(
			"outcome = %#v, want _SystemError_Descriptor__ExitOutcome_PtyProxy_{%v}",
			exitOutcome,
			error_injected,
		)
	}
	expectReaped__PtyProcess_test(
		t,
		harness.PtyProxy.PtyProcess,
		_Signaled__Termination_PtyProcess_{
			Signal_PtyProcess: int(_SYSCALL.SIGHUP),
			Status_CoreDump:   NOT_DUMPED__Status_CoreDump__PtyProcess,
		},
	)
}

// The drain counts from the reader's start, not only the process's end: a process that exits before the
// coordinator activates the PTY keeps its output, which a master closed before the reader ran would discard.
func TestPtyTeardown_DrainWaitsForReaderStart(t *_TESTING.T) {
	harness := spawn__PtyTeardown_test(
		t,
		"/bin/sh",
	)
	harness.Write("printf 'teardown-%s\\n' early; exit 0\n")
	expectFinal__PtyTeardown_test(
		t,
		harness.PtyProxy.PtyProcess,
	)
	_TIME.Sleep(3 * TIMEOUT_DRAIN__PtyProxy)
	harness.Activate()
	harness.ExpectOutcome(
		t,
		_Success__ExitOutcome_PtyProxy_{},
	)
	if !_BYTES.Contains(harness.Output(), []byte("teardown-early")) {
		t.Fatalf(
			"output produced before activation was lost: %q",
			harness.Output(),
		)
	}
}

// §3E: Cancel (no reader ever started) unplugs and hangs up, then ends without an outcome.
func TestPtyTeardown_CancelHangsUpWithoutOutcome(t *_TESTING.T) {
	harness := spawn__PtyTeardown_test(
		t,
		"/bin/sh",
	)
	harness.Cancel()
	expectReaped__PtyProcess_test(
		t,
		harness.PtyProxy.PtyProcess,
		_Signaled__Termination_PtyProcess_{
			Signal_PtyProcess: int(_SYSCALL.SIGHUP),
			Status_CoreDump:   NOT_DUMPED__Status_CoreDump__PtyProcess,
		},
	)
	harness.ExpectMasterClosed(t)
	expectReturned__PtyTeardown_test(
		t,
		harness.PtyProxy.PtyTeardown,
	)
	harness.ExpectNoOutcome(t)
}

// §3E: a cancelled spawn whose process ignores the hangup goes through the same escalation — group SIGKILL after
// TIMEOUT_UNRESPONSIVE__PtyProcess — and still ends without an outcome.
func TestPtyTeardown_CancelEscalatesWhenHangupIgnored(t *_TESTING.T) {
	harness := spawn__PtyTeardown_test(
		t,
		writeScript__PtyTeardown_test(
			t,
			"trap '' HUP\nexec sleep 30\n",
		),
	)
	pid_PtyProcess := readPid__PtyProcess_test(
		t,
		harness.PtyProxy.PtyProcess,
	)
	eventually__PtyProcess_test(
		t,
		"script exec'd sleep with SIGHUP ignored",
		func() bool {
			commBytes, _ := _OS.ReadFile(_FMT.Sprintf("/proc/%d/comm", pid_PtyProcess))
			return "sleep" == _STRINGS.TrimSpace(string(commBytes))
		},
	)
	instant_cancel := _TIME.Now()
	harness.Cancel()
	expectReaped__PtyProcess_test(
		t,
		harness.PtyProxy.PtyProcess,
		_Signaled__Termination_PtyProcess_{
			Signal_PtyProcess: int(_SYSCALL.SIGKILL),
			Status_CoreDump:   NOT_DUMPED__Status_CoreDump__PtyProcess,
		},
	)
	if _TIME.Since(instant_cancel) < TIMEOUT_UNRESPONSIVE__PtyProcess {
		t.Fatalf(
			"killed after %v, want TIMEOUT_UNRESPONSIVE__PtyProcess (%v) first",
			_TIME.Since(instant_cancel),
			TIMEOUT_UNRESPONSIVE__PtyProcess,
		)
	}
	expectReturned__PtyTeardown_test(
		t,
		harness.PtyProxy.PtyTeardown,
	)
	harness.ExpectNoOutcome(t)
}

// §3E, §3F, §5B.9: a foreign debugger holds the PTY process. The hangup leaves it in a signal-delivery stop, and
// the group SIGKILL leaves a zombie only the debugger can see (Zombie_Withheld). The teardown gives up after
// TIMEOUT_UNINTERRUPTIBLE__PtyProcess and reports _Unresponsive_Debugged_{TracerPid}. _PtyProcess_ still reaches
// Reaped once the debugger collects its report.
func TestPtyTeardown_GivesUpOnDebuggerHeldProcess(t *_TESTING.T) {
	harness := spawn__PtyTeardown_test(
		t,
		writeScript__PtyTeardown_test(
			t,
			"exec sleep 30\n",
		),
	)
	pid_PtyProcess := readPid__PtyProcess_test(
		t,
		harness.PtyProxy.PtyProcess,
	)
	foreignDebugger := start__ForeignDebugger_test(
		t,
		pid_PtyProcess,
	)
	snapshot_Proc, error_read__Proc__maybe := read__Snapshot_Proc(pid_PtyProcess)
	if error_read__Proc__maybe != nil || FOREIGN__Status_Tracer__Proc != snapshot_Proc.Status_Tracer {
		t.Fatalf(
			"after attach: snapshot %#v, error %v; want a foreign tracer",
			snapshot_Proc,
			error_read__Proc__maybe,
		)
	}
	harness.Activate()
	harness.PtyProxy.PtyTeardown.HandleTerminate()
	harness.ExpectOutcome(
		t,
		_Unresponsive_Debugged__TeardownReport_PtyProcess_{
			TracerPid: snapshot_Proc.TracerPid,
		}.Resolve__ExitOutcome_PtyProxy(),
	)
	harness.PtyProxy.PtyProcess.Mutex.Lock()
	State__PtyProcess__current := harness.PtyProxy.PtyProcess.State_state
	harness.PtyProxy.PtyProcess.Mutex.Unlock()
	if _, isWithheld := State__PtyProcess__current.(*_Zombie_Withheld__State_PtyProcess_); !isWithheld {
		t.Fatalf(
			"state at give-up = %T, want *_Zombie_Withheld__State_PtyProcess_",
			State__PtyProcess__current,
		)
	}
	foreignDebugger.Collect(t)
	expectReaped__PtyProcess_test(
		t,
		harness.PtyProxy.PtyProcess,
		_Signaled__Termination_PtyProcess_{
			Signal_PtyProcess: int(_SYSCALL.SIGKILL),
			Status_CoreDump:   NOT_DUMPED__Status_CoreDump__PtyProcess,
		},
	)
}

// §3F: the /proc observation of real kernel states — sleeping (Rogue), group-stopped (Stopped), trace-stopped
// under suprasole's own thread (Stopped), and held by a foreign debugger (Debugged{TracerPid}).
func TestObserve_Pid_Unresponsive_ClassifiesKernelStates(t *_TESTING.T) {
	pid_sleeping, fileDescriptor_pidfd__Sleeping := forkExecSleep__PtyProcess_test(
		t,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	fileDescriptor_pidfd__PtyProcess__Sleeping := ownPidfd__PtyTeardown_test(
		t,
		fileDescriptor_pidfd__Sleeping,
	)
	eventually__PtyProcess_test(
		t,
		"sleeping",
		func() bool {
			return "S" == readStateLetter__Proc_test(pid_sleeping)
		},
	)
	expectObservation__PtyTeardown_test(
		t,
		pid_sleeping,
		_Unresponsive_Rogue__TeardownReport_PtyProcess_{},
	)
	_ = sendSignal_Pidfd(
		fileDescriptor_pidfd__PtyProcess__Sleeping,
		_SYSCALL.SIGSTOP,
		PIDFD_SIGNAL_THREAD_GROUP__Linux,
	)
	eventually__PtyProcess_test(
		t,
		"group-stopped",
		func() bool {
			return "T" == readStateLetter__Proc_test(pid_sleeping)
		},
	)
	expectObservation__PtyTeardown_test(
		t,
		pid_sleeping,
		_Unresponsive_Stopped__TeardownReport_PtyProcess_{
			StopSignal_Teardown: _SYSCALL.SIGSTOP,
		},
	)
	_RUNTIME.LockOSThread() // the forking thread becomes the tracer
	defer _RUNTIME.UnlockOSThread()
	pid_traced, fileDescriptor_pidfd__Traced := forkExecSleep__PtyProcess_test(
		t,
		&_SYSCALL.SysProcAttr{Setsid: true, Ptrace: true},
	)
	ownPidfd__PtyTeardown_test(
		t,
		fileDescriptor_pidfd__Traced,
	)
	eventually__PtyProcess_test(
		t,
		"trace-stopped at exec",
		func() bool {
			return "t" == readStateLetter__Proc_test(pid_traced)
		},
	)
	expectObservation__PtyTeardown_test(
		t,
		pid_traced,
		_Unresponsive_Stopped__TeardownReport_PtyProcess_{
			StopSignal_Teardown: _SYSCALL.SIGTRAP,
		},
	)
	pid_debugged, fileDescriptor_pidfd__Debugged := forkExecSleep__PtyProcess_test(
		t,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	ownPidfd__PtyTeardown_test(
		t,
		fileDescriptor_pidfd__Debugged,
	)
	start__ForeignDebugger_test(
		t,
		pid_debugged,
	)
	snapshot_Proc, error_read__Proc__maybe := read__Snapshot_Proc(pid_debugged)
	if error_read__Proc__maybe != nil || 0 == snapshot_Proc.TracerPid {
		t.Fatalf(
			"debugged snapshot %#v, error %v; want a TracerPid",
			snapshot_Proc,
			error_read__Proc__maybe,
		)
	}
	expectObservation__PtyTeardown_test(
		t,
		pid_debugged,
		_Unresponsive_Debugged__TeardownReport_PtyProcess_{
			TracerPid: snapshot_Proc.TracerPid,
		},
	)
}

// §3F: the classification of states no harness reaches cheaply. D wins over any tracer; a zombie leader not
// held by a debugger is rogue (its other threads run on); an unrecognized letter is reported, never guessed.
func TestClassify_Snapshot_Proc_CoversUnobservedStates(t *_TESTING.T) {
	cases := []struct {
		Snapshot _Snapshot__Proc_
		Expected _TeardownReport_PtyProcess_
	}{
		{_Snapshot__Proc_{StateLetter: "D"}, _Unresponsive_Uninterruptible__TeardownReport_PtyProcess_{}},
		{_Snapshot__Proc_{StateLetter: "D", TracerPid: 42, Status_Tracer: FOREIGN__Status_Tracer__Proc}, _Unresponsive_Uninterruptible__TeardownReport_PtyProcess_{}},
		{_Snapshot__Proc_{StateLetter: "Z", TracerPid: 42, Status_Tracer: FOREIGN__Status_Tracer__Proc}, _Unresponsive_Debugged__TeardownReport_PtyProcess_{TracerPid: 42}},
		{_Snapshot__Proc_{StateLetter: "t", TracerPid: 42, Status_Tracer: FOREIGN__Status_Tracer__Proc}, _Unresponsive_Debugged__TeardownReport_PtyProcess_{TracerPid: 42}},
		{_Snapshot__Proc_{StateLetter: "t", ExitCode: int(_SYSCALL.SIGHUP), TracerPid: 42, Status_Tracer: OWN__Status_Tracer__Proc}, _Unresponsive_Stopped__TeardownReport_PtyProcess_{StopSignal_Teardown: _SYSCALL.SIGHUP}},
		{_Snapshot__Proc_{StateLetter: "Z"}, _Unresponsive_Rogue__TeardownReport_PtyProcess_{}},
		{_Snapshot__Proc_{StateLetter: "R"}, _Unresponsive_Rogue__TeardownReport_PtyProcess_{}},
	}
	for _, case_current := range cases {
		report_classified := classify__Snapshot_Proc(case_current.Snapshot)
		if case_current.Expected != report_classified {
			t.Fatalf(
				"classify(%#v) = %#v, want %#v",
				case_current.Snapshot,
				report_classified,
				case_current.Expected,
			)
		}
	}
	report_unrecognized := classify__Snapshot_Proc(_Snapshot__Proc_{StateLetter: "I"})
	if _, isSystemError := report_unrecognized.(_SystemError__TeardownReport_PtyProcess_); !isSystemError {
		t.Fatalf(
			"classify(state I) = %#v, want _SystemError__TeardownReport_PtyProcess_",
			report_unrecognized,
		)
	}
}

// §3F: Zombie, Reaped and SystemError terminated in the window — no /proc read, no report; Channel_FinalState is
// about to close.
func TestObserve_Unresponsive_TerminatedStatesReportNothing(t *_TESTING.T) {
	for _, State__PtyProcess__terminated := range []_State_PtyProcess_{
		&_Zombie__State_PtyProcess_{Pid: -1},
		&_Reaped__State_PtyProcess_{},
		&_SystemError__State_PtyProcess_{},
	} {
		status_Observation, report_maybe := State__PtyProcess__terminated.Observe_Unresponsive()
		if TERMINATED__Status_Observation__PtyProcess != status_Observation || report_maybe != nil {
			t.Fatalf(
				"%T.Observe_Unresponsive() = %v, %#v; want TERMINATED, nil",
				State__PtyProcess__terminated,
				status_Observation,
				report_maybe,
			)
		}
	}
}

func spawn__PtyTeardown_test(
	t *_TESTING.T,
	path_ShellBinary string,
) *_Harness__PtyTeardown_test_ {
	t.Helper()
	harness := &_Harness__PtyTeardown_test_{
		Channel_ExitOutcome: make(chan _ExitOutcome_PtyProxy_, 4),
		Status_Ended:        PENDING__Status_Ended__Harness_test,
	}
	ptyProxy_spawned, error_spawn__PtyProxy__maybe := Spawn_PtyProxy(_SpawnApi_PtyProxy_{
		OnOutput_Live__PtyProxy__:              harness.HandleOutput,
		OnOutput_Snapshot__PtyProxy__:          harness.HandleOutput,
		OnOutput_PostSnapshot__PtyProxy__:      harness.HandleOutput,
		OnExitOutcome__PtyProxy__:              harness.HandleExitOutcome,
		Id_WorkspacePty:                        ID_WORKSPACE_PTY__PtyTeardown_test,
		ColumnCount_PtyTerminal:                80,
		RowCount_PtyTerminal:                   24,
		Path_PtyExec:                           path__PtyExec_test(t),
		Path_ShellBinary__PtyCommand:           path_ShellBinary,
		DirectoryPath_PtyCommand:               t.TempDir(),
		EnvironmentVariables_PtyCommand:        []string{"PATH=/usr/bin:/bin", "TERM=xterm", "PS1=$ "},
		Count_ScrollbackLine__PtyTerminal:      100,
		Size_PostSnapshotBuffer__PtyProxy:      1024,
		Size_QueueBuffer__InputOrder_PtyWriter: 8,
	})
	if error_spawn__PtyProxy__maybe != nil {
		t.Fatalf(
			"Spawn_PtyProxy(%s): %v",
			path_ShellBinary,
			error_spawn__PtyProxy__maybe,
		)
	}
	harness.PtyProxy = ptyProxy_spawned
	t.Cleanup(func() {
		killAndAwait__PtyProcess_test(ptyProxy_spawned.PtyProcess)
		if PENDING__Status_Ended__Harness_test == harness.Status_Ended {
			ptyProxy_spawned.PtyTeardown.HandleCancel()
		}
	})
	return harness
}

// Activate does what HandleStatus_SpawnPty__Success__Coordinator does for a visible PTY: live mode, then
// ReaderStarted, then the dispatcher and the reader.
func (This *_Harness__PtyTeardown_test_) Activate() {
	This.Status_Ended = ACTIVATED__Status_Ended__Harness_test
	This.PtyProxy.Mutex.Lock()
	This.PtyProxy.Mode_state = LIVE_RUNNING__Mode_PtyProxy
	This.PtyProxy.Mutex.Unlock()
	This.PtyProxy.PtyTeardown.HandleReaderStarted()
	go This.PtyProxy.PtyReader.PtyDispatcher.RunWorker()
	go This.PtyProxy.PtyReader.RunWorker()
}

// Cancel does what the cancelled branch of HandleStatus_SpawnPty__Success__Coordinator does.
func (This *_Harness__PtyTeardown_test_) Cancel() {
	This.Status_Ended = CANCELED__Status_Ended__Harness_test
	This.PtyProxy.PtyTeardown.HandleCancel()
}

func (This *_Harness__PtyTeardown_test_) Write(inputData string) {
	_, _ = This.PtyProxy.FileDescriptor_Master__Pty__shared.Write([]byte(inputData))
}

func (This *_Harness__PtyTeardown_test_) HandleOutput(
	_ uint32,
	outputData []byte,
) {
	This.Mutex.Lock()
	This.OutputData = append(
		This.OutputData,
		outputData...,
	)
	This.Mutex.Unlock()
}

func (This *_Harness__PtyTeardown_test_) HandleExitOutcome(
	id_WorkspacePty uint32,
	exitOutcome _ExitOutcome_PtyProxy_,
) {
	if ID_WORKSPACE_PTY__PtyTeardown_test != id_WorkspacePty {
		exitOutcome = _SystemError_Process__ExitOutcome_PtyProxy_{
			SystemError_PtyProcess: _FMT.Errorf("outcome for workspace PTY %d", id_WorkspacePty),
		}
	}
	This.Channel_ExitOutcome <- exitOutcome
}

func (This *_Harness__PtyTeardown_test_) Output() []byte {
	This.Mutex.Lock()
	defer This.Mutex.Unlock()
	return _BYTES.Clone(This.OutputData)
}

func (This *_Harness__PtyTeardown_test_) AwaitOutput(
	t *_TESTING.T,
	text_expected string,
) {
	t.Helper()
	eventually__PtyProcess_test(
		t,
		"output "+text_expected,
		func() bool {
			return _BYTES.Contains(This.Output(), []byte(text_expected))
		},
	)
}

func (This *_Harness__PtyTeardown_test_) ReceiveOutcome(t *_TESTING.T) _ExitOutcome_PtyProxy_ {
	t.Helper()
	select {
	case exitOutcome := <-This.Channel_ExitOutcome:
		return exitOutcome
	case <-_TIME.After(DEADLINE__PtyTeardown_test):
		t.Fatalf(
			"no outcome within %v",
			DEADLINE__PtyTeardown_test,
		)
		return nil
	}
}

func (This *_Harness__PtyTeardown_test_) ExpectOutcome(
	t *_TESTING.T,
	exitOutcome_expected _ExitOutcome_PtyProxy_,
) {
	t.Helper()
	exitOutcome := This.ReceiveOutcome(t)
	if !equal__ExitOutcome_test(exitOutcome_expected, exitOutcome) {
		t.Fatalf(
			"outcome = %#v, want %#v",
			exitOutcome,
			exitOutcome_expected,
		)
	}
	select {
	case exitOutcome_second := <-This.Channel_ExitOutcome:
		t.Fatalf(
			"second outcome %#v: the teardown must emit exactly once",
			exitOutcome_second,
		)
	default:
	}
}

func (This *_Harness__PtyTeardown_test_) ExpectNoOutcome(t *_TESTING.T) {
	t.Helper()
	select {
	case exitOutcome := <-This.Channel_ExitOutcome:
		t.Fatalf(
			"outcome %#v emitted, want none",
			exitOutcome,
		)
	case <-_TIME.After(GRACE_NO_OUTCOME__PtyTeardown_test):
	}
}

// ExpectMasterClosed: every path ends with the master closed (issue §9E), which a resize detects — RawConn.Control
// fails on a closed file (only "fails" is relied on, §6-AJ).
func (This *_Harness__PtyTeardown_test_) ExpectMasterClosed(t *_TESTING.T) {
	t.Helper()
	eventually__PtyProcess_test(
		t,
		"master closed",
		func() bool {
			return Resize_PtyDescriptor(This.PtyProxy.FileDescriptor_Master__Pty__shared, 80, 24) != nil
		},
	)
}

// equal__ExitOutcome_test compares outcomes; error-carrying ones by message, since errors are not comparable values.
func equal__ExitOutcome_test(
	exitOutcome_expected _ExitOutcome_PtyProxy_,
	exitOutcome_actual _ExitOutcome_PtyProxy_,
) bool {
	exitOutcome_expected__process, isProcess_expected := exitOutcome_expected.(_SystemError_Process__ExitOutcome_PtyProxy_)
	exitOutcome_actual__process, isProcess_actual := exitOutcome_actual.(_SystemError_Process__ExitOutcome_PtyProxy_)
	if isProcess_expected || isProcess_actual {
		return isProcess_expected && isProcess_actual && exitOutcome_expected__process.SystemError_PtyProcess.Error() == exitOutcome_actual__process.SystemError_PtyProcess.Error()
	}
	return exitOutcome_expected == exitOutcome_actual
}

// expectReturned__PtyTeardown_test: once RunWorker returns nobody drains the queue, so Terminates (dropped when it
// is full) fill it to exactly SIZE_QUEUE_BUFFER__Order_PtyTeardown; while it runs, it consumes them.
func expectReturned__PtyTeardown_test(
	t *_TESTING.T,
	ptyTeardown *_PtyTeardown_,
) {
	t.Helper()
	eventually__PtyProcess_test(
		t,
		"_PtyTeardown_ RunWorker returned",
		func() bool {
			ptyTeardown.HandleTerminate()
			return SIZE_QUEUE_BUFFER__Order_PtyTeardown == len(ptyTeardown.QueueChannel__Order_PtyTeardown)
		},
	)
}

func expectFinal__PtyTeardown_test(
	t *_TESTING.T,
	ptyProcess *_PtyProcess_,
) {
	t.Helper()
	select {
	case <-ptyProcess.Channel_FinalState:
	case <-_TIME.After(DEADLINE__PtyTeardown_test):
		t.Fatalf(
			"Channel_FinalState not closed within %v",
			DEADLINE__PtyTeardown_test,
		)
	}
}

func expectObservation__PtyTeardown_test(
	t *_TESTING.T,
	pid int,
	report_expected _TeardownReport_PtyProcess_,
) {
	t.Helper()
	report_observed := observe__Pid_Unresponsive(pid)
	if report_expected != report_observed {
		t.Fatalf(
			"observe__Pid_Unresponsive(%d) = %#v, want %#v",
			pid,
			report_observed,
			report_expected,
		)
	}
}

func writeScript__PtyTeardown_test(
	t *_TESTING.T,
	body string,
) string {
	t.Helper()
	path_script := _PATHFILEPATH.Join(
		t.TempDir(),
		"pty-program",
	)
	error_write__Script__maybe := _OS.WriteFile(
		path_script,
		[]byte("#!/bin/sh\n"+body),
		0o755,
	)
	if error_write__Script__maybe != nil {
		t.Fatalf(
			"write script: %v",
			error_write__Script__maybe,
		)
	}
	return path_script
}

// ownPidfd__PtyTeardown_test gives a bare forked child's pidfd an owner, and at cleanup kills and collects the
// child through it (abandon__PtyProcess_test), so no signal ever addresses a PID number.
func ownPidfd__PtyTeardown_test(
	t *_TESTING.T,
	fileDescriptor_pidfd int,
) *_OS.File {
	t.Helper()
	fileDescriptor_pidfd__PtyProcess := _OS.NewFile(
		uintptr(fileDescriptor_pidfd),
		NAME_FILE__Pidfd,
	)
	t.Cleanup(func() {
		abandon__PtyProcess_test(fileDescriptor_pidfd__PtyProcess)
	})
	return fileDescriptor_pidfd__PtyProcess
}
