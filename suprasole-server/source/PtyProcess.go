package main

import (
	_BINARY "encoding/binary"
	_ERRORS "errors"
	_IO "io"
	_OS "os"
	_EXEC "os/exec"
	_FILEPATH "path/filepath"
	_STRINGS "strings"
	_SYNC "sync"
	_SYSCALL "syscall"
	_TIME "time"
)

const (
	// [phase 3] TIMEOUT_UNRESPONSIVE__PtyProcess = 1 * _TIME.Second
	// [phase 3] TIMEOUT_UNINTERRUPTIBLE__PtyProcess = 1 * _TIME.Second
	INTERVAL_REPEEK__PtyProcess         = 1 * _TIME.Second
	INTERVAL_REAP_ABANDONED__PtyProcess = 1 * _TIME.Second
	NAME_FILE__Pidfd                    = "pidfd"
	SIZE_REPORT__PtyExec                = 16
)

type _Step__PtyExec_ uint64

const (
	ARGUMENTS__Step__PtyExec      _Step__PtyExec_ = 1
	SETSID__Step__PtyExec         _Step__PtyExec_ = 2
	TIOCSCTTY__Step__PtyExec      _Step__PtyExec_ = 3
	CHDIR__Step__PtyExec          _Step__PtyExec_ = 4
	RT_SIGPROCMASK__Step__PtyExec _Step__PtyExec_ = 5
	EXECVE__Step__PtyExec         _Step__PtyExec_ = 6
)

type _Status_Report__PtyExec_ uint8

const (
	ABSENT__Status_Report__PtyExec   _Status_Report__PtyExec_ = 0
	REPORTED__Status_Report__PtyExec _Status_Report__PtyExec_ = 1
)

type _Report_PtyExec_ struct {
	Step  _Step__PtyExec_
	Errno _SYSCALL.Errno
}

type _Command_PtyProcess_ struct {
	Path_Executable string
	Argv            []string
	Path_Directory  string
	Environment     []string
}

type _PrepareApi__Command_PtyProcess_ struct {
	Path_ShellBinary string
	Path_Directory   string
	Environment      []string
}

const (
	SYS_PIDFD_SEND_SIGNAL__Linux      = 424
	PIDFD_SIGNAL_THREAD_GROUP__Linux  = 1 << 1
	PIDFD_SIGNAL_PROCESS_GROUP__Linux = 1 << 2
	P_PIDFD__WAITID__Linux            = 3
	POLLIN__Linux                     = 0x1
	CLD_EXITED__SIGCHLD__Linux        = 1
	CLD_KILLED__SIGCHLD__Linux        = 2
	CLD_DUMPED__SIGCHLD__Linux        = 3
	CLD_TRAPPED__SIGCHLD__Linux       = 4
	CLD_STOPPED__SIGCHLD__Linux       = 5
)

type _SigInfo_Waitid_ struct {
	Signo  int32
	Errno  int32
	Code   int32
	_pad   int32
	Pid    int32
	Uid    uint32
	Status int32
	_rest  [128 - 28]byte
}

type _PollFd_Ppoll_ struct {
	FileDescriptor int32
	Events         int16
	Revents        int16
}

type _PtyProcess_ struct {
	Channel_FinalState chan struct{}
	Mutex              _SYNC.Mutex
	State_state        _State_PtyProcess_
}

type _State_PtyProcess_ interface {
	Signal(signalNumber_PtyProcess _SYSCALL.Signal, flags__PIDFD_SEND_SIGNAL uintptr) error
	// [phase 3] ProbeStatus_Unresponsive() _TeardownReport_PtyProcess_
}

type _Alive__State_PtyProcess_ struct {
	FileDescriptor_Pidfd *_OS.File
	Pid                  int
}

type _Zombie_Withheld__State_PtyProcess_ struct {
	FileDescriptor_Pidfd *_OS.File
	Pid                  int
}

type _Zombie__State_PtyProcess_ struct {
	FileDescriptor_Pidfd *_OS.File
	Pid                  int
	Termination          _Termination_PtyProcess_
}

type _Reaped__State_PtyProcess_ struct {
	Termination _Termination_PtyProcess_
}

type _SystemError__State_PtyProcess_ struct {
	SystemError_Waitid error
}

type _Termination_PtyProcess_ interface {
	Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_
}

type _Exited__Termination_PtyProcess_ struct {
	ExitCode_PtyProcess int
}

type _Signaled__Termination_PtyProcess_ struct {
	Signal_PtyProcess int
	Status_CoreDump   _Status_CoreDump__PtyProcess_
}

