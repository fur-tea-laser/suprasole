//go:build tests_overlay

package main

import (
	_BUFIO "bufio"
	_ERRORS "errors"
	_FMT "fmt"
	_OS "os"
	_EXEC "os/exec"
	_PATHFILEPATH "path/filepath"
	_RUNTIME "runtime"
	_STRCONV "strconv"
	_STRINGS "strings"
	_SYSCALL "syscall"
	_TESTING "testing"
	_TIME "time"
)

const (
	DEADLINE__PtyProcess_test = 5 * _TIME.Second
	INTERVAL__PtyProcess_test = 5 * _TIME.Millisecond

	ENVIRONMENT_VARIABLE_TRACEE_PID__ForeignDebugger_test = "SUPRASOLE_TEST_TRACEE_PID"
	ENVIRONMENT_VARIABLE_TRACEME_EXEC__PtyProcess_test    = "SUPRASOLE_TEST_TRACEME_EXEC"
	ENVIRONMENT_VARIABLE_PATH__PtyExec_test               = "SUPRASOLE_PTY_EXEC"
)

type _ForeignDebugger_test_ struct {
	Stdin interface{ Write([]byte) (int, error) }
	Lines *_BUFIO.Scanner
}

func TestMain(m *_TESTING.M) {
	traceePidText := _OS.Getenv(ENVIRONMENT_VARIABLE_TRACEE_PID__ForeignDebugger_test)
	if traceePidText != "" {
		traceePid, _ := _STRCONV.Atoi(traceePidText)
		run__ForeignDebugger_test(traceePid)
		return
	}
	path_tracemeExec := _OS.Getenv(ENVIRONMENT_VARIABLE_TRACEME_EXEC__PtyProcess_test)
	if path_tracemeExec != "" {
		run__TracemeExec_test(path_tracemeExec)
		return
	}
	_OS.Exit(m.Run())
}

// run__TracemeExec_test runs as the PTY process itself (the test binary re-executed): it makes its parent —
// the test, standing in for suprasole — its tracer with PTRACE_TRACEME, then execs path_exec, which stops at
// the exec trap. This is the one way a trap can reach suprasole: the PTY process asks to be traced (§5B.1).
func run__TracemeExec_test(path_exec string) {
	_RUNTIME.LockOSThread() // PTRACE_TRACEME traces this thread; the exec must run on it
	_, _, errorCode__PTRACE := _SYSCALL.RawSyscall(
		_SYSCALL.SYS_PTRACE,
		_SYSCALL.PTRACE_TRACEME,
		0,
		0,
	)
	if errorCode__PTRACE != 0 {
		_OS.Exit(2)
	}
	_ = _SYSCALL.Exec(
		path_exec,
		[]string{"sleep", "30"},
		_OS.Environ(),
	)
	_OS.Exit(3)
}

// §3C, §4B: a natural exit runs Alive → Zombie → Reaped{Exited}, and the final state resolves the
// exit outcome itself — 0 is Success, anything else Failure with the code.
func TestRunWorker_PtyProcess_ExitReachesReaped(t *_TESTING.T) {
	ptyProcess_succeeded := spawn__PtyProcess_test(
		t,
		[]string{"sh", "-c", "exit 0"},
		nil,
	)
	State__PtyProcess__succeeded := expectReaped__PtyProcess_test(
		t,
		ptyProcess_succeeded,
		_Exited__Termination_PtyProcess_{
			ExitCode_PtyProcess: 0,
		},
	)
	exitOutcome_succeeded := State__PtyProcess__succeeded.Resolve__ExitOutcome_PtyProxy()
	exitOutcome_expected__Succeeded := _Success__ExitOutcome_PtyProxy_{}
	if exitOutcome_expected__Succeeded != exitOutcome_succeeded {
		t.Fatalf(
			"exit 0 resolved to %#v, want %#v",
			exitOutcome_succeeded,
			exitOutcome_expected__Succeeded,
		)
	}
	ptyProcess_failed := spawn__PtyProcess_test(
		t,
		[]string{"sh", "-c", "exit 3"},
		nil,
	)
	State__PtyProcess__failed := expectReaped__PtyProcess_test(
		t,
		ptyProcess_failed,
		_Exited__Termination_PtyProcess_{
			ExitCode_PtyProcess: 3,
		},
	)
	exitOutcome_failed := State__PtyProcess__failed.Resolve__ExitOutcome_PtyProxy()
	exitOutcome_expected := _Failure__ExitOutcome_PtyProxy_{
		ExitCode_PtyProcess: 3,
	}
	if exitOutcome_expected != exitOutcome_failed {
		t.Fatalf(
			"exit 3 resolved to %#v, want %#v",
			exitOutcome_failed,
			exitOutcome_expected,
		)
	}
}

