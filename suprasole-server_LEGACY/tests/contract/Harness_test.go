// Package contract pins the kernel and Go-runtime behavior that the pidfd /
// native-PTY design builds on (issues/pidfd-refactor-and-support.md §5C).
//
// Every test encodes one "Depends" row of §5C and cites the §6 evidence row it
// replaces. The tests use real processes and real PTYs — no mocks, because a
// mock of kernel behavior would only restate our assumptions. Run them on every
// kernel / host / seccomp profile suprasole is deployed to:
//
//	go test ./tests/contract -count=1 -v
package contract

import (
	_BUFIO "bufio"
	_FMT "fmt"
	_OS "os"
	_EXEC "os/exec"
	_RUNTIME "runtime"
	_STRCONV "strconv"
	_STRINGS "strings"
	_SYSCALL "syscall"
	_TESTING "testing"
	_TIME "time"
	_UNSAFE "unsafe"
)

const (
	_SYS_PIDFD_SEND_SIGNAL      = 424 // post-5.1 unified syscall table (identical on amd64 / arm64)
	_SYS_PIDFD_OPEN             = 434
	_SYS_CLOSE_RANGE            = 436    // Linux 5.9
	_CLOSE_RANGE_CLOEXEC        = 1 << 2 // Linux 5.11
	_P_PIDFD                    = 3
	_PIDFD_SIGNAL_PROCESS_GROUP = 1 << 2 // Linux 6.9
	_TIOCGPTPEER                = 0x5441 // _IO('T', 0x41), Linux 4.13
	_POLLIN                     = 0x1

	_CLD_EXITED  = 1
	_CLD_KILLED  = 2
	_CLD_DUMPED  = 3
	_CLD_TRAPPED = 4
	_CLD_STOPPED = 5

	FLOOR_MAJOR__Kernel = 6
	FLOOR_MINOR__Kernel = 9

	TIMEOUT__Condition  = 5 * _TIME.Second // generous: these tests assert outcomes, never timings
	INTERVAL__Condition = 5 * _TIME.Millisecond

	ENVIRONMENT_VARIABLE_TRACEE_PID__ForeignDebugger = "SUPRASOLE_CONTRACT_TRACEE_PID"
	ENVIRONMENT_VARIABLE_SCENARIO__InheritingServer  = "SUPRASOLE_CONTRACT_INHERITING_SERVER_SCENARIO"
)

// 64-bit siginfo_t layout for SIGCHLD (amd64 / arm64), as used by waitid(2).
type _SigInfo_Waitid_ struct {
	Signo  int32
	Errno  int32
	Code   int32
	_      int32
	Pid    int32
	Uid    uint32
	Status int32
	_      [128 - 28]byte
}

type _Winsize_Pty_ struct {
	RowCount    uint16
	ColumnCount uint16
	_           [2]uint16
}

type _PollFd_Ppoll_ struct {
	FileDescriptor int32
	Events         int16
	Revents        int16
}

type _Child_ struct {
	Pid   int
	Pidfd int
}

type _Pty_ struct {
	Master  *_OS.File // non-blocking from birth, netpoller-registered
	SlaveFd int       // blocking, O_CLOEXEC; the caller passes it to the child, then closes it
	Number  int       // N in /dev/pts/N
}

type _ReadOutcome_Pty_ struct {
	OutputData []byte
	ExitSignal error
}

type _ForeignDebugger_ struct {
	ThreadId int
	Stdin    interface{ Write([]byte) (int, error) }
	Lines    *_BUFIO.Scanner
}

// ---------------------------------------------------------------------------
// TestMain: kernel floor + capability check, and the helper modes in which the
// test binary is re-executed as a separate process (foreign debugger, inheriting server).
// ---------------------------------------------------------------------------

