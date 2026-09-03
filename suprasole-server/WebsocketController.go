package main

import (
	_CONTEXT "context"
	_ERRORS "errors"
	_FMT "fmt"
	_IO "io"
	_NET "net"
	_HTTP "net/http"
	_SYNC "sync"
	_TIME "time"

	_WEBSOCKET "github.com/gorilla/websocket"
)

var SUPERSEDED_ERROR__GET_WEBSOCKET_CONNECTION_SUBMISSION = _ERRORS.New("get websocket connection submission superseded by newer entry")
var NOT_CONNECTED_ERROR__WRITE_BINARY_MESSAGE = _ERRORS.New("websocket is not connected")
var CONNECTION_ID_MISALIGNED_ERROR__WRITE_BINARY_MESSAGE = _ERRORS.New("websocket connection id does not align")

type WebsocketConnectionStatus int

const (
	STANDBY__WebsocketConnectionStatus WebsocketConnectionStatus = iota
	CONNECTING__WebsocketConnectionStatus
	TAKEOVER_CONNECTING__WebsocketConnectionStatus
	UPGRADE_FAILED__WebsocketConnectionStatus
	TAKEOVER_UPGRADE_FAILED__WebsocketConnectionStatus
	CONNECTED__WebsocketConnectionStatus
	DISCONNECTED__WebsocketConnectionStatus
)

type _Reply_GetWebsocketConnection_ struct {
	MaybeSubmissionError error
}

type _Submission_GetWebsocketConnection_ struct {
	ReplyChannel   chan _Reply_GetWebsocketConnection_
	HttpRequest    *_HTTP.Request
	ResponseWriter _HTTP.ResponseWriter
}

type _WebsocketController_ struct {
	Mutex                                            _SYNC.Mutex
	EgressMutex                                      _SYNC.Mutex
	ConnectionStatus                                 WebsocketConnectionStatus
	IsTakeoverPending                                bool
	Id_WebsocketConnection                           uint64
	WebsocketConnection                              *_WEBSOCKET.Conn
	ReadDeadlineTimeout__                            _TIME.Duration
	WriteDeadlineTimeout__                           _TIME.Duration
	OnConnected__                                    func(id_WebsocketConnection uint64)
	OnTakeoverConnected__                            func(id_WebsocketConnection uint64)
	OnDisconnected__                                 func(id_WebsocketConnection uint64, readMessageError error)
	OnTakeoverDisconnected__                         func(id_WebsocketConnection uint64)
	OnBinaryMessageFrame__                           func(id_WebsocketConnection uint64, binaryMessageFrame []byte)
	QueueChannel__Submission_GetWebsocketConnection  chan _Submission_GetWebsocketConnection_
	WorkerContext__Submission_GetWebsocketConnection _CONTEXT.Context
	WorkerCancel__Submission_GetWebsocketConnection  _CONTEXT.CancelFunc
}

func (this *_WebsocketController_) HandleRequest_GetWebsocketConnection(
	responseWriter_getWebsocketConnection _HTTP.ResponseWriter,
	request_getWebsocketConnection *_HTTP.Request,
) {
	this.Mutex.Lock()
	shouldCloseForTakeover := CONNECTED__WebsocketConnectionStatus == this.ConnectionStatus
	if shouldCloseForTakeover {
		this.IsTakeoverPending = true
	}
	this.Mutex.Unlock()
	if shouldCloseForTakeover {
		this.CloseWithCode(
			4000,
			"Session Taken Over",
		)
	}
	submissionReplyChannel_getWebsocketConnection := make(
		chan _Reply_GetWebsocketConnection_,
		1,
	)
	submission_getWebsocketConnection := _Submission_GetWebsocketConnection_{
		ReplyChannel:   submissionReplyChannel_getWebsocketConnection,
		HttpRequest:    request_getWebsocketConnection,
		ResponseWriter: responseWriter_getWebsocketConnection,
	}
	select {
	case this.QueueChannel__Submission_GetWebsocketConnection <- submission_getWebsocketConnection:
	default:
		_HTTP.Error(
			responseWriter_getWebsocketConnection,
			"server busy",
			_HTTP.StatusServiceUnavailable,
		)
		return
	}
	select {
	case <-request_getWebsocketConnection.Context().Done():
		_HTTP.Error(
			responseWriter_getWebsocketConnection,
			"request canceled",
			499,
		)
		return
	case submissionReply_getWebsocketConnection := <-submissionReplyChannel_getWebsocketConnection:
		if nil == submissionReply_getWebsocketConnection.MaybeSubmissionError {
			return
		} else if _ERRORS.Is(submissionReply_getWebsocketConnection.MaybeSubmissionError, SUPERSEDED_ERROR__GET_WEBSOCKET_CONNECTION_SUBMISSION) {
			_HTTP.Error(
				responseWriter_getWebsocketConnection,
				submissionReply_getWebsocketConnection.MaybeSubmissionError.Error(),
				_HTTP.StatusConflict,
			)
			return
		} else if _ERRORS.As(submissionReply_getWebsocketConnection.MaybeSubmissionError, new(_WEBSOCKET.HandshakeError)) ||
			_ERRORS.Is(submissionReply_getWebsocketConnection.MaybeSubmissionError, _HTTP.ErrNotSupported) {
			return
		} else if _ERRORS.As(submissionReply_getWebsocketConnection.MaybeSubmissionError, new(*_NET.OpError)) ||
			_ERRORS.Is(submissionReply_getWebsocketConnection.MaybeSubmissionError, _IO.EOF) ||
			_ERRORS.Is(submissionReply_getWebsocketConnection.MaybeSubmissionError, _IO.ErrUnexpectedEOF) {
			return
		} else {
			// submissionReply_getWebsocketConnection.MaybeSubmissionError != nil
			_FMT.Println("invalid path: HandleRequest_GetWebsocketConnection")
			return
		}
	}
}

