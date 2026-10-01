package main

import (
	_TIME "time"
)

type _PtyFlusher_ struct {
	OnBlockingFlush__                    func(UnflushedSlice_StagingBuffer []byte)
	OnExited_Closed__                    func(exitSignal_PtyReader error)
	OnExited_Eio__                       func(exitSignal_PtyReader error)
	OnExited_SystemError__               func(exitSignal_PtyReader error)
	Timeout__Timer_FlushPacing           _TIME.Duration
	PoolChannel___Data__Order_PtyFlusher chan *_Data__Order_PtyFlusher_
	QueueChannel__Order_PtyFlusher       chan _Order_PtyFlusher_
	StagingBuffer                        []byte
	Status_state                         _Status_PtyFlusher_
	Timer_FlushPacing                    *_TIME.Timer
}

type _Order_PtyFlusher_ interface {
	Execute(PtyFlusher_forwarded *_PtyFlusher_) bool
}

type _Data__Order_PtyFlusher_ struct {
	ReadBuffer_PtyDevice []byte
}

type _ExitSignal__Order_PtyFlusher_ struct {
	ExitSignal error
}

type _Status_PtyFlusher_ int

const (
	IDLE__Status_PtyFlusher _Status_PtyFlusher_ = iota
	PACING__Status_PtyFlusher
)

const (
	N_TTY_BUF_SIZE__LinuxKernel               = 4 * 1024
	SIZE_READ_BUFFER__PtyReader               = N_TTY_BUF_SIZE__LinuxKernel - 1
	SIZE_STAGING_BUFFER__PtyFlusher           = 96 * 1024
	SIZE_QUEUE_BUFFER__Order_PtyFlusher       = SIZE_STAGING_BUFFER__PtyFlusher / N_TTY_BUF_SIZE__LinuxKernel
	SIZE_POOL_BUFFER___Data__Order_PtyFlusher = SIZE_QUEUE_BUFFER__Order_PtyFlusher + 2
	HERTZ_TARGET_FRAMERATE__Xtermjs           = 60
	TIMEOUT__TIMER_FLUSH_PACING___PtyFlusher  = _TIME.Second / HERTZ_TARGET_FRAMERATE__Xtermjs
)