type _Status_CoreDump__PtyProcess_ uint8

const (
	NOT_DUMPED__Status_CoreDump__PtyProcess _Status_CoreDump__PtyProcess_ = 0
	DUMPED__Status_CoreDump__PtyProcess     _Status_CoreDump__PtyProcess_ = 1
)

type _Status_TerminationReport__PtyProcess_ uint8

const (
	ABSENT__Status_TerminationReport__PtyProcess   _Status_TerminationReport__PtyProcess_ = 0
	REPORTED__Status_TerminationReport__PtyProcess _Status_TerminationReport__PtyProcess_ = 1
	WITHHELD__Status_TerminationReport__PtyProcess _Status_TerminationReport__PtyProcess_ = 2
	FAILED__Status_TerminationReport__PtyProcess   _Status_TerminationReport__PtyProcess_ = 3
)

type _Status_PollIn__Pidfd_ uint8

const (
	CLEAR__Status_PollIn__Pidfd _Status_PollIn__Pidfd_ = 0
	SET__Status_PollIn__Pidfd   _Status_PollIn__Pidfd_ = 1
)

func Prepare__Command_PtyProcess(
	api _PrepareApi__Command_PtyProcess_,
) (_Command_PtyProcess_, error) {
	error_nul__PtyCommand__maybe := check__NUL_PtyCommand(
		api.Path_ShellBinary,
		api.Path_Directory,
		api.Environment,
	)
	if error_nul__PtyCommand__maybe != nil {
		return _Command_PtyProcess_{}, error_nul__PtyCommand__maybe
	}
	path_Executable__ShellBinary, error_resolve__ShellBinary__maybe := resolve__Path_ShellBinary(api.Path_ShellBinary)
	if error_resolve__ShellBinary__maybe != nil {
		return _Command_PtyProcess_{}, error_resolve__ShellBinary__maybe
	}
	command_PtyProcess_result := _Command_PtyProcess_{
		Path_Executable: path_Executable__ShellBinary,
		Argv:            []string{api.Path_ShellBinary},
		Path_Directory:  api.Path_Directory,
		Environment:     api.Environment,
	}
	return command_PtyProcess_result, nil
}

func resolve__Path_ShellBinary(
	path_ShellBinary__PtyCommand string,
) (string, error) {
	name_Base__ShellBinary := _FILEPATH.Base(path_ShellBinary__PtyCommand)
	if name_Base__ShellBinary == path_ShellBinary__PtyCommand {
		path_resolved__ShellBinary, error_lookPath__ShellBinary__maybe := _EXEC.LookPath(path_ShellBinary__PtyCommand)
		return path_resolved__ShellBinary, error_lookPath__ShellBinary__maybe
	} else {
		return path_ShellBinary__PtyCommand, nil
	}
}

func check__NUL_PtyCommand(
	path_ShellBinary__PtyCommand string,
	directoryPath_PtyCommand string,
	environmentVariables_PtyCommand []string,
) error {
	index__NUL__ShellBinary := _STRINGS.IndexByte(
		path_ShellBinary__PtyCommand,
		0,
	)
	if index__NUL__ShellBinary != -1 {
		pathError_forkExec := &_OS.PathError{
			Op:   "fork/exec",
			Path: path_ShellBinary__PtyCommand,
			Err:  _SYSCALL.EINVAL,
		}
		return pathError_forkExec
	}
	index__NUL__Directory := _STRINGS.IndexByte(
		directoryPath_PtyCommand,
		0,
	)
	if index__NUL__Directory != -1 {
		pathError_chdir := &_OS.PathError{
			Op:   "chdir",
			Path: directoryPath_PtyCommand,
			Err:  _SYSCALL.EINVAL,
		}
		return pathError_chdir
	}
	for _, environmentVariable_PtyCommand := range environmentVariables_PtyCommand {
		index__NUL__EnvironmentVariable := _STRINGS.IndexByte(
			environmentVariable_PtyCommand,
			0,
		)
		if index__NUL__EnvironmentVariable != -1 {
			error_environmentVariable__NUL := _ERRORS.New("exec: environment variable contains NUL")
			return error_environmentVariable__NUL
		}
	}
	return nil
}

