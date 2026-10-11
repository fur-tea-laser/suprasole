package contract

import (
	_BYTES "bytes"
	_ERRORS "errors"
	_FMT "fmt"
	_OS "os"
	_SYSCALL "syscall"
	_TESTING "testing"
	_TIME "time"
)

// §3A, ioctl_tty(2): /dev/ptmx + TIOCSPTLCK, TIOCSWINSZ, TIOCGPTPEER give a working PTY — the child
// sees the size set before it started and its controlling terminal is /dev/pts/N. Master non-blocking +
// close-on-exec; slave close-on-exec. (§6-L)
func TestPty_NativeDescriptorWorksWithoutCreackPty(t *_TESTING.T) {
	pty := open__Pty(
		t,
		24,
		80,
	)
	statusFlags_master, descriptorFlags_master := readFlags__Descriptor(
		t,
		int(pty.Master.Fd()),
	)
	if 0 == statusFlags_master&_SYSCALL.O_NONBLOCK || 0 == descriptorFlags_master&_SYSCALL.FD_CLOEXEC {
		t.Fatalf(
			"master: status flags %#x, descriptor flags %#x; want O_NONBLOCK and FD_CLOEXEC",
			statusFlags_master,
			descriptorFlags_master,
		)
	}
	_, descriptorFlags_slave := readFlags__Descriptor(
		t,
		pty.SlaveFd,
	)
	if 0 == descriptorFlags_slave&_SYSCALL.FD_CLOEXEC {
		t.Fatalf("slave from TIOCGPTPEER is not FD_CLOEXEC")
	}
	_ = spawnOn__Pty(
		t,
		pty,
		"sh",
		"-c",
		"stty size; tty",
	)
	path_slave := _FMT.Sprintf(
		"/dev/pts/%d",
		pty.Number,
	)
	select {
	case readOutcome := <-readUntilExitSignal__Pty(pty.Master):
		if !_ERRORS.Is(readOutcome.ExitSignal, _SYSCALL.EIO) {
			t.Fatalf(
				"master read ended with %v, want EIO",
				readOutcome.ExitSignal,
			)
		}
		if !_BYTES.Contains(readOutcome.OutputData, []byte("24 80")) {
			t.Fatalf(
				"child did not see the 24x80 size: %q",
				readOutcome.OutputData,
			)
		}
		if !_BYTES.Contains(readOutcome.OutputData, []byte(path_slave)) {
			t.Fatalf(
				"child's tty is not %s: %q",
				path_slave,
				readOutcome.OutputData,
			)
		}
	case <-_TIME.After(TIMEOUT__Condition):
		t.Fatalf(
			"no EIO within %v",
			TIMEOUT__Condition,
		)
	}
}

// §5C, Go os docs ("NewFile ... pollable"; Fd: "SetDeadline methods will stop working"): a master created
// non-blocking and wrapped by os.NewFile stays non-blocking even after Fd() — the creack/pty failure mode
// (§1.5, §6-A) cannot recur.
func TestPty_NewFileOfNonblockingMasterSurvivesFd(t *_TESTING.T) {
	pty := open__Pty(
		t,
		24,
		80,
	)
	_ = pty.Master.Fd()
	statusFlags_master, _ := readFlags__Descriptor(
		t,
		int(pty.Master.Fd()),
	)
	if 0 == statusFlags_master&_SYSCALL.O_NONBLOCK {
		t.Fatalf("O_NONBLOCK cleared by Fd()")
	}
	deadline := _TIME.Now().Add(TIMEOUT__Condition)
	error_setReadDeadline__maybe := pty.Master.SetReadDeadline(deadline)
	if error_setReadDeadline__maybe != nil {
		t.Fatalf(
			"master not pollable: %v",
			error_setReadDeadline__maybe,
		)
	}
}

// §5C, pty driver: closing the (non-blocking) master unblocks the parked reader with os.ErrClosed
// and hangs up the slave — the session leader receives SIGHUP. This is the forced unplug. (§6-B)
func TestPty_MasterCloseUnblocksReaderAndHangsUpSessionLeader(t *_TESTING.T) {
	pty := open__Pty(
		t,
		24,
		80,
	)
	child := spawnOn__Pty(
		t,
		pty,
		"sleep",
		"30",
	)
	channel_readOutcome := readUntilExitSignal__Pty(pty.Master)
	_TIME.Sleep(100 * _TIME.Millisecond) // let the reader park
	_ = pty.Master.Close()
	select {
	case readOutcome := <-channel_readOutcome:
		if !_ERRORS.Is(readOutcome.ExitSignal, _OS.ErrClosed) {
			t.Fatalf(
				"reader ended with %v, want os.ErrClosed",
				readOutcome.ExitSignal,
			)
		}
	case <-_TIME.After(TIMEOUT__Condition):
		t.Fatalf(
			"Close() did not unblock the reader within %v",
			TIMEOUT__Condition,
		)
	}
	sigInfo_reported := awaitTermination__Pidfd(
		t,
		child.Pidfd,
	)
	if _CLD_KILLED != sigInfo_reported.Code || int32(_SYSCALL.SIGHUP) != sigInfo_reported.Status {
		t.Fatalf(
			"session leader termination = code %d status %d, want CLD_KILLED/SIGHUP",
			sigInfo_reported.Code,
			sigInfo_reported.Status,
		)
	}
}

