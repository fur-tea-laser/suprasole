// suprasole-pty-exec — the spawn helper.
// Semantics: documentation/launchPtyExec.md, documentation/PtyExec.md, issue §3B.
//
// WHAT IT IS
//   launchPtyExec__Command_PtyProcess (source/PtyProcess.go) starts this program with
//   syscall.ForkExec. This program then turns itself into the PTY program (the shell):
//     reset signal dispositions -> setsid -> take the terminal -> chdir -> clear the signal mask
//     -> execve the shell.
//   Every step that can block on a client-chosen path (chdir, execve) happens here, in a
//   separate process — never inside Go's vfork window in the server.
//
// WHAT IT RECEIVES
//   argv  = [helper, directory, executable, shell argv[0], shell argv[1], ...]
//   envp  = the shell's environment, exactly as the client sent it
//   fd 0, 1, 2 = the PTY slave          fd 3 = write end of the report pipe
//
// WHAT IT TELLS THE SERVER
//   Success: the shell's execve closes fd 3 (marked close-on-exec below); the server reads EOF.
//   Failure: one 16-byte write to fd 3 = {step number, errno}; then exit with code 127.
//
// WHY ASSEMBLY, AND NO GO RUNTIME
//   Built with: go build -ldflags='-E main.Entry_PtyExec'. The -E flag makes Entry_PtyExec the
//   ELF entry point, so the kernel jumps straight here. The Go runtime is linked in but never
//   starts: no threads, no signal handlers, no garbage collector, and a client-sent GODEBUG or
//   GOGC configures nothing.
//   Built WITHOUT -E, the empty Go main in PtyExec.go runs instead: it exits 0, closing fd 3
//   without a report, so every spawn falsely "succeeds" and the PTY resolves as Success at once.
//   Nothing detects that at run time; building with -E is the build and launcher's job (§5A.3).
//
// HOW TO READ THIS FILE (Go's arm64 assembler, just what this file uses)
//   MOVD  src, dst        Copy a 64-bit value. Operands flow LEFT TO RIGHT: source first.
//   $25, $NAME            An immediate: the number itself, not memory.
//   $sym(SB)              The ADDRESS of a static symbol (SB = "static base" pseudo-register).
//   8(R20)                The 64-bit value in memory at address R20 + 8.
//   MOVBU 0(R0), R1       Load ONE byte from address R0 into R1 (zero-extended).
//   ADD   $8, RSP, R20    R20 = RSP + 8.              LSL $3, R21, R21  =  R21 = R21 << 3 (x8).
//   NEG   R0, R23         R23 = -R0.
//   CMP   $4, R19         Compare R19 with 4; sets flags for the next branch.
//   BLT   label           Branch if that comparison found "less than".
//   CBZ   R1, label       Branch if R1 is zero.
//   B     label           Branch always.
//   SVC                   System call. The kernel runs syscall number R8 with arguments
//                         R0, R1, R2, R3 and returns the result in R0. Every other register
//                         comes back unchanged (verified: arm64 kernel_exit restores x1-x29).
//   CMN $4095, R0 / BCS   The failure test. Linux returns -errno (-4095..-1) on failure.
//                         Adding 4095 to such a value overflows as an unsigned number, which
//                         sets the carry flag; BCS ("branch if carry set") fires exactly then.
//
// REGISTERS THIS FILE KEEPS (survive every SVC, see above)
//   R19 = argc        R20 = &argv[0]        R21 = &envp[0]
//   R22 = number of the step being attempted (goes into the report)
//   R23, R24 = report assembly.   R25 = signal-number counter while resetting dispositions.
//   Never touched: R18 (platform), R26-R28 (reserved by Go).

#include "textflag.h"

// Linux arm64 syscall numbers (the generic table; Go's zsysnum_linux_arm64.go agrees).
#define SYS_FCNTL__Linux 25
#define SYS_IOCTL__Linux 29
#define SYS_CHDIR__Linux 49
#define SYS_WRITE__Linux 64
#define SYS_EXIT_GROUP__Linux 94
#define SYS_RT_SIGACTION__Linux 134
#define SYS_RT_SIGPROCMASK__Linux 135
#define SYS_SETSID__Linux 157
#define SYS_EXECVE__Linux 221

