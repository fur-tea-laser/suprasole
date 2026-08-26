package main

import (
	_BINARY "encoding/binary"
	_FMT    "fmt"
	_SYNC   "sync"
)

type _PtyProxyDefaults_ struct {
	ScrollbackLineCount_PtyTerminal int
	StagingBufferSize_PtyReader     int
	PostSnapshotBufferSize_PtyProxy int
}

type _WorkspaceController_ struct {
	Mutex            _SYNC.Mutex
	WorkspaceNetwork *_WorkspaceNetwork_
	PtyPool          map[uint32]*_WorkspacePty_
	NextPtyId        uint32
	PtyProxyDefaults _PtyProxyDefaults_
}

type _NewWorkspaceControllerApi_ struct {
	HostPortAddress  string
	PtyProxyDefaults _PtyProxyDefaults_
}

func NewWorkspaceController(
	api _NewWorkspaceControllerApi_,
) *_WorkspaceController_ {
	newWorkspaceControllerResult := &_WorkspaceController_{
		Mutex:            _SYNC.Mutex{},
		PtyPool:          make(map[uint32]*_WorkspacePty_),
		NextPtyId:        1,
		PtyProxyDefaults: api.PtyProxyDefaults,
		WorkspaceNetwork: nil,
	}
	newWorkspaceControllerResult.WorkspaceNetwork = NewWorkspaceNetwork(_NewWorkspaceNetworkApi_{
		HostPortAddress:                     api.HostPortAddress,
		OnConnected_PtyWebsocket:            newWorkspaceControllerResult.HandleConnected_PtyWebsocket,
		OnTakeoverConnected_PtyWebsocket:    newWorkspaceControllerResult.HandleTakeoverConnected_PtyWebsocket,
		OnDisconnected_PtyWebsocket:         newWorkspaceControllerResult.HandleDisconnected_PtyWebsocket,
		OnTakeoverDisconnected_PtyWebsocket: newWorkspaceControllerResult.HandleTakeoverDisconnected_PtyWebsocket,
		OnBinaryMessageFrame_PtyWebsocket:   newWorkspaceControllerResult.HandleBinaryMessageFrame_PtyWebsocket,
	})
	return newWorkspaceControllerResult
}

func (this *_WorkspaceController_) HandleConnected_PtyWebsocket() {
}

func (this *_WorkspaceController_) HandleTakeoverConnected_PtyWebsocket() {
}

func (this *_WorkspaceController_) HandleDisconnected_PtyWebsocket(
	readMessageError error,
) {
}

func (this *_WorkspaceController_) HandleTakeoverDisconnected_PtyWebsocket() {
}

func (this *_WorkspaceController_) HandleBinaryMessageFrame_PtyWebsocket(
	binaryMessageFrame []byte,
) {
	if len(binaryMessageFrame) < 2 {
		_FMT.Printf(
			"pty websocket client message error: frame too short: %d bytes\n",
			len(binaryMessageFrame),
		)
		return
	}
	messageCode := MessageCode_PtyWebsocket(_BINARY.BigEndian.Uint16(binaryMessageFrame[0:2]))
	decodeMessage := DECODE_MESSAGE_MAP__PTY_WEBSOCKET[messageCode]
	if nil == decodeMessage {
		_FMT.Printf(
			"pty websocket client message error: unrecognized message code: 0x%04x\n",
			messageCode,
		)
		return
	}
	ptyMessage, decodeError_ptyMessage := decodeMessage(binaryMessageFrame)
	if decodeError_ptyMessage != nil {
		_FMT.Printf(
			"pty websocket client message error: decode failure: %v\n",
			decodeError_ptyMessage,
		)
		return
	}
	ptyMessage.Execute(this)
}
