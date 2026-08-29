package main

import (
	_CONTEXT "context"
)

type _MessageCoalescer_SyncWorkspace_ struct {
	WorkerContext   _CONTEXT.Context
	WorkerCancel    _CONTEXT.CancelFunc
	QueueChannel    chan _SyncWorkspace_Message_
	OnSyncWorkspace func(syncWorkspaceMessage _SyncWorkspace_Message_)
}

type _NewApi__MessageCoalescer_SyncWorkspace_ struct {
	OnSyncWorkspace func(syncWorkspaceMessage _SyncWorkspace_Message_)
}

func New__MessageCoalescer_SyncWorkspace(
	api _NewApi__MessageCoalescer_SyncWorkspace_,
) *_MessageCoalescer_SyncWorkspace_ {
	__WorkerContext, __WorkerCancel := _CONTEXT.WithCancel(_CONTEXT.Background())
	return &_MessageCoalescer_SyncWorkspace_{
		WorkerContext:   __WorkerContext,
		WorkerCancel:    __WorkerCancel,
		QueueChannel:    make(chan _SyncWorkspace_Message_, 16),
		OnSyncWorkspace: api.OnSyncWorkspace,
	}
}

func (this *_MessageCoalescer_SyncWorkspace_) RunWorker() {
	for {
		select {
		case <-this.WorkerContext.Done():
			return
		case leadingMessage := <-this.QueueChannel:
			latestMessage := this.DrainAndCoalesceQueueChannel(leadingMessage)
			this.OnSyncWorkspace(latestMessage)
		}
	}
}

func (this *_MessageCoalescer_SyncWorkspace_) DrainAndCoalesceQueueChannel(
	leadingMessage _SyncWorkspace_Message_,
) _SyncWorkspace_Message_ {
	latestMessage := leadingMessage
	for {
		select {
		case nextMessage := <-this.QueueChannel:
			latestMessage = nextMessage
		default:
			return latestMessage
		}
	}
}
