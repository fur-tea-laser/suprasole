package main

import (
	_SYNC "sync"
	_TIME "time"
)

type _WorkspaceController_ struct {
	OptionConfig_PtyProxy__default__  _OptionConfig_PtyProxy_
	Mutex                             _SYNC.Mutex
	PtyPool                           map[uint32]*_WorkspacePty_
	Id_WorkspacePty_next              uint32
	WorkspaceNetwork                  *_WorkspaceNetwork_
	MessageDebouncer__Batch_ResizePty *_MessageDebouncer__Batch_ResizePty_
	LifecycleCoordinator_WorkspacePty *_LifecycleCoordinator_WorkspacePty_
}

type _MakeApi_WorkspaceController_ struct {
	Address_HttpServer__             string
	OptionConfig_PtyProxy__default__ _OptionConfig_PtyProxy_
}

func Make_WorkspaceController(
	api _MakeApi_WorkspaceController_,
) *_WorkspaceController_ {
	workspaceController_result := &_WorkspaceController_{
		OptionConfig_PtyProxy__default__:  api.OptionConfig_PtyProxy__default__,
		Mutex:                             _SYNC.Mutex{},
		PtyPool:                           make(map[uint32]*_WorkspacePty_),
		Id_WorkspacePty_next:              0,
		WorkspaceNetwork:                  nil,
		MessageDebouncer__Batch_ResizePty: nil,
		LifecycleCoordinator_WorkspacePty: nil,
	}
	workspaceController_result.WorkspaceNetwork = Make_WorkspaceNetwork(_MakeApi_WorkspaceNetwork_{
		Address_HttpServer__:                    api.Address_HttpServer__,
		OnConnected_PtyWebsocket__:              workspaceController_result.HandleConnected_PtyWebsocket,
		OnConnected_Takeover__PtyWebsocket__:    workspaceController_result.HandleConnected_Takeover__PtyWebsocket,
		OnDisconnected_PtyWebsocket__:           workspaceController_result.HandleDisconnected_PtyWebsocket,
		OnDisconnected_Takeover__PtyWebsocket__: workspaceController_result.HandleDisconnected_Takeover__PtyWebsocket,
		OnPayload_BinaryMessage__PtyWebsocket__: workspaceController_result.HandlePayload_BinaryMessage__PtyWebsocket,
	})
	workspaceController_result.MessageDebouncer__Batch_ResizePty = Make__MessageDebouncer__Batch_ResizePty(_MakeApi__MessageDebouncer__Batch_ResizePty_{
		DebounceTimeout__:               50 * _TIME.Millisecond,
		OnFlush__OrderBatch_ResizePty__: workspaceController_result.HandleBatch_ResizePty__Debouncer,
	})
	workspaceController_result.LifecycleCoordinator_WorkspacePty = Make__LifecycleCoordinator_WorkspacePty(_MakeApi__LifecycleCoordinator_WorkspacePty_{
		OnConnect_PtyWebsocket__:     workspaceController_result.HandleConnect_PtyWebsocket__Coordinator,
		OnDisconnect_PtyWebsocket__:  workspaceController_result.HandleDisconnect_PtyWebsocket__Coordinator,
		OnSync_PtyPool__:             workspaceController_result.HandleSync_PtyPool__Coordinator,
		OnExit_PtyProxy__:            workspaceController_result.HandleExit_PtyProxy__Coordinator,
		OnSpawnPty__:                 workspaceController_result.HandleSpawnPty__Coordinator,
		OnStatus_SpawnPty__Success__: workspaceController_result.HandleStatus_SpawnPty__Success__Coordinator,
		OnStatus_SpawnPty__Failure__: workspaceController_result.HandleStatus_SpawnPty__Failure__Coordinator,
	})
	return workspaceController_result
}