// Arguments those syscalls take here.
#define F_SETFD__FCNTL__Linux 2
#define FD_CLOEXEC__FCNTL__Linux 1
#define TIOCSCTTY__IOCTL__Linux 0x540E
#define SIG_SETMASK__RT_SIGPROCMASK__Linux 2
#define EINVAL__Linux 22
#define SIGKILL__Linux 9
#define SIGSTOP__Linux 19
#define NSIG__Linux 64

// This program's contract with launchPtyExec__Command_PtyProcess.
#define FILE_DESCRIPTOR__REPORT__PtyExec 3
#define COUNT_ARGUMENT__MINIMUM__PtyExec 4
#define EXIT_CODE__FAILURE__PtyExec 127
#define SIZE_REPORT__PtyExec 16

// Step numbers written into the report. Must match _Step__PtyExec_ in source/PtyProcess.go.
#define ARGUMENTS__Step__PtyExec 1
#define SETSID__Step__PtyExec 2
#define TIOCSCTTY__Step__PtyExec 3
#define CHDIR__Step__PtyExec 4
#define RT_SIGPROCMASK__Step__PtyExec 5
#define EXECVE__Step__PtyExec 6
#define RT_SIGACTION__Step__PtyExec 7

// An all-zero signal set: 8 bytes = 64 bits = one bit per signal, none set.
// Read-only data, containing no Go pointers (RODATA|NOPTR).
DATA ·sigset_empty__PtyExec+0(SB)/8, $0
GLOBL ·sigset_empty__PtyExec(SB), RODATA|NOPTR, $8

// An all-zero struct sigaction (32 bytes: handler, flags, restorer, mask on arm64). Handler 0 is
// SIG_DFL, so passing this resets a signal to its default action. Zero-initialised, never written.
GLOBL ·sigaction_default__PtyExec(SB), NOPTR, $32

// The 16-byte failure report {step, errno}: zero-initialised, writable, no Go pointers.
// Kept in static memory so this program never touches its stack.
GLOBL ·report__PtyExec(SB), NOPTR, $SIZE_REPORT__PtyExec

// The ELF entry point. NOSPLIT|NOFRAME: no Go stack check and no stack frame, because nothing
// here may ever enter the Go runtime. $0-0: no locals, no arguments (go vet checks this
// against the declaration in PtyExec.go).
TEXT ·Entry_PtyExec(SB), NOSPLIT|NOFRAME, $0-0
	// At entry the kernel has laid out the stack as:
	//   RSP -> argc, argv[0], ..., argv[argc-1], NULL, envp[0], ..., NULL
	// Find argc, argv and envp; nothing else is needed from the start-up state.
	MOVD	0(RSP), R19         // R19 = argc: the first word on the stack
	ADD	$8, RSP, R20         // R20 = &argv[0]: the word right after argc
	ADD	$1, R19, R21         // R21 = argc + 1: argv's entries plus its NULL terminator
	LSL	$3, R21, R21         // R21 = (argc + 1) * 8: 8 bytes per pointer
	ADD	R20, R21, R21        // R21 = &envp[0]: right after argv's NULL

// Mark fd 3 close-on-exec, so the shell's execve closes it: EOF then means "the shell started".
// Fails only if fd 3 is not open (someone ran this program by hand). There is then nowhere to
// report, so exit 127 silently. Unreachable from the server: fd 3 is always passed.
closeOnExec__Report:
	MOVD	$FILE_DESCRIPTOR__REPORT__PtyExec, R0  // argument 1: fd 3
	MOVD	$F_SETFD__FCNTL__Linux, R1            // argument 2: F_SETFD (set descriptor flags)
	MOVD	$FD_CLOEXEC__FCNTL__Linux, R2         // argument 3: FD_CLOEXEC
	MOVD	$SYS_FCNTL__Linux, R8                 // syscall: fcntl
	SVC
	CMN	$4095, R0                            // failed? (carry set = -errno came back)
	BCS	exit__Failure                        // yes: exit 127 without a report

