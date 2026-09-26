package main

import (
	_CONTEXT "context"
	_TIME "time"
)

type _MessageReducer__LayoutUpdate_PtyProxy_ struct {
	DebounceTimeout__         _TIME.Duration
	OnFlush__ResizeOnly__      func(batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_)
	OnFlush__SpawnPty__        func(order_SpawnPty _SpawnPty___MessageOrder__LayoutUpdate_PtyProxy_, batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_)
	OnFlush__Batch_RemovePty__ func(order_Batch_RemovePty _Batch_RemovePty___MessageOrder__LayoutUpdate_PtyProxy_, batch__LayoutUpdate_PtyProxy map[uint32]*_LayoutUpdate_PtyProxy_)
	OnFlush__Batch_SyncPty__   func(order_Batch_SyncPty _Batch_SyncPty___MessageOrder__LayoutUpdate_PtyProxy_)
	QueueChannel              chan _MessageOrder__LayoutUpdate_PtyProxy_
	WorkerContext             _CONTEXT.Context
	WorkerCancel              _CONTEXT.CancelFunc
}

type _MessageOrder__LayoutUpdate_PtyProxy_ interface {
	compiletimemarker_MessageOrder__LayoutUpdate_PtyProxy()
}

type _Batch_ResizePty___MessageOrder__LayoutUpdate_PtyProxy_ struct {
	Message__Batch_ResizePty _Batch_ResizePty__PtyMessage_Ingress_
}

func (_Batch_ResizePty___MessageOrder__LayoutUpdate_PtyProxy_) compiletimemarker_MessageOrder__LayoutUpdate_PtyProxy() {}

type _Batch_SyncPty___MessageOrder__LayoutUpdate_PtyProxy_ struct {
	Id_WebsocketConnection_expected uint64
	Message__Batch_SyncPty          _Batch_SyncPty__PtyMessage_Ingress_
}

func (_Batch_SyncPty___MessageOrder__LayoutUpdate_PtyProxy_) compiletimemarker_MessageOrder__LayoutUpdate_PtyProxy() {}

type _SpawnPty___MessageOrder__LayoutUpdate_PtyProxy_ struct {
	Id_WebsocketConnection_expected uint64
	Message__SpawnPty               _SpawnPty__PtyMessage_Ingress_
}

func (_SpawnPty___MessageOrder__LayoutUpdate_PtyProxy_) compiletimemarker_MessageOrder__LayoutUpdate_PtyProxy() {}

type _Batch_RemovePty___MessageOrder__LayoutUpdate_PtyProxy_ struct {
	Id_WebsocketConnection_expected uint64
	Message__Batch_RemovePty        _Batch_RemovePty__PtyMessage_Ingress_
}

func (_Batch_RemovePty___MessageOrder__LayoutUpdate_PtyProxy_) compiletimemarker_MessageOrder__LayoutUpdate_PtyProxy() {}

type _LayoutUpdate_PtyProxy_ struct {
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}
