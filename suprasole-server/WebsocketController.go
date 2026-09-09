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

var ERROR_SUPERSEDED___SUBMISSION__GET_WEBSOCKET_CONNECTION = _ERRORS.New("get websocket connection submission superseded by newer entry")
var ERROR__NOT_CONNECTED___WRITE_PAYLOAD__BINARY_MESSAGE = _ERRORS.New("websocket is not connected")
var ERROR__CONNECTION_ID_MISALIGNED___WRITE_PAYLOAD__BINARY_MESSAGE = _ERRORS.New("websocket connection id does not align")

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

type _TakeoverStatus_WebsocketConnection_ int

const (
	NOT_PENDING__TakeoverStatus_WebsocketConnection _TakeoverStatus_WebsocketConnection_ = 0
	PENDING__TakeoverStatus_WebsocketConnection     _TakeoverStatus_WebsocketConnection_ = 1
)

type _SubmissionReply_GetWebsocketConnection_ struct {
	Error_Submission_maybe error
}

type _Submission_GetWebsocketConnection_ struct {
	ResponseWriter_GetWebsocketConnection _HTTP.ResponseWriter
	Request_GetWebsocketConnection        *_HTTP.Request
	ReplyChannel                          chan _SubmissionReply_GetWebsocketConnection_
}

type _WebsocketController_ struct {
	DeadlineTimeout_Read__                           _TIME.Duration
	DeadlineTimeout_Write__                          _TIME.Duration
	OnConnected__                                    func(id_WebsocketConnection_new uint64)
	OnConnected_Takeover__                           func(id_WebsocketConnection_new uint64)
	OnDisconnected__                                 func()
	OnDisconnected_Takeover__                        func()
	OnPayload_BinaryMessage__                        func(id_WebsocketConnection_expected uint64, payload_binaryMessage []byte)
	Mutex                                            _SYNC.Mutex
	EgressMutex                                      _SYNC.Mutex
	WebsocketConnection_current                      *_WEBSOCKET.Conn
	Status_WebsocketConnection_current               _Status_WebsocketConnection_
	Id_WebsocketConnection_current                   uint64
	TakeoverStatus_WebsocketConnection_current       _TakeoverStatus_WebsocketConnection_
	QueueChannel__Submission_GetWebsocketConnection  chan _Submission_GetWebsocketConnection_
	WorkerContext__Submission_GetWebsocketConnection _CONTEXT.Context
	WorkerCancel__Submission_GetWebsocketConnection  _CONTEXT.CancelFunc
}

func (this *_WebsocketController_) HandleRequest_GetWebsocketConnection(
	responseWriter_GetWebsocketConnection _HTTP.ResponseWriter,
	request_GetWebsocketConnection *_HTTP.Request,
) {
	this.Mutex.Lock()
	status_WebsocketConnection_captured := this.Status_WebsocketConnection_current
	if CONNECTED__Status_WebsocketConnection == status_WebsocketConnection_captured {
		this.TakeoverStatus_WebsocketConnection_current = PENDING__TakeoverStatus_WebsocketConnection
	}
	this.Mutex.Unlock()
	if CONNECTED__Status_WebsocketConnection == status_WebsocketConnection_captured {
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
		if nil == submissionReply_GetWebsocketConnection.Error_Submission_maybe {
			return
		} else if _ERRORS.Is(submissionReply_GetWebsocketConnection.Error_Submission_maybe, ERROR_SUPERSEDED___SUBMISSION__GET_WEBSOCKET_CONNECTION) {
			_HTTP.Error(
				responseWriter_GetWebsocketConnection,
				submissionReply_GetWebsocketConnection.Error_Submission_maybe.Error(),
				_HTTP.StatusConflict,
			)
			return
		} else if _ERRORS.As(submissionReply_GetWebsocketConnection.Error_Submission_maybe, new(_WEBSOCKET.HandshakeError)) ||
			_ERRORS.Is(submissionReply_GetWebsocketConnection.Error_Submission_maybe, _HTTP.ErrNotSupported) {
			return
		} else if _ERRORS.As(submissionReply_GetWebsocketConnection.Error_Submission_maybe, new(*_NET.OpError)) ||
			_ERRORS.Is(submissionReply_GetWebsocketConnection.Error_Submission_maybe, _IO.EOF) ||
			_ERRORS.Is(submissionReply_GetWebsocketConnection.Error_Submission_maybe, _IO.ErrUnexpectedEOF) {
			return
		} else {
			// submissionReply_GetWebsocketConnection.Error_Submission_maybe != nil
			_FMT.Println("invalid path: _WebsocketController_ HandleRequest_GetWebsocketConnection")
			return
		}
	}
}