func (this *_WebsocketController_) RunWorker__Submission_GetWebsocketConnection() {
	for {
		select {
		case <-this.WorkerContext__Submission_GetWebsocketConnection.Done():
			return
		case leadingSubmission_getWebsocketConnection := <-this.QueueChannel__Submission_GetWebsocketConnection:
			latestSubmission_getWebsocketConnection := this.GetLatestSubmissionAndRejectPreceding__Submission_GetWebsocketConnection(leadingSubmission_getWebsocketConnection)
			isAttached, newId_WebsocketConnection := this.UpdateWebsocketConnection(latestSubmission_getWebsocketConnection)
			if isAttached {
				for {
					messageType, binaryMessageFrame, readMessageError := this.WebsocketConnection.ReadMessage()
					if _WEBSOCKET.BinaryMessage == messageType {
						this.OnBinaryMessageFrame__(
							newId_WebsocketConnection,
							binaryMessageFrame,
						)
					} else if readMessageError != nil {
						this.HandleConnectionTeardown(
							newId_WebsocketConnection,
							readMessageError,
						)
						break
					} else if _WEBSOCKET.TextMessage == messageType {
						this.CloseWithCode(
							1003,
							"Text Frames Unsupported",
						)
						break
					} else {
						_FMT.Println("invalid path: RunWorker__Submission_GetWebsocketConnection read loop")
					}
				}
			}
		}
	}
}

func (this *_WebsocketController_) GetLatestSubmissionAndRejectPreceding__Submission_GetWebsocketConnection(
	leadingSubmission_getWebsocketConnection _Submission_GetWebsocketConnection_,
) _Submission_GetWebsocketConnection_ {
	latestSubmission_getWebsocketConnection := leadingSubmission_getWebsocketConnection
	for {
		select {
		case nextSubmission_getWebsocketConnection := <-this.QueueChannel__Submission_GetWebsocketConnection:
			latestSubmission_getWebsocketConnection.ReplyChannel <- _Reply_GetWebsocketConnection_{
				MaybeSubmissionError: SUPERSEDED_ERROR__GET_WEBSOCKET_CONNECTION_SUBMISSION,
			}
			latestSubmission_getWebsocketConnection = nextSubmission_getWebsocketConnection
		default:
			return latestSubmission_getWebsocketConnection
		}
	}
}

func (this *_WebsocketController_) UpdateWebsocketConnection(
	latestSubmission_getWebsocketConnection _Submission_GetWebsocketConnection_,
) (bool, uint64) {
	this.Mutex.Lock()
	if this.ConnectionStatus != TAKEOVER_CONNECTING__WebsocketConnectionStatus {
		this.ConnectionStatus = CONNECTING__WebsocketConnectionStatus
	}
	this.Mutex.Unlock()
	var nilUpgradeResponseHeader _HTTP.Header = nil
	websocketRequestUpgrader := _WEBSOCKET.Upgrader{}
	newWebsocketConnection, upgradeRequestError := websocketRequestUpgrader.Upgrade(
		latestSubmission_getWebsocketConnection.ResponseWriter,
		latestSubmission_getWebsocketConnection.HttpRequest,
		nilUpgradeResponseHeader,
	)
	latestSubmission_getWebsocketConnection.ReplyChannel <- _Reply_GetWebsocketConnection_{
		MaybeSubmissionError: upgradeRequestError,
	}
	this.Mutex.Lock()
	isTakeoverConnecting := this.ConnectionStatus == TAKEOVER_CONNECTING__WebsocketConnectionStatus
	this.Mutex.Unlock()
	if newWebsocketConnection != nil && isTakeoverConnecting {
		newId_WebsocketConnection := this.AttachConnection(
			newWebsocketConnection,
			this.OnTakeoverConnected__,
		)
		return true, newId_WebsocketConnection
	} else if newWebsocketConnection != nil {
		newId_WebsocketConnection := this.AttachConnection(
			newWebsocketConnection,
			this.OnConnected__,
		)
		return true, newId_WebsocketConnection
	} else if upgradeRequestError != nil && isTakeoverConnecting {
		this.Mutex.Lock()
		this.ConnectionStatus = TAKEOVER_UPGRADE_FAILED__WebsocketConnectionStatus
		this.Mutex.Unlock()
		return false, 0
	} else if upgradeRequestError != nil {
		this.Mutex.Lock()
		this.ConnectionStatus = UPGRADE_FAILED__WebsocketConnectionStatus
		this.Mutex.Unlock()
		return false, 0
	} else {
		_FMT.Println("invalid path: UpdateWebsocketConnection")
		return false, 0
	}
}