// §5C, pty driver (commit messages; not in pty(7)): the master returns every buffered byte before EIO. (§6-E)
func TestPty_BufferedOutputDrainsBeforeEIO(t *_TESTING.T) {
	const count_bytes = 1_000_000
	pty := open__Pty(
		t,
		24,
		80,
	)
	command_burst := _FMT.Sprintf(
		"head -c %d /dev/zero | tr '\\0' x; exit 0",
		count_bytes,
	)
	_ = spawnOn__Pty(
		t,
		pty,
		"sh",
		"-c",
		command_burst,
	)
	select {
	case readOutcome := <-readUntilExitSignal__Pty(pty.Master):
		if !_ERRORS.Is(readOutcome.ExitSignal, _SYSCALL.EIO) {
			t.Fatalf(
				"master read ended with %v, want EIO",
				readOutcome.ExitSignal,
			)
		}
		if count_bytes != len(readOutcome.OutputData) {
			t.Fatalf(
				"read %d bytes before EIO, want %d",
				len(readOutcome.OutputData),
				count_bytes,
			)
		}
	case <-_TIME.After(TIMEOUT__Condition):
		t.Fatalf(
			"no EIO within %v",
			TIMEOUT__Condition,
		)
	}
}

// §5C, pty driver: EIO waits for the last slave fd anywhere — an outside holder that opened /dev/pts/N
// by path blocks it even after the PTY process exited; releasing that holder lets EIO through. (§6-Y)
func TestPty_EIOWaitsForEverySlaveHolder(t *_TESTING.T) {
	pty := open__Pty(
		t,
		24,
		80,
	)
	path_slave := _FMT.Sprintf(
		"/dev/pts/%d",
		pty.Number,
	)
	slaveFd_outsideHolder, error_open__Slave__maybe := _SYSCALL.Open(
		path_slave,
		_SYSCALL.O_RDWR|_SYSCALL.O_NOCTTY|_SYSCALL.O_CLOEXEC,
		0,
	)
	if error_open__Slave__maybe != nil {
		t.Fatalf(
			"open %s by path: %v",
			path_slave,
			error_open__Slave__maybe,
		)
	}
	child := spawnOn__Pty(
		t,
		pty,
		"sh",
		"-c",
		"exit 0",
	)
	_ = awaitTermination__Pidfd(
		t,
		child.Pidfd,
	)
	channel_readOutcome := readUntilExitSignal__Pty(pty.Master)
	select {
	case readOutcome := <-channel_readOutcome:
		t.Fatalf(
			"master read ended (%v) while an outside holder still had the slave open",
			readOutcome.ExitSignal,
		)
	case <-_TIME.After(300 * _TIME.Millisecond):
	}
	_ = _SYSCALL.Close(slaveFd_outsideHolder)
	select {
	case readOutcome := <-channel_readOutcome:
		if !_ERRORS.Is(readOutcome.ExitSignal, _SYSCALL.EIO) {
			t.Fatalf(
				"master read ended with %v, want EIO",
				readOutcome.ExitSignal,
			)
		}
	case <-_TIME.After(TIMEOUT__Condition):
		t.Fatalf(
			"no EIO within %v after the last holder closed",
			TIMEOUT__Condition,
		)
	}
}

// §5C, pty driver: EIO is undone by reopening the slave while the master is open; closing the master
// removes /dev/pts/N so nothing can reconnect — why _PtyTeardown_ closes the master on the first EIO. (§6-Z)
func TestPty_EIOReversibleUntilMasterCloses(t *_TESTING.T) {
	pty := open__Pty(
		t,
		24,
		80,
	)
	child := spawnOn__Pty(
		t,
		pty,
		"true",
	)
	_ = awaitTermination__Pidfd(
		t,
		child.Pidfd,
	)
	select {
	case readOutcome := <-readUntilExitSignal__Pty(pty.Master):
		if !_ERRORS.Is(readOutcome.ExitSignal, _SYSCALL.EIO) {
			t.Fatalf(
				"master read ended with %v, want EIO",
				readOutcome.ExitSignal,
			)
		}
	case <-_TIME.After(TIMEOUT__Condition):
		t.Fatalf(
			"no EIO within %v",
			TIMEOUT__Condition,
		)
	}
	path_slave := _FMT.Sprintf(
		"/dev/pts/%d",
		pty.Number,
	)
	slaveFd_reopened, error_open__Reopen__maybe := _SYSCALL.Open(
		path_slave,
		_SYSCALL.O_RDWR|_SYSCALL.O_NOCTTY|_SYSCALL.O_CLOEXEC,
		0,
	)
	if error_open__Reopen__maybe != nil {
		t.Fatalf(
			"reopen %s after EIO: %v (the design defends against this succeeding; if it now fails, §5C can be relaxed)",
			path_slave,
			error_open__Reopen__maybe,
		)
	}
	_, _ = _SYSCALL.Write(
		slaveFd_reopened,
		[]byte("late\n"),
	)
	deadline := _TIME.Now().Add(TIMEOUT__Condition)
	_ = pty.Master.SetReadDeadline(deadline)
	readBuffer := make([]byte, 64)
	bytesRed, error_read__maybe := pty.Master.Read(readBuffer)
	if error_read__maybe != nil || !_BYTES.Contains(readBuffer[:bytesRed], []byte("late")) {
		t.Fatalf(
			"master read after reopen = %q, %v; want the late write",
			readBuffer[:bytesRed],
			error_read__maybe,
		)
	}
	_ = _SYSCALL.Close(slaveFd_reopened)
	_ = pty.Master.Close()
	_, error_stat__Slave__maybe := _OS.Stat(path_slave)
	if !_ERRORS.Is(error_stat__Slave__maybe, _OS.ErrNotExist) {
		t.Fatalf(
			"%s still exists after the master closed: %v",
			path_slave,
			error_stat__Slave__maybe,
		)
	}
}