func TestMain(m *_TESTING.M) {
	traceePidText := _OS.Getenv(ENVIRONMENT_VARIABLE_TRACEE_PID__ForeignDebugger)
	if traceePidText != "" {
		traceePid, _ := _STRCONV.Atoi(traceePidText)
		run__ForeignDebugger(traceePid)
		return
	}
	scenario_inheritingServer := _OS.Getenv(ENVIRONMENT_VARIABLE_SCENARIO__InheritingServer)
	if scenario_inheritingServer != "" {
		run__InheritingServer(scenario_inheritingServer)
		return
	}
	reason_belowFloor := checkFloor__Kernel()
	if reason_belowFloor != "" {
		_FMT.Fprintln(
			_OS.Stderr,
			"contract: environment below the design's floor (§2, §5A):",
			reason_belowFloor,
		)
		_OS.Exit(1)
	}
	_OS.Exit(m.Run())
}

// checkFloor__Kernel mirrors the launcher check of §5A: kernel ≥ 6.9 and pidfd syscalls permitted by seccomp.
func checkFloor__Kernel() string {
	releaseBytes, error_read__OsRelease__maybe := _OS.ReadFile("/proc/sys/kernel/osrelease")
	if error_read__OsRelease__maybe != nil {
		return error_read__OsRelease__maybe.Error()
	}
	release := _STRINGS.TrimSpace(string(releaseBytes))
	var major, minor int
	_, _ = _FMT.Sscanf(
		release,
		"%d.%d",
		&major,
		&minor,
	)
	if major < FLOOR_MAJOR__Kernel || (FLOOR_MAJOR__Kernel == major && minor < FLOOR_MINOR__Kernel) {
		return _FMT.Sprintf(
			"kernel %s < %d.%d",
			release,
			FLOOR_MAJOR__Kernel,
			FLOOR_MINOR__Kernel,
		)
	}
	selfPidfd, _, errorCode__PIDFD_OPEN := _SYSCALL.Syscall(
		_SYS_PIDFD_OPEN,
		uintptr(_OS.Getpid()),
		0,
		0,
	)
	if errorCode__PIDFD_OPEN != 0 {
		return "pidfd_open(self): " + errorCode__PIDFD_OPEN.Error()
	}
	defer _SYSCALL.Close(int(selfPidfd))
	var sigInfo_ignored _SigInfo_Waitid_
	_, _, errorCode__WAITID := _SYSCALL.Syscall6(
		_SYSCALL.SYS_WAITID,
		_P_PIDFD,
		selfPidfd,
		uintptr(_UNSAFE.Pointer(&sigInfo_ignored)),
		_SYSCALL.WEXITED|_SYSCALL.WNOHANG,
		0,
		0,
	)
	if _SYSCALL.ECHILD != errorCode__WAITID {
		return "waitid(P_PIDFD, self) expected ECHILD, got: " + errorCode__WAITID.Error()
	}
	// Signal 0 is a pure existence/permission check. EINVAL = the flag is unknown (kernel < 6.9).
	// ESRCH = the flag is known, but this process leads no process group (the flag addresses the
	// group *led by* the pidfd's process — pidfd_send_signal(2)); the test binary is not a leader.
	_, _, errorCode__PIDFD_SEND_SIGNAL := _SYSCALL.Syscall6(
		_SYS_PIDFD_SEND_SIGNAL,
		selfPidfd,
		0,
		0,
		_PIDFD_SIGNAL_PROCESS_GROUP,
		0,
		0,
	)
	if errorCode__PIDFD_SEND_SIGNAL != 0 && errorCode__PIDFD_SEND_SIGNAL != _SYSCALL.ESRCH {
		return "pidfd_send_signal(self, 0, PIDFD_SIGNAL_PROCESS_GROUP): " + errorCode__PIDFD_SEND_SIGNAL.Error()
	}
	return ""
}

// ---------------------------------------------------------------------------
// Foreign debugger: the test binary re-executed in debugger mode (a separate process).
// ---------------------------------------------------------------------------