// §3D: Signal goes through the pidfd; a signal death is Reaped{Signaled} and resolves to Killed.
func TestSignal_PtyProcess_SignalDeathReachesReaped(t *_TESTING.T) {
	ptyProcess := spawn__PtyProcess_test(
		t,
		[]string{"sleep", "30"},
		nil,
	)
	ptyProcess.Mutex.Lock()
	error_signal__maybe := ptyProcess.State_state.Signal(
		_SYSCALL.SIGTERM,
		PIDFD_SIGNAL_THREAD_GROUP__Linux,
	)
	ptyProcess.Mutex.Unlock()
	if error_signal__maybe != nil {
		t.Fatalf(
			"Signal(SIGTERM): %v",
			error_signal__maybe,
		)
	}
	State__PtyProcess__reaped := expectReaped__PtyProcess_test(
		t,
		ptyProcess,
		_Signaled__Termination_PtyProcess_{
			Signal_PtyProcess: int(_SYSCALL.SIGTERM),
			Status_CoreDump:   NOT_DUMPED__Status_CoreDump__PtyProcess,
		},
	)
	exitOutcome_killed := State__PtyProcess__reaped.Resolve__ExitOutcome_PtyProxy()
	exitOutcome_expected := _Killed__ExitOutcome_PtyProxy_{
		ExitSignal_PtyProcess: int(_SYSCALL.SIGTERM),
	}
	if exitOutcome_expected != exitOutcome_killed {
		t.Fatalf(
			"SIGTERM death resolved to %#v, want %#v",
			exitOutcome_killed,
			exitOutcome_expected,
		)
	}
}

// §3D: the PROCESS_GROUP scope sets PIDFD_SIGNAL_PROCESS_GROUP — one call reaches every member of the
// PTY process's group, here a SIGHUP-ignoring child sharing it.
func TestSignal_PtyProcess_ProcessGroupScopeReachesGroupMembers(t *_TESTING.T) {
	path_memberPid := _PATHFILEPATH.Join(
		t.TempDir(),
		"member.pid",
	)
	ptyProcess := spawn__PtyProcess_test(
		t,
		[]string{"sh", "-c", `trap "" HUP; sleep 30 & echo $! > ` + path_memberPid + `; wait`},
		nil,
	)
	var memberPid int
	eventually__PtyProcess_test(
		t,
		"group member started",
		func() bool {
			memberPidText, error_read__MemberPid__maybe := _OS.ReadFile(path_memberPid)
			if error_read__MemberPid__maybe != nil {
				return false
			}
			memberPidText__trimmed := _STRINGS.TrimSpace(string(memberPidText))
			memberPid, _ = _STRCONV.Atoi(memberPidText__trimmed)
			return memberPid > 0
		},
	)
	ptyProcess.Mutex.Lock()
	error_signal__maybe := ptyProcess.State_state.Signal(
		_SYSCALL.SIGKILL,
		PIDFD_SIGNAL_PROCESS_GROUP__Linux,
	)
	ptyProcess.Mutex.Unlock()
	if error_signal__maybe != nil {
		t.Fatalf(
			"Signal(SIGKILL, PROCESS_GROUP): %v",
			error_signal__maybe,
		)
	}
	expectReaped__PtyProcess_test(
		t,
		ptyProcess,
		_Signaled__Termination_PtyProcess_{
			Signal_PtyProcess: int(_SYSCALL.SIGKILL),
			Status_CoreDump:   NOT_DUMPED__Status_CoreDump__PtyProcess,
		},
	)
	eventually__PtyProcess_test(
		t,
		"group member killed",
		func() bool {
			stateLetter := readStateLetter__Proc_test(memberPid)
			return "Z" == stateLetter || "gone" == stateLetter
		},
	)
}

// §4B invariant 2: once Reaped there is no handle, so "signal a reaped process" cannot reach the
// kernel at all — both scopes report os.ErrProcessDone.
func TestSignal_PtyProcess_ReapedIsProcessDone(t *_TESTING.T) {
	ptyProcess := spawn__PtyProcess_test(
		t,
		[]string{"sh", "-c", "exit 0"},
		nil,
	)
	expectReaped__PtyProcess_test(
		t,
		ptyProcess,
		_Exited__Termination_PtyProcess_{
			ExitCode_PtyProcess: 0,
		},
	)
	ptyProcess.Mutex.Lock()
	error_signal__maybe := ptyProcess.State_state.Signal(
		_SYSCALL.SIGTERM,
		PIDFD_SIGNAL_THREAD_GROUP__Linux,
	)
	ptyProcess.Mutex.Unlock()
	if !_ERRORS.Is(error_signal__maybe, _OS.ErrProcessDone) {
		t.Fatalf(
			"Signal after Reaped = %v, want os.ErrProcessDone",
			error_signal__maybe,
		)
	}
	ptyProcess.Mutex.Lock()
	error_signal__Group__maybe := ptyProcess.State_state.Signal(
		_SYSCALL.SIGTERM,
		PIDFD_SIGNAL_PROCESS_GROUP__Linux,
	)
	ptyProcess.Mutex.Unlock()
	if !_ERRORS.Is(error_signal__Group__maybe, _OS.ErrProcessDone) {
		t.Fatalf(
			"Signal(PROCESS_GROUP) after Reaped = %v, want os.ErrProcessDone",
			error_signal__Group__maybe,
		)
	}
}

