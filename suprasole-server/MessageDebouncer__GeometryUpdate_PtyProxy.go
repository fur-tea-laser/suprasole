package main

import (
	_CONTEXT "context"
	_TIME "time"
)

type _MessageDebouncer__GeometryUpdate_PtyProxy_ struct {
	DebounceTimeout__                             _TIME.Duration
	OnFlush__OrderBatch_GeometryUpdate_PtyProxy__ func(pendingBatch *_PendingBatch__GeometryUpdate_PtyProxy_)
	QueueChannel                                  chan _MessageOrder__GeometryUpdate_PtyProxy_
	WorkerContext                                 _CONTEXT.Context
	WorkerCancel                                  _CONTEXT.CancelFunc
}

type _MessageOrder__GeometryUpdate_PtyProxy_ interface {
	Apply(pendingBatch *_PendingBatch__GeometryUpdate_PtyProxy_)
}

type _Resize___MessageOrder__GeometryUpdate_PtyProxy_ struct {
	Id_WebsocketConnection_expected uint64
	Message__Batch_ResizePty        _Batch_ResizePty__PtyMessage_Ingress_
}

type _Sync___MessageOrder__GeometryUpdate_PtyProxy_ struct {
	Id_WebsocketConnection_expected uint64
	Message__Batch_SyncPty          _Batch_SyncPty__PtyMessage_Ingress_
}

type _Target__GeometryUpdate_PtyProxy_ struct {
	ColumnCount_PtyTerminal int
	RowCount_PtyTerminal    int
}

type _SyncStatus___PendingBatch__GeometryUpdate_PtyProxy_ string

const (
	NONE___SyncStatus___PendingBatch__GeometryUpdate_PtyProxy _SyncStatus___PendingBatch__GeometryUpdate_PtyProxy_ = "NONE"
	SYNC___SyncStatus___PendingBatch__GeometryUpdate_PtyProxy _SyncStatus___PendingBatch__GeometryUpdate_PtyProxy_ = "SYNC"
)

type _PendingBatch__GeometryUpdate_PtyProxy_ struct {
	SyncStatus                      _SyncStatus___PendingBatch__GeometryUpdate_PtyProxy_
	Id_WebsocketConnection_expected uint64
	TargetBatch_pending             map[uint32]*_Target__GeometryUpdate_PtyProxy_
}

type _MakeApi__MessageDebouncer__GeometryUpdate_PtyProxy_ struct {
	DebounceTimeout__                             _TIME.Duration
	OnFlush__OrderBatch_GeometryUpdate_PtyProxy__ func(pendingBatch *_PendingBatch__GeometryUpdate_PtyProxy_)
}

func Make__MessageDebouncer__GeometryUpdate_PtyProxy(
	api _MakeApi__MessageDebouncer__GeometryUpdate_PtyProxy_,
) *_MessageDebouncer__GeometryUpdate_PtyProxy_ {
	workerContext, workerCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	return &_MessageDebouncer__GeometryUpdate_PtyProxy_{
		DebounceTimeout__:                             api.DebounceTimeout__,
		OnFlush__OrderBatch_GeometryUpdate_PtyProxy__: api.OnFlush__OrderBatch_GeometryUpdate_PtyProxy__,
		QueueChannel:                                  make(chan _MessageOrder__GeometryUpdate_PtyProxy_, 512),
		WorkerContext:                                 workerContext,
		WorkerCancel:                                  workerCancel,
	}
}
