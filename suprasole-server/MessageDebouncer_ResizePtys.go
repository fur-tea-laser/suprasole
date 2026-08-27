package main

import (
	_CONTEXT "context"
	_TIME "time"
)

type _MessageDebouncer_ResizePtys_ struct {
	WorkerContext   _CONTEXT.Context
	WorkerCancel    _CONTEXT.CancelFunc
	QueueChannel    chan _ResizePtys_Message_
	DebounceTimeout _TIME.Duration
	OnResizePtys    func(pendingEntries_ResizePtys map[uint32]_ResizePtys_Entry_)
}

type _NewApi__MessageDebouncer_ResizePtys_ struct {
	DebounceTimeout _TIME.Duration
	OnResizePtys    func(pendingEntries_ResizePtys map[uint32]_ResizePtys_Entry_)
}

func New__MessageDebouncer_ResizePtys(
	api _NewApi__MessageDebouncer_ResizePtys_,
) *_MessageDebouncer_ResizePtys_ {
	__WorkerContext, __WorkerCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	return &_MessageDebouncer_ResizePtys_{
		WorkerContext:   __WorkerContext,
		WorkerCancel:    __WorkerCancel,
		QueueChannel:    make(chan _ResizePtys_Message_, 512),
		DebounceTimeout: api.DebounceTimeout,
		OnResizePtys:    api.OnResizePtys,
	}
}

func (this *_MessageDebouncer_ResizePtys_) RunWorker() {
	pendingEntries_ResizePtys := make(map[uint32]_ResizePtys_Entry_)
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
				pendingEntries_ResizePtys,
			)
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = _TIME.NewTimer(this.DebounceTimeout)
			debounceTimerChannel = debounceTimer.C
		case <-debounceTimerChannel:
			if len(pendingEntries_ResizePtys) > 0 {
				this.OnResizePtys(pendingEntries_ResizePtys)
				clear(pendingEntries_ResizePtys)
			}
			debounceTimer = nil
			debounceTimerChannel = nil
		}
	}
}

func (this *_MessageDebouncer_ResizePtys_) DrainAndCoalesceQueueChannel(
	leadingMessage _ResizePtys_Message_,
	pendingEntries_ResizePtys map[uint32]_ResizePtys_Entry_,
) {
	for _, someEntry_leadingMessage := range leadingMessage.Entries_PtyProxy {
		pendingEntries_ResizePtys[someEntry_leadingMessage.Id_PtyProxy] = someEntry_leadingMessage
	}
	for {
		select {
		case nextMessage := <-this.QueueChannel:
			for _, someEntry_nextMessage := range nextMessage.Entries_PtyProxy {
				pendingEntries_ResizePtys[someEntry_nextMessage.Id_PtyProxy] = someEntry_nextMessage
			}
		default:
			return
		}
	}
}