// The server always passes at least 4 arguments: helper, directory, executable, argv[0].
// Fewer means a manual run. Report step ARGUMENTS with EINVAL (R0 holds -EINVAL, the same
// shape a failing syscall leaves, so the report code below needs no special case).
checkCount__Arguments:
	MOVD	$ARGUMENTS__Step__PtyExec, R22          // step being attempted: ARGUMENTS
	MOVD	$-EINVAL__Linux, R0                     // R0 = -22, as if a syscall had failed
	CMP	$COUNT_ARGUMENT__MINIMUM__PtyExec, R19   // compare argc with 4
	BLT	report__Failure                         // argc < 4: report and exit

// Reset every signal's disposition to its default action (node-pty and VTE do the same). Caught
// handlers are already gone (execve resets them), but an IGNORED signal survives execve: a server
// started under nohup would otherwise hand the shell an ignored SIGHUP. SIGKILL and SIGSTOP are
// skipped because they cannot be changed (the kernel answers EINVAL). Cannot fail otherwise: every
// signal number is valid and the struct is constant. All signals are still blocked here (the server
// blocked them around the fork), so nothing can arrive while the dispositions change.
resetDispositions:
	MOVD	$RT_SIGACTION__Step__PtyExec, R22  // step: RT_SIGACTION
	MOVD	$1, R25                            // R25 = signal number, starting at 1
resetDispositions__next:
	CMP	$SIGKILL__Linux, R25                // SIGKILL?
	BEQ	resetDispositions__advance          // yes: skip it
	CMP	$SIGSTOP__Linux, R25                // SIGSTOP?
	BEQ	resetDispositions__advance          // yes: skip it
	MOVD	R25, R0                            // argument 1: the signal number
	MOVD	$·sigaction_default__PtyExec(SB), R1 // argument 2: address of the all-zero sigaction
	MOVD	$0, R2                             // argument 3: NULL, old action not wanted
	MOVD	$8, R3                             // argument 4: signal-set size, 8 bytes
	MOVD	$SYS_RT_SIGACTION__Linux, R8       // syscall: rt_sigaction
	SVC
	CMN	$4095, R0                           // failed?
	BCS	report__Failure                     // yes: report and exit
resetDispositions__advance:
	ADD	$1, R25, R25                        // next signal
	CMP	$NSIG__Linux, R25                   // compare with 64, the highest signal number
	BLE	resetDispositions__next             // R25 <= 64: continue

// Start a new session with this process as its leader; it has no controlling terminal yet.
// Cannot fail here: this process is a fresh child (not a leader), and its newly allocated PID
// is not in use as any process-group ID.
setsid:
	MOVD	$SETSID__Step__PtyExec, R22  // step: SETSID
	MOVD	$SYS_SETSID__Linux, R8      // syscall: setsid (no arguments)
	SVC
	CMN	$4095, R0                  // failed?
	BCS	report__Failure            // yes: report and exit

// Make fd 0 (the PTY slave) this session's controlling terminal. The third argument 0 means
// "never take a terminal that belongs to another session" (Go passes 1, which lets
// CAP_SYS_ADMIN steal it). Cannot fail here unless a security module (SELinux, TOMOYO; Landlock
// from 6.10) denies the ioctl, and the launcher guarantees none does (issue §5A.2). Otherwise
// setsid just ran, and the slave is fresh, read-write, and nobody's terminal.
tiocsctty:
	MOVD	$TIOCSCTTY__Step__PtyExec, R22  // step: TIOCSCTTY
	MOVD	$0, R0                         // argument 1: fd 0 (the slave)
	MOVD	$TIOCSCTTY__IOCTL__Linux, R1   // argument 2: TIOCSCTTY (set controlling terminal)
	MOVD	$0, R2                         // argument 3: 0 = never steal
	MOVD	$SYS_IOCTL__Linux, R8          // syscall: ioctl
	SVC
	CMN	$4095, R0                     // failed?
	BCS	report__Failure               // yes: report and exit

