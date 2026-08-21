package source

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

type WebsocketStatus int

const (
	WebsocketStatus_Standby WebsocketStatus = iota
	WebsocketStatus_Connecting
	WebsocketStatus_TakeoverConnecting
	WebsocketStatus_UpgradeFailed
	WebsocketStatus_TakeoverUpgradeFailed
	WebsocketStatus_Connected
	WebsocketStatus_Disconnected
)

type _Reply_GetWebsocketConnection_ struct {
	Reply_MaybeError error
}

type _Submission_GetWebsocketConnection_ struct {
	Submission_ReplyChannel   chan _Reply_GetWebsocketConnection_
	Submission_HttpRequest    *_HTTP.Request
	Submission_ResponseWriter _HTTP.ResponseWriter
}

type _WebsocketProxy_ struct {
	Websocket_Mutex                  _SYNC.Mutex
	Websocket_Status                 WebsocketStatus
	Websocket_IsTakeoverPending      bool
	Websocket_Connection             *_WEBSOCKET.Conn
	Websocket_ReadDeadlineTimeout    _TIME.Duration
	Websocket_WriteDeadlineTimeout   _TIME.Duration
	Websocket_LifecycleLoopContext   _CONTEXT.Context
	Websocket_LifecycleLoopCancel    _CONTEXT.CancelFunc
	Websocket_SubmissionQueue        chan _Submission_GetWebsocketConnection_
	Websocket_OnConnected            func()
	Websocket_OnTakeoverConnected    func()
	Websocket_OnDisconnected         func(readMessageError error)
	Websocket_OnTakeoverDisconnected func()
	Websocket_OnBinaryMessage        func(binaryMessageFrame []byte)
}

var SUPERSEDED_ERROR__GET_WEBSOCKET_CONNECTION_SUBMISSION = _ERRORS.New("get websocket connection submission superseded by newer entry")

func (thisWebsocket *_WebsocketProxy_) Websocket_HandleGetPtyRequest(
	responseWriter_getPty _HTTP.ResponseWriter,
	request_getPty *_HTTP.Request,
) {
	thisWebsocket.Websocket_Mutex.Lock()
	if WebsocketStatus_Connected == thisWebsocket.Websocket_Status {
		thisWebsocket.Websocket_IsTakeoverPending = true
		thisWebsocket.Websocket_CloseWithCode(
			4000,
			"Session Taken Over",
		)
	}
	thisWebsocket.Websocket_Mutex.Unlock()
	submissionReplyChannel_getWebsocketConnection := make(
		chan _Reply_GetWebsocketConnection_,
		1,
	)
	submission_getWebsocketConnection := _Submission_GetWebsocketConnection_{
		Submission_ReplyChannel:   submissionReplyChannel_getWebsocketConnection,
		Submission_HttpRequest:    request_getPty,
		Submission_ResponseWriter: responseWriter_getPty,
	}
	select {
	case thisWebsocket.Websocket_SubmissionQueue <- submission_getWebsocketConnection:
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
		if nil == submissionReply_getWebsocketConnection.Reply_MaybeError {
			return
		} else if _ERRORS.Is(submissionReply_getWebsocketConnection.Reply_MaybeError, SUPERSEDED_ERROR__GET_WEBSOCKET_CONNECTION_SUBMISSION) {
			_HTTP.Error(
				responseWriter_getPty,
				submissionReply_getWebsocketConnection.Reply_MaybeError.Error(),
				_HTTP.StatusConflict,
			)
			return
		} else if _ERRORS.As(submissionReply_getWebsocketConnection.Reply_MaybeError, new(_WEBSOCKET.HandshakeError)) ||
			_ERRORS.Is(submissionReply_getWebsocketConnection.Reply_MaybeError, _HTTP.ErrNotSupported) {
			return
		} else if _ERRORS.As(submissionReply_getWebsocketConnection.Reply_MaybeError, new(*_NET.OpError)) ||
			_ERRORS.Is(submissionReply_getWebsocketConnection.Reply_MaybeError, _IO.EOF) ||
			_ERRORS.Is(submissionReply_getWebsocketConnection.Reply_MaybeError, _IO.ErrUnexpectedEOF) {
			return
		} else {
			// submissionReply_getWebsocketConnection.Reply_MaybeError != nil
			_FMT.Println("invalid path: Websocket_HandleGetPtyRequest")
			return
		}
	}
}