func Spawn_PtyProcess(
	path_PtyExec string,
	command_PtyProcess _Command_PtyProcess_,
	fileDescriptor_slave__Pty *_OS.File,
) (*_PtyProcess_, error) {
	pid_PtyProcess, fileDescriptor_pidfd__PtyProcess, error_launchPtyExec__maybe := launchPtyExec__Command_PtyProcess(
		path_PtyExec,
		command_PtyProcess,
		fileDescriptor_slave__Pty,
	)
	error_setReadDeadline__Pidfd__maybe := fileDescriptor_pidfd__PtyProcess.SetReadDeadline(_TIME.Time{})
	if error_launchPtyExec__maybe != nil && nil == fileDescriptor_pidfd__PtyProcess {
		return nil, error_launchPtyExec__maybe
	} else if error_launchPtyExec__maybe != nil && fileDescriptor_pidfd__PtyProcess != nil {
		go reapAbandoned__PtyProcess_Pidfd(fileDescriptor_pidfd__PtyProcess)
		return nil, error_launchPtyExec__maybe
	} else if nil == error_launchPtyExec__maybe && fileDescriptor_pidfd__PtyProcess != nil && error_setReadDeadline__Pidfd__maybe != nil {
		_ = sendSignal_Pidfd(
			fileDescriptor_pidfd__PtyProcess,
			_SYSCALL.SIGKILL,
			PIDFD_SIGNAL_PROCESS_GROUP__Linux,
		)
		go reapAbandoned__PtyProcess_Pidfd(fileDescriptor_pidfd__PtyProcess)
		pathError_netpoll__Pidfd := &_OS.PathError{
			Op:   "netpoll",
			Path: NAME_FILE__Pidfd,
			Err:  error_setReadDeadline__Pidfd__maybe,
		}
		return nil, pathError_netpoll__Pidfd
	} else if nil == error_launchPtyExec__maybe && fileDescriptor_pidfd__PtyProcess != nil && nil == error_setReadDeadline__Pidfd__maybe {
		ptyProcess_result := &_PtyProcess_{
			Channel_FinalState: make(chan struct{}),
			State_state: &_Alive__State_PtyProcess_{
				Pid:                  pid_PtyProcess,
				FileDescriptor_Pidfd: fileDescriptor_pidfd__PtyProcess,
			},
		}
		return ptyProcess_result, nil
	} else {
		// nil == error_launchPtyExec__maybe && nil == fileDescriptor_pidfd__PtyProcess
		panic("invalid path: Spawn_PtyProcess")
	}
}

func launchPtyExec__Command_PtyProcess(
	path_PtyExec string,
	command_PtyProcess _Command_PtyProcess_,
	fileDescriptor_slave__Pty *_OS.File,
) (int, *_OS.File, error) {
	fileDescriptor_report__PtyExec__read, fileDescriptor_report__PtyExec__write, error_pipe__Report__maybe := _OS.Pipe()
	if error_pipe__Report__maybe != nil {
		return 0, nil, error_pipe__Report__maybe
	}
	argv_PtyExec := append(
		[]string{
			path_PtyExec,
			command_PtyProcess.Path_Directory,
			command_PtyProcess.Path_Executable,
		},
		command_PtyProcess.Argv...,
	)
	fileDescriptor_pidfd := -1
	fileDescriptor_slave := fileDescriptor_slave__Pty.Fd()
	procAttr_PtyExec := &_SYSCALL.ProcAttr{
		Env: command_PtyProcess.Environment,
		Files: []uintptr{
			fileDescriptor_slave,
			fileDescriptor_slave,
			fileDescriptor_slave,
			fileDescriptor_report__PtyExec__write.Fd(),
		},
		Sys: &_SYSCALL.SysProcAttr{
			PidFD: &fileDescriptor_pidfd,
		},
	}
	pid_PtyProcess, error_forkExec__maybe := _SYSCALL.ForkExec(
		path_PtyExec,
		argv_PtyExec,
		procAttr_PtyExec,
	)
	_ = fileDescriptor_report__PtyExec__write.Close()
	if error_forkExec__maybe != nil {
		_ = fileDescriptor_report__PtyExec__read.Close()
		pathError_forkExec := &_OS.PathError{
			Op:   "fork/exec",
			Path: path_PtyExec,
			Err:  error_forkExec__maybe,
		}
		return 0, nil, pathError_forkExec
	}
	fileDescriptor_pidfd__PtyProcess := wrap__Pidfd(fileDescriptor_pidfd)
	status_Report__PtyExec, report_PtyExec := await__Report_PtyExec(fileDescriptor_report__PtyExec__read)
	if ABSENT__Status_Report__PtyExec == status_Report__PtyExec {
		return pid_PtyProcess, fileDescriptor_pidfd__PtyProcess, nil
	} else if REPORTED__Status_Report__PtyExec == status_Report__PtyExec {
		pathError_PtyExec := resolve__PathError_PtyExec(
			report_PtyExec,
			command_PtyProcess,
		)
		return 0, fileDescriptor_pidfd__PtyProcess, pathError_PtyExec
	} else {
		panic("invalid path: launchPtyExec__Command_PtyProcess")
	}
}

