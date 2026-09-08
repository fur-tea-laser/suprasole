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

var ERROR__SUPERSEDED___SUBMISSION__GET_WEBSOCKET_CONNECTION = _ERRORS.New("get websocket connection submission superseded by newer entry")
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
	Error_Submission__maybe error
}

type _Submission_GetWebsocketConnection_ struct {
	ResponseWriter_GetWebsocketConnection _HTTP.ResponseWriter
	Request_GetWebsocketConnection        *_HTTP.Request
	ReplyChannel                          chan _SubmissionReply_GetWebsocketConnection_
}

type _WebsocketController_ struct {
	DeadlineTimeout_Read__                           _TIME.Duration
	DeadlineTimeout_Write__                          _TIME.Duration
	OnConnected__                                    func(id_WebsocketConnection__new uint64)
	OnConnected_Takeover__                           func(id_WebsocketConnection__new uint64)
	OnDisconnected__                                 func()
	OnDisconnected_Takeover__                        func()
	OnPayload_BinaryMessage__                        func(id_WebsocketConnection__expected uint64, payload_binaryMessage []byte)
	Mutex                                            _SYNC.Mutex
	EgressMutex                                      _SYNC.Mutex
	WebsocketConnection                              *_WEBSOCKET.Conn
	Status_WebsocketConnection                       _Status_WebsocketConnection_
	Id_WebsocketConnection                           uint64
	TakeoverStatus_WebsocketConnection               _TakeoverStatus_WebsocketConnection_
	QueueChannel__Submission_GetWebsocketConnection  chan _Submission_GetWebsocketConnection_
	WorkerContext__Submission_GetWebsocketConnection _CONTEXT.Context
	WorkerCancel__Submission_GetWebsocketConnection  _CONTEXT.CancelFunc
}