func (this *_WebsocketController_) RunWorker__Submission_GetWebsocketConnection() {
	for {
		select {
		case <-this.WorkerContext__Submission_GetWebsocketConnection.Done():
			return
		case submission_GetWebsocketConnection_leading := <-this.QueueChannel__Submission_GetWebsocketConnection:
			submission_GetWebsocketConnection_latest := this.GetLatestAndRejectPreceding__Submission_GetWebsocketConnection(submission_GetWebsocketConnection_leading)
			isConnected_WebsocketConnection, id_WebsocketConnection_new := this.Update_WebsocketConnection(submission_GetWebsocketConnection_latest)
			if isConnected_WebsocketConnection {
				for {
					messageType_Gorilla, payload_binaryMessage, error_ReadMessage_maybe := this.WebsocketConnection_current.ReadMessage()
					if _WEBSOCKET.BinaryMessage == messageType_Gorilla {
						this.OnPayload_BinaryMessage__(
							id_WebsocketConnection_new,
							payload_binaryMessage,
						)
					} else if error_ReadMessage_maybe != nil {
						this.Teardown_WebsocketConnection()
						break
					} else if _WEBSOCKET.TextMessage == messageType_Gorilla {
						this.CloseWithCode_WebsocketConnection(
							1003,
							"Text Payloads Unsupported",
						)
						break
					} else {
						_FMT.Println("invalid path: _WebsocketController_ RunWorker__Submission_GetWebsocketConnection read loop")
					}
				}
			}
		}
	}
}

func (this *_WebsocketController_) GetLatestAndRejectPreceding__Submission_GetWebsocketConnection(
	submission_GetWebsocketConnection_leading _Submission_GetWebsocketConnection_,
) _Submission_GetWebsocketConnection_ {
	submission_GetWebsocketConnection_latest := submission_GetWebsocketConnection_leading
	for {
		select {
		case submission_GetWebsocketConnection__next := <-this.QueueChannel__Submission_GetWebsocketConnection:
			submission_GetWebsocketConnection_latest.ReplyChannel <- _SubmissionReply_GetWebsocketConnection_{
				Error_Submission_maybe: ERROR_SUPERSEDED___SUBMISSION__GET_WEBSOCKET_CONNECTION,
			}
			submission_GetWebsocketConnection_latest = submission_GetWebsocketConnection__next
		default:
			return submission_GetWebsocketConnection_latest
		}
	}
}

func (this *_WebsocketController_) Update_WebsocketConnection(
	submission_GetWebsocketConnection_latest _Submission_GetWebsocketConnection_,
) (bool, uint64) {
	this.Mutex.Lock()
	if this.Status_WebsocketConnection_current != TAKEOVER_CONNECTING__Status_WebsocketConnection {
		this.Status_WebsocketConnection_current = CONNECTING__Status_WebsocketConnection
	}
	this.Mutex.Unlock()
	var header_UpgradeResponse_nil _HTTP.Header = nil
	requestUpgrader_GetWebsocketConnection := _WEBSOCKET.Upgrader{}
	websocketConnection_new, error_UpgradeRequest_maybe := requestUpgrader_GetWebsocketConnection.Upgrade(
		submission_GetWebsocketConnection_latest.ResponseWriter_GetWebsocketConnection,
		submission_GetWebsocketConnection_latest.Request_GetWebsocketConnection,
		header_UpgradeResponse_nil,
	)
	submission_GetWebsocketConnection_latest.ReplyChannel <- _SubmissionReply_GetWebsocketConnection_{
		Error_Submission_maybe: error_UpgradeRequest_maybe,
	}
	this.Mutex.Lock()
	status_WebsocketConnection_captured := this.Status_WebsocketConnection_current
	this.Mutex.Unlock()
	if websocketConnection_new != nil && TAKEOVER_CONNECTING__Status_WebsocketConnection == status_WebsocketConnection_captured {
		id_WebsocketConnection_new := this.Attach_WebsocketConnection(
			this.OnConnected_Takeover__,
			websocketConnection_new,
		)
		return true, id_WebsocketConnection_new
	} else if websocketConnection_new != nil && CONNECTING__Status_WebsocketConnection == status_WebsocketConnection_captured {
		id_WebsocketConnection_new := this.Attach_WebsocketConnection(
			this.OnConnected__,
			websocketConnection_new,
		)
		return true, id_WebsocketConnection_new
	} else if error_UpgradeRequest_maybe != nil && TAKEOVER_CONNECTING__Status_WebsocketConnection == status_WebsocketConnection_captured {
		this.Mutex.Lock()
		this.Status_WebsocketConnection_current = TAKEOVER_UPGRADE_FAILED__Status_WebsocketConnection
		this.Mutex.Unlock()
		return false, 0
	} else if error_UpgradeRequest_maybe != nil && CONNECTING__Status_WebsocketConnection == status_WebsocketConnection_captured {
		this.Mutex.Lock()
		this.Status_WebsocketConnection_current = UPGRADE_FAILED__Status_WebsocketConnection
		this.Mutex.Unlock()
		return false, 0
	} else {
		_FMT.Println("invalid path: _WebsocketController_ Update_WebsocketConnection")
		return false, 0
	}
}