func wrap__Pidfd(
	fileDescriptor_pidfd int,
) *_OS.File {
	error_setNonblock__Pidfd__maybe := _SYSCALL.SetNonblock(
		fileDescriptor_pidfd,
		true,
	)
	fileDescriptor_pidfd__PtyProcess := _OS.NewFile(
		uintptr(fileDescriptor_pidfd),
		NAME_FILE__Pidfd,
	)
	if nil == error_setNonblock__Pidfd__maybe {
		return fileDescriptor_pidfd__PtyProcess
	} else {
		// error_setNonblock__Pidfd__maybe != nil
		panic("invalid path: wrap__Pidfd")
	}
}

func await__Report_PtyExec(
	fileDescriptor_report__PtyExec__read *_OS.File,
) (_Status_Report__PtyExec_, _Report_PtyExec_) {
	var bytes_Report__PtyExec [SIZE_REPORT__PtyExec]byte
	_, error_readReport__PtyExec__maybe := _IO.ReadFull(
		fileDescriptor_report__PtyExec__read,
		bytes_Report__PtyExec[:],
	)
	_ = fileDescriptor_report__PtyExec__read.Close()
	if _IO.EOF == error_readReport__PtyExec__maybe {
		return ABSENT__Status_Report__PtyExec, _Report_PtyExec_{}
	} else if nil == error_readReport__PtyExec__maybe {
		report_PtyExec := _Report_PtyExec_{
			Step:  _Step__PtyExec_(_BINARY.LittleEndian.Uint64(bytes_Report__PtyExec[0:8])),
			Errno: _SYSCALL.Errno(_BINARY.LittleEndian.Uint64(bytes_Report__PtyExec[8:16])),
		}
		return REPORTED__Status_Report__PtyExec, report_PtyExec
	} else {
		// _IO.ErrUnexpectedEOF == error_readReport__PtyExec__maybe
		// error_readReport__PtyExec__maybe: any other read error
		panic("invalid path: await__Report_PtyExec")
	}
}

func resolve__PathError_PtyExec(
	report_PtyExec _Report_PtyExec_,
	command_PtyProcess _Command_PtyProcess_,
) error {
	if CHDIR__Step__PtyExec == report_PtyExec.Step {
		pathError_chdir := &_OS.PathError{
			Op:   "chdir",
			Path: command_PtyProcess.Path_Directory,
			Err:  report_PtyExec.Errno,
		}
		return pathError_chdir
	} else if EXECVE__Step__PtyExec == report_PtyExec.Step {
		pathError_execve := &_OS.PathError{
			Op:   "fork/exec",
			Path: command_PtyProcess.Path_Executable,
			Err:  report_PtyExec.Errno,
		}
		return pathError_execve
	} else {
		// ARGUMENTS / SETSID / TIOCSCTTY / RT_SIGPROCMASK __Step__PtyExec
		panic("invalid path: resolve__PathError_PtyExec")
	}
}

func reapAbandoned__PtyProcess_Pidfd(
	fileDescriptor_pidfd__PtyProcess *_OS.File,
) {
	rawConn_Pidfd, _ := fileDescriptor_pidfd__PtyProcess.SyscallConn()
	for {
		var sigInfo_reaped _SigInfo_Waitid_
		var errorCode__WAITID _SYSCALL.Errno
		error_control__maybe := rawConn_Pidfd.Control(
			func(fileDescriptor_pidfd uintptr) {
				sigInfo_reaped, errorCode__WAITID = waitid__PtyProcess(
					fileDescriptor_pidfd,
					_SYSCALL.WEXITED|_SYSCALL.WNOHANG,
				)
			},
		)
		if nil == error_control__maybe && 0 == errorCode__WAITID && 0 == sigInfo_reaped.Pid {
			_TIME.Sleep(INTERVAL_REAP_ABANDONED__PtyProcess)
		} else if nil == error_control__maybe && 0 == errorCode__WAITID && sigInfo_reaped.Pid != 0 && (CLD_EXITED__SIGCHLD__Linux == sigInfo_reaped.Code || CLD_KILLED__SIGCHLD__Linux == sigInfo_reaped.Code || CLD_DUMPED__SIGCHLD__Linux == sigInfo_reaped.Code) {
			break
		} else {
			// error_control__maybe != nil
			// errorCode__WAITID != 0
			// 0 == errorCode__WAITID && sigInfo_reaped.Pid != 0 && CLD_TRAPPED__SIGCHLD__Linux == sigInfo_reaped.Code
			// CLD_STOPPED / CLD_CONTINUED: not selected without WSTOPPED / WCONTINUED
			panic("invalid path: reapAbandoned__PtyProcess_Pidfd")
		}
	}
	_ = fileDescriptor_pidfd__PtyProcess.Close()
}

