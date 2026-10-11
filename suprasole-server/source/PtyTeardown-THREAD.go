package main

import (
	_ERRORS "errors"
	_OS "os"
	_SYSCALL "syscall"
)

func (This *_PtyTeardown_) RunWorker() {
	for {
		select {
		case order_next := <-This.QueueChannel__Order_PtyTeardown:
			order_next.Execute(This)
		case <-This.Channel_FinalState:
			This.Channel_FinalState = nil
			This.HandleFinalState()
		case <-This.Timer_Drain.C:
			This.HandleDrain()
		case <-This.Timer_Unresponsive.C:
			This.HandleUnresponsive()
		case <-This.Timer_Uninterruptible.C:
			This.HandleUninterruptible()
		}
		switch This.Settle() {
		case CONTINUE_WORKER__Directive__PtyTeardown:
		case EXIT_WORKER__Directive__PtyTeardown:
			return
		default:
			panic("invalid path: _PtyTeardown_ RunWorker")
		}
	}
}

func (this *_Terminate__Order_PtyTeardown_) Execute(
	PtyTeardown_forwarded *_PtyTeardown_,
) {
	switch PtyTeardown_forwarded.Status_ProcessSide {
	case UNESCALATED__Status_ProcessSide__PtyTeardown:
		PtyTeardown_forwarded.HangUp_PtyProcess()
		PtyTeardown_forwarded.Timer_Unresponsive.Reset(TIMEOUT_UNRESPONSIVE__PtyProcess)
		PtyTeardown_forwarded.Status_ProcessSide = AWAITING_HANGUP__Status_ProcessSide__PtyTeardown
	case AWAITING_HANGUP__Status_ProcessSide__PtyTeardown,
		AWAITING_KILL__Status_ProcessSide__PtyTeardown,
		AWAITING_FINAL__Status_ProcessSide__PtyTeardown,
		FINAL__Status_ProcessSide__PtyTeardown,
		GIVEN_UP__Status_ProcessSide__PtyTeardown:
		// Idempotent: escalation has begun, or the process side has ended and the drain finishes the session.
	default:
		panic("invalid path: _Terminate__Order_PtyTeardown_ Execute")
	}
}

func (this *_Cancel__Order_PtyTeardown_) Execute(
	PtyTeardown_forwarded *_PtyTeardown_,
) {
	if PtyTeardown_forwarded.Status_ReaderSide != PENDING__Status_ReaderSide__PtyTeardown {
		// RUNNING / FINISHED: the coordinator cancels only a spawn whose reader it never started
		panic("invalid path: _Cancel__Order_PtyTeardown_ Execute [READER_SIDE]")
	}
	PtyTeardown_forwarded.Close_Master()
	switch PtyTeardown_forwarded.Status_ProcessSide {
	case UNESCALATED__Status_ProcessSide__PtyTeardown:
		PtyTeardown_forwarded.HangUp_PtyProcess()
		PtyTeardown_forwarded.Timer_Unresponsive.Reset(TIMEOUT_UNRESPONSIVE__PtyProcess)
		PtyTeardown_forwarded.Status_ProcessSide = AWAITING_HANGUP__Status_ProcessSide__PtyTeardown
	case FINAL__Status_ProcessSide__PtyTeardown:
	default:
		// AWAITING_* / GIVEN_UP: escalation starts only on Terminate or ReaderFinished, both of which need a reader
		panic("invalid path: _Cancel__Order_PtyTeardown_ Execute [PROCESS_SIDE]")
	}
	PtyTeardown_forwarded.Status_ReaderSide = FINISHED__Status_ReaderSide__PtyTeardown
	PtyTeardown_forwarded.Status_Cancellation = CANCELED__Status_Cancellation__PtyTeardown
}

func (this *_ReaderStarted__Order_PtyTeardown_) Execute(
	PtyTeardown_forwarded *_PtyTeardown_,
) {
	if PtyTeardown_forwarded.Status_ReaderSide != PENDING__Status_ReaderSide__PtyTeardown {
		panic("invalid path: _ReaderStarted__Order_PtyTeardown_ Execute")
	}
	PtyTeardown_forwarded.Status_ReaderSide = RUNNING__Status_ReaderSide__PtyTeardown
	if FINAL__Status_ProcessSide__PtyTeardown == PtyTeardown_forwarded.Status_ProcessSide {
		PtyTeardown_forwarded.Timer_Drain.Reset(TIMEOUT_DRAIN__PtyProxy)
	}
}

