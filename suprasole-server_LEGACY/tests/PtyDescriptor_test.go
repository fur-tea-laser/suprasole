//go:build tests_overlay

package main

import (
	_BYTES "bytes"
	_ERRORS "errors"
	_OS "os"
	_STRINGS "strings"
	_SYSCALL "syscall"
	_TESTING "testing"
	_TIME "time"
	_UNSAFE "unsafe"
)

const DEADLINE__PtyDescriptor_test = 5 * _TIME.Second

func TestOpen_PtyDescriptor(t *_TESTING.T) {
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

	statusFlags_master, descriptorFlags_master := flags__PtyDescriptor_test(
		t,
		fileDescriptor_master__Pty__shared,
	)
	if 0 == statusFlags_master&_SYSCALL.O_NONBLOCK || 0 == descriptorFlags_master&_SYSCALL.FD_CLOEXEC {
		t.Fatalf(
			"master: status flags %#x, descriptor flags %#x; want O_NONBLOCK and FD_CLOEXEC",
			statusFlags_master,
			descriptorFlags_master,
		)
	}
	statusFlags_slave, descriptorFlags_slave := flags__PtyDescriptor_test(
		t,
		fileDescriptor_slave__Pty,
	)
	if 0 != statusFlags_slave&_SYSCALL.O_NONBLOCK || 0 == descriptorFlags_slave&_SYSCALL.FD_CLOEXEC {
		t.Fatalf(
			"slave: status flags %#x, descriptor flags %#x; want blocking and FD_CLOEXEC",
			statusFlags_slave,
			descriptorFlags_slave,
		)
	}
	_, error_stat__Slave__maybe := _OS.Stat(fileDescriptor_slave__Pty.Name())
	if !_STRINGS.HasPrefix(fileDescriptor_slave__Pty.Name(), PATH_PREFIX_SLAVE__PtyDescriptor) || error_stat__Slave__maybe != nil {
		t.Fatalf(
			"slave name %q does not name an existing /dev/pts/N: %v",
			fileDescriptor_slave__Pty.Name(),
			error_stat__Slave__maybe,
		)
	}
	rawConn_Slave, _ := fileDescriptor_slave__Pty.SyscallConn()
	var winsize_observed _Winsize_PtyDescriptor_
	var errorCode__TIOCGWINSZ _SYSCALL.Errno
	_ = rawConn_Slave.Control(func(fileDescriptor_slave uintptr) {
		_, _, errorCode__TIOCGWINSZ = _SYSCALL.Syscall(
			_SYSCALL.SYS_IOCTL,
			fileDescriptor_slave,
			_SYSCALL.TIOCGWINSZ,
			uintptr(_UNSAFE.Pointer(&winsize_observed)),
		)
	})
	if errorCode__TIOCGWINSZ != 0 || 24 != winsize_observed.RowCount_PtyTerminal || 80 != winsize_observed.ColumnCount_PtyTerminal {
		t.Fatalf(
			"slave TIOCGWINSZ = %d rows x %d columns (%v), want 24 x 80",
			winsize_observed.RowCount_PtyTerminal,
			winsize_observed.ColumnCount_PtyTerminal,
			errorCode__TIOCGWINSZ,
		)
	}
}

// EIO arriving at all proves the parent closed its own slave copy after the fork.
func TestSpawn_PtyProxy_ShellOwnsTheTerminal(t *_TESTING.T) {
	ptyProxy_spawned := spawnShell__PtyDescriptor_test(
		t,
		80,
		24,
	)
	_, _ = ptyProxy_spawned.FileDescriptor_Master__Pty__shared.Write([]byte("stty size; tty; exit 0\n"))
	outputData_PtyDescriptor, exitSignal_PtyReader := readUntilExitSignal__PtyDescriptor_test(
		t,
		ptyProxy_spawned.FileDescriptor_Master__Pty__shared,
	)
	if !_ERRORS.Is(exitSignal_PtyReader, _SYSCALL.EIO) {
		t.Fatalf(
			"master read ended with %v, want EIO",
			exitSignal_PtyReader,
		)
	}
	if !_BYTES.Contains(outputData_PtyDescriptor, []byte("24 80")) || !_BYTES.Contains(outputData_PtyDescriptor, []byte(PATH_PREFIX_SLAVE__PtyDescriptor)) {
		t.Fatalf(
			"shell did not report a 24x80 /dev/pts terminal: %q",
			outputData_PtyDescriptor,
		)
	}
	expectReaped__PtyProcess_test(
		t,
		ptyProxy_spawned.PtyProcess,
		_Exited__Termination_PtyProcess_{
			ExitCode_PtyProcess: 0,
		},
	)
}

func TestResize_PtyDescriptor(t *_TESTING.T) {
	ptyProxy_spawned := spawnShell__PtyDescriptor_test(
		t,
		80,
		24,
	)
	error_resize__maybe := ptyProxy_spawned.Resize(
		100,
		30,
	)
	if error_resize__maybe != nil {
		t.Fatalf(
			"Resize: %v",
			error_resize__maybe,
		)
	}
	_, _ = ptyProxy_spawned.FileDescriptor_Master__Pty__shared.Write([]byte("stty size; exit 0\n"))
	outputData_PtyDescriptor, _ := readUntilExitSignal__PtyDescriptor_test(
		t,
		ptyProxy_spawned.FileDescriptor_Master__Pty__shared,
	)
	if !_BYTES.Contains(outputData_PtyDescriptor, []byte("30 100")) {
		t.Fatalf(
			"shell did not see the 30x100 resize: %q",
			outputData_PtyDescriptor,
		)
	}
	_ = ptyProxy_spawned.FileDescriptor_Master__Pty__shared.Close()
	// Only "fails" is asserted: which error value RawConn.Control returns for a closed file is undocumented. (§6-AJ)
	error_resize__Closed__maybe := Resize_PtyDescriptor(
		ptyProxy_spawned.FileDescriptor_Master__Pty__shared,
		90,
		20,
	)
	if nil == error_resize__Closed__maybe {
		t.Fatalf("Resize after close succeeded, want a failure")
	}
}

