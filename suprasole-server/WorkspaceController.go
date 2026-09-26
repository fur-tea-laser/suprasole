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