func (this *_ReaderFinished__Order_PtyTeardown_) Execute(
	PtyTeardown_forwarded *_PtyTeardown_,
) {
	if PtyTeardown_forwarded.Status_ReaderSide != RUNNING__Status_ReaderSide__PtyTeardown {
		panic("invalid path: _ReaderFinished__Order_PtyTeardown_ Execute")
	}
	PtyTeardown_forwarded.Status_ReaderSide = FINISHED__Status_ReaderSide__PtyTeardown
	PtyTeardown_forwarded.ExitSignal_PtyReader = this.ExitSignal
	PtyTeardown_forwarded.Timer_Drain.Stop()
	// Closing the master is what makes EIO final: while it stays open, a reopened slave would undo EIO. (issue §6-Z)
	PtyTeardown_forwarded.Close_Master()
	if UNESCALATED__Status_ProcessSide__PtyTeardown == PtyTeardown_forwarded.Status_ProcessSide {
		// Rogue: the terminal ended while its process lives on. (issue §9E)
		PtyTeardown_forwarded.Timer_Unresponsive.Reset(TIMEOUT_UNRESPONSIVE__PtyProcess)
		PtyTeardown_forwarded.Status_ProcessSide = AWAITING_HANGUP__Status_ProcessSide__PtyTeardown
	}
}

func (This *_PtyTeardown_) HandleFinalState() {
	This.Timer_Unresponsive.Stop()
	This.Timer_Uninterruptible.Stop()
	This.Status_ProcessSide = FINAL__Status_ProcessSide__PtyTeardown
	if RUNNING__Status_ReaderSide__PtyTeardown == This.Status_ReaderSide {
		// EIO may never come: any other holder of the slave keeps it from the reader. (issue §9D)
		This.Timer_Drain.Reset(TIMEOUT_DRAIN__PtyProxy)
	}
}

func (This *_PtyTeardown_) HandleDrain() {
	This.Close_Master()
}

func (This *_PtyTeardown_) HandleUnresponsive() {
	This.Close_Master()
	This.PtyProcess.Mutex.Lock()
	status_Observation, report_Unresponsive_maybe := This.PtyProcess.State_state.Observe_Unresponsive()
	if UNRESPONSIVE__Status_Observation__PtyProcess == status_Observation {
		_ = This.PtyProcess.State_state.Signal(
			_SYSCALL.SIGKILL,
			PIDFD_SIGNAL_PROCESS_GROUP__Linux,
		)
	}
	This.PtyProcess.Mutex.Unlock()
	switch status_Observation {
	case UNRESPONSIVE__Status_Observation__PtyProcess:
		This.Report_Observation_latest = report_Unresponsive_maybe
		This.Timer_Uninterruptible.Reset(TIMEOUT_UNINTERRUPTIBLE__PtyProcess)
		This.Status_ProcessSide = AWAITING_KILL__Status_ProcessSide__PtyTeardown
	case TERMINATED__Status_Observation__PtyProcess:
		// Zombie / Reaped / SystemError: terminated in the window, and the worker closes Channel_FinalState within
		// microseconds. No group kill: after the PTY process terminates, the policy is unplug only. (issue §8.1)
		This.Status_ProcessSide = AWAITING_FINAL__Status_ProcessSide__PtyTeardown
	default:
		panic("invalid path: _PtyTeardown_ HandleUnresponsive")
	}
}

func (This *_PtyTeardown_) HandleUninterruptible() {
	This.PtyProcess.Mutex.Lock()
	status_Observation, report_Unresponsive_maybe := This.PtyProcess.State_state.Observe_Unresponsive()
	This.PtyProcess.Mutex.Unlock()
	switch status_Observation {
	case UNRESPONSIVE__Status_Observation__PtyProcess:
		// Give up on the process side; _PtyProcess_ stays parked on its pidfd (the accepted cost, issue §3C).
		This.Report_Observation_latest = report_Unresponsive_maybe
		This.Status_ProcessSide = GIVEN_UP__Status_ProcessSide__PtyTeardown
	case TERMINATED__Status_Observation__PtyProcess:
		This.Status_ProcessSide = AWAITING_FINAL__Status_ProcessSide__PtyTeardown
	default:
		panic("invalid path: _PtyTeardown_ HandleUninterruptible")
	}
}

