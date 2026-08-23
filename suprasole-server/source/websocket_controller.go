package source

import (
	_CONTEXT   "context"
	_ERRORS    "errors"
	_FMT       "fmt"
	_IO        "io"
	_NET       "net"
	_HTTP      "net/http"
	_SYNC      "sync"
	_TIME      "time"

	_WEBSOCKET "github.com/gorilla/websocket"
)

var SUPERSEDED_ERROR__GET_WEBSOCKET_CONNECTION_SUBMISSION = _ERRORS.New("get websocket connection submission superseded by newer entry")

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
	Mutex                       _SYNC.Mutex
	EgressMutex                 _SYNC.Mutex
	ConnectionStatus            WebsocketConnectionStatus
	IsTakeoverPending           bool
	WebsocketConnection         *_WEBSOCKET.Conn
	ReadDeadlineTimeout         _TIME.Duration
	WriteDeadlineTimeout        _TIME.Duration
	LifecycleLoopContext        _CONTEXT.Context
	LifecycleLoopCancel         _CONTEXT.CancelFunc
	SubmissionQueue             chan _Submission_GetWebsocketConnection_
	OnConnected                 func()
	OnTakeoverConnected         func()
	OnDisconnected              func(readMessageError error)
	OnTakeoverDisconnected      func()
	OnBinaryMessage             func(binaryMessageFrame []byte)
}

func (this *_WebsocketController_) HandleGetPtyRequest(
	responseWriter_getPty _HTTP.ResponseWriter,
	request_getPty *_HTTP.Request,
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
		HttpRequest:    request_getPty,
		ResponseWriter: responseWriter_getPty,
	}
	select {
	case this.SubmissionQueue <- submission_getWebsocketConnection:
	default:
		_HTTP.Error(
			responseWriter_getPty,
			"server busy",
			_HTTP.StatusServiceUnavailable,
		)
		return
	}
	select {
	case <-request_getPty.Context().Done():
		_HTTP.Error(
			responseWriter_getPty,
			"request canceled",
			499,
		)
		return
	case submissionReply_getWebsocketConnection := <-submissionReplyChannel_getWebsocketConnection:
		if nil == submissionReply_getWebsocketConnection.MaybeSubmissionError {
			return
		} else if _ERRORS.Is(submissionReply_getWebsocketConnection.MaybeSubmissionError, SUPERSEDED_ERROR__GET_WEBSOCKET_CONNECTION_SUBMISSION) {
			_HTTP.Error(
				responseWriter_getPty,
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
			_FMT.Println("invalid path: HandleGetPtyRequest")
			return
		}
	}
}

func (this *_WebsocketController_) RunLifecycleLoop() {
	for {
		select {
		case <-this.LifecycleLoopContext.Done():
			return
		case leadingSubmission_getWebsocketConnection := <-this.SubmissionQueue:
			latestSubmission_getWebsocketConnection := this.GetLatestSubmissionAndRejectPreceding_SubmissionQueue(leadingSubmission_getWebsocketConnection)
			if this.UpdateWebsocketConnection(latestSubmission_getWebsocketConnection) {
				for {
					messageType, binaryMessageFrame, readMessageError := this.WebsocketConnection.ReadMessage()
					if _WEBSOCKET.BinaryMessage == messageType {
						this.OnBinaryMessage(binaryMessageFrame)
					} else if readMessageError != nil {
						this.HandleConnectionTeardown(readMessageError)
						break
					} else if _WEBSOCKET.TextMessage == messageType {
						this.CloseWithCode(
							1003,
							"Text Frames Unsupported",
						)
						break
					} else {
						_FMT.Println("invalid path: RunLifecycleLoop read loop")
					}
				}
			}
		}
	}
}

