package main

import (
	_SYNC "sync"
)

type _WorkspaceController_ struct {
	OptionConfig_PtyProxy__default__          _OptionConfig_PtyProxy_
	Mutex                                     _SYNC.Mutex
	PtyPool                                   map[uint32]*_WorkspacePty_
	Id_WorkspacePty_next                      uint32
	WorkspaceNetwork                          *_WorkspaceNetwork_
	LifecycleCoordinator_WorkspacePty         *_LifecycleCoordinator_WorkspacePty_
	MessageReducer__LayoutUpdate_PtyProxy     *_MessageReducer__LayoutUpdate_PtyProxy_
}

func (This *_WorkspaceController_) HandleOutput_Pty(
	id_WorkspacePty uint32,
	outputData_PtyProxy []byte,
) {
	This.WorkspaceNetwork.WebsocketController_Pty.Mutex.Lock()
	id_WebsocketConnection_captured := This.WorkspaceNetwork.WebsocketController_Pty.Id_WebsocketConnection_state
	This.WorkspaceNetwork.WebsocketController_Pty.Mutex.Unlock()
	Emit__PtyMessage_Egress__WebsocketController_Pty(
		This.WorkspaceNetwork.WebsocketController_Pty,
		id_WebsocketConnection_captured,
		_PtyOutput__PtyMessage_Egress_{
			Id_WorkspacePty:     id_WorkspacePty,
			OutputData_PtyProxy: outputData_PtyProxy,
		},
	)
}
