* **CLI Option Ingestion & Fail-Fast Validation (_FLAG.Parse())**:
  * **Pre-Allocation Execution**: Parses command-line arguments (os.Args[1:]) synchronously on main before allocating subsystem graphs, buffers, or channels.
  * **ExitOnError Semantics (_OS.Exit(2))**: Flag parse errors, unrecognized flags, or -h/--help automatically write usage to _OS.Stderr and terminate execution with exit code 2, bypassing downstream allocations entirely.
  * **Pointer Dereference Ordering**: Flags are returned as pointers; dereferencing (*cliOption__Address_HttpServer) occurs strictly after _FLAG.Parse(), ensuring CLI overrides take effect before constructing WorkspaceController.
  * **POSIX Option Terminator**: Honors the -- delimiter in os.Args to terminate flag parsing and treat subsequent arguments as positional values.

* **Static Graph Assembly (Pre-Concurrency Construction)**:
  * **Synchronous Wiring (Make_WorkspaceController)**: Fully links callbacks, internal channels, state maps, buffer configs, and HTTP routes on main before spawning goroutines.
  * **Zero-Lock Race Immunity**: Pre-concurrency assembly guarantees race-free publication across goroutines without startup locks.

* **Isolated HTTP Route Multiplexing (_HTTP.NewServeMux)**:
  * **Global DefaultServeMux Air-Gapping**: Allocates a private ServeMux rather than attaching to http.DefaultServeMux, preventing imported packages (e.g., pprof) from exposing unintended endpoints on the public server port.
  * **Exact Match Semantics (/pty)**: Pattern /pty (without trailing slash) enforces strict 1:1 route targeting, preventing automatic 301 subtree redirects that would break WebSocket upgrade handshakes.

* **Consumer Readiness Invariant (Downstream-First Activation)**:
  * **Inside-Out Activation Cascade (WorkspaceController.Start())**:
    1. Core queue workers start first (MessageReducer, LifecycleCoordinator).
    2. TCP socket binds synchronously (_NET.Listen).
    3. Ingress submission worker starts (RunWorker__Submission_GetWebsocketConnection).
    4. HTTP transport starts accepting connections (HttpServer.Serve).
  * **Queue Deadlock Prevention**: Active drain loops precede network producers, preventing ingress goroutines from blocking on unserviced channels.

* **Synchronous Listener Fail-Fast Gate (_NET.Listen)**:
  * **Socket Viability Verification**: Synchronous _NET.Listen verifies socket availability (EADDRINUSE, EACCES, EADDRNOTAVAIL, EMFILE) before spawning network workers.
  * **Fail-Fast Abort (_OS.Exit(1))**: Bind errors bypass defers and exit immediately, preventing main() from parking on a dead network stack.

* **Process Lifetime Anchor**: Background goroutines do not sustain execution in Go; main() explicitly blocks on <-channel_shutdownSignal to sustain the active serving phase.

* **Signal Interception & Wait Pipeline (_SIGNAL.Notify & <-channel_shutdownSignal)**:
  * **Targeted Signal Scope (_OS.Interrupt & _SYSCALL.SIGTERM)**:
    * **_OS.Interrupt (SIGINT / Signal 2)**: Dispatched by Linux tty driver on Ctrl+C for interactive termination.
    * **_SYSCALL.SIGTERM (SIGTERM / Signal 15)**: Dispatched by supervisors and orchestrators (systemd, Docker stop, Kubernetes) for graceful drain before SIGKILL.
    * **Omission of SIGQUIT (Signal 3 / Ctrl+\)**: Left unhandled to preserve Go runtime diagnostic dumps and goroutine stack traces on stderr (GOTRACEBACK).
    * **Uncatchability of SIGKILL (Signal 9)**: Kernel-level uncatchable (sigaction returns EINVAL); immediately forces unconditional termination.
  * **Buffer Sizing & Race Mechanics (make(chan _OS.Signal, 1))**:
    * **Pre-Wait Race (Capacity 1 vs. 0)**: Non-blocking Go signal dispatch (runtime.sigsend / selectnbsend) drops events arriving before main() parks on <-channel_shutdownSignal; capacity 1 eliminates this registration-to-wait deadlock.
    * **One-Shot Binary Latch (Why not capacity > 1)**: main() reads only once to initiate teardown. Because the Linux kernel coalesces standard signals into a sigpending bitmask, capacity > 1 stores redundant, unread events.
  * **Zero-CPU Scheduler Park**: Parks main() in runtime.chanrecv1 (0% CPU) until a signal unparks the thread to execute WorkspaceController.Shutdown().

* **Ephemeral Worker Teardown (Absence of sync.WaitGroup)**:
  * **Context Preemption (WorkerCancel())**: Cancels worker contexts (context.WithCancel) to signal background runloops to terminate.
  * **Omission of Join Barrier**: Ephemeral in-memory queues require no persistent flush; omitting sync.WaitGroup prevents teardown deadlocks and relies on process exit to reclaim goroutines.

* **Wire Protocol Teardown (RFC 6455 Status 1001 & TCP 4-Way Teardown)**:
  * **Application Close Frame (Status 1001)**: CloseWithCode_WebsocketConnection emits Close frame 1001 ("Server Shutting Down") before transport shutdown, signaling intentional disconnect.
  * **Graceful TCP Teardown vs. RST**: Enables a clean TCP 4-way FIN/ACK handshake, preventing kernel RST generation and in-flight data loss.

* **HttpServer.Shutdown Drain Semantics & Context Execution**:
  * **Ingress Cutoff (http.ErrServerClosed)**: Closes listeners immediately, rejecting new handshakes and unblocking HttpServer.Serve with http.ErrServerClosed.
  * **StateHijacked Connection Polling**: Polls hijacked WebSocket connections at backoff intervals up to 500ms until sockets close or context deadline expires.
  * **Socket Buffer Purging (SO_RCVBUF)**: Closing socket descriptors discards any unread ingress bytes remaining in kernel receive buffers (SO_RCVBUF).
  * **Bounded Drain Deadline (5-Second Timeout)**: Bounds HttpServer.Shutdown with a 5-second context (context_shutdownDeadline__HttpServer via context.WithTimeout), preventing lingering connections from stalling process exit indefinitely.
  * **Error Suppression & Timer Deregistration**: Discards context.DeadlineExceeded (_ = ...) to guarantee unconditional exit 0, immediately calling cancel_shutdownDeadline__HttpServer() to free timer heap resources.
