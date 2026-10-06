package main

import (
	_TIME "time"
)

type _PtyDispatcher_ struct {
	OnBlockingFlush__                       func(UnflushedSlice_StagingBuffer []byte)
	OnExited__                              func(exitSignal_PtyReader error)
	Timeout__Timer_FlushPacing              _TIME.Duration
	PoolChannel___Data__Order_PtyDispatcher chan *_Data__Order_PtyDispatcher_
	QueueChannel__Order_PtyDispatcher       chan _Order_PtyDispatcher_
	StagingBuffer                           []byte
	Status_state                            _Status_PtyDispatcher_
	Timer_FlushPacing                       *_TIME.Timer
}

type _Directive__Order_PtyDispatcher_ int

const (
	CONTINUE_WORKER__Directive__Order_PtyDispatcher _Directive__Order_PtyDispatcher_ = iota
	EXIT_WORKER__Directive__Order_PtyDispatcher
)

type _Order_PtyDispatcher_ interface {
	Execute(PtyDispatcher_forwarded *_PtyDispatcher_) _Directive__Order_PtyDispatcher_
}

type _Data__Order_PtyDispatcher_ struct {
	ReadBuffer_PtyDevice []byte
}

type _ExitSignal__Order_PtyDispatcher_ struct {
	ExitSignal error
}

type _Status_PtyDispatcher_ int

const (
	IDLE__Status_PtyDispatcher _Status_PtyDispatcher_ = iota
	PACING__Status_PtyDispatcher
)

const (
	N_TTY_BUF_SIZE__LinuxKernel                  = 4 * 1024
	SIZE_READ_BUFFER__PtyReader                  = N_TTY_BUF_SIZE__LinuxKernel - 1
	SIZE_STAGING_BUFFER__PtyDispatcher           = 96 * 1024
	SIZE_QUEUE_BUFFER__Order_PtyDispatcher       = SIZE_STAGING_BUFFER__PtyDispatcher / N_TTY_BUF_SIZE__LinuxKernel
	SIZE_POOL_BUFFER___Data__Order_PtyDispatcher = SIZE_QUEUE_BUFFER__Order_PtyDispatcher + 2
	HERTZ_TARGET_FRAMERATE__Xtermjs              = 60
	TIMEOUT__TIMER_FLUSH_PACING___PtyDispatcher  = _TIME.Second / HERTZ_TARGET_FRAMERATE__Xtermjs
)
