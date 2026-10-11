package contract

import (
	_FMT "fmt"
	_OS "os"
	_EXEC "os/exec"
	_PATHFILEPATH "path/filepath"
	_STRINGS "strings"
	_SYSCALL "syscall"
	_TESTING "testing"
)

const (
	FILE_DESCRIPTOR__Inherited = 3

	SCENARIO_AS_INHERITED__InheritingServer        = "AS_INHERITED"
	SCENARIO_CLOSE_RANGE_CLOEXEC__InheritingServer = "CLOSE_RANGE_CLOEXEC"

	TARGET_ABSENT__InheritingServer = "absent"
)

type _Report_InheritingServer_ struct {
	DescriptorFlags_Inherited int
	Target_Server             string
	Target_Child              string
}

// §5C, execve(2) "by default, file descriptors remain open across an execve()": a descriptor the
// launcher passes to suprasole carries no close-on-exec — that is why it survived — so it reaches
// every child suprasole spawns, at the same number. (§5B.4, §8.4, §6-AK)
func TestCloseOnExec_InheritedDescriptorReachesSpawnedChild(t *_TESTING.T) {
	path_marker, report := start__InheritingServer(
		t,
		SCENARIO_AS_INHERITED__InheritingServer,
	)
	if 0 != report.DescriptorFlags_Inherited&_SYSCALL.FD_CLOEXEC {
		t.Fatalf(
			"inherited descriptor flags %#x carry FD_CLOEXEC, want none (it survived an execve)",
			report.DescriptorFlags_Inherited,
		)
	}
	if path_marker != report.Target_Server {
		t.Fatalf(
			"server fd %d = %q, want the launcher's %q",
			FILE_DESCRIPTOR__Inherited,
			report.Target_Server,
			path_marker,
		)
	}
	if path_marker != report.Target_Child {
		t.Fatalf(
			"spawned child fd %d = %q, want the launcher's %q (inherited through the server)",
			FILE_DESCRIPTOR__Inherited,
			report.Target_Child,
			path_marker,
		)
	}
}

// §5C, close_range(2) CLOSE_RANGE_CLOEXEC (5.11): marks a range close-on-exec without closing it —
// suprasole keeps using the descriptor, and its spawned children no longer receive it. (§8.4, §6-AK)
func TestCloseOnExec_CloseRangeCloexecStopsInheritance(t *_TESTING.T) {
	path_marker, report := start__InheritingServer(
		t,
		SCENARIO_CLOSE_RANGE_CLOEXEC__InheritingServer,
	)
	if 0 == report.DescriptorFlags_Inherited&_SYSCALL.FD_CLOEXEC {
		t.Fatalf(
			"inherited descriptor flags %#x after close_range(CLOSE_RANGE_CLOEXEC), want FD_CLOEXEC",
			report.DescriptorFlags_Inherited,
		)
	}
	if path_marker != report.Target_Server {
		t.Fatalf(
			"server fd %d = %q after marking, want the launcher's %q still open",
			FILE_DESCRIPTOR__Inherited,
			report.Target_Server,
			path_marker,
		)
	}
	if TARGET_ABSENT__InheritingServer != report.Target_Child {
		t.Fatalf(
			"spawned child fd %d = %q, want absent",
			FILE_DESCRIPTOR__Inherited,
			report.Target_Child,
		)
	}
}

func start__InheritingServer(
	t *_TESTING.T,
	scenario string,
) (string, _Report_InheritingServer_) {
	t.Helper()
	path_marker := _PATHFILEPATH.Join(
		t.TempDir(),
		"inherited.marker",
	)
	error_write__Marker__maybe := _OS.WriteFile(
		path_marker,
		nil,
		0o600,
	)
	if error_write__Marker__maybe != nil {
		t.Fatalf(
			"create marker: %v",
			error_write__Marker__maybe,
		)
	}
	path_marker__resolved, error_evalSymlinks__maybe := _PATHFILEPATH.EvalSymlinks(path_marker)
	if error_evalSymlinks__maybe != nil {
		t.Fatalf(
			"resolve marker: %v",
			error_evalSymlinks__maybe,
		)
	}
	markerFile, error_open__Marker__maybe := _OS.Open(path_marker)
	if error_open__Marker__maybe != nil {
		t.Fatalf(
			"open marker: %v",
			error_open__Marker__maybe,
		)
	}
	defer markerFile.Close()
	serverCommand := _EXEC.Command(
		_OS.Args[0],
		"-test.run=^$",
	)
	scenarioAssignment := ENVIRONMENT_VARIABLE_SCENARIO__InheritingServer + "=" + scenario
	serverCommand.Env = append(
		_OS.Environ(),
		scenarioAssignment,
	)
	serverCommand.ExtraFiles = []*_OS.File{markerFile}
	outputData_Server, error_output__maybe := serverCommand.Output()
	if error_output__maybe != nil {
		t.Fatalf(
			"inheriting server: %v (output %q)",
			error_output__maybe,
			outputData_Server,
		)
	}
	report := _Report_InheritingServer_{DescriptorFlags_Inherited: -1}
	lines_report := _STRINGS.Split(
		_STRINGS.TrimSpace(string(outputData_Server)),
		"\n",
	)
	for _, line := range lines_report {
		key, value, _ := _STRINGS.Cut(
			line,
			" ",
		)
		switch key {
		case "failed":
			t.Fatalf(
				"inheriting server failed: %s",
				value,
			)
		case "descriptorFlags":
			_, _ = _FMT.Sscanf(
				value,
				"%d",
				&report.DescriptorFlags_Inherited,
			)
		case "server":
			report.Target_Server = value
		case "child":
			report.Target_Child = value
		}
	}
	t.Logf(
		"scenario %s: %+v",
		scenario,
		report,
	)
	return path_marker__resolved, report
}

