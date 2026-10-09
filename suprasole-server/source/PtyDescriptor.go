package main

import (
	_OS "os"
	_EXEC "os/exec"
	_STRCONV "strconv"
	_SYSCALL "syscall"
	_UNSAFE "unsafe"
)

const (
	PATH_MULTIPLEXER__PtyDescriptor  = "/dev/ptmx"
	PATH_PREFIX_SLAVE__PtyDescriptor = "/dev/pts/"
	_TIOCGPTPEER                     = 0x5441
)

type _Winsize_PtyDescriptor_ struct {
	RowCount_PtyTerminal    uint16
	ColumnCount_PtyTerminal uint16
	_                       [2]uint16
}

func Open_PtyDescriptor(
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) (*_OS.File, *_OS.File, error) {
	fileDescriptor_master, error_open__PtyMultiplexer__maybe := _SYSCALL.Open(
		PATH_MULTIPLEXER__PtyDescriptor,
		_SYSCALL.O_RDWR|_SYSCALL.O_NOCTTY|_SYSCALL.O_CLOEXEC|_SYSCALL.O_NONBLOCK,
		0,
	)
	if error_open__PtyMultiplexer__maybe != nil {
		error_open__PtyDescriptor := _OS.NewSyscallError(
			"open "+PATH_MULTIPLEXER__PtyDescriptor,
			error_open__PtyMultiplexer__maybe,
		)
		return nil, nil, error_open__PtyDescriptor
	}
	var lockStatus_unlocked int32
	_, _, errorCode_TIOCSPTLCK := _SYSCALL.Syscall(
		_SYSCALL.SYS_IOCTL,
		uintptr(fileDescriptor_master),
		_SYSCALL.TIOCSPTLCK,
		uintptr(_UNSAFE.Pointer(&lockStatus_unlocked)),
	)
	if errorCode_TIOCSPTLCK != 0 {
		error_open__PtyDescriptor := abortOpen__PtyDescriptor(
			fileDescriptor_master,
			"TIOCSPTLCK",
			errorCode_TIOCSPTLCK,
		)
		return nil, nil, error_open__PtyDescriptor
	}
	var number_PtyDescriptor uint32
	_, _, errorCode_TIOCGPTN := _SYSCALL.Syscall(
		_SYSCALL.SYS_IOCTL,
		uintptr(fileDescriptor_master),
		_SYSCALL.TIOCGPTN,
		uintptr(_UNSAFE.Pointer(&number_PtyDescriptor)),
	)
	if errorCode_TIOCGPTN != 0 {
		error_open__PtyDescriptor := abortOpen__PtyDescriptor(
			fileDescriptor_master,
			"TIOCGPTN",
			errorCode_TIOCGPTN,
		)
		return nil, nil, error_open__PtyDescriptor
	}
	errorCode_TIOCSWINSZ := setWindowSize__PtyDescriptor(
		uintptr(fileDescriptor_master),
		columnCount_PtyTerminal,
		rowCount_PtyTerminal,
	)
	if errorCode_TIOCSWINSZ != 0 {
		error_open__PtyDescriptor := abortOpen__PtyDescriptor(
			fileDescriptor_master,
			"TIOCSWINSZ",
			errorCode_TIOCSWINSZ,
		)
		return nil, nil, error_open__PtyDescriptor
	}
	fileDescriptor_slave, _, errorCode_TIOCGPTPEER := _SYSCALL.Syscall(
		_SYSCALL.SYS_IOCTL,
		uintptr(fileDescriptor_master),
		_TIOCGPTPEER,
		_SYSCALL.O_RDWR|_SYSCALL.O_NOCTTY|_SYSCALL.O_CLOEXEC,
	)
	if errorCode_TIOCGPTPEER != 0 {
		error_open__PtyDescriptor := abortOpen__PtyDescriptor(
			fileDescriptor_master,
			"TIOCGPTPEER",
			errorCode_TIOCGPTPEER,
		)
		return nil, nil, error_open__PtyDescriptor
	}
	path_slave__PtyDescriptor := PATH_PREFIX_SLAVE__PtyDescriptor + _STRCONV.FormatUint(
		uint64(number_PtyDescriptor),
		10,
	)
	fileDescriptor_master__Pty__shared := _OS.NewFile(
		uintptr(fileDescriptor_master),
		PATH_MULTIPLEXER__PtyDescriptor,
	)
	fileDescriptor_slave__Pty := _OS.NewFile(
		fileDescriptor_slave,
		path_slave__PtyDescriptor,
	)
	return fileDescriptor_master__Pty__shared, fileDescriptor_slave__Pty, nil
}

func abortOpen__PtyDescriptor(
	fileDescriptor_master int,
	request_IOCTL string,
	errorCode_IOCTL _SYSCALL.Errno,
) error {
	_ = _SYSCALL.Close(fileDescriptor_master)
	return _OS.NewSyscallError(
		"ioctl "+request_IOCTL,
		errorCode_IOCTL,
	)
}

func AttachSlave_PtyDescriptor(
	ptyCommand_target *_EXEC.Cmd,
	fileDescriptor_slave__Pty *_OS.File,
) {
	ptyCommand_target.Stdin = fileDescriptor_slave__Pty
	ptyCommand_target.Stdout = fileDescriptor_slave__Pty
	ptyCommand_target.Stderr = fileDescriptor_slave__Pty
	ptyCommand_target.SysProcAttr = &_SYSCALL.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}
}

func Resize_PtyDescriptor(
	FileDescriptor_Master__Pty__shared *_OS.File,
	columnCount_PtyTerminal_next int,
	rowCount_PtyTerminal_next int,
) error {
	rawConn_Master, error_syscallConn__maybe := FileDescriptor_Master__Pty__shared.SyscallConn()
	if error_syscallConn__maybe != nil {
		return error_syscallConn__maybe
	}
	var errorCode_TIOCSWINSZ _SYSCALL.Errno
	error_control__maybe := rawConn_Master.Control(func(fileDescriptor_master uintptr) {
		errorCode_TIOCSWINSZ = setWindowSize__PtyDescriptor(
			fileDescriptor_master,
			columnCount_PtyTerminal_next,
			rowCount_PtyTerminal_next,
		)
	})
	if error_control__maybe != nil {
		return error_control__maybe
	}
	if errorCode_TIOCSWINSZ != 0 {
		return _OS.NewSyscallError(
			"ioctl TIOCSWINSZ",
			errorCode_TIOCSWINSZ,
		)
	}
	return nil
}

func setWindowSize__PtyDescriptor(
	fileDescriptor_master uintptr,
	columnCount_PtyTerminal int,
	rowCount_PtyTerminal int,
) _SYSCALL.Errno {
	winsize_next := &_Winsize_PtyDescriptor_{
		RowCount_PtyTerminal:    uint16(rowCount_PtyTerminal),
		ColumnCount_PtyTerminal: uint16(columnCount_PtyTerminal),
	}
	_, _, errorCode_TIOCSWINSZ := _SYSCALL.Syscall(
		_SYSCALL.SYS_IOCTL,
		fileDescriptor_master,
		_SYSCALL.TIOCSWINSZ,
		uintptr(_UNSAFE.Pointer(winsize_next)),
	)
	return errorCode_TIOCSWINSZ
}