func (this *_WebsocketController_) HandleRequest_GetWebsocketConnection(
	responseWriter_GetWebsocketConnection _HTTP.ResponseWriter,
	request_GetWebsocketConnection *_HTTP.Request,
) {
	this.Mutex.Lock()
	status_WebsocketConnection__captured := this.Status_WebsocketConnection
	if CONNECTED__Status_WebsocketConnection == status_WebsocketConnection__captured {
		this.TakeoverStatus_WebsocketConnection = PENDING__TakeoverStatus_WebsocketConnection
	}
	this.Mutex.Unlock()
	if CONNECTED__Status_WebsocketConnection == status_WebsocketConnection__captured {
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
		if nil == submissionReply_GetWebsocketConnection.Error_Submission__maybe {
			return
		} else if _ERRORS.Is(submissionReply_GetWebsocketConnection.Error_Submission__maybe, ERROR__SUPERSEDED___SUBMISSION__GET_WEBSOCKET_CONNECTION) {
			_HTTP.Error(
				responseWriter_GetWebsocketConnection,
				submissionReply_GetWebsocketConnection.Error_Submission__maybe.Error(),
				_HTTP.StatusConflict,
			)
			return
		} else if _ERRORS.As(submissionReply_GetWebsocketConnection.Error_Submission__maybe, new(_WEBSOCKET.HandshakeError)) ||
			_ERRORS.Is(submissionReply_GetWebsocketConnection.Error_Submission__maybe, _HTTP.ErrNotSupported) {
			return
		} else if _ERRORS.As(submissionReply_GetWebsocketConnection.Error_Submission__maybe, new(*_NET.OpError)) ||
			_ERRORS.Is(submissionReply_GetWebsocketConnection.Error_Submission__maybe, _IO.EOF) ||
			_ERRORS.Is(submissionReply_GetWebsocketConnection.Error_Submission__maybe, _IO.ErrUnexpectedEOF) {
			return
		} else {
			// submissionReply_GetWebsocketConnection.Error_Submission__maybe != nil
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
		case submission_GetWebsocketConnection__leading := <-this.QueueChannel__Submission_GetWebsocketConnection:
			submission_GetWebsocketConnection__latest := this.GetLatestAndRejectPreceding__Submission_GetWebsocketConnection(submission_GetWebsocketConnection__leading)
			isConnected_WebsocketConnection, id_WebsocketConnection__new := this.Update_WebsocketConnection(submission_GetWebsocketConnection__latest)
			if isConnected_WebsocketConnection {
				for {
					messageType_Gorilla, payload_binaryMessage, maybeError_ReadMessage := this.WebsocketConnection.ReadMessage()
					if _WEBSOCKET.BinaryMessage == messageType_Gorilla {
						this.OnPayload_BinaryMessage__(
							id_WebsocketConnection__new,
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
	submission_GetWebsocketConnection__leading _Submission_GetWebsocketConnection_,
) _Submission_GetWebsocketConnection_ {
	submission_GetWebsocketConnection__latest := submission_GetWebsocketConnection__leading
	for {
		select {
		case submission_GetWebsocketConnection__next := <-this.QueueChannel__Submission_GetWebsocketConnection:
			submission_GetWebsocketConnection__latest.ReplyChannel <- _SubmissionReply_GetWebsocketConnection_{
				Error_Submission__maybe: ERROR__SUPERSEDED___SUBMISSION__GET_WEBSOCKET_CONNECTION,
			}
			submission_GetWebsocketConnection__latest = submission_GetWebsocketConnection__next
		default:
			return submission_GetWebsocketConnection__latest
		}
	}
}

func (this *_WebsocketController_) Update_WebsocketConnection(
	submission_GetWebsocketConnection__latest _Submission_GetWebsocketConnection_,
) (bool, uint64) {
	this.Mutex.Lock()
	if this.Status_WebsocketConnection != TAKEOVER_CONNECTING__Status_WebsocketConnection {
		this.Status_WebsocketConnection = CONNECTING__Status_WebsocketConnection
	}
	this.Mutex.Unlock()
	var header_UpgradeResponse__nil _HTTP.Header = nil
	requestUpgrader_GetWebsocketConnection := _WEBSOCKET.Upgrader{}
	websocketConnection__new, error_UpgradeRequest__maybe := requestUpgrader_GetWebsocketConnection.Upgrade(
		submission_GetWebsocketConnection__latest.ResponseWriter_GetWebsocketConnection,
		submission_GetWebsocketConnection__latest.Request_GetWebsocketConnection,
		header_UpgradeResponse__nil,
	)
	submission_GetWebsocketConnection__latest.ReplyChannel <- _SubmissionReply_GetWebsocketConnection_{
		Error_Submission__maybe: error_UpgradeRequest__maybe,
	}
	this.Mutex.Lock()
	status_WebsocketConnection__captured := this.Status_WebsocketConnection
	this.Mutex.Unlock()
	if websocketConnection__new != nil && TAKEOVER_CONNECTING__Status_WebsocketConnection == status_WebsocketConnection__captured {
		id_WebsocketConnection__new := this.Attach_WebsocketConnection(
			this.OnConnected_Takeover__,
			websocketConnection__new,
		)
		return true, id_WebsocketConnection__new
	} else if websocketConnection__new != nil && CONNECTING__Status_WebsocketConnection == status_WebsocketConnection__captured {
		id_WebsocketConnection__new := this.Attach_WebsocketConnection(
			this.OnConnected__,
			websocketConnection__new,
		)
		return true, id_WebsocketConnection__new
	} else if error_UpgradeRequest__maybe != nil && TAKEOVER_CONNECTING__Status_WebsocketConnection == status_WebsocketConnection__captured {
		this.Mutex.Lock()
		this.Status_WebsocketConnection = TAKEOVER_UPGRADE_FAILED__Status_WebsocketConnection
		this.Mutex.Unlock()
		return false, 0
	} else if error_UpgradeRequest__maybe != nil && CONNECTING__Status_WebsocketConnection == status_WebsocketConnection__captured {
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
	onConnected__ func(id_WebsocketConnection__new uint64),
	websocketConnection__new *_WEBSOCKET.Conn,
) uint64 {
	this.Mutex.Lock()
	this.Id_WebsocketConnection++
	id_WebsocketConnection__new := this.Id_WebsocketConnection
	this.WebsocketConnection = websocketConnection__new
	_ = this.WebsocketConnection.SetReadDeadline(
		_TIME.Now().Add(this.DeadlineTimeout_Read__),
	)
	this.Status_WebsocketConnection = CONNECTED__Status_WebsocketConnection
	this.Mutex.Unlock()
	onConnected__(id_WebsocketConnection__new)
	return id_WebsocketConnection__new
}

func (this *_WebsocketController_) Teardown_WebsocketConnection() {
	var takeoverStatus_WebsocketConnection__captured _TakeoverStatus_WebsocketConnection_
	var WebsocketConnection_closing *_WEBSOCKET.Conn
	this.Mutex.Lock()
	takeoverStatus_WebsocketConnection__captured = this.TakeoverStatus_WebsocketConnection
	if PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection__captured {
		this.Status_WebsocketConnection = TAKEOVER_CONNECTING__Status_WebsocketConnection
		this.TakeoverStatus_WebsocketConnection = NOT_PENDING__TakeoverStatus_WebsocketConnection
	} else if NOT_PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection__captured {
		this.Status_WebsocketConnection = DISCONNECTED__Status_WebsocketConnection
		WebsocketConnection_closing = this.WebsocketConnection
	} else {
		_FMT.Println("invalid path: Teardown_WebsocketConnection state transition")
	}
	this.WebsocketConnection = nil
	this.Mutex.Unlock()
	if WebsocketConnection_closing != nil {
		_ = WebsocketConnection_closing.Close()
	}
	if PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection__captured {
		this.OnDisconnected_Takeover__()
	} else if NOT_PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection__captured {
		this.OnDisconnected__()
	} else {
		_FMT.Println("invalid path: Teardown_WebsocketConnection callback dispatch")
	}
}

func (this *_WebsocketController_) CloseWithCode_WebsocketConnection(
	closeCode_WebsocketConnection int,
	closeReason_WebsocketConnection string,
) {
	var WebsocketConnection_captured *_WEBSOCKET.Conn
	this.Mutex.Lock()
	if CONNECTED__Status_WebsocketConnection == this.Status_WebsocketConnection {
		WebsocketConnection_captured = this.WebsocketConnection
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
	id_WebsocketConnection__expected uint64,
	payload_binaryMessage []byte,
) error {
	var WebsocketConnection_captured *_WEBSOCKET.Conn
	var error_state__WebsocketConnection__maybe error
	this.Mutex.Lock()
	if CONNECTED__Status_WebsocketConnection == this.Status_WebsocketConnection && id_WebsocketConnection__expected == this.Id_WebsocketConnection {
		WebsocketConnection_captured = this.WebsocketConnection
	} else if this.Status_WebsocketConnection != CONNECTED__Status_WebsocketConnection {
		error_state__WebsocketConnection__maybe = ERROR__NOT_CONNECTED___WRITE_PAYLOAD__BINARY_MESSAGE
	} else if this.Id_WebsocketConnection != id_WebsocketConnection__expected {
		error_state__WebsocketConnection__maybe = ERROR__CONNECTION_ID_MISALIGNED___WRITE_PAYLOAD__BINARY_MESSAGE
	} else {
		_FMT.Println("invalid path: WritePayload_BinaryMessage")
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
