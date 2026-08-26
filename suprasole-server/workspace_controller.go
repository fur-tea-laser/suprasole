package main

import (
	_BINARY "encoding/binary"
	_FMT "fmt"
	_SYNC "sync"
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
	__decodeBinaryMessageFrame(
		this,
		"pty websocket client",
		DECODE_MESSAGE_MAP__PTY_MESSAGE_INGRESS,
		binaryMessageFrame,
	)
}

func __decodeBinaryMessageFrame[
	MessageCodeType ~uint16,
	MessageType interface {
		Execute(workspaceController *_WorkspaceController_)
	},
](
	workspaceController *_WorkspaceController_,
	websocketContextLabel string,
	map_decodeWebsocketMessage map[MessageCodeType]func([]byte) (MessageType, error),
	binaryMessageFrame []byte,
) {
	if len(binaryMessageFrame) < 2 {
		_FMT.Printf(
			"%s message error: frame too short: %d bytes\n",
			websocketContextLabel,
			len(binaryMessageFrame),
		)
		return
	}
	messageCode := MessageCodeType(_BINARY.BigEndian.Uint16(binaryMessageFrame[0:2]))
	decodeWebsocketMessage := map_decodeWebsocketMessage[messageCode]
	if nil == decodeWebsocketMessage {
		_FMT.Printf(
			"%s message error: unrecognized message code: 0x%04x\n",
			websocketContextLabel,
			messageCode,
		)
		return
	}
	websocketMessage, decodeError_websocketMessage := decodeWebsocketMessage(binaryMessageFrame)
	if decodeError_websocketMessage != nil {
		_FMT.Printf(
			"%s message error: decode failure: %v\n",
			websocketContextLabel,
			decodeError_websocketMessage,
		)
		return
	}
	websocketMessage.Execute(workspaceController)
}

func (this *_WorkspaceController_) HandlePtySpawned(
	ptyProxy *_PtyProxy_,
) {
	workspacePty := &_WorkspacePty_{
		PtyProxy:        ptyProxy,
		IsActive:        true,
		MaybeExitResult: nil,
	}
	this.Mutex.Lock()
	this.PtyPool[ptyProxy.Id] = workspacePty
	this.Mutex.Unlock()
	_SpawnPtyStatusMessage_{
		PtyId:  ptyProxy.Id,
		Status: SUCCESS__Status_SpawnPty,
	}.Emit(this.WorkspaceNetwork.PtyWebsocketController)
}

func (this *_WorkspaceController_) HandlePtySpawnFailed(
	ptyId uint32,
) {
	_SpawnPtyStatusMessage_{
		PtyId:  ptyId,
		Status: FAILURE__Status_SpawnPty,
	}.Emit(this.WorkspaceNetwork.PtyWebsocketController)
}

func (this *_WorkspaceController_) HandlePtyOutput(
	ptyProxy *_PtyProxy_,
	ptyOutputData []byte,
) {
	_PtyOutputMessage_{
		PtyId:      ptyProxy.Id,
		OutputData: ptyOutputData,
	}.Emit(this.WorkspaceNetwork.PtyWebsocketController)
}

func (this *_WorkspaceController_) HandlePtyExited_Eio_Success(
	ptyProxy *_PtyProxy_,
) {
}

func (this *_WorkspaceController_) HandlePtyExited_Eio_Failure(
	ptyProxy *_PtyProxy_,
) {
}

func (this *_WorkspaceController_) HandlePtyExited_Eio_Killed(
	ptyProxy *_PtyProxy_,
) {
}

func (this *_WorkspaceController_) HandlePtyExited_Closed(
	ptyProxy *_PtyProxy_,
) {
}

func (this *_WorkspaceController_) HandlePtyExited_SystemError(
	ptyProxy *_PtyProxy_,
	readerTerminalSignal error,
) {
}

func (this *_WorkspaceController_) HandleProcessExit(
	ptyId uint32,
	exitResult _PtyExitResult_,
) {
}
