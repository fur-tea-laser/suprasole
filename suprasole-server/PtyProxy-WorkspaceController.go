package main

import (
	_SYSCALL "syscall"
)

func (This *_WorkspaceController_) HandleOutput_Pty(
	id_WorkspacePty uint32,
	outputData_PtyProxy []byte,
) {
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_current,
		_PtyOutput__PtyMessage_Egress_{
			Id_PtyProxy:         id_WorkspacePty,
			OutputData_PtyProxy: outputData_PtyProxy,
		},
	)
}

func (This *_WorkspaceController_) HandleExited_Eio_Success__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy:   _Success__ExitOutcome_PtyProxy_{},
	}
}

func (This *_WorkspaceController_) HandleExited_Eio_Failure__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy: _Failure__ExitOutcome_PtyProxy_{
			ExitCode_PtyProcess: PtyProxy_exited.PtyCommand.ProcessState.ExitCode(),
		},
	}
}

func (This *_WorkspaceController_) HandleExited_Eio_Killed__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	waitStatus_PtyProcess := PtyProxy_exited.PtyCommand.ProcessState.Sys().(_SYSCALL.WaitStatus)
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy: _Killed__ExitOutcome_PtyProxy_{
			ExitSignal_PtyProcess: int(waitStatus_PtyProcess.Signal()),
		},
	}
}

func (This *_WorkspaceController_) HandleExited_Closed__Pty(
	PtyProxy_exited *_PtyProxy_,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy:   _Closed__ExitOutcome_PtyProxy_{},
	}
}

func (This *_WorkspaceController_) HandleExited_SystemError__Pty(
	PtyProxy_exited *_PtyProxy_,
	exitSignal_PtyReader error,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _ExitPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WorkspacePty_exited: PtyProxy_exited.Id_WorkspacePty,
		ExitOutcome_PtyProxy: _SystemError__ExitOutcome_PtyProxy_{
			SystemError_PtyDevice: exitSignal_PtyReader,
		},
	}
}