// run__ForeignDebugger runs in a separate process (the test binary re-executed):
// ordinary PTRACE_ATTACH, report "attached" (or the error), then collect the
// tracee's termination report once a line arrives on stdin.
func run__ForeignDebugger(traceePid int) {
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
	// The tracer is this thread; /proc TracerPid names the thread.
	_FMT.Println(
		"attached",
		_SYSCALL.Gettid(),
	)
	_, _ = _BUFIO.NewReader(_OS.Stdin).ReadString('\n')
	_, _ = _SYSCALL.Wait4(
		traceePid,
		&waitStatus,
		_SYSCALL.WALL,
		nil,
	)
	_FMT.Println("collected")
}

// start__ForeignDebugger attaches a separate process to traceePid, or skips the test when
// the host forbids it (Yama ptrace_scope, seccomp) — reachability is host-dependent (§5B.9).
func start__ForeignDebugger(
	t *_TESTING.T,
	traceePid int,
) *_ForeignDebugger_ {
	t.Helper()
	debuggerCommand := _EXEC.Command(
		_OS.Args[0],
		"-test.run=^$",
	)
	traceePidAssignment := ENVIRONMENT_VARIABLE_TRACEE_PID__ForeignDebugger + "=" + _STRCONV.Itoa(traceePid)
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
	reportFields := _STRINGS.Fields(debuggerLines.Text())
	if 0 == len(reportFields) || "attached" != reportFields[0] {
		t.Skipf(
			"host does not permit a foreign debugger to attach (%s) — reachability is host-dependent (§5B.9)",
			debuggerLines.Text(),
		)
	}
	debuggerThreadId, _ := _STRCONV.Atoi(reportFields[1])
	return &_ForeignDebugger_{
		ThreadId: debuggerThreadId,
		Stdin:    debuggerStdin,
		Lines:    debuggerLines,
	}
}

// Collect makes the debugger collect its own termination report, which releases the zombie to its real parent.
func (this *_ForeignDebugger_) Collect(t *_TESTING.T) {
	t.Helper()
	_, _ = this.Stdin.Write([]byte("collect\n"))
	if !this.Lines.Scan() || "collected" != this.Lines.Text() {
		t.Fatalf(
			"debugger did not collect: %q",
			this.Lines.Text(),
		)
	}
}

// ---------------------------------------------------------------------------
// Process helpers
// ---------------------------------------------------------------------------

// spawn__Child forks/execs through syscall.ForkExec with SysProcAttr.PidFD (CLONE_PIDFD),
// exactly as §3B does, and registers a cleanup that kills and reaps it if still present.
func spawn__Child(
	t *_TESTING.T,
	argv []string,
	files []uintptr,
	sysProcAttr *_SYSCALL.SysProcAttr,
) _Child_ {
	t.Helper()
	path, error_lookPath__maybe := _EXEC.LookPath(argv[0])
	if error_lookPath__maybe != nil {
		t.Fatalf(
			"LookPath(%s): %v",
			argv[0],
			error_lookPath__maybe,
		)
	}
	if nil == files {
		devNull := open__DevNull(t)
		files = []uintptr{uintptr(devNull), uintptr(devNull), uintptr(devNull)}
	}
	pidfd := -1
	sysProcAttr.PidFD = &pidfd
	pid, error_forkExec__maybe := _SYSCALL.ForkExec(
		path,
		argv,
		&_SYSCALL.ProcAttr{
			Dir:   t.TempDir(),
			Env:   []string{"PATH=/usr/bin:/bin", "TERM=xterm"},
			Files: files,
			Sys:   sysProcAttr,
		},
	)
	if error_forkExec__maybe != nil {
		t.Fatalf(
			"ForkExec(%v): %v",
			argv,
			error_forkExec__maybe,
		)
	}
	if pidfd < 0 {
		t.Fatalf(
			"ForkExec returned no pidfd (PidFD = %d)",
			pidfd,
		)
	}
	t.Cleanup(func() {
		_ = sendSignal__Pidfd(
			pidfd,
			_SYSCALL.SIGKILL,
			0,
		)
		deadline := _TIME.Now().Add(TIMEOUT__Condition)
		for _TIME.Now().Before(deadline) {
			sigInfo_reaped, errorCode__WAITID := reap__Pidfd(pidfd)
			if errorCode__WAITID != 0 || sigInfo_reaped.Pid != 0 {
				break
			}
			_TIME.Sleep(INTERVAL__Condition)
		}
		_ = _SYSCALL.Close(pidfd)
	})
	return _Child_{
		Pid:   pid,
		Pidfd: pidfd,
	}
}

