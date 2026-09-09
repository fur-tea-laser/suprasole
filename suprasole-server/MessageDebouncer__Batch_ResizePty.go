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

type _NewApi__MessageDebouncer__Batch_ResizePty_ struct {
	DebounceTimeout__               _TIME.Duration
	OnFlush__OrderBatch_ResizePty__ func(orderBatch_ResizePty_pending map[uint32]_Order_ResizePty_)
}

func New__MessageDebouncer__Batch_ResizePty(
	api _NewApi__MessageDebouncer__Batch_ResizePty_,
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

func (this *_MessageDebouncer__Batch_ResizePty_) RunWorker() {
	orderBatch_ResizePty_pending := make(map[uint32]_Order_ResizePty_)
	var debounceTimer *_TIME.Timer
	var channel_debounceTimer <-chan _TIME.Time
	for {
		select {
		case <-this.WorkerContext.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
		case message_leading := <-this.QueueChannel:
			this.DrainAndCoalesceQueueChannel(
				message_leading,
				orderBatch_ResizePty_pending,
			)
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = _TIME.NewTimer(this.DebounceTimeout__)
			channel_debounceTimer = debounceTimer.C
		case <-channel_debounceTimer:
			if len(orderBatch_ResizePty_pending) > 0 {
				this.OnFlush__OrderBatch_ResizePty__(orderBatch_ResizePty_pending)
				clear(orderBatch_ResizePty_pending)
			}
			debounceTimer = nil
			channel_debounceTimer = nil
		}
	}
}

func (this *_MessageDebouncer__Batch_ResizePty_) DrainAndCoalesceQueueChannel(
	message_leading _Batch_ResizePty__PtyMessage_Ingress_,
	orderBatch_ResizePty_pending map[uint32]_Order_ResizePty_,
) {
	for _, order_ResizePty_some := range message_leading.OrderBatch_ResizePty {
		orderBatch_ResizePty_pending[order_ResizePty_some.Id_PtyProxy] = order_ResizePty_some
	}
	for {
		select {
		case message_next := <-this.QueueChannel:
			for _, order_ResizePty_some := range message_next.OrderBatch_ResizePty {
				orderBatch_ResizePty_pending[order_ResizePty_some.Id_PtyProxy] = order_ResizePty_some
			}
		default:
			return
		}
	}
}