func (This *_PtyTeardown_) Settle() _Directive__PtyTeardown_ {
	if This.Status_ReaderSide != FINISHED__Status_ReaderSide__PtyTeardown {
		return CONTINUE_WORKER__Directive__PtyTeardown
	}
	switch This.Status_ProcessSide {
	case UNESCALATED__Status_ProcessSide__PtyTeardown,
		AWAITING_HANGUP__Status_ProcessSide__PtyTeardown,
		AWAITING_KILL__Status_ProcessSide__PtyTeardown,
		AWAITING_FINAL__Status_ProcessSide__PtyTeardown:
		return CONTINUE_WORKER__Directive__PtyTeardown
	case FINAL__Status_ProcessSide__PtyTeardown, GIVEN_UP__Status_ProcessSide__PtyTeardown:
	default:
		panic("invalid path: _PtyTeardown_ Settle")
	}
	This.Timer_Drain.Stop()
	This.Timer_Unresponsive.Stop()
	This.Timer_Uninterruptible.Stop()
	switch This.Status_Cancellation {
	case NOT_CANCELED__Status_Cancellation__PtyTeardown:
		This.OnExitOutcome__(
			This.Id_WorkspacePty,
			This.Resolve__ExitOutcome_PtyProxy(),
		)
	case CANCELED__Status_Cancellation__PtyTeardown:
	default:
		panic("invalid path: _PtyTeardown_ Settle [CANCELLATION]")
	}
	return EXIT_WORKER__Directive__PtyTeardown
}

func (This *_PtyTeardown_) Resolve__ExitOutcome_PtyProxy() _ExitOutcome_PtyProxy_ {
	if _ERRORS.Is(This.ExitSignal_PtyReader, _SYSCALL.EIO) || _ERRORS.Is(This.ExitSignal_PtyReader, _OS.ErrClosed) {
		return This.Resolve__TeardownReport_PtyProcess().Resolve__ExitOutcome_PtyProxy()
	} else if This.ExitSignal_PtyReader != nil {
		return _SystemError_Descriptor__ExitOutcome_PtyProxy_{
			SystemError_PtyDescriptor: This.ExitSignal_PtyReader,
		}
	} else {
		// nil == This.ExitSignal_PtyReader: PtyReader reports only a non-nil read error
		panic("invalid path: _PtyTeardown_ Resolve__ExitOutcome_PtyProxy")
	}
}

// The final state is the teardown report; once the process side was given up, the latest observation is. (issue §3F)
func (This *_PtyTeardown_) Resolve__TeardownReport_PtyProcess() _TeardownReport_PtyProcess_ {
	switch This.Status_ProcessSide {
	case FINAL__Status_ProcessSide__PtyTeardown:
		This.PtyProcess.Mutex.Lock()
		State__PtyProcess__final := This.PtyProcess.State_state
		This.PtyProcess.Mutex.Unlock()
		switch State__PtyProcess__the := State__PtyProcess__final.(type) {
		case *_Reaped__State_PtyProcess_:
			return State__PtyProcess__the
		case *_SystemError__State_PtyProcess_:
			return State__PtyProcess__the
		default:
			// *_Alive__State_PtyProcess_
			// *_Zombie_Withheld__State_PtyProcess_
			// *_Zombie__State_PtyProcess_
			// Channel_FinalState closes strictly after the final state is written
			panic("invalid path: _PtyTeardown_ Resolve__TeardownReport_PtyProcess [FINAL_STATE]")
		}
	case GIVEN_UP__Status_ProcessSide__PtyTeardown:
		return This.Report_Observation_latest
	default:
		panic("invalid path: _PtyTeardown_ Resolve__TeardownReport_PtyProcess")
	}
}

// SIGHUP + SIGCONT: exactly what the kernel sends a session leader on hangup; a stopped process needs the SIGCONT. (issue §3D)
func (This *_PtyTeardown_) HangUp_PtyProcess() {
	This.PtyProcess.Mutex.Lock()
	_ = This.PtyProcess.State_state.Signal(
		_SYSCALL.SIGHUP,
		PIDFD_SIGNAL_THREAD_GROUP__Linux,
	)
	_ = This.PtyProcess.State_state.Signal(
		_SYSCALL.SIGCONT,
		PIDFD_SIGNAL_THREAD_GROUP__Linux,
	)
	This.PtyProcess.Mutex.Unlock()
}

// The forced unplug: the last reference to the master closes, the slave hangs up, and PtyReader returns
// os.ErrClosed. _PtyTeardown_ is the only code that closes the master. (issue §3E)
func (This *_PtyTeardown_) Close_Master() {
	switch This.Status_Master {
	case OPEN__Status_Master__PtyTeardown:
		_ = This.FileDescriptor_Master__Pty__shared.Close()
		This.Status_Master = CLOSED__Status_Master__PtyTeardown
	case CLOSED__Status_Master__PtyTeardown:
	default:
		panic("invalid path: _PtyTeardown_ Close_Master")
	}
}