// §3C, §5B.1: a PTY process that makes suprasole its tracer (it calls PTRACE_TRACEME itself, then
// execs sleep — run__TracemeExec_test) produces trap reports. The worker consumes them without collecting, stays
// Alive while the process is trace-stopped, and still ends Reaped once it is killed. (§6-J, §6-R)
func TestRunWorker_PtyProcess_ConsumesTrapsOfSelfTracedProcess(t *_TESTING.T) {
	_RUNTIME.LockOSThread() // the forking thread becomes the tracer
	defer _RUNTIME.UnlockOSThread()
	path_sleep, error_lookPath__maybe := _EXEC.LookPath("sleep")
	if error_lookPath__maybe != nil {
		t.Fatalf(
			"LookPath(sleep): %v",
			error_lookPath__maybe,
		)
	}
	tracemeExecAssignment := ENVIRONMENT_VARIABLE_TRACEME_EXEC__PtyProcess_test + "=" + path_sleep
	ptyProcess := spawn__PtyProcess_test(
		t,
		[]string{_OS.Args[0], "-test.run=^$"},
		[]string{tracemeExecAssignment},
	)
	pid_PtyProcess := readPid__PtyProcess_test(
		t,
		ptyProcess,
	)
	eventually__PtyProcess_test(
		t,
		"trace-stopped at exec",
		func() bool {
			return "t" == readStateLetter__Proc_test(pid_PtyProcess)
		},
	)
	ptyProcess.Mutex.Lock()
	State__PtyProcess__traceStopped := ptyProcess.State_state
	ptyProcess.Mutex.Unlock()
	_, isAlive := State__PtyProcess__traceStopped.(*_Alive__State_PtyProcess_)
	if !isAlive {
		t.Fatalf(
			"state while trace-stopped = %T, want *_Alive__State_PtyProcess_",
			State__PtyProcess__traceStopped,
		)
	}
	ptyProcess.Mutex.Lock()
	error_signal__maybe := ptyProcess.State_state.Signal(
		_SYSCALL.SIGKILL,
		PIDFD_SIGNAL_THREAD_GROUP__Linux,
	)
	ptyProcess.Mutex.Unlock()
	if error_signal__maybe != nil {
		t.Fatalf(
			"Signal(SIGKILL): %v",
			error_signal__maybe,
		)
	}
	expectReaped__PtyProcess_test(
		t,
		ptyProcess,
		_Signaled__Termination_PtyProcess_{
			Signal_PtyProcess: int(_SYSCALL.SIGKILL),
			Status_CoreDump:   NOT_DUMPED__Status_CoreDump__PtyProcess,
		},
	)
}

// §3C, §4B: under a foreign debugger a terminated PTY process is a zombie only its tracer can see.
// The worker detects it deterministically (pidfd readable, peek empty) as Zombie_Withheld, re-peeks
// on INTERVAL_REPEEK__PtyProcess, and reaches Reaped once the debugger collects its report. (§6-V, §6-AE)
func TestRunWorker_PtyProcess_ForeignDebuggerWithholdsThenReleases(t *_TESTING.T) {
	ptyProcess := spawn__PtyProcess_test(
		t,
		[]string{"sleep", "30"},
		nil,
	)
	pid_PtyProcess := readPid__PtyProcess_test(
		t,
		ptyProcess,
	)
	foreignDebugger := start__ForeignDebugger_test(
		t,
		pid_PtyProcess,
	)
	ptyProcess.Mutex.Lock()
	error_signal__maybe := ptyProcess.State_state.Signal(
		_SYSCALL.SIGKILL,
		PIDFD_SIGNAL_THREAD_GROUP__Linux,
	)
	ptyProcess.Mutex.Unlock()
	if error_signal__maybe != nil {
		t.Fatalf(
			"Signal(SIGKILL): %v",
			error_signal__maybe,
		)
	}
	eventually__PtyProcess_test(
		t,
		"Zombie_Withheld",
		func() bool {
			ptyProcess.Mutex.Lock()
			State__PtyProcess__current := ptyProcess.State_state
			ptyProcess.Mutex.Unlock()
			_, isWithheld := State__PtyProcess__current.(*_Zombie_Withheld__State_PtyProcess_)
			return isWithheld
		},
	)
	foreignDebugger.Collect(t)
	expectReaped__PtyProcess_test(
		t,
		ptyProcess,
		_Signaled__Termination_PtyProcess_{
			Signal_PtyProcess: int(_SYSCALL.SIGKILL),
			Status_CoreDump:   NOT_DUMPED__Status_CoreDump__PtyProcess,
		},
	)
}

// §3B: the helper reports which step failed, so a missing directory is named as the directory,
// not as the shell ("chdir <dir>: …", the op os.StartProcess uses for its own Dir check).
func TestSpawn_PtyProcess_ChdirFailureNamesDirectory(t *_TESTING.T) {
	command_PtyProcess := _Command_PtyProcess_{
		Path_Executable: "/bin/sh",
		Argv:            []string{"/bin/sh"},
		Path_Directory:  "/nonexistent-suprasole-test",
	}
	error_spawn := spawnExpectingFailure__PtyProcess_test(
		t,
		path__PtyExec_test(t),
		command_PtyProcess,
	)
	expectPathError__PtyProcess_test(
		t,
		error_spawn,
		"chdir",
		"/nonexistent-suprasole-test",
		_SYSCALL.ENOENT,
	)
}

// §3B: an execve failure inside the helper reads as exec's did ("fork/exec <shell>: …").
func TestSpawn_PtyProcess_ExecveFailureNamesShell(t *_TESTING.T) {
	command_PtyProcess := _Command_PtyProcess_{
		Path_Executable: "/nonexistent-suprasole-shell",
		Argv:            []string{"/nonexistent-suprasole-shell"},
	}
	error_spawn := spawnExpectingFailure__PtyProcess_test(
		t,
		path__PtyExec_test(t),
		command_PtyProcess,
	)
	expectPathError__PtyProcess_test(
		t,
		error_spawn,
		"fork/exec",
		"/nonexistent-suprasole-shell",
		_SYSCALL.ENOENT,
	)
}