func (thisWebsocket *_WebsocketProxy_) Websocket_RunLifecycleLoop() {
	for {
		select {
		case <-thisWebsocket.Websocket_LifecycleLoopContext.Done():
			return
		case leadingSubmission_getWebsocketConnection := <-thisWebsocket.Websocket_SubmissionQueue:
			latestSubmission_getWebsocketConnection := thisWebsocket.SubmissionQueue_GetLatestSubmissionAndRejectPreceding(leadingSubmission_getWebsocketConnection)
			if thisWebsocket.Websocket_UpdateWebsocketConnection(latestSubmission_getWebsocketConnection) {
				for {
					messageType, binaryMessageFrame, readMessageError := thisWebsocket.Websocket_Connection.ReadMessage()
					if _WEBSOCKET.BinaryMessage == messageType {
						thisWebsocket.Websocket_OnBinaryMessage(binaryMessageFrame)
					} else if readMessageError != nil {
						thisWebsocket.Websocket_HandleConnectionTeardown(readMessageError)
						break
					} else if _WEBSOCKET.TextMessage == messageType {
						_FMT.Println("unsupported websocket message type: text messages are not supported, only binary messages are supported")
					} else {
						_FMT.Println("invalid path: Websocket_RunLifecycleLoop read loop")
					}
				}
			}
		}
	}
}

func (thisWebsocket *_WebsocketProxy_) SubmissionQueue_GetLatestSubmissionAndRejectPreceding(
	leadingSubmission_getWebsocketConnection _Submission_GetWebsocketConnection_,
) _Submission_GetWebsocketConnection_ {
	latestSubmission_getWebsocketConnection := leadingSubmission_getWebsocketConnection
	for {
		select {
		case nextSubmission_getWebsocketConnection := <-thisWebsocket.Websocket_SubmissionQueue:
			latestSubmission_getWebsocketConnection.Submission_ReplyChannel <- _Reply_GetWebsocketConnection_{
				Reply_MaybeError: SUPERSEDED_ERROR__GET_WEBSOCKET_CONNECTION_SUBMISSION,
			}
			latestSubmission_getWebsocketConnection = nextSubmission_getWebsocketConnection
		default:
			return latestSubmission_getWebsocketConnection
		}
	}
}

func (thisWebsocket *_WebsocketProxy_) Websocket_UpdateWebsocketConnection(
	latestSubmission_getWebsocketConnection _Submission_GetWebsocketConnection_,
) bool {
	thisWebsocket.Websocket_Mutex.Lock()
	if thisWebsocket.Websocket_Status != WebsocketStatus_TakeoverConnecting {
		thisWebsocket.Websocket_Status = WebsocketStatus_Connecting
	}
	thisWebsocket.Websocket_Mutex.Unlock()
	websocketRequestUpgrader := _WEBSOCKET.Upgrader{}
	newWebsocketConnection, upgradeRequestError := websocketRequestUpgrader.Upgrade(
		latestSubmission_getWebsocketConnection.Submission_ResponseWriter,
		latestSubmission_getWebsocketConnection.Submission_HttpRequest,
		nil,
	)
	latestSubmission_getWebsocketConnection.Submission_ReplyChannel <- _Reply_GetWebsocketConnection_{
		Reply_MaybeError: upgradeRequestError,
	}
	thisWebsocket.Websocket_Mutex.Lock()
	isTakeoverConnecting := thisWebsocket.Websocket_Status == WebsocketStatus_TakeoverConnecting
	thisWebsocket.Websocket_Mutex.Unlock()
	if newWebsocketConnection != nil && isTakeoverConnecting {
		thisWebsocket.Websocket_AttachConnection(
			newWebsocketConnection,
			thisWebsocket.Websocket_OnTakeoverConnected,
		)
		return true
	} else if newWebsocketConnection != nil {
		thisWebsocket.Websocket_AttachConnection(
			newWebsocketConnection,
			thisWebsocket.Websocket_OnConnected,
		)
		return true
	} else if upgradeRequestError != nil && isTakeoverConnecting {
		thisWebsocket.Websocket_Mutex.Lock()
		thisWebsocket.Websocket_Status = WebsocketStatus_TakeoverUpgradeFailed
		thisWebsocket.Websocket_Mutex.Unlock()
		return false
	} else if upgradeRequestError != nil {
		thisWebsocket.Websocket_Mutex.Lock()
		thisWebsocket.Websocket_Status = WebsocketStatus_UpgradeFailed
		thisWebsocket.Websocket_Mutex.Unlock()
		return false
	} else {
		_FMT.Println("invalid path: Websocket_UpdateWebsocketConnection")
		return false
	}
}

