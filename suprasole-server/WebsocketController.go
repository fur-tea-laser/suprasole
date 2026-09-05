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

var SUPERSEDED__ERROR___SUBMISSION__GET_WEBSOCKET_CONNECTION = _ERRORS.New("get websocket connection submission superseded by newer entry")
var NOT_CONNECTED__ERROR___WRITE_PAYLOAD__BINARY_MESSAGE = _ERRORS.New("websocket is not connected")
var CONNECTION_ID_MISALIGNED__ERROR___WRITE_PAYLOAD__BINARY_MESSAGE = _ERRORS.New("websocket connection id does not align")

type _Status_WebsocketConnection_ int

const (
	STANDBY__Status_WebsocketConnection _Status_WebsocketConnection_ = iota
	CONNECTING__Status_WebsocketConnection
	TAKEOVER_CONNECTING__Status_WebsocketConnection
	UPGRADE_FAILED__Status_WebsocketConnection
	TAKEOVER_UPGRADE_FAILED__Status_WebsocketConnection
	CONNECTED__Status_WebsocketConnection
	DISCONNECTED__Status_WebsocketConnection
)

type _Reply_GetWebsocketConnection_ struct {
	MaybeError_Submission error
}

type _Submission_GetWebsocketConnection_ struct {
	ReplyChannel   chan _Reply_GetWebsocketConnection_
	HttpRequest    *_HTTP.Request
	ResponseWriter _HTTP.ResponseWriter
}

type _WebsocketController_ struct {
	DeadlineTimeout_Read__                           _TIME.Duration
	DeadlineTimeout_Write__                          _TIME.Duration
	OnConnected__                                    func(newId_WebsocketConnection uint64)
	OnTakeoverConnected__                            func(newId_WebsocketConnection uint64)
	OnDisconnected__                                 func()
	OnTakeoverDisconnected__                         func()
	OnPayload_BinaryMessage__                        func(expectedId_WebsocketConnection uint64, payload_binaryMessage []byte)
	Mutex                                            _SYNC.Mutex
	EgressMutex                                      _SYNC.Mutex
	WebsocketConnection                              *_WEBSOCKET.Conn
	Status_WebsocketConnection                       _Status_WebsocketConnection_
	Id_WebsocketConnection                           uint64
	IsTakeoverPending_WebsocketConnection            bool
	QueueChannel__Submission_GetWebsocketConnection  chan _Submission_GetWebsocketConnection_
	WorkerContext__Submission_GetWebsocketConnection _CONTEXT.Context
	WorkerCancel__Submission_GetWebsocketConnection  _CONTEXT.CancelFunc
}

