package main

import (
	_SYNC "sync"
	_TIME "time"
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

type _MakeApi_WorkspaceController_ struct {
	Address_HttpServer__             string
	OptionConfig_PtyProxy__default__ _OptionConfig_PtyProxy_
}

func Make_WorkspaceController(
	api _MakeApi_WorkspaceController_,
) *_WorkspaceController_ {
	workspaceController_result := &_WorkspaceController_{
		OptionConfig_PtyProxy__default__:          api.OptionConfig_PtyProxy__default__,
		Mutex:                                     _SYNC.Mutex{},
		PtyPool:                                   make(map[uint32]*_WorkspacePty_),
		Id_WorkspacePty_next:                      0,
		WorkspaceNetwork:                          nil,
		LifecycleCoordinator_WorkspacePty:         nil,
		MessageReducer__LayoutUpdate_PtyProxy:     nil,
	}
	workspaceController_result.WorkspaceNetwork = Make_WorkspaceNetwork(
		_MakeApi_WorkspaceNetwork_{
			Address_HttpServer__:                    api.Address_HttpServer__,
			OnConnected_PtyWebsocket__:              workspaceController_result.HandleConnected_PtyWebsocket,
			OnConnected_Takeover__PtyWebsocket__:    workspaceController_result.HandleConnected_Takeover__PtyWebsocket,
			OnDisconnected_PtyWebsocket__:           workspaceController_result.HandleDisconnected_PtyWebsocket,
			OnDisconnected_Takeover__PtyWebsocket__: workspaceController_result.HandleDisconnected_Takeover__PtyWebsocket,
			OnPayload_BinaryMessage__PtyWebsocket__: workspaceController_result.HandlePayload_BinaryMessage__PtyWebsocket,
		},
	)
	workspaceController_result.LifecycleCoordinator_WorkspacePty = Make__LifecycleCoordinator_WorkspacePty(
		_MakeApi__LifecycleCoordinator_WorkspacePty_{
			OnConnect_PtyWebsocket__:     workspaceController_result.HandleConnect_PtyWebsocket__Coordinator,
			OnDisconnect_PtyWebsocket__:  workspaceController_result.HandleDisconnect_PtyWebsocket__Coordinator,
			OnSyncVisibility__:           workspaceController_result.HandleSyncVisibility__Coordinator,
			OnEmitSnapshot_PtyProxy__:    workspaceController_result.HandleEmitSnapshot_PtyProxy__Coordinator,
			OnSpawnPty__:                 workspaceController_result.HandleSpawnPty__Coordinator,
			OnStatus_SpawnPty__Success__: workspaceController_result.HandleStatus_SpawnPty__Success__Coordinator,
			OnStatus_SpawnPty__Failure__: workspaceController_result.HandleStatus_SpawnPty__Failure__Coordinator,
			OnTerminatePty__:             workspaceController_result.HandleTerminatePty__Coordinator,
			OnExit_PtyProxy__:            workspaceController_result.HandleExit_PtyProxy__Coordinator,
			OnRemovePty__:                workspaceController_result.HandleRemovePty__Coordinator,
		},
	)
	workspaceController_result.MessageReducer__LayoutUpdate_PtyProxy = Make__MessageReducer__LayoutUpdate_PtyProxy(
		_MakeApi__MessageReducer__LayoutUpdate_PtyProxy_{
			DebounceTimeout__:           50 * _TIME.Millisecond,
			OnFlush__ResizeOnly__:        workspaceController_result.HandleFlush__ResizeOnly__Reducer,
			OnFlush__SpawnPty__:          workspaceController_result.HandleFlush__SpawnPty__Reducer,
			OnFlush__Batch_RemovePty__:   workspaceController_result.HandleFlush__Batch_RemovePty__Reducer,
			OnFlush__Batch_SyncPty__:     workspaceController_result.HandleFlush__Batch_SyncPty__Reducer,
		},
	)
	return workspaceController_result
}