// The defect phase 1 removes (issue §1.5): closing the master must unblock the parked reader and
// hang up the slave, so the session leader receives SIGHUP.
func TestMasterClose_UnblocksReaderAndHangsUpShell(t *_TESTING.T) {
	ptyProxy_spawned := spawnShell__PtyDescriptor_test(
		t,
		80,
		24,
	)
	channel_exitSignal := make(chan error, 1)
	go func() {
		_, exitSignal_PtyReader := readUntilExitSignal__PtyDescriptor_test(
			t,
			ptyProxy_spawned.FileDescriptor_Master__Pty__shared,
		)
		channel_exitSignal <- exitSignal_PtyReader
	}()
	_TIME.Sleep(200 * _TIME.Millisecond)
	_ = ptyProxy_spawned.FileDescriptor_Master__Pty__shared.Close()
	select {
	case exitSignal_PtyReader := <-channel_exitSignal:
		if !_ERRORS.Is(exitSignal_PtyReader, _OS.ErrClosed) {
			t.Fatalf(
				"reader ended with %v, want os.ErrClosed",
				exitSignal_PtyReader,
			)
		}
	case <-_TIME.After(DEADLINE__PtyDescriptor_test):
		t.Fatalf(
			"Close did not unblock the reader within %v",
			DEADLINE__PtyDescriptor_test,
		)
	}
	expectReaped__PtyProcess_test(
		t,
		ptyProxy_spawned.PtyProcess,
		_Signaled__Termination_PtyProcess_{
			Signal_PtyProcess: int(_SYSCALL.SIGHUP),
			Status_CoreDump:   NOT_DUMPED__Status_CoreDump__PtyProcess,
		},
	)
}

func spawnShell__PtyDescriptor_test(
	t *_TESTING.T,
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) *_PtyProxy_ {
	t.Helper()
	ptyProxy_spawned, error_spawn__PtyProxy__maybe := Spawn_PtyProxy(_SpawnApi_PtyProxy_{
		Id_WorkspacePty:                        1,
		ColumnCount_PtyTerminal:                columnCount_PtyTerminal,
		RowCount_PtyTerminal:                   rowCount_PtyTerminal,
		Path_PtyExec:                           path__PtyExec_test(t),
		Path_ShellBinary__PtyCommand:           "/bin/sh",
		DirectoryPath_PtyCommand:               t.TempDir(),
		EnvironmentVariables_PtyCommand:        []string{"PATH=/usr/bin:/bin", "TERM=xterm", "PS1=$ "},
		Count_ScrollbackLine__PtyTerminal:      100,
		Size_PostSnapshotBuffer__PtyProxy:      1024,
		Size_QueueBuffer__InputOrder_PtyWriter: 8,
	})
	if error_spawn__PtyProxy__maybe != nil {
		t.Fatalf(
			"Spawn_PtyProxy: %v",
			error_spawn__PtyProxy__maybe,
		)
	}
	// No reader is ever started here, so the session ends as a cancelled spawn does: _PtyTeardown_ closes the
	// master and returns without an outcome.
	t.Cleanup(func() {
		killAndAwait__PtyProcess_test(ptyProxy_spawned.PtyProcess)
		ptyProxy_spawned.PtyTeardown.HandleCancel()
	})
	return ptyProxy_spawned
}

func readUntilExitSignal__PtyDescriptor_test(
	t *_TESTING.T,
	fileDescriptor_master__Pty__shared *_OS.File,
) ([]byte, error) {
	t.Helper()
	_ = fileDescriptor_master__Pty__shared.SetReadDeadline(_TIME.Now().Add(DEADLINE__PtyDescriptor_test))
	var outputData_PtyDescriptor []byte
	readBuffer_PtyDescriptor := make([]byte, SIZE_READ_BUFFER__PtyReader)
	for {
		bytesRed_PtyDescriptor, exitSignal_PtyReader := fileDescriptor_master__Pty__shared.Read(readBuffer_PtyDescriptor)
		outputData_PtyDescriptor = append(
			outputData_PtyDescriptor,
			readBuffer_PtyDescriptor[:bytesRed_PtyDescriptor]...,
		)
		if exitSignal_PtyReader != nil {
			return outputData_PtyDescriptor, exitSignal_PtyReader
		}
	}
}

func flags__PtyDescriptor_test(
	t *_TESTING.T,
	fileDescriptor_target *_OS.File,
) (int, int) {
	t.Helper()
	rawConn_target, _ := fileDescriptor_target.SyscallConn()
	var statusFlags, descriptorFlags uintptr
	_ = rawConn_target.Control(func(fileDescriptor_raw uintptr) {
		statusFlags, _, _ = _SYSCALL.Syscall(
			_SYSCALL.SYS_FCNTL,
			fileDescriptor_raw,
			_SYSCALL.F_GETFL,
			0,
		)
		descriptorFlags, _, _ = _SYSCALL.Syscall(
			_SYSCALL.SYS_FCNTL,
			fileDescriptor_raw,
			_SYSCALL.F_GETFD,
			0,
		)
	})
	return int(statusFlags), int(descriptorFlags)
}