// Change into the client's directory. An empty string means "stay in the server's directory"
// (exec.Cmd's rule for an empty Dir), so skip the call. Reachable failure: the client chooses
// the path (ENOENT, ENOTDIR, EACCES, ...). On a hung network filesystem this call can block;
// only this process waits.
chdir:
	MOVD	$CHDIR__Step__PtyExec, R22  // step: CHDIR
	MOVD	8(R20), R0                 // R0 = argv[1]: pointer to the directory string
	MOVBU	0(R0), R1                 // R1 = the string's first byte
	CBZ	R1, rtSigprocmask          // first byte is NUL: empty string, skip chdir
	MOVD	$SYS_CHDIR__Linux, R8      // syscall: chdir(argv[1]); R0 already holds argument 1
	SVC
	CMN	$4095, R0                 // failed?
	BCS	report__Failure           // yes: report and exit

// Block nothing: the shell starts with an empty signal mask, stated here instead of inherited
// from whatever the server's thread had. Cannot fail: every argument is a constant.
rtSigprocmask:
	MOVD	$RT_SIGPROCMASK__Step__PtyExec, R22        // step: RT_SIGPROCMASK
	MOVD	$SIG_SETMASK__RT_SIGPROCMASK__Linux, R0    // argument 1: replace the whole mask
	MOVD	$·sigset_empty__PtyExec(SB), R1           // argument 2: address of the empty set
	MOVD	$0, R2                                    // argument 3: NULL, old mask not wanted
	MOVD	$8, R3                                    // argument 4: set size, 8 bytes
	MOVD	$SYS_RT_SIGPROCMASK__Linux, R8            // syscall: rt_sigprocmask
	SVC
	CMN	$4095, R0                                // failed?
	BCS	report__Failure                          // yes: report and exit

// Become the shell. On success execve never returns: this process image is replaced, fd 3
// closes (close-on-exec), and the server reads EOF. On failure it returns -errno in R0 and
// execution falls straight through into the report. Reachable: the client chooses the path
// and the environment (ENOENT, EACCES, ENOEXEC, ...).
execve:
	MOVD	$EXECVE__Step__PtyExec, R22  // step: EXECVE
	MOVD	16(R20), R0                 // argument 1: argv[2] = path of the executable
	ADD	$24, R20, R1                 // argument 2: &argv[3] = the shell's own argv (NULL-ended)
	MOVD	R21, R2                     // argument 3: envp, passed through untouched
	MOVD	$SYS_EXECVE__Linux, R8      // syscall: execve
	SVC

// Reached only on failure, with R22 = the step and R0 = -errno. Write {step, errno} to fd 3 in
// one 16-byte write: below PIPE_BUF (4096), so the server never sees half a report. A failed
// write is ignored: there is nothing left to tell, and the exit follows either way.
report__Failure:
	NEG	R0, R23                                // R23 = errno (turn -errno positive)
	MOVD	$·report__PtyExec(SB), R24              // R24 = address of the report buffer
	MOVD	R22, 0(R24)                            // report bytes 0-7:  step number
	MOVD	R23, 8(R24)                            // report bytes 8-15: errno
	MOVD	$FILE_DESCRIPTOR__REPORT__PtyExec, R0   // argument 1: fd 3
	MOVD	R24, R1                                // argument 2: the report's address
	MOVD	$SIZE_REPORT__PtyExec, R2               // argument 3: 16 bytes
	MOVD	$SYS_WRITE__Linux, R8                  // syscall: write
	SVC

// Exit the whole process with 127, the shell convention for "command could not be run". The
// server reads the report, not this code. exit_group never returns; the branch back is only
// there so execution can never run past the end of this function.
exit__Failure:
	MOVD	$EXIT_CODE__FAILURE__PtyExec, R0  // argument 1: exit code 127
	MOVD	$SYS_EXIT_GROUP__Linux, R8        // syscall: exit_group
	SVC
	B	exit__Failure                      // unreachable: exit_group does not return