func (this *_Alive__State_PtyProcess_) Signal(
	signalNumber_PtyProcess _SYSCALL.Signal,
	flags__PIDFD_SEND_SIGNAL uintptr,
) error {
	return sendSignal_Pidfd(
		this.FileDescriptor_Pidfd,
		signalNumber_PtyProcess,
		flags__PIDFD_SEND_SIGNAL,
	)
}

func (this *_Zombie_Withheld__State_PtyProcess_) Signal(
	signalNumber_PtyProcess _SYSCALL.Signal,
	flags__PIDFD_SEND_SIGNAL uintptr,
) error {
	return sendSignal_Pidfd(
		this.FileDescriptor_Pidfd,
		signalNumber_PtyProcess,
		flags__PIDFD_SEND_SIGNAL,
	)
}

func (this *_Zombie__State_PtyProcess_) Signal(
	signalNumber_PtyProcess _SYSCALL.Signal,
	flags__PIDFD_SEND_SIGNAL uintptr,
) error {
	return sendSignal_Pidfd(
		this.FileDescriptor_Pidfd,
		signalNumber_PtyProcess,
		flags__PIDFD_SEND_SIGNAL,
	)
}

func (this *_Reaped__State_PtyProcess_) Signal(
	_ _SYSCALL.Signal,
	_ uintptr,
) error {
	return _OS.ErrProcessDone
}

func (this *_SystemError__State_PtyProcess_) Signal(
	_ _SYSCALL.Signal,
	_ uintptr,
) error {
	return this.SystemError_Waitid
}

func sendSignal_Pidfd(
	fileDescriptor_pidfd__PtyProcess *_OS.File,
	signalNumber_PtyProcess _SYSCALL.Signal,
	flags__PIDFD_SEND_SIGNAL uintptr,
) error {
	rawConn_Pidfd, _ := fileDescriptor_pidfd__PtyProcess.SyscallConn()
	var errorCode__PIDFD_SEND_SIGNAL _SYSCALL.Errno
	error_control__maybe := rawConn_Pidfd.Control(
		func(fileDescriptor_pidfd uintptr) {
			_, _, errorCode__PIDFD_SEND_SIGNAL = _SYSCALL.Syscall6(
				SYS_PIDFD_SEND_SIGNAL__Linux,
				fileDescriptor_pidfd,
				uintptr(signalNumber_PtyProcess),
				0,
				flags__PIDFD_SEND_SIGNAL,
				0,
				0,
			)
		},
	)
	if nil == error_control__maybe && errorCode__PIDFD_SEND_SIGNAL != 0 {
		return _OS.NewSyscallError(
			"pidfd_send_signal",
			errorCode__PIDFD_SEND_SIGNAL,
		)
	} else if nil == error_control__maybe && 0 == errorCode__PIDFD_SEND_SIGNAL {
		return nil
	} else {
		// error_control__maybe != nil
		panic("invalid path: sendSignal_Pidfd")
	}
}

func (this *_Reaped__State_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return this.Termination.Resolve__ExitOutcome_PtyProxy()
}

func (this *_SystemError__State_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return _SystemError_Process__ExitOutcome_PtyProxy_{
		SystemError_PtyProcess: this.SystemError_Waitid,
	}
}

func (this _Exited__Termination_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	if 0 == this.ExitCode_PtyProcess {
		return _Success__ExitOutcome_PtyProxy_{}
	}
	return _Failure__ExitOutcome_PtyProxy_{
		ExitCode_PtyProcess: this.ExitCode_PtyProcess,
	}
}

func (this _Signaled__Termination_PtyProcess_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	return _Killed__ExitOutcome_PtyProxy_{
		ExitSignal_PtyProcess: this.Signal_PtyProcess,
	}
}