func (this *_WebsocketController_) AttachConnection(
	newWebsocketConnection *_WEBSOCKET.Conn,
	onConnectedCallback func(id_WebsocketConnection uint64),
) uint64 {
	this.Mutex.Lock()
	this.Id_WebsocketConnection++
	newId_WebsocketConnection := this.Id_WebsocketConnection
	this.WebsocketConnection = newWebsocketConnection
	_ = this.WebsocketConnection.SetReadDeadline(
		_TIME.Now().Add(this.ReadDeadlineTimeout__),
	)
	this.ConnectionStatus = CONNECTED__WebsocketConnectionStatus
	this.Mutex.Unlock()
	onConnectedCallback(newId_WebsocketConnection)
	return newId_WebsocketConnection
}

func (this *_WebsocketController_) HandleConnectionTeardown(
	id_WebsocketConnection uint64,
	readMessageError error,
) {
	var wasTakeoverPending bool
	this.Mutex.Lock()
	if this.IsTakeoverPending {
		this.ConnectionStatus = TAKEOVER_CONNECTING__WebsocketConnectionStatus
		this.IsTakeoverPending = false
		this.WebsocketConnection = nil
		wasTakeoverPending = true
	} else {
		this.ConnectionStatus = DISCONNECTED__WebsocketConnectionStatus
		_ = this.WebsocketConnection.Close()
		this.WebsocketConnection = nil
	}
	this.Mutex.Unlock()
	if wasTakeoverPending {
		this.OnTakeoverDisconnected__(id_WebsocketConnection)
	} else {
		this.OnDisconnected__(
			id_WebsocketConnection,
			readMessageError,
		)
	}
}

func (this *_WebsocketController_) WriteBinaryMessage(
	targetId_WebsocketConnection uint64,
	binaryMessageData []byte,
) error {
	var capturedWebsocketConnection *_WEBSOCKET.Conn
	var writePreparationError error
	this.Mutex.Lock()
	if this.ConnectionStatus != CONNECTED__WebsocketConnectionStatus {
		writePreparationError = NOT_CONNECTED_ERROR__WRITE_BINARY_MESSAGE
	} else if this.Id_WebsocketConnection != targetId_WebsocketConnection {
		writePreparationError = CONNECTION_ID_MISALIGNED_ERROR__WRITE_BINARY_MESSAGE
	} else {
		capturedWebsocketConnection = this.WebsocketConnection
	}
	this.Mutex.Unlock()
	if writePreparationError != nil {
		return writePreparationError
	}
	this.EgressMutex.Lock()
	_ = capturedWebsocketConnection.SetWriteDeadline(
		_TIME.Now().Add(this.WriteDeadlineTimeout__),
	)
	writeMessageError := capturedWebsocketConnection.WriteMessage(
		_WEBSOCKET.BinaryMessage,
		binaryMessageData,
	)
	this.EgressMutex.Unlock()
	return writeMessageError
}

func (this *_WebsocketController_) CloseWithCode(
	websocketCloseCode int,
	websocketCloseReason string,
) {
	var capturedWebsocketConnection *_WEBSOCKET.Conn
	this.Mutex.Lock()
	if CONNECTED__WebsocketConnectionStatus == this.ConnectionStatus {
		capturedWebsocketConnection = this.WebsocketConnection
	}
	this.Mutex.Unlock()
	if capturedWebsocketConnection != nil {
		this.EgressMutex.Lock()
		_ = capturedWebsocketConnection.WriteControl(
			_WEBSOCKET.CloseMessage,
			_WEBSOCKET.FormatCloseMessage(
				websocketCloseCode,
				websocketCloseReason,
			),
			_TIME.Now().Add(this.WriteDeadlineTimeout__),
		)
		_ = capturedWebsocketConnection.Close()
		this.EgressMutex.Unlock()
	}
}