// §3B, §5A: a helper that is not installed fails the spawn before any shell work, naming the
// helper itself.
func TestSpawn_PtyProcess_MissingHelperNamesHelper(t *_TESTING.T) {
	path_PtyExec__Missing := _PATHFILEPATH.Join(
		t.TempDir(),
		"suprasole-pty-exec",
	)
	error_spawn := spawnExpectingFailure__PtyProcess_test(
		t,
		path_PtyExec__Missing,
		_Command_PtyProcess_{
			Path_Executable: "/bin/sh",
			Argv:            []string{"/bin/sh"},
		},
	)
	expectPathError__PtyProcess_test(
		t,
		error_spawn,
		"fork/exec",
		path_PtyExec__Missing,
		_SYSCALL.ENOENT,
	)
}

// Spawn_PtyProxy refuses a NUL byte in the client's request before resolving the shell name or
// opening a PTY, naming the field that carries it. The environment message is exec.Cmd.Start's own,
// so clients see the same text as before.
func TestCheck_NUL_PtyCommand_NamesTheOwner(t *_TESTING.T) {
	error_clean__maybe := check__NUL_PtyCommand(
		"/bin/sh",
		"/tmp",
		[]string{"PATH=/usr/bin:/bin"},
	)
	if error_clean__maybe != nil {
		t.Fatalf(
			"check__NUL_PtyCommand on a clean request = %v, want nil",
			error_clean__maybe,
		)
	}
	error_shellBinary := check__NUL_PtyCommand(
		"/bin/s\x00h",
		"/tmp",
		nil,
	)
	expectPathError__PtyProcess_test(
		t,
		error_shellBinary,
		"fork/exec",
		"/bin/s\x00h",
		_SYSCALL.EINVAL,
	)
	error_directory := check__NUL_PtyCommand(
		"/bin/sh",
		"/tmp\x00elsewhere",
		nil,
	)
	expectPathError__PtyProcess_test(
		t,
		error_directory,
		"chdir",
		"/tmp\x00elsewhere",
		_SYSCALL.EINVAL,
	)
	environmentVariables_nul := []string{
		"PATH=/usr/bin:/bin",
		"SUPRASOLE_TEST=before\x00after",
	}
	error_startNul__maybe := (&_EXEC.Cmd{
		Path: "/bin/true",
		Env:  environmentVariables_nul,
	}).Start()
	error_environment__maybe := check__NUL_PtyCommand(
		"/bin/sh",
		"/tmp",
		environmentVariables_nul,
	)
	if nil == error_startNul__maybe || nil == error_environment__maybe || error_startNul__maybe.Error() != error_environment__maybe.Error() {
		t.Fatalf(
			"check__NUL_PtyCommand environment error = %v, want exec.Cmd.Start's error %v",
			error_environment__maybe,
			error_startNul__maybe,
		)
	}
}

// §3B: the helper makes the shell a session leader (setsid), gives it the slave as controlling
// terminal (TIOCSCTTY), and hands it an empty signal mask, all before execve.
func TestSpawn_PtyProcess_ShellLeadsSessionWithControllingTerminal(t *_TESTING.T) {
	ptyProcess := spawn__PtyProcess_test(
		t,
		[]string{
			"/bin/sleep",
			"30",
		},
		nil,
	)
	ptyProcess.Mutex.Lock()
	State__PtyProcess__alive, ok_alive := ptyProcess.State_state.(*_Alive__State_PtyProcess_)
	ptyProcess.Mutex.Unlock()
	if !ok_alive {
		t.Fatalf("state after spawn = %T, want *_Alive__State_PtyProcess_", ptyProcess.State_state)
	}
	pid_PtyProcess := State__PtyProcess__alive.Pid
	statBytes, error_read__Stat__maybe := _OS.ReadFile(
		_FMT.Sprintf(
			"/proc/%d/stat",
			pid_PtyProcess,
		),
	)
	if error_read__Stat__maybe != nil {
		t.Fatalf(
			"read /proc/%d/stat: %v",
			pid_PtyProcess,
			error_read__Stat__maybe,
		)
	}
	stat := string(statBytes)
	index_commandEnd := _STRINGS.LastIndex(
		stat,
		")",
	)
	fields_afterCommand := _STRINGS.Fields(stat[index_commandEnd+2:])
	processGroup := fields_afterCommand[2]
	session := fields_afterCommand[3]
	ttyNumber := fields_afterCommand[4]
	pid_text := _STRCONV.Itoa(pid_PtyProcess)
	if pid_text != session || pid_text != processGroup || "0" == ttyNumber {
		t.Fatalf(
			"pid %d: session %s, process group %s, tty_nr %s; want session = group = pid and a controlling terminal",
			pid_PtyProcess,
			session,
			processGroup,
			ttyNumber,
		)
	}
	statusBytes, error_read__Status__maybe := _OS.ReadFile(
		_FMT.Sprintf(
			"/proc/%d/status",
			pid_PtyProcess,
		),
	)
	if error_read__Status__maybe != nil {
		t.Fatalf(
			"read /proc/%d/status: %v",
			pid_PtyProcess,
			error_read__Status__maybe,
		)
	}
	if !_STRINGS.Contains(string(statusBytes), "\nSigBlk:\t0000000000000000\n") {
		t.Fatalf(
			"pid %d signal mask is not empty:\n%s",
			pid_PtyProcess,
			statusBytes,
		)
	}
}

