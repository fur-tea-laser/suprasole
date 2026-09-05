package main

import (
	_CONTEXT "context"
	_TIME "time"
)

type _MessageDebouncer_ResizePtys_ struct {
	WorkerContext   _CONTEXT.Context
	WorkerCancel    _CONTEXT.CancelFunc
	QueueChannel    chan _ResizePtys__PtyMessage_Ingress_
	DebounceTimeout _TIME.Duration
	OnResizePtys    func(pendingOrders_ResizePtys map[uint32]_ResizePtyOrder_ResizePtys_)
}

type _NewApi__MessageDebouncer_ResizePtys_ struct {
	DebounceTimeout _TIME.Duration
	OnResizePtys    func(pendingOrders_ResizePtys map[uint32]_ResizePtyOrder_ResizePtys_)
}

func New__MessageDebouncer_ResizePtys(
	api _NewApi__MessageDebouncer_ResizePtys_,
) *_MessageDebouncer_ResizePtys_ {
	workerContext, workerCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	return &_MessageDebouncer_ResizePtys_{
		WorkerContext:   workerContext,
		WorkerCancel:    workerCancel,
		QueueChannel:    make(chan _ResizePtys__PtyMessage_Ingress_, 512),
		DebounceTimeout: api.DebounceTimeout,
		OnResizePtys:    api.OnResizePtys,
	}
}

func (this *_MessageDebouncer_ResizePtys_) RunWorker() {
	pendingOrders_ResizePtys := make(map[uint32]_ResizePtyOrder_ResizePtys_)
	var debounceTimer *_TIME.Timer
	var debounceTimerChannel <-chan _TIME.Time
	for {
		select {
		case <-this.WorkerContext.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
		case leadingMessage := <-this.QueueChannel:
			this.DrainAndCoalesceQueueChannel(
				leadingMessage,
				pendingOrders_ResizePtys,
			)
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = _TIME.NewTimer(this.DebounceTimeout)
			debounceTimerChannel = debounceTimer.C
		case <-debounceTimerChannel:
			if len(pendingOrders_ResizePtys) > 0 {
				this.OnResizePtys(pendingOrders_ResizePtys)
				clear(pendingOrders_ResizePtys)
			}
			debounceTimer = nil
			debounceTimerChannel = nil
		}
	}
}

func (this *_MessageDebouncer_ResizePtys_) DrainAndCoalesceQueueChannel(
	leadingMessage _ResizePtys__PtyMessage_Ingress_,
	pendingOrders_ResizePtys map[uint32]_ResizePtyOrder_ResizePtys_,
) {
	for _, currentOrder_leadingMessage := range leadingMessage.ResizePtyOrders {
		pendingOrders_ResizePtys[currentOrder_leadingMessage.Id_PtyProxy] = currentOrder_leadingMessage
	}
	for {
		select {
		case nextMessage := <-this.QueueChannel:
			for _, currentOrder_nextMessage := range nextMessage.ResizePtyOrders {
				pendingOrders_ResizePtys[currentOrder_nextMessage.Id_PtyProxy] = currentOrder_nextMessage
			}
		default:
			return
		}
	}
}