// run__InheritingServer runs in a separate process (the test binary re-executed, with the launcher's
// descriptor at fd 3 via exec.Cmd.ExtraFiles) and plays suprasole: optionally mark, spawn a child the
// way §3B does, and report what the server and the child hold at fd 3.
func run__InheritingServer(scenario string) {
	if SCENARIO_CLOSE_RANGE_CLOEXEC__InheritingServer == scenario {
		_, _, errorCode__CLOSE_RANGE := _SYSCALL.Syscall(
			_SYS_CLOSE_RANGE,
			FILE_DESCRIPTOR__Inherited,
			uintptr(^uint32(0)),
			_CLOSE_RANGE_CLOEXEC,
		)
		if errorCode__CLOSE_RANGE != 0 {
			exit__InheritingServer("close_range: " + errorCode__CLOSE_RANGE.Error())
		}
	}
	descriptorFlags_Inherited, _, errorCode__F_GETFD := _SYSCALL.Syscall(
		_SYSCALL.SYS_FCNTL,
		FILE_DESCRIPTOR__Inherited,
		_SYSCALL.F_GETFD,
		0,
	)
	if errorCode__F_GETFD != 0 {
		exit__InheritingServer("F_GETFD on the inherited descriptor: " + errorCode__F_GETFD.Error())
	}
	path_inherited__Server := _FMT.Sprintf(
		"/proc/self/fd/%d",
		FILE_DESCRIPTOR__Inherited,
	)
	target_server, error_readlink__Server__maybe := _OS.Readlink(path_inherited__Server)
	if error_readlink__Server__maybe != nil {
		exit__InheritingServer("readlink server: " + error_readlink__Server__maybe.Error())
	}
	path_sleep, error_lookPath__maybe := _EXEC.LookPath("sleep")
	if error_lookPath__maybe != nil {
		exit__InheritingServer("LookPath(sleep): " + error_lookPath__maybe.Error())
	}
	devNull, error_open__DevNull__maybe := _SYSCALL.Open(
		"/dev/null",
		_SYSCALL.O_RDWR|_SYSCALL.O_CLOEXEC,
		0,
	)
	if error_open__DevNull__maybe != nil {
		exit__InheritingServer("open /dev/null: " + error_open__DevNull__maybe.Error())
	}
	childPid, error_forkExec__maybe := _SYSCALL.ForkExec(
		path_sleep,
		[]string{"sleep", "30"},
		&_SYSCALL.ProcAttr{
			Files: []uintptr{uintptr(devNull), uintptr(devNull), uintptr(devNull)},
			Sys:   &_SYSCALL.SysProcAttr{Setsid: true},
		},
	)
	if error_forkExec__maybe != nil {
		exit__InheritingServer("ForkExec: " + error_forkExec__maybe.Error())
	}
	path_inherited__Child := _FMT.Sprintf(
		"/proc/%d/fd/%d",
		childPid,
		FILE_DESCRIPTOR__Inherited,
	)
	target_child, error_readlink__Child__maybe := _OS.Readlink(path_inherited__Child)
	if error_readlink__Child__maybe != nil {
		target_child = TARGET_ABSENT__InheritingServer
	}
	_ = _SYSCALL.Kill(
		childPid,
		_SYSCALL.SIGKILL,
	)
	var waitStatus _SYSCALL.WaitStatus
	_, _ = _SYSCALL.Wait4(
		childPid,
		&waitStatus,
		0,
		nil,
	)
	_FMT.Println(
		"descriptorFlags",
		descriptorFlags_Inherited,
	)
	_FMT.Println(
		"server",
		target_server,
	)
	_FMT.Println(
		"child",
		target_child,
	)
}

func exit__InheritingServer(reason string) {
	_FMT.Println(
		"failed",
		reason,
	)
	_OS.Exit(0)
}