func (this *_WebsocketController_) GetLatestSubmissionAndRejectPreceding_SubmissionQueue(
	leadingSubmission_getWebsocketConnection _Submission_GetWebsocketConnection_,
) _Submission_GetWebsocketConnection_ {
	latestSubmission_getWebsocketConnection := leadingSubmission_getWebsocketConnection
	for {
		select {
		case nextSubmission_getWebsocketConnection := <-this.SubmissionQueue:
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
) bool {
	this.Mutex.Lock()
	if this.ConnectionStatus != TAKEOVER_CONNECTING__WebsocketConnectionStatus {
		this.ConnectionStatus = CONNECTING__WebsocketConnectionStatus
	}
	this.Mutex.Unlock()
	websocketRequestUpgrader := _WEBSOCKET.Upgrader{}
	newWebsocketConnection, upgradeRequestError := websocketRequestUpgrader.Upgrade(
		latestSubmission_getWebsocketConnection.ResponseWriter,
		latestSubmission_getWebsocketConnection.HttpRequest,
		nil,
	)
	latestSubmission_getWebsocketConnection.ReplyChannel <- _Reply_GetWebsocketConnection_{
		MaybeSubmissionError: upgradeRequestError,
	}
	this.Mutex.Lock()
	isTakeoverConnecting := this.ConnectionStatus == TAKEOVER_CONNECTING__WebsocketConnectionStatus
	this.Mutex.Unlock()
	if newWebsocketConnection != nil && isTakeoverConnecting {
		this.AttachConnection(
			newWebsocketConnection,
			this.OnTakeoverConnected,
		)
		return true
	} else if newWebsocketConnection != nil {
		this.AttachConnection(
			newWebsocketConnection,
			this.OnConnected,
		)
		return true
	} else if upgradeRequestError != nil && isTakeoverConnecting {
		this.Mutex.Lock()
		this.ConnectionStatus = TAKEOVER_UPGRADE_FAILED__WebsocketConnectionStatus
		this.Mutex.Unlock()
		return false
	} else if upgradeRequestError != nil {
		this.Mutex.Lock()
		this.ConnectionStatus = UPGRADE_FAILED__WebsocketConnectionStatus
		this.Mutex.Unlock()
		return false
	} else {
		_FMT.Println("invalid path: UpdateWebsocketConnection")
		return false
	}
}

func (this *_WebsocketController_) AttachConnection(
	newWebsocketConnection *_WEBSOCKET.Conn,
	onConnectedCallback func(),
) {
	this.Mutex.Lock()
	this.WebsocketConnection = newWebsocketConnection
	_ = this.WebsocketConnection.SetReadDeadline(
		_TIME.Now().Add(this.ReadDeadlineTimeout),
	)
	this.ConnectionStatus = CONNECTED__WebsocketConnectionStatus
	this.Mutex.Unlock()
	onConnectedCallback()
}

func (this *_WebsocketController_) HandleConnectionTeardown(
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
		this.OnTakeoverDisconnected()
	} else {
		this.OnDisconnected(readMessageError)
	}
}

func (this *_WebsocketController_) WriteBinaryMessage(
	binaryMessageData []byte,
) error {
	var capturedWebsocketConnection *_WEBSOCKET.Conn
	this.Mutex.Lock()
	if CONNECTED__WebsocketConnectionStatus == this.ConnectionStatus {
		capturedWebsocketConnection = this.WebsocketConnection
	}
	this.Mutex.Unlock()
	if capturedWebsocketConnection != nil {
		this.EgressMutex.Lock()
		_ = capturedWebsocketConnection.SetWriteDeadline(
			_TIME.Now().Add(this.WriteDeadlineTimeout),
		)
		writeMessageError := capturedWebsocketConnection.WriteMessage(
			_WEBSOCKET.BinaryMessage,
			binaryMessageData,
		)
		this.EgressMutex.Unlock()
		return writeMessageError
	}
	return _ERRORS.New("websocket is not connected")
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
			_TIME.Now().Add(this.WriteDeadlineTimeout),
		)
		_ = capturedWebsocketConnection.Close()
		this.EgressMutex.Unlock()
	}
}