func (thisWebsocket *_WebsocketProxy_) Websocket_AttachConnection(
	newWebsocketConnection *_WEBSOCKET.Conn,
	onConnectedCallback func(),
) {
	thisWebsocket.Websocket_Mutex.Lock()
	thisWebsocket.Websocket_Connection = newWebsocketConnection
	_ = thisWebsocket.Websocket_Connection.SetReadDeadline(
		_TIME.Now().Add(thisWebsocket.Websocket_ReadDeadlineTimeout),
	)
	thisWebsocket.Websocket_Status = WebsocketStatus_Connected
	thisWebsocket.Websocket_Mutex.Unlock()
	onConnectedCallback()
}

func (thisWebsocket *_WebsocketProxy_) Websocket_HandleConnectionTeardown(
	readMessageError error,
) {
	var wasTakeoverPending bool
	thisWebsocket.Websocket_Mutex.Lock()
	if thisWebsocket.Websocket_IsTakeoverPending {
		thisWebsocket.Websocket_Status = WebsocketStatus_TakeoverConnecting
		thisWebsocket.Websocket_IsTakeoverPending = false
		thisWebsocket.Websocket_Connection = nil
		wasTakeoverPending = true
	} else {
		thisWebsocket.Websocket_Status = WebsocketStatus_Disconnected
		_ = thisWebsocket.Websocket_Connection.Close()
		thisWebsocket.Websocket_Connection = nil
	}
	thisWebsocket.Websocket_Mutex.Unlock()
	if wasTakeoverPending {
		thisWebsocket.Websocket_OnTakeoverDisconnected()
	} else {
		thisWebsocket.Websocket_OnDisconnected(readMessageError)
	}
}

func (thisWebsocket *_WebsocketProxy_) Websocket_WriteBinaryMessage(
	binaryMessageData []byte,
) error {
	thisWebsocket.Websocket_Mutex.Lock()
	defer thisWebsocket.Websocket_Mutex.Unlock()
	if WebsocketStatus_Connected == thisWebsocket.Websocket_Status {
		_ = thisWebsocket.Websocket_Connection.SetWriteDeadline(
			_TIME.Now().Add(thisWebsocket.Websocket_WriteDeadlineTimeout),
		)
		return thisWebsocket.Websocket_Connection.WriteMessage(
			_WEBSOCKET.BinaryMessage,
			binaryMessageData,
		)
	}
	return _ERRORS.New("websocket is not connected")
}

func (thisWebsocket *_WebsocketProxy_) Websocket_CloseWithCode(
	websocketCloseCode int,
	websocketCloseReason string,
) {
	thisWebsocket.Websocket_Mutex.Lock()
	defer thisWebsocket.Websocket_Mutex.Unlock()
	if WebsocketStatus_Connected == thisWebsocket.Websocket_Status {
		_ = thisWebsocket.Websocket_Connection.WriteControl(
			_WEBSOCKET.CloseMessage,
			_WEBSOCKET.FormatCloseMessage(
				websocketCloseCode,
				websocketCloseReason,
			),
			_TIME.Now().Add(thisWebsocket.Websocket_WriteDeadlineTimeout),
		)
		_ = thisWebsocket.Websocket_Connection.Close()
	}
}
