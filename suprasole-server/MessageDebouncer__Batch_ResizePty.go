package main

import (
	_CONTEXT "context"
	_TIME "time"
)

type _MessageDebouncer__Batch_ResizePty_ struct {
	DebounceTimeout__               _TIME.Duration
	OnFlush__OrderBatch_ResizePty__ func(orderBatch_ResizePty_pending map[uint32]_Order_ResizePty_)
	QueueChannel                    chan _Batch_ResizePty__PtyMessage_Ingress_
	WorkerContext                   _CONTEXT.Context
	WorkerCancel                    _CONTEXT.CancelFunc
}

type _MakeApi__MessageDebouncer__Batch_ResizePty_ struct {
	DebounceTimeout__               _TIME.Duration
	OnFlush__OrderBatch_ResizePty__ func(orderBatch_ResizePty_pending map[uint32]_Order_ResizePty_)
}

func Make__MessageDebouncer__Batch_ResizePty(
	api _MakeApi__MessageDebouncer__Batch_ResizePty_,
) *_MessageDebouncer__Batch_ResizePty_ {
	workerContext, workerCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	return &_MessageDebouncer__Batch_ResizePty_{
		DebounceTimeout__:               api.DebounceTimeout__,
		OnFlush__OrderBatch_ResizePty__: api.OnFlush__OrderBatch_ResizePty__,
		QueueChannel:                    make(chan _Batch_ResizePty__PtyMessage_Ingress_, 512),
		WorkerContext:                   workerContext,
		WorkerCancel:                    workerCancel,
	}
}