func open__DevNull(t *_TESTING.T) int {
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

func waitid__Pidfd(
	pidfd int,
	options int,
) (_SigInfo_Waitid_, _SYSCALL.Errno) {
	for {
		var sigInfo_result _SigInfo_Waitid_ // zeroed every call: si_pid == 0 means "nothing reportable"
		_, _, errorCode__WAITID := _SYSCALL.Syscall6(
			_SYSCALL.SYS_WAITID,
			_P_PIDFD,
			uintptr(pidfd),
			uintptr(_UNSAFE.Pointer(&sigInfo_result)),
			uintptr(options),
			0,
			0,
		)
		if _SYSCALL.EINTR == errorCode__WAITID {
			continue
		}
		return sigInfo_result, errorCode__WAITID
	}
}

func peek__Pidfd(pidfd int) (_SigInfo_Waitid_, _SYSCALL.Errno) {
	sigInfo_peek, errorCode__WAITID := waitid__Pidfd(
		pidfd,
		_SYSCALL.WEXITED|_SYSCALL.WNOHANG|_SYSCALL.WNOWAIT,
	)
	return sigInfo_peek, errorCode__WAITID
}

func reap__Pidfd(pidfd int) (_SigInfo_Waitid_, _SYSCALL.Errno) {
	sigInfo_reaped, errorCode__WAITID := waitid__Pidfd(
		pidfd,
		_SYSCALL.WEXITED|_SYSCALL.WNOHANG,
	)
	return sigInfo_reaped, errorCode__WAITID
}

func sendSignal__Pidfd(
	pidfd int,
	signal _SYSCALL.Signal,
	flags uintptr,
) _SYSCALL.Errno {
	_, _, errorCode__PIDFD_SEND_SIGNAL := _SYSCALL.Syscall6(
		_SYS_PIDFD_SEND_SIGNAL,
		uintptr(pidfd),
		uintptr(signal),
		0,
		flags,
		0,
		0,
	)
	return errorCode__PIDFD_SEND_SIGNAL
}

func isReadable__Pidfd(pidfd int) bool {
	pollFd := _PollFd_Ppoll_{
		FileDescriptor: int32(pidfd),
		Events:         _POLLIN,
	}
	var timeout_immediate _SYSCALL.Timespec // zero: do not block
	_, _, _ = _SYSCALL.Syscall6(
		_SYSCALL.SYS_PPOLL,
		uintptr(_UNSAFE.Pointer(&pollFd)),
		1,
		uintptr(_UNSAFE.Pointer(&timeout_immediate)),
		0,
		0,
		0,
	)
	return 0 != pollFd.Revents&_POLLIN
}

// readState__Proc returns the /proc/<pid>/stat state letter (proc(5)), or "gone".
func readState__Proc(pid int) string {
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

// readTracerPid__Proc returns /proc/<pid>/status TracerPid (proc(5)), or -1.
func readTracerPid__Proc(pid int) int {
	path_status := _FMT.Sprintf(
		"/proc/%d/status",
		pid,
	)
	statusBytes, error_read__Status__maybe := _OS.ReadFile(path_status)
	if error_read__Status__maybe != nil {
		return -1
	}
	lines_status := _STRINGS.Split(
		string(statusBytes),
		"\n",
	)
	for _, line := range lines_status {
		value_tracerPid, found := _STRINGS.CutPrefix(
			line,
			"TracerPid:",
		)
		if found {
			value_tracerPid__trimmed := _STRINGS.TrimSpace(value_tracerPid)
			tracerPid, _ := _STRCONV.Atoi(value_tracerPid__trimmed)
			return tracerPid
		}
	}
	return -1
}

// eventually__Condition polls condition until it holds or the deadline passes. Tests observe
// with polling; the design itself never polls (§3C).
func eventually__Condition(
	t *_TESTING.T,
	what string,
	condition func() bool,
) {
	t.Helper()
	deadline := _TIME.Now().Add(TIMEOUT__Condition)
	for !condition() {
		if _TIME.Now().After(deadline) {
			t.Fatalf(
				"timed out after %v waiting for: %s",
				TIMEOUT__Condition,
				what,
			)
		}
		_TIME.Sleep(INTERVAL__Condition)
	}
}

// awaitTermination__Pidfd waits until the peek reports a termination (the child is a zombie) and returns it.
func awaitTermination__Pidfd(
	t *_TESTING.T,
	pidfd int,
) _SigInfo_Waitid_ {
	t.Helper()
	var sigInfo_reported _SigInfo_Waitid_
	eventually__Condition(
		t,
		"a termination report",
		func() bool {
			sigInfo_peek, errorCode__WAITID := peek__Pidfd(pidfd)
			if errorCode__WAITID != 0 {
				t.Fatalf(
					"peek: %v",
					errorCode__WAITID,
				)
			}
			sigInfo_reported = sigInfo_peek
			return sigInfo_peek.Pid != 0 && sigInfo_peek.Code != _CLD_TRAPPED
		},
	)
	return sigInfo_reported
}

func readFlags__Descriptor(
	t *_TESTING.T,
	fileDescriptor int,
) (int, int) {
	t.Helper()
	statusFlags, _, errorCode__F_GETFL := _SYSCALL.Syscall(
		_SYSCALL.SYS_FCNTL,
		uintptr(fileDescriptor),
		_SYSCALL.F_GETFL,
		0,
	)
	if errorCode__F_GETFL != 0 {
		t.Fatalf(
			"F_GETFL: %v",
			errorCode__F_GETFL,
		)
	}
	descriptorFlags, _, errorCode__F_GETFD := _SYSCALL.Syscall(
		_SYSCALL.SYS_FCNTL,
		uintptr(fileDescriptor),
		_SYSCALL.F_GETFD,
		0,
	)
	if errorCode__F_GETFD != 0 {
		t.Fatalf(
			"F_GETFD: %v",
			errorCode__F_GETFD,
		)
	}
	return int(statusFlags), int(descriptorFlags)
}

// ---------------------------------------------------------------------------
// Native PTY (§3A): /dev/ptmx + TIOCSPTLCK, TIOCSWINSZ, TIOCGPTPEER — no creack/pty.
// ---------------------------------------------------------------------------

// Pointer→uintptr conversions stay inside each syscall.Syscall call expression: only there does Go
// keep the pointed-to memory alive and unmoved for the duration of the call (unsafe.Pointer rule 4).
func open__Pty(
	t *_TESTING.T,
	rowCount uint16,
	columnCount uint16,
) *_Pty_ {
	t.Helper()
	masterFd, error_open__Ptmx__maybe := _SYSCALL.Open(
		"/dev/ptmx",
		_SYSCALL.O_RDWR|_SYSCALL.O_NOCTTY|_SYSCALL.O_CLOEXEC|_SYSCALL.O_NONBLOCK,
		0,
	)
	if error_open__Ptmx__maybe != nil {
		t.Fatalf(
			"open /dev/ptmx: %v",
			error_open__Ptmx__maybe,
		)
	}
	var lockStatus_unlocked int32
	_, _, errorCode__TIOCSPTLCK := _SYSCALL.Syscall(
		_SYSCALL.SYS_IOCTL,
		uintptr(masterFd),
		_SYSCALL.TIOCSPTLCK,
		uintptr(_UNSAFE.Pointer(&lockStatus_unlocked)),
	)
	if errorCode__TIOCSPTLCK != 0 {
		t.Fatalf(
			"TIOCSPTLCK: %v",
			errorCode__TIOCSPTLCK,
		)
	}
	var number uint32
	_, _, errorCode__TIOCGPTN := _SYSCALL.Syscall(
		_SYSCALL.SYS_IOCTL,
		uintptr(masterFd),
		_SYSCALL.TIOCGPTN,
		uintptr(_UNSAFE.Pointer(&number)),
	)
	if errorCode__TIOCGPTN != 0 {
		t.Fatalf(
			"TIOCGPTN: %v",
			errorCode__TIOCGPTN,
		)
	}
	winsize := _Winsize_Pty_{
		RowCount:    rowCount,
		ColumnCount: columnCount,
	}
	_, _, errorCode__TIOCSWINSZ := _SYSCALL.Syscall(
		_SYSCALL.SYS_IOCTL,
		uintptr(masterFd),
		_SYSCALL.TIOCSWINSZ,
		uintptr(_UNSAFE.Pointer(&winsize)),
	)
	if errorCode__TIOCSWINSZ != 0 {
		t.Fatalf(
			"TIOCSWINSZ: %v",
			errorCode__TIOCSWINSZ,
		)
	}
	slaveFd, _, errorCode__TIOCGPTPEER := _SYSCALL.Syscall(
		_SYSCALL.SYS_IOCTL,
		uintptr(masterFd),
		_TIOCGPTPEER,
		_SYSCALL.O_RDWR|_SYSCALL.O_NOCTTY|_SYSCALL.O_CLOEXEC,
	)
	if errorCode__TIOCGPTPEER != 0 {
		t.Fatalf(
			"TIOCGPTPEER: %v",
			errorCode__TIOCGPTPEER,
		)
	}
	master := _OS.NewFile(
		uintptr(masterFd),
		"/dev/ptmx",
	)
	pty := &_Pty_{
		Master:  master,
		SlaveFd: int(slaveFd),
		Number:  int(number),
	}
	t.Cleanup(func() {
		_ = pty.Master.Close()
	})
	return pty
}

// spawnOn__Pty starts argv as session leader with the slave as controlling terminal (§3B), then closes the parent's slave copy (§5B.5).
func spawnOn__Pty(
	t *_TESTING.T,
	pty *_Pty_,
	argv ...string,
) _Child_ {
	t.Helper()
	child := spawn__Child(
		t,
		argv,
		[]uintptr{uintptr(pty.SlaveFd), uintptr(pty.SlaveFd), uintptr(pty.SlaveFd)},
		&_SYSCALL.SysProcAttr{
			Setsid:  true,
			Setctty: true,
			Ctty:    0,
		},
	)
	_ = _SYSCALL.Close(pty.SlaveFd)
	pty.SlaveFd = -1
	return child
}

// readUntilExitSignal__Pty drains the master until Read fails, collecting everything read.
func readUntilExitSignal__Pty(master *_OS.File) <-chan _ReadOutcome_Pty_ {
	channel_readOutcome := make(chan _ReadOutcome_Pty_, 1)
	go func() {
		var outputData []byte
		readBuffer := make([]byte, 4095)
		for {
			bytesRed, exitSignal := master.Read(readBuffer)
			outputData = append(
				outputData,
				readBuffer[:bytesRed]...,
			)
			if exitSignal != nil {
				channel_readOutcome <- _ReadOutcome_Pty_{
					OutputData: outputData,
					ExitSignal: exitSignal,
				}
				return
			}
		}
	}()
	return channel_readOutcome
}
