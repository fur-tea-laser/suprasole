* **PtyDescriptor charter:** the server's native pseudo-terminal pair, replacing creack/pty.
  * **Master (`/dev/ptmx`):** opened `O_RDWR|O_NOCTTY|O_CLOEXEC|O_NONBLOCK`.
    * **Non-blocking, therefore pollable:** `os.NewFile` of a non-blocking descriptor registers it with the netpoller, so `Close` unblocks a parked `Read`, and a stray `Fd()` can no longer flip it to blocking.
    * **The defect this removes (issue §1.5):** creack/pty v1.1.24 ran every ioctl through `Fd()`, which left the master blocking, so `Close` never unblocked the reader.
    * **`O_NOCTTY`:** opening the master never makes it the server's controlling terminal.
  * **Open sequence:** `TIOCSPTLCK(0)` unlocks the slave → `TIOCGPTN` reads N for `/dev/pts/N` → `TIOCSWINSZ` sets the initial size → `TIOCGPTPEER` opens the slave.
    * **`TIOCGPTPEER__Linux = 0x5441`:** `_IO('T', 0x41)`, Linux 4.13. It opens the slave through the master instead of by path, and is absent from package syscall.
    * **`abortOpen__PtyDescriptor`:** releases the half-opened master and names the ioctl that failed.
    * **unsafe.Pointer rule 4:** each `uintptr(unsafe.Pointer(&x))` conversion stays inside the `Syscall(...)` call expression, so the referent stays live and unmoved for the duration of the call.
  * **Slave:** blocking, because the child expects a blocking terminal; close-on-exec, so only the child's fds 0–2 carry it past `execve`.
  * **`_Winsize_PtyDescriptor_`:** the kernel's `struct winsize` (asm-generic/termios.h). Rows come first, then columns; `ws_xpixel` and `ws_ypixel` are unused.
  * **The parent's slave copy (`Spawn_PtyProxy`):** closed right after `Spawn_PtyProcess`, whether or not the fork succeeded.
    * **Success:** EIO on the master waits for every slave descriptor, the parent's included.
    * **Failure:** nothing else would close it, and with no child the master is useless, so it is closed too.
  * **`Resize_PtyDescriptor`:** `RawConn.Control` holds a reference on the descriptor for the duration of the ioctl, so a resize racing the master's close fails instead of reaching a reused descriptor number.
    * **Failure value:** poll's "use of closed file". os translates it to `os.ErrClosed` only for its own I/O methods, not for `Control`.
  * **Tests (`tests/PtyDescriptor_test.go`, compiled into `source/` by `tests/runTests.sh`):**
    * **`TestSpawn_PtyProxy_ShellOwnsTheTerminal`:** EIO arriving at all proves that the parent closed its own slave copy after the fork.
    * **`TestResize_PtyDescriptor`:** after close, the test asserts only that the resize fails, because which error value `RawConn.Control` returns for a closed file is undocumented.
    * **`TestMasterClose_UnblocksReaderAndHangsUpShell`:** closing the master must unblock the parked reader and hang up the slave, so the session leader receives SIGHUP.
