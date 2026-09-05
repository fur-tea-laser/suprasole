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

type _SubmissionReply_GetWebsocketConnection_ struct {
	MaybeError_Submission error
}

type _Submission_GetWebsocketConnection_ struct {
	ResponseWriter_GetWebsocketConnection _HTTP.ResponseWriter
	Request_GetWebsocketConnection        *_HTTP.Request
	ReplyChannel                          chan _SubmissionReply_GetWebsocketConnection_
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
	responseWriter_GetWebsocketConnection _HTTP.ResponseWriter,
	request_GetWebsocketConnection *_HTTP.Request,
) {
	this.Mutex.Lock()
	shouldCloseForTakeover := this.Status_WebsocketConnection == CONNECTED__Status_WebsocketConnection
	if shouldCloseForTakeover {
		this.IsTakeoverPending_WebsocketConnection = true
	}
	this.Mutex.Unlock()
	if shouldCloseForTakeover {
		this.CloseWithCode_WebsocketConnection(
			4000,
			"Session Taken Over",
		)
	}
	submission_GetWebsocketConnection := _Submission_GetWebsocketConnection_{
		ResponseWriter_GetWebsocketConnection: responseWriter_GetWebsocketConnection,
		Request_GetWebsocketConnection:        request_GetWebsocketConnection,
		ReplyChannel:                          make(chan _SubmissionReply_GetWebsocketConnection_, 1),
	}
	select {
	case this.QueueChannel__Submission_GetWebsocketConnection <- submission_GetWebsocketConnection:
	default:
		_HTTP.Error(
			responseWriter_GetWebsocketConnection,
			"server busy",
			_HTTP.StatusServiceUnavailable,
		)
		return
	}
	select {
	case <-request_GetWebsocketConnection.Context().Done():
		_HTTP.Error(
			responseWriter_GetWebsocketConnection,
			"request canceled",
			499,
		)
		return
	case submissionReply_GetWebsocketConnection := <-submission_GetWebsocketConnection.ReplyChannel:
		if nil == submissionReply_GetWebsocketConnection.MaybeError_Submission {
			return
		} else if _ERRORS.Is(submissionReply_GetWebsocketConnection.MaybeError_Submission, SUPERSEDED__ERROR___SUBMISSION__GET_WEBSOCKET_CONNECTION) {
			_HTTP.Error(
				responseWriter_GetWebsocketConnection,
				submissionReply_GetWebsocketConnection.MaybeError_Submission.Error(),
				_HTTP.StatusConflict,
			)
			return
		} else if _ERRORS.As(submissionReply_GetWebsocketConnection.MaybeError_Submission, new(_WEBSOCKET.HandshakeError)) ||
			_ERRORS.Is(submissionReply_GetWebsocketConnection.MaybeError_Submission, _HTTP.ErrNotSupported) {
			return
		} else if _ERRORS.As(submissionReply_GetWebsocketConnection.MaybeError_Submission, new(*_NET.OpError)) ||
			_ERRORS.Is(submissionReply_GetWebsocketConnection.MaybeError_Submission, _IO.EOF) ||
			_ERRORS.Is(submissionReply_GetWebsocketConnection.MaybeError_Submission, _IO.ErrUnexpectedEOF) {
			return
		} else {
			// submissionReply_GetWebsocketConnection.MaybeError_Submission != nil
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
		case leadingSubmission_GetWebsocketConnection := <-this.QueueChannel__Submission_GetWebsocketConnection:
			latestSubmission_GetWebsocketConnection := this.GetLatestAndRejectPreceding__Submission_GetWebsocketConnection(leadingSubmission_GetWebsocketConnection)
			isConnected_WebsocketConnection, newId_WebsocketConnection := this.Update_WebsocketConnection(latestSubmission_GetWebsocketConnection)
			if isConnected_WebsocketConnection {
				for {
					messageType_Gorilla, payload_binaryMessage, maybeError_ReadMessage := this.WebsocketConnection.ReadMessage()
					if _WEBSOCKET.BinaryMessage == messageType_Gorilla {
						this.OnPayload_BinaryMessage__(
							newId_WebsocketConnection,
							payload_binaryMessage,
						)
					} else if maybeError_ReadMessage != nil {
						this.Teardown_WebsocketConnection()
						break
					} else if _WEBSOCKET.TextMessage == messageType_Gorilla {
						this.CloseWithCode_WebsocketConnection(
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

func (this *_WebsocketController_) GetLatestAndRejectPreceding__Submission_GetWebsocketConnection(
	leadingSubmission_GetWebsocketConnection _Submission_GetWebsocketConnection_,
) _Submission_GetWebsocketConnection_ {
	latestSubmission_GetWebsocketConnection := leadingSubmission_GetWebsocketConnection
	for {
		select {
		case nextSubmission_GetWebsocketConnection := <-this.QueueChannel__Submission_GetWebsocketConnection:
			latestSubmission_GetWebsocketConnection.ReplyChannel <- _SubmissionReply_GetWebsocketConnection_{
				MaybeError_Submission: SUPERSEDED__ERROR___SUBMISSION__GET_WEBSOCKET_CONNECTION,
			}
			latestSubmission_GetWebsocketConnection = nextSubmission_GetWebsocketConnection
		default:
			return latestSubmission_GetWebsocketConnection
		}
	}
}

func (this *_WebsocketController_) Update_WebsocketConnection(
	latestSubmission_GetWebsocketConnection _Submission_GetWebsocketConnection_,
) (bool, uint64) {
	this.Mutex.Lock()
	if this.Status_WebsocketConnection != TAKEOVER_CONNECTING__Status_WebsocketConnection {
		this.Status_WebsocketConnection = CONNECTING__Status_WebsocketConnection
	}
	this.Mutex.Unlock()
	var nilHeader_UpgradeResponse _HTTP.Header = nil
	requestUpgrader_GetWebsocketConnection := _WEBSOCKET.Upgrader{}
	newWebsocketConnection, maybeError_UpgradeRequest := requestUpgrader_GetWebsocketConnection.Upgrade(
		latestSubmission_GetWebsocketConnection.ResponseWriter_GetWebsocketConnection,
		latestSubmission_GetWebsocketConnection.Request_GetWebsocketConnection,
		nilHeader_UpgradeResponse,
	)
	latestSubmission_GetWebsocketConnection.ReplyChannel <- _SubmissionReply_GetWebsocketConnection_{
		MaybeError_Submission: maybeError_UpgradeRequest,
	}
	this.Mutex.Lock()
	isTakeoverConnecting := this.Status_WebsocketConnection == TAKEOVER_CONNECTING__Status_WebsocketConnection
	this.Mutex.Unlock()
	if newWebsocketConnection != nil && isTakeoverConnecting {
		newId_WebsocketConnection := this.Attach_WebsocketConnection(
			this.OnTakeoverConnected__,
			newWebsocketConnection,
		)
		return true, newId_WebsocketConnection
	} else if newWebsocketConnection != nil {
		newId_WebsocketConnection := this.Attach_WebsocketConnection(
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
		_FMT.Println("invalid path: Update_WebsocketConnection")
		return false, 0
	}
}

func (this *_WebsocketController_) Attach_WebsocketConnection(
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

func (this *_WebsocketController_) Teardown_WebsocketConnection() {
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

func (this *_WebsocketController_) CloseWithCode_WebsocketConnection(
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