// The shell receives exactly the environment it was given: no deduplication, no dropped or added
// entries (exec.Cmd.Environ() kept only the later of two duplicates).
func TestSpawn_PtyProcess_EnvironmentPassesThroughUnchanged(t *_TESTING.T) {
	ptyProcess := spawn__PtyProcess_test(
		t,
		[]string{
			"sleep",
			"30",
		},
		[]string{
			"SUPRASOLE_TEST_DUPLICATE=first",
			"SUPRASOLE_TEST_DUPLICATE=second",
		},
	)
	ptyProcess.Mutex.Lock()
	State__PtyProcess__alive, ok_alive := ptyProcess.State_state.(*_Alive__State_PtyProcess_)
	ptyProcess.Mutex.Unlock()
	if !ok_alive {
		t.Fatalf("state after spawn = %T, want *_Alive__State_PtyProcess_", ptyProcess.State_state)
	}
	environBytes, error_read__Environ__maybe := _OS.ReadFile(
		_FMT.Sprintf(
			"/proc/%d/environ",
			State__PtyProcess__alive.Pid,
		),
	)
	if error_read__Environ__maybe != nil {
		t.Fatalf(
			"read /proc/%d/environ: %v",
			State__PtyProcess__alive.Pid,
			error_read__Environ__maybe,
		)
	}
	environment_received := _STRINGS.Split(
		_STRINGS.TrimSuffix(
			string(environBytes),
			"\x00",
		),
		"\x00",
	)
	environment_expected := []string{
		"PATH=/usr/bin:/bin",
		"SUPRASOLE_TEST_DUPLICATE=first",
		"SUPRASOLE_TEST_DUPLICATE=second",
	}
	if _STRINGS.Join(environment_received, "\n") != _STRINGS.Join(environment_expected, "\n") {
		t.Fatalf(
			"environment received = %q, want exactly %q",
			environment_received,
			environment_expected,
		)
	}
}

// A shell name without "/" is looked up in the server's PATH, exactly as exec.Command did; a name
// with "/" is passed to execve untouched, so a bad one is reported by the helper's execve step.
func TestResolve_Path_ShellBinary_LooksUpOnlyBareNames(t *_TESTING.T) {
	path_expected, error_lookPath__maybe := _EXEC.LookPath("sh")
	if error_lookPath__maybe != nil {
		t.Fatalf(
			"exec.LookPath(\"sh\"): %v",
			error_lookPath__maybe,
		)
	}
	path_resolved, error_resolve__maybe := resolve__Path_ShellBinary("sh")
	if error_resolve__maybe != nil || path_expected != path_resolved {
		t.Fatalf(
			"resolve__Path_ShellBinary(\"sh\") = %q, %v; want %q, nil",
			path_resolved,
			error_resolve__maybe,
			path_expected,
		)
	}
	path_resolved__Slash, error_resolve__Slash__maybe := resolve__Path_ShellBinary("/nonexistent-suprasole-shell")
	if error_resolve__Slash__maybe != nil || "/nonexistent-suprasole-shell" != path_resolved__Slash {
		t.Fatalf(
			"resolve__Path_ShellBinary(\"/nonexistent-suprasole-shell\") = %q, %v; want it unchanged, nil",
			path_resolved__Slash,
			error_resolve__Slash__maybe,
		)
	}
	_, error_resolve__Missing__maybe := resolve__Path_ShellBinary("nonexistent-suprasole-shell")
	var error_exec *_EXEC.Error
	if !_ERRORS.As(error_resolve__Missing__maybe, &error_exec) {
		t.Fatalf(
			"resolve__Path_ShellBinary(\"nonexistent-suprasole-shell\") error = %v, want an *exec.Error",
			error_resolve__Missing__maybe,
		)
	}
}

func spawnExpectingFailure__PtyProcess_test(
	t *_TESTING.T,
	path_PtyExec string,
	command_PtyProcess _Command_PtyProcess_,
) error {
	t.Helper()
	fileDescriptor_master__Pty__shared, fileDescriptor_slave__Pty, error_open__PtyDescriptor__maybe := Open_PtyDescriptor(
		80,
		24,
	)
	if error_open__PtyDescriptor__maybe != nil {
		t.Fatalf(
			"Open_PtyDescriptor: %v",
			error_open__PtyDescriptor__maybe,
		)
	}
	defer fileDescriptor_master__Pty__shared.Close()
	defer fileDescriptor_slave__Pty.Close()
	ptyProcess, error_spawn__maybe := Spawn_PtyProcess(
		path_PtyExec,
		command_PtyProcess,
		fileDescriptor_slave__Pty,
	)
	if ptyProcess != nil {
		killAndAwait__PtyProcess_test(ptyProcess)
		t.Fatalf(
			"Spawn_PtyProcess(%v) returned a process, want a failure",
			command_PtyProcess.Argv,
		)
	}
	if nil == error_spawn__maybe {
		t.Fatalf(
			"Spawn_PtyProcess(%v) returned neither a process nor an error",
			command_PtyProcess.Argv,
		)
	}
	return error_spawn__maybe
}

func expectPathError__PtyProcess_test(
	t *_TESTING.T,
	error_spawn error,
	op_expected string,
	path_expected string,
	errorCode_expected _SYSCALL.Errno,
) {
	t.Helper()
	var error_path *_OS.PathError
	if !_ERRORS.As(error_spawn, &error_path) || op_expected != error_path.Op || path_expected != error_path.Path || !_ERRORS.Is(error_path.Err, errorCode_expected) {
		t.Fatalf(
			"Spawn_PtyProcess error = %#v, want *os.PathError{Op: %q, Path: %q, Err: %v}",
			error_spawn,
			op_expected,
			path_expected,
			errorCode_expected,
		)
	}
}

