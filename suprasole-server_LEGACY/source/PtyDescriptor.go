package main

import (
	_OS "os"
	_STRCONV "strconv"
	_SYSCALL "syscall"
	_UNSAFE "unsafe"
)

const (
	PATH_MULTIPLEXER__PtyDescriptor  = "/dev/ptmx"
	PATH_PREFIX_SLAVE__PtyDescriptor = "/dev/pts/"
	TIOCGPTPEER__Linux               = 0x5441
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
	_, _, errorCode__TIOCSPTLCK := _SYSCALL.Syscall(
		_SYSCALL.SYS_IOCTL,
		uintptr(fileDescriptor_master),
		_SYSCALL.TIOCSPTLCK,
		uintptr(_UNSAFE.Pointer(&lockStatus_unlocked)),
	)
	if errorCode__TIOCSPTLCK != 0 {
		error_open__PtyDescriptor := abortOpen__PtyDescriptor(
			fileDescriptor_master,
			"TIOCSPTLCK",
			errorCode__TIOCSPTLCK,
		)
		return nil, nil, error_open__PtyDescriptor
	}
	var number_PtyDescriptor uint32
	_, _, errorCode__TIOCGPTN := _SYSCALL.Syscall(
		_SYSCALL.SYS_IOCTL,
		uintptr(fileDescriptor_master),
		_SYSCALL.TIOCGPTN,
		uintptr(_UNSAFE.Pointer(&number_PtyDescriptor)),
	)
	if errorCode__TIOCGPTN != 0 {
		error_open__PtyDescriptor := abortOpen__PtyDescriptor(
			fileDescriptor_master,
			"TIOCGPTN",
			errorCode__TIOCGPTN,
		)
		return nil, nil, error_open__PtyDescriptor
	}
	errorCode__TIOCSWINSZ := setWindowSize__PtyDescriptor(
		uintptr(fileDescriptor_master),
		columnCount_PtyTerminal,
		rowCount_PtyTerminal,
	)
	if errorCode__TIOCSWINSZ != 0 {
		error_open__PtyDescriptor := abortOpen__PtyDescriptor(
			fileDescriptor_master,
			"TIOCSWINSZ",
			errorCode__TIOCSWINSZ,
		)
		return nil, nil, error_open__PtyDescriptor
	}
	fileDescriptor_slave, _, errorCode__TIOCGPTPEER := _SYSCALL.Syscall(
		_SYSCALL.SYS_IOCTL,
		uintptr(fileDescriptor_master),
		TIOCGPTPEER__Linux,
		_SYSCALL.O_RDWR|_SYSCALL.O_NOCTTY|_SYSCALL.O_CLOEXEC,
	)
	if errorCode__TIOCGPTPEER != 0 {
		error_open__PtyDescriptor := abortOpen__PtyDescriptor(
			fileDescriptor_master,
			"TIOCGPTPEER",
			errorCode__TIOCGPTPEER,
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
	request__IOCTL string,
	errorCode__IOCTL _SYSCALL.Errno,
) error {
	_ = _SYSCALL.Close(fileDescriptor_master)
	return _OS.NewSyscallError(
		"ioctl "+request__IOCTL,
		errorCode__IOCTL,
	)
}

func Resize_PtyDescriptor(
	FileDescriptor_Master__Pty__shared *_OS.File,
	columnCount_PtyTerminal_next int,
	rowCount_PtyTerminal_next int,
) error {
	rawConn_Master, error_syscallConn__maybe := FileDescriptor_Master__Pty__shared.SyscallConn()
	// [unreachable by construction] SyscallConn fails only for a nil *os.File; the master never is
	if error_syscallConn__maybe != nil {
		return error_syscallConn__maybe
	}
	var errorCode__TIOCSWINSZ _SYSCALL.Errno
	error_control__maybe := rawConn_Master.Control(func(fileDescriptor_master uintptr) {
		errorCode__TIOCSWINSZ = setWindowSize__PtyDescriptor(
			fileDescriptor_master,
			columnCount_PtyTerminal_next,
			rowCount_PtyTerminal_next,
		)
	})
	if error_control__maybe != nil {
		return error_control__maybe
	} else if errorCode__TIOCSWINSZ != 0 {
		return _OS.NewSyscallError(
			"ioctl TIOCSWINSZ",
			errorCode__TIOCSWINSZ,
		)
	} else {
		return nil
	}
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
	_, _, errorCode__TIOCSWINSZ := _SYSCALL.Syscall(
		_SYSCALL.SYS_IOCTL,
		fileDescriptor_master,
		_SYSCALL.TIOCSWINSZ,
		uintptr(_UNSAFE.Pointer(winsize_next)),
	)
	return errorCode__TIOCSWINSZ
}