func (this *_WebsocketController_) Attach_WebsocketConnection(
	onConnected__ func(id_WebsocketConnection_new uint64),
	websocketConnection_new *_WEBSOCKET.Conn,
) uint64 {
	this.Mutex.Lock()
	this.Id_WebsocketConnection_current++
	id_WebsocketConnection_new := this.Id_WebsocketConnection_current
	this.WebsocketConnection_current = websocketConnection_new
	_ = this.WebsocketConnection_current.SetReadDeadline(
		_TIME.Now().Add(this.DeadlineTimeout_Read__),
	)
	this.Status_WebsocketConnection_current = CONNECTED__Status_WebsocketConnection
	this.Mutex.Unlock()
	onConnected__(id_WebsocketConnection_new)
	return id_WebsocketConnection_new
}

func (this *_WebsocketController_) Teardown_WebsocketConnection() {
	var takeoverStatus_WebsocketConnection_captured _TakeoverStatus_WebsocketConnection_
	var WebsocketConnection_closing *_WEBSOCKET.Conn
	this.Mutex.Lock()
	takeoverStatus_WebsocketConnection_captured = this.TakeoverStatus_WebsocketConnection_current
	if PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection_captured {
		this.Status_WebsocketConnection_current = TAKEOVER_CONNECTING__Status_WebsocketConnection
		this.TakeoverStatus_WebsocketConnection_current = NOT_PENDING__TakeoverStatus_WebsocketConnection
	} else if NOT_PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection_captured {
		this.Status_WebsocketConnection_current = DISCONNECTED__Status_WebsocketConnection
		WebsocketConnection_closing = this.WebsocketConnection_current
	} else {
		_FMT.Println("invalid path: _WebsocketController_ Teardown_WebsocketConnection state transition")
	}
	this.WebsocketConnection_current = nil
	this.Mutex.Unlock()
	if WebsocketConnection_closing != nil {
		_ = WebsocketConnection_closing.Close()
	}
	if PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection_captured {
		this.OnDisconnected_Takeover__()
	} else if NOT_PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection_captured {
		this.OnDisconnected__()
	} else {
		_FMT.Println("invalid path: _WebsocketController_ Teardown_WebsocketConnection callback dispatch")
	}
}

func (this *_WebsocketController_) CloseWithCode_WebsocketConnection(
	closeCode_WebsocketConnection int,
	closeReason_WebsocketConnection string,
) {
	var WebsocketConnection_captured *_WEBSOCKET.Conn
	this.Mutex.Lock()
	if CONNECTED__Status_WebsocketConnection == this.Status_WebsocketConnection_current {
		WebsocketConnection_captured = this.WebsocketConnection_current
	}
	this.Mutex.Unlock()
	if WebsocketConnection_captured != nil {
		this.EgressMutex.Lock()
		_ = WebsocketConnection_captured.WriteControl(
			_WEBSOCKET.CloseMessage,
			_WEBSOCKET.FormatCloseMessage(
				closeCode_WebsocketConnection,
				closeReason_WebsocketConnection,
			),
			_TIME.Now().Add(this.DeadlineTimeout_Write__),
		)
		_ = WebsocketConnection_captured.Close()
		this.EgressMutex.Unlock()
	}
}

func (this *_WebsocketController_) WritePayload_BinaryMessage(
	id_WebsocketConnection_expected uint64,
	payload_binaryMessage []byte,
) error {
	var WebsocketConnection_captured *_WEBSOCKET.Conn
	var error_state__WebsocketConnection__maybe error
	this.Mutex.Lock()
	if CONNECTED__Status_WebsocketConnection == this.Status_WebsocketConnection_current && id_WebsocketConnection_expected == this.Id_WebsocketConnection_current {
		WebsocketConnection_captured = this.WebsocketConnection_current
	} else if this.Status_WebsocketConnection_current != CONNECTED__Status_WebsocketConnection {
		error_state__WebsocketConnection__maybe = ERROR__NOT_CONNECTED___WRITE_PAYLOAD__BINARY_MESSAGE
	} else if this.Id_WebsocketConnection_current != id_WebsocketConnection_expected {
		error_state__WebsocketConnection__maybe = ERROR__CONNECTION_ID_MISALIGNED___WRITE_PAYLOAD__BINARY_MESSAGE
	} else {
		_FMT.Println("invalid path: _WebsocketController_ WritePayload_BinaryMessage")
	}
	this.Mutex.Unlock()
	if error_state__WebsocketConnection__maybe != nil {
		return error_state__WebsocketConnection__maybe
	}
	this.EgressMutex.Lock()
	_ = WebsocketConnection_captured.SetWriteDeadline(
		_TIME.Now().Add(this.DeadlineTimeout_Write__),
	)
	error_write__WebsocketConnection_captured__maybe := WebsocketConnection_captured.WriteMessage(
		_WEBSOCKET.BinaryMessage,
		payload_binaryMessage,
	)
	this.EgressMutex.Unlock()
	return error_write__WebsocketConnection_captured__maybe
}