func path__PtyExec_test(t *_TESTING.T) string {
	t.Helper()
	path_PtyExec := _OS.Getenv(ENVIRONMENT_VARIABLE_PATH__PtyExec_test)
	if "" == path_PtyExec {
		t.Fatalf("%s is unset: run the tests through tests/runTests.sh, which builds the helper", ENVIRONMENT_VARIABLE_PATH__PtyExec_test)
	}
	return path_PtyExec
}

// §3B: a spawn that cannot become a _PtyProcess_ (the netpoller refused its pidfd) is killed and
// collected through the pidfd, never leaked (§0: every signal through the pidfd). The reaper polls
// with WNOHANG between sleeps, so it never holds an OS thread in a blocking waitid.
func TestReapAbandoned_PtyProcess_Pidfd_CollectsAfterGroupKill(t *_TESTING.T) {
	pid_PtyProcess, fileDescriptor_pidfd := forkExecSleep__PtyProcess_test(
		t,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	fileDescriptor_pidfd__PtyProcess := _OS.NewFile(
		uintptr(fileDescriptor_pidfd),
		NAME_FILE__Pidfd,
	)
	abandon__PtyProcess_test(fileDescriptor_pidfd__PtyProcess)
	eventually__PtyProcess_test(
		t,
		"abandoned process killed and collected",
		func() bool {
			return "gone" == readStateLetter__Proc_test(pid_PtyProcess)
		},
	)
}

// §3B, §6-AN: a self-traced child (its exec trap still pending when abandoned) is collected without the
// reaper ever seeing the trap: SIGKILL's fatal wake clears JOBCTL_TRACED inside kill() (kernel >= 5.19). The
// reaper has no trap branch on that basis — its final else panics — so a trap report here would fail this test.
func TestReapAbandoned_PtyProcess_Pidfd_ReapsPastTrapReports(t *_TESTING.T) {
	_RUNTIME.LockOSThread() // the forking thread becomes the tracer
	defer _RUNTIME.UnlockOSThread()
	pid_PtyProcess, fileDescriptor_pidfd := forkExecSleep__PtyProcess_test(
		t,
		&_SYSCALL.SysProcAttr{Setsid: true, Ptrace: true},
	)
	eventually__PtyProcess_test(
		t,
		"trace-stopped at exec",
		func() bool {
			return "t" == readStateLetter__Proc_test(pid_PtyProcess)
		},
	)
	fileDescriptor_pidfd__PtyProcess := _OS.NewFile(
		uintptr(fileDescriptor_pidfd),
		NAME_FILE__Pidfd,
	)
	abandon__PtyProcess_test(fileDescriptor_pidfd__PtyProcess)
	eventually__PtyProcess_test(
		t,
		"abandoned process killed and collected",
		func() bool {
			return "gone" == readStateLetter__Proc_test(pid_PtyProcess)
		},
	)
}

// §3B: the spawn's pollability check. Go documents that NewFile "will attempt to return a pollable
// file" and that "only pollable files support SetReadDeadline": a pidfd left blocking (as when
// O_NONBLOCK fails) is not pollable, so SetReadDeadline reports os.ErrNoDeadline; a non-blocking one
// is. A failed netpoll registration (epoll_ctl ENOMEM / ENOSPC) leaves the file in the same state as
// the blocking case, which is why one check covers both spawn failures.
func TestSpawn_PtyProcess_SetReadDeadlineDetectsUnpollablePidfd(t *_TESTING.T) {
	_, fileDescriptor_pidfd__Blocking := forkExecSleep__PtyProcess_test(
		t,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	fileDescriptor_pidfd__PtyProcess__Blocking := _OS.NewFile(
		uintptr(fileDescriptor_pidfd__Blocking),
		NAME_FILE__Pidfd,
	)
	error_setReadDeadline__Blocking__maybe := fileDescriptor_pidfd__PtyProcess__Blocking.SetReadDeadline(_TIME.Time{})
	abandon__PtyProcess_test(fileDescriptor_pidfd__PtyProcess__Blocking)
	if !_ERRORS.Is(error_setReadDeadline__Blocking__maybe, _OS.ErrNoDeadline) {
		t.Fatalf(
			"SetReadDeadline on a blocking pidfd = %v, want os.ErrNoDeadline",
			error_setReadDeadline__Blocking__maybe,
		)
	}
	_, fileDescriptor_pidfd__Nonblocking := forkExecSleep__PtyProcess_test(
		t,
		&_SYSCALL.SysProcAttr{Setsid: true},
	)
	error_setNonblock__maybe := _SYSCALL.SetNonblock(
		fileDescriptor_pidfd__Nonblocking,
		true,
	)
	if error_setNonblock__maybe != nil {
		t.Fatalf(
			"SetNonblock: %v",
			error_setNonblock__maybe,
		)
	}
	fileDescriptor_pidfd__PtyProcess__Nonblocking := _OS.NewFile(
		uintptr(fileDescriptor_pidfd__Nonblocking),
		NAME_FILE__Pidfd,
	)
	error_setReadDeadline__Nonblocking__maybe := fileDescriptor_pidfd__PtyProcess__Nonblocking.SetReadDeadline(_TIME.Time{})
	abandon__PtyProcess_test(fileDescriptor_pidfd__PtyProcess__Nonblocking)
	if error_setReadDeadline__Nonblocking__maybe != nil {
		t.Fatalf(
			"SetReadDeadline on a non-blocking pidfd = %v, want nil (pollable)",
			error_setReadDeadline__Nonblocking__maybe,
		)
	}
}

func forkExecSleep__PtyProcess_test(
	t *_TESTING.T,
	sysProcAttr *_SYSCALL.SysProcAttr,
) (int, int) {
	t.Helper()
	path_sleep, error_lookPath__maybe := _EXEC.LookPath("sleep")
	if error_lookPath__maybe != nil {
		t.Fatalf(
			"LookPath(sleep): %v",
			error_lookPath__maybe,
		)
	}
	devNull := open__DevNull_test(t)
	fileDescriptor_pidfd := -1
	sysProcAttr.PidFD = &fileDescriptor_pidfd
	pid_PtyProcess, error_forkExec__maybe := _SYSCALL.ForkExec(
		path_sleep,
		[]string{"sleep", "30"},
		&_SYSCALL.ProcAttr{
			Dir:   t.TempDir(),
			Files: []uintptr{uintptr(devNull), uintptr(devNull), uintptr(devNull)},
			Sys:   sysProcAttr,
		},
	)
	if error_forkExec__maybe != nil {
		t.Fatalf(
			"ForkExec(sleep): %v",
			error_forkExec__maybe,
		)
	}
	return pid_PtyProcess, fileDescriptor_pidfd
}

func spawn__PtyProcess_test(
	t *_TESTING.T,
	argv []string,
	environmentVariables_extra []string,
) *_PtyProcess_ {
	t.Helper()
	fileDescriptor_master__Pty__shared, fileDescriptor_slave__Pty, error_open__PtyDescriptor__maybe := Open_PtyDescriptor(
		80,
		24,
	)
	if error_open__PtyDescriptor__maybe != nil {
		t.Fatalf(
			"Open_PtyDescriptor: %v",
			error_open__PtyDescriptor__maybe,
		)
	}
	path_Executable, error_lookPath__maybe := _EXEC.LookPath(argv[0])
	if error_lookPath__maybe != nil {
		_ = fileDescriptor_master__Pty__shared.Close()
		_ = fileDescriptor_slave__Pty.Close()
		t.Fatalf(
			"exec.LookPath(%q): %v",
			argv[0],
			error_lookPath__maybe,
		)
	}
	command_PtyProcess := _Command_PtyProcess_{
		Path_Executable: path_Executable,
		Argv:            argv,
		Path_Directory:  t.TempDir(),
		Environment: append(
			[]string{"PATH=/usr/bin:/bin"},
			environmentVariables_extra...,
		),
	}
	ptyProcess, error_spawn__maybe := Spawn_PtyProcess(
		path__PtyExec_test(t),
		command_PtyProcess,
		fileDescriptor_slave__Pty,
	)
	_ = fileDescriptor_slave__Pty.Close()
	if error_spawn__maybe != nil {
		_ = fileDescriptor_master__Pty__shared.Close()
		t.Fatalf(
			"Spawn_PtyProcess(%v): %v",
			argv,
			error_spawn__maybe,
		)
	}
	go ptyProcess.RunWorker()
	t.Cleanup(func() {
		killAndAwait__PtyProcess_test(ptyProcess)
		_ = fileDescriptor_master__Pty__shared.Close()
	})
	return ptyProcess
}

func killAndAwait__PtyProcess_test(
	ptyProcess *_PtyProcess_,
) {
	ptyProcess.Mutex.Lock()
	_ = ptyProcess.State_state.Signal(
		_SYSCALL.SIGKILL,
		PIDFD_SIGNAL_PROCESS_GROUP__Linux,
	)
	ptyProcess.Mutex.Unlock()
	select {
	case <-ptyProcess.Channel_FinalState:
	case <-_TIME.After(DEADLINE__PtyProcess_test):
	}
}

func expectReaped__PtyProcess_test(
	t *_TESTING.T,
	ptyProcess *_PtyProcess_,
	termination_expected _Termination_PtyProcess_,
) *_Reaped__State_PtyProcess_ {
	t.Helper()
	select {
	case <-ptyProcess.Channel_FinalState:
	case <-_TIME.After(DEADLINE__PtyProcess_test):
		t.Fatalf(
			"Channel_FinalState not closed within %v",
			DEADLINE__PtyProcess_test,
		)
	}
	ptyProcess.Mutex.Lock()
	State__PtyProcess__final := ptyProcess.State_state
	ptyProcess.Mutex.Unlock()
	State__PtyProcess__reaped, isReaped := State__PtyProcess__final.(*_Reaped__State_PtyProcess_)
	if !isReaped {
		t.Fatalf(
			"final state = %#v, want *_Reaped__State_PtyProcess_",
			State__PtyProcess__final,
		)
	}
	if termination_expected != State__PtyProcess__reaped.Termination {
		t.Fatalf(
			"termination = %#v, want %#v",
			State__PtyProcess__reaped.Termination,
			termination_expected,
		)
	}
	return State__PtyProcess__reaped
}

func readPid__PtyProcess_test(
	t *_TESTING.T,
	ptyProcess *_PtyProcess_,
) int {
	t.Helper()
	ptyProcess.Mutex.Lock()
	State__PtyProcess__current := ptyProcess.State_state
	ptyProcess.Mutex.Unlock()
	State__PtyProcess__alive, isAlive := State__PtyProcess__current.(*_Alive__State_PtyProcess_)
	if !isAlive {
		t.Fatalf(
			"state = %T, want *_Alive__State_PtyProcess_",
			State__PtyProcess__current,
		)
	}
	return State__PtyProcess__alive.Pid
}

func open__DevNull_test(t *_TESTING.T) int {
	t.Helper()
	devNull, error_open__DevNull__maybe := _SYSCALL.Open(
		"/dev/null",
		_SYSCALL.O_RDWR|_SYSCALL.O_CLOEXEC,
		0,
	)
	if error_open__DevNull__maybe != nil {
		t.Fatalf(
			"open /dev/null: %v",
			error_open__DevNull__maybe,
		)
	}
	t.Cleanup(func() {
		_ = _SYSCALL.Close(devNull)
	})
	return devNull
}

// readStateLetter__Proc_test returns the /proc/<pid>/stat state letter (proc(5)), or "gone".
func readStateLetter__Proc_test(pid int) string {
	path_stat := _FMT.Sprintf(
		"/proc/%d/stat",
		pid,
	)
	statBytes, error_read__Stat__maybe := _OS.ReadFile(path_stat)
	if error_read__Stat__maybe != nil {
		return "gone"
	}
	stat := string(statBytes)
	index_commandEnd := _STRINGS.LastIndex(
		stat,
		")",
	)
	fields_afterCommand := _STRINGS.Fields(stat[index_commandEnd+2:])
	return fields_afterCommand[0]
}

func eventually__PtyProcess_test(
	t *_TESTING.T,
	what string,
	condition func() bool,
) {
	t.Helper()
	deadline := _TIME.Now().Add(DEADLINE__PtyProcess_test)
	for !condition() {
		if _TIME.Now().After(deadline) {
			t.Fatalf(
				"timed out after %v waiting for: %s",
				DEADLINE__PtyProcess_test,
				what,
			)
		}
		_TIME.Sleep(INTERVAL__PtyProcess_test)
	}
}

// run__ForeignDebugger_test runs in a separate process (the test binary re-executed): ordinary
// PTRACE_ATTACH, report "attached" (or the error), then collect the tracee's termination report
// once a line arrives on stdin, which releases the zombie to its real parent.
func run__ForeignDebugger_test(traceePid int) {
	_RUNTIME.LockOSThread()
	error_attach__maybe := _SYSCALL.PtraceAttach(traceePid)
	if error_attach__maybe != nil {
		_FMT.Println(
			"attach-failed",
			error_attach__maybe,
		)
		_OS.Exit(0)
	}
	var waitStatus _SYSCALL.WaitStatus
	_, _ = _SYSCALL.Wait4(
		traceePid,
		&waitStatus,
		_SYSCALL.WALL,
		nil,
	)
	_ = _SYSCALL.PtraceCont(
		traceePid,
		0,
	)
	_FMT.Println("attached")
	_, _ = _BUFIO.NewReader(_OS.Stdin).ReadString('\n')
	_, _ = _SYSCALL.Wait4(
		traceePid,
		&waitStatus,
		_SYSCALL.WALL,
		nil,
	)
	_FMT.Println("collected")
}

// start__ForeignDebugger_test attaches a separate process to traceePid, or skips the test when the
// host forbids it (Yama ptrace_scope, seccomp) — reachability is host-dependent (§5B.9).
func start__ForeignDebugger_test(
	t *_TESTING.T,
	traceePid int,
) *_ForeignDebugger_test_ {
	t.Helper()
	debuggerCommand := _EXEC.Command(
		_OS.Args[0],
		"-test.run=^$",
	)
	traceePidAssignment := ENVIRONMENT_VARIABLE_TRACEE_PID__ForeignDebugger_test + "=" + _STRCONV.Itoa(traceePid)
	debuggerCommand.Env = append(
		_OS.Environ(),
		traceePidAssignment,
	)
	debuggerStdin, _ := debuggerCommand.StdinPipe()
	debuggerStdout, _ := debuggerCommand.StdoutPipe()
	error_start__ForeignDebugger__maybe := debuggerCommand.Start()
	if error_start__ForeignDebugger__maybe != nil {
		t.Fatalf(
			"start debugger: %v",
			error_start__ForeignDebugger__maybe,
		)
	}
	t.Cleanup(func() {
		_ = debuggerCommand.Process.Kill()
		_ = debuggerCommand.Wait()
	})
	debuggerLines := _BUFIO.NewScanner(debuggerStdout)
	if !debuggerLines.Scan() {
		t.Fatalf("debugger produced no output")
	}
	if "attached" != debuggerLines.Text() {
		t.Skipf(
			"host does not permit a foreign debugger to attach (%s) — reachability is host-dependent (§5B.9)",
			debuggerLines.Text(),
		)
	}
	return &_ForeignDebugger_test_{
		Stdin: debuggerStdin,
		Lines: debuggerLines,
	}
}

// Collect makes the debugger collect its own termination report, which releases the zombie to its real parent.
func (this *_ForeignDebugger_test_) Collect(t *_TESTING.T) {
	t.Helper()
	_, _ = this.Stdin.Write([]byte("collect\n"))
	if !this.Lines.Scan() || "collected" != this.Lines.Text() {
		t.Fatalf(
			"debugger did not collect: %q",
			this.Lines.Text(),
		)
	}
}

// The abandon sequence Spawn_PtyProcess runs inline when the netpoller refuses a pidfd: group SIGKILL
// through the pidfd (result ignored: the reaper collects the process whenever it terminates), then the
// WNOHANG reaper, which closes the pidfd after the reap.
func abandon__PtyProcess_test(fileDescriptor_pidfd__PtyProcess *_OS.File) {
	_ = sendSignal_Pidfd(
		fileDescriptor_pidfd__PtyProcess,
		_SYSCALL.SIGKILL,
		PIDFD_SIGNAL_PROCESS_GROUP__Linux,
	)
	go reapAbandoned__PtyProcess_Pidfd(fileDescriptor_pidfd__PtyProcess)
}