func (this *_WebsocketController_) HandleRequest_GetWebsocketConnection(
	responseWriter_getWebsocketConnection _HTTP.ResponseWriter,
	request_getWebsocketConnection *_HTTP.Request,
) {
	this.Mutex.Lock()
	shouldCloseForTakeover := this.Status_WebsocketConnection == CONNECTED__Status_WebsocketConnection
	if shouldCloseForTakeover {
		this.IsTakeoverPending_WebsocketConnection = true
	}
	this.Mutex.Unlock()
	if shouldCloseForTakeover {
		this.CloseWithCode(
			4000,
			"Session Taken Over",
		)
	}
	submissionReplyChannel_getWebsocketConnection := make(chan _Reply_GetWebsocketConnection_, 1)
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
		if nil == submissionReply_getWebsocketConnection.MaybeError_Submission {
			return
		} else if _ERRORS.Is(submissionReply_getWebsocketConnection.MaybeError_Submission, SUPERSEDED__ERROR___SUBMISSION__GET_WEBSOCKET_CONNECTION) {
			_HTTP.Error(
				responseWriter_getWebsocketConnection,
				submissionReply_getWebsocketConnection.MaybeError_Submission.Error(),
				_HTTP.StatusConflict,
			)
			return
		} else if _ERRORS.As(submissionReply_getWebsocketConnection.MaybeError_Submission, new(_WEBSOCKET.HandshakeError)) ||
			_ERRORS.Is(submissionReply_getWebsocketConnection.MaybeError_Submission, _HTTP.ErrNotSupported) {
			return
		} else if _ERRORS.As(submissionReply_getWebsocketConnection.MaybeError_Submission, new(*_NET.OpError)) ||
			_ERRORS.Is(submissionReply_getWebsocketConnection.MaybeError_Submission, _IO.EOF) ||
			_ERRORS.Is(submissionReply_getWebsocketConnection.MaybeError_Submission, _IO.ErrUnexpectedEOF) {
			return
		} else {
			// submissionReply_getWebsocketConnection.MaybeError_Submission != nil
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
			isConnected_WebsocketConnection, newId_WebsocketConnection := this.UpdateWebsocketConnection(latestSubmission_getWebsocketConnection)
			if isConnected_WebsocketConnection {
				for {
					messageType_Gorilla, payload_binaryMessage, readError_WebsocketConnection := this.WebsocketConnection.ReadMessage()
					if _WEBSOCKET.BinaryMessage == messageType_Gorilla {
						this.OnPayload_BinaryMessage__(
							newId_WebsocketConnection,
							payload_binaryMessage,
						)
					} else if readError_WebsocketConnection != nil {
						this.HandleConnectionTeardown()
						break
					} else if _WEBSOCKET.TextMessage == messageType_Gorilla {
						this.CloseWithCode(
							1003,
							"Text Payloads Unsupported",
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
				MaybeError_Submission: SUPERSEDED__ERROR___SUBMISSION__GET_WEBSOCKET_CONNECTION,
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
	if this.Status_WebsocketConnection != TAKEOVER_CONNECTING__Status_WebsocketConnection {
		this.Status_WebsocketConnection = CONNECTING__Status_WebsocketConnection
	}
	this.Mutex.Unlock()
	var nilHeader_UpgradeResponse _HTTP.Header = nil
	websocketRequestUpgrader := _WEBSOCKET.Upgrader{}
	newWebsocketConnection, maybeError_UpgradeRequest := websocketRequestUpgrader.Upgrade(
		latestSubmission_getWebsocketConnection.ResponseWriter,
		latestSubmission_getWebsocketConnection.HttpRequest,
		nilHeader_UpgradeResponse,
	)
	latestSubmission_getWebsocketConnection.ReplyChannel <- _Reply_GetWebsocketConnection_{
		MaybeError_Submission: maybeError_UpgradeRequest,
	}
	this.Mutex.Lock()
	isTakeoverConnecting := this.Status_WebsocketConnection == TAKEOVER_CONNECTING__Status_WebsocketConnection
	this.Mutex.Unlock()
	if newWebsocketConnection != nil && isTakeoverConnecting {
		newId_WebsocketConnection := this.AttachConnection(
			this.OnTakeoverConnected__,
			newWebsocketConnection,
		)
		return true, newId_WebsocketConnection
	} else if newWebsocketConnection != nil {
		newId_WebsocketConnection := this.AttachConnection(
			this.OnConnected__,
			newWebsocketConnection,
		)
		return true, newId_WebsocketConnection
	} else if maybeError_UpgradeRequest != nil && isTakeoverConnecting {
		this.Mutex.Lock()
		this.Status_WebsocketConnection = TAKEOVER_UPGRADE_FAILED__Status_WebsocketConnection
		this.Mutex.Unlock()
		return false, 0
	} else if maybeError_UpgradeRequest != nil {
		this.Mutex.Lock()
		this.Status_WebsocketConnection = UPGRADE_FAILED__Status_WebsocketConnection
		this.Mutex.Unlock()
		return false, 0
	} else {
		_FMT.Println("invalid path: UpdateWebsocketConnection")
		return false, 0
	}
}

func (this *_WebsocketController_) AttachConnection(
	OnConnected__ func(newId_WebsocketConnection uint64),
	newWebsocketConnection *_WEBSOCKET.Conn,
) uint64 {
	this.Mutex.Lock()
	this.Id_WebsocketConnection++
	newId_WebsocketConnection := this.Id_WebsocketConnection
	this.WebsocketConnection = newWebsocketConnection
	_ = this.WebsocketConnection.SetReadDeadline(
		_TIME.Now().Add(this.DeadlineTimeout_Read__),
	)
	this.Status_WebsocketConnection = CONNECTED__Status_WebsocketConnection
	this.Mutex.Unlock()
	OnConnected__(newId_WebsocketConnection)
	return newId_WebsocketConnection
}

func (this *_WebsocketController_) HandleConnectionTeardown() {
	var wasTakeoverPending bool
	this.Mutex.Lock()
	if this.IsTakeoverPending_WebsocketConnection {
		this.Status_WebsocketConnection = TAKEOVER_CONNECTING__Status_WebsocketConnection
		this.IsTakeoverPending_WebsocketConnection = false
		this.WebsocketConnection = nil
		wasTakeoverPending = true
	} else {
		this.Status_WebsocketConnection = DISCONNECTED__Status_WebsocketConnection
		_ = this.WebsocketConnection.Close()
		this.WebsocketConnection = nil
	}
	this.Mutex.Unlock()
	if wasTakeoverPending {
		this.OnTakeoverDisconnected__()
	} else {
		this.OnDisconnected__()
	}
}

func (this *_WebsocketController_) WritePayload_BinaryMessage(
	expectedId_WebsocketConnection uint64,
	payload_binaryMessage []byte,
) error {
	var capturedWebsocketConnection *_WEBSOCKET.Conn
	var maybeError_state__WebsocketConnection error
	this.Mutex.Lock()
	if CONNECTED__Status_WebsocketConnection == this.Status_WebsocketConnection && expectedId_WebsocketConnection == this.Id_WebsocketConnection {
		capturedWebsocketConnection = this.WebsocketConnection
	} else if this.Status_WebsocketConnection != CONNECTED__Status_WebsocketConnection {
		maybeError_state__WebsocketConnection = NOT_CONNECTED__ERROR___WRITE_PAYLOAD__BINARY_MESSAGE
	} else if this.Id_WebsocketConnection != expectedId_WebsocketConnection {
		maybeError_state__WebsocketConnection = CONNECTION_ID_MISALIGNED__ERROR___WRITE_PAYLOAD__BINARY_MESSAGE
	} else {
		_FMT.Println("invalid path: WritePayload_BinaryMessage")
	}
	this.Mutex.Unlock()
	if maybeError_state__WebsocketConnection != nil {
		return maybeError_state__WebsocketConnection
	}
	this.EgressMutex.Lock()
	_ = capturedWebsocketConnection.SetWriteDeadline(
		_TIME.Now().Add(this.DeadlineTimeout_Write__),
	)
	maybeError_write__capturedWebsocketConnection := capturedWebsocketConnection.WriteMessage(
		_WEBSOCKET.BinaryMessage,
		payload_binaryMessage,
	)
	this.EgressMutex.Unlock()
	return maybeError_write__capturedWebsocketConnection
}

func (this *_WebsocketController_) CloseWithCode(
	closeCode_WebsocketConnection int,
	closeReason_WebsocketConnection string,
) {
	var capturedWebsocketConnection *_WEBSOCKET.Conn
	this.Mutex.Lock()
	if CONNECTED__Status_WebsocketConnection == this.Status_WebsocketConnection {
		capturedWebsocketConnection = this.WebsocketConnection
	}
	this.Mutex.Unlock()
	if capturedWebsocketConnection != nil {
		this.EgressMutex.Lock()
		_ = capturedWebsocketConnection.WriteControl(
			_WEBSOCKET.CloseMessage,
			_WEBSOCKET.FormatCloseMessage(
				closeCode_WebsocketConnection,
				closeReason_WebsocketConnection,
			),
			_TIME.Now().Add(this.DeadlineTimeout_Write__),
		)
		_ = capturedWebsocketConnection.Close()
		this.EgressMutex.Unlock()
	}
}
