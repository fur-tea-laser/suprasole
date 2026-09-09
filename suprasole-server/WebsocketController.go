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

func (This *_WebsocketController_) HandleRequest_GetWebsocketConnection(
	responseWriter_GetWebsocketConnection _HTTP.ResponseWriter,
	request_GetWebsocketConnection *_HTTP.Request,
) {
	This.Mutex.Lock()
	status_WebsocketConnection_captured := This.Status_WebsocketConnection_current
	if CONNECTED__Status_WebsocketConnection == status_WebsocketConnection_captured {
		This.TakeoverStatus_WebsocketConnection_current = PENDING__TakeoverStatus_WebsocketConnection
	}
	This.Mutex.Unlock()
	if CONNECTED__Status_WebsocketConnection == status_WebsocketConnection_captured {
		This.CloseWithCode_WebsocketConnection(
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
	case This.QueueChannel__Submission_GetWebsocketConnection <- submission_GetWebsocketConnection:
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

func (This *_WebsocketController_) RunWorker__Submission_GetWebsocketConnection() {
	for {
		select {
		case <-This.WorkerContext__Submission_GetWebsocketConnection.Done():
			return
		case submission_GetWebsocketConnection_leading := <-This.QueueChannel__Submission_GetWebsocketConnection:
			submission_GetWebsocketConnection_latest := This.GetLatestAndRejectPreceding__Submission_GetWebsocketConnection(submission_GetWebsocketConnection_leading)
			isConnected_WebsocketConnection, id_WebsocketConnection_new := This.Update_WebsocketConnection(submission_GetWebsocketConnection_latest)
			if isConnected_WebsocketConnection {
				for {
					messageType_Gorilla, payload_binaryMessage, error_ReadMessage_maybe := This.WebsocketConnection_current.ReadMessage()
					if _WEBSOCKET.BinaryMessage == messageType_Gorilla {
						This.OnPayload_BinaryMessage__(
							id_WebsocketConnection_new,
							payload_binaryMessage,
						)
					} else if error_ReadMessage_maybe != nil {
						This.Teardown_WebsocketConnection()
						break
					} else if _WEBSOCKET.TextMessage == messageType_Gorilla {
						This.CloseWithCode_WebsocketConnection(
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

func (This *_WebsocketController_) GetLatestAndRejectPreceding__Submission_GetWebsocketConnection(
	submission_GetWebsocketConnection_leading _Submission_GetWebsocketConnection_,
) _Submission_GetWebsocketConnection_ {
	submission_GetWebsocketConnection_latest := submission_GetWebsocketConnection_leading
	for {
		select {
		case submission_GetWebsocketConnection__next := <-This.QueueChannel__Submission_GetWebsocketConnection:
			submission_GetWebsocketConnection_latest.ReplyChannel <- _SubmissionReply_GetWebsocketConnection_{
				Error_Submission_maybe: ERROR_SUPERSEDED___SUBMISSION__GET_WEBSOCKET_CONNECTION,
			}
			submission_GetWebsocketConnection_latest = submission_GetWebsocketConnection__next
		default:
			return submission_GetWebsocketConnection_latest
		}
	}
}

func (This *_WebsocketController_) Update_WebsocketConnection(
	submission_GetWebsocketConnection_latest _Submission_GetWebsocketConnection_,
) (bool, uint64) {
	This.Mutex.Lock()
	if This.Status_WebsocketConnection_current != TAKEOVER_CONNECTING__Status_WebsocketConnection {
		This.Status_WebsocketConnection_current = CONNECTING__Status_WebsocketConnection
	}
	This.Mutex.Unlock()
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
	This.Mutex.Lock()
	status_WebsocketConnection_captured := This.Status_WebsocketConnection_current
	This.Mutex.Unlock()
	if websocketConnection_new != nil && TAKEOVER_CONNECTING__Status_WebsocketConnection == status_WebsocketConnection_captured {
		id_WebsocketConnection_new := This.Attach_WebsocketConnection(
			This.OnConnected_Takeover__,
			websocketConnection_new,
		)
		return true, id_WebsocketConnection_new
	} else if websocketConnection_new != nil && CONNECTING__Status_WebsocketConnection == status_WebsocketConnection_captured {
		id_WebsocketConnection_new := This.Attach_WebsocketConnection(
			This.OnConnected__,
			websocketConnection_new,
		)
		return true, id_WebsocketConnection_new
	} else if error_UpgradeRequest_maybe != nil && TAKEOVER_CONNECTING__Status_WebsocketConnection == status_WebsocketConnection_captured {
		This.Mutex.Lock()
		This.Status_WebsocketConnection_current = TAKEOVER_UPGRADE_FAILED__Status_WebsocketConnection
		This.Mutex.Unlock()
		return false, 0
	} else if error_UpgradeRequest_maybe != nil && CONNECTING__Status_WebsocketConnection == status_WebsocketConnection_captured {
		This.Mutex.Lock()
		This.Status_WebsocketConnection_current = UPGRADE_FAILED__Status_WebsocketConnection
		This.Mutex.Unlock()
		return false, 0
	} else {
		_FMT.Println("invalid path: _WebsocketController_ Update_WebsocketConnection")
		return false, 0
	}
}

func (This *_WebsocketController_) Attach_WebsocketConnection(
	onConnected__ func(id_WebsocketConnection_new uint64),
	websocketConnection_new *_WEBSOCKET.Conn,
) uint64 {
	This.Mutex.Lock()
	This.Id_WebsocketConnection_current++
	id_WebsocketConnection_new := This.Id_WebsocketConnection_current
	This.WebsocketConnection_current = websocketConnection_new
	_ = This.WebsocketConnection_current.SetReadDeadline(
		_TIME.Now().Add(This.DeadlineTimeout_Read__),
	)
	This.Status_WebsocketConnection_current = CONNECTED__Status_WebsocketConnection
	This.Mutex.Unlock()
	onConnected__(id_WebsocketConnection_new)
	return id_WebsocketConnection_new
}

func (This *_WebsocketController_) Teardown_WebsocketConnection() {
	var takeoverStatus_WebsocketConnection_captured _TakeoverStatus_WebsocketConnection_
	var WebsocketConnection_closing *_WEBSOCKET.Conn
	This.Mutex.Lock()
	takeoverStatus_WebsocketConnection_captured = This.TakeoverStatus_WebsocketConnection_current
	if PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection_captured {
		This.Status_WebsocketConnection_current = TAKEOVER_CONNECTING__Status_WebsocketConnection
		This.TakeoverStatus_WebsocketConnection_current = NOT_PENDING__TakeoverStatus_WebsocketConnection
	} else if NOT_PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection_captured {
		This.Status_WebsocketConnection_current = DISCONNECTED__Status_WebsocketConnection
		WebsocketConnection_closing = This.WebsocketConnection_current
	} else {
		_FMT.Println("invalid path: _WebsocketController_ Teardown_WebsocketConnection state transition")
	}
	This.WebsocketConnection_current = nil
	This.Mutex.Unlock()
	if WebsocketConnection_closing != nil {
		_ = WebsocketConnection_closing.Close()
	}
	if PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection_captured {
		This.OnDisconnected_Takeover__()
	} else if NOT_PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection_captured {
		This.OnDisconnected__()
	} else {
		_FMT.Println("invalid path: _WebsocketController_ Teardown_WebsocketConnection callback dispatch")
	}
}

func (This *_WebsocketController_) CloseWithCode_WebsocketConnection(
	closeCode_WebsocketConnection int,
	closeReason_WebsocketConnection string,
) {
	var WebsocketConnection_captured *_WEBSOCKET.Conn
	This.Mutex.Lock()
	if CONNECTED__Status_WebsocketConnection == This.Status_WebsocketConnection_current {
		WebsocketConnection_captured = This.WebsocketConnection_current
	}
	This.Mutex.Unlock()
	if WebsocketConnection_captured != nil {
		This.EgressMutex.Lock()
		_ = WebsocketConnection_captured.WriteControl(
			_WEBSOCKET.CloseMessage,
			_WEBSOCKET.FormatCloseMessage(
				closeCode_WebsocketConnection,
				closeReason_WebsocketConnection,
			),
			_TIME.Now().Add(This.DeadlineTimeout_Write__),
		)
		_ = WebsocketConnection_captured.Close()
		This.EgressMutex.Unlock()
	}
}

func (This *_WebsocketController_) WritePayload_BinaryMessage(
	id_WebsocketConnection_expected uint64,
	payload_binaryMessage []byte,
) error {
	var WebsocketConnection_captured *_WEBSOCKET.Conn
	var error_state__WebsocketConnection__maybe error
	This.Mutex.Lock()
	if CONNECTED__Status_WebsocketConnection == This.Status_WebsocketConnection_current && id_WebsocketConnection_expected == This.Id_WebsocketConnection_current {
		WebsocketConnection_captured = This.WebsocketConnection_current
	} else if This.Status_WebsocketConnection_current != CONNECTED__Status_WebsocketConnection {
		error_state__WebsocketConnection__maybe = ERROR__NOT_CONNECTED___WRITE_PAYLOAD__BINARY_MESSAGE
	} else if This.Id_WebsocketConnection_current != id_WebsocketConnection_expected {
		error_state__WebsocketConnection__maybe = ERROR__CONNECTION_ID_MISALIGNED___WRITE_PAYLOAD__BINARY_MESSAGE
	} else {
		_FMT.Println("invalid path: _WebsocketController_ WritePayload_BinaryMessage")
	}
	This.Mutex.Unlock()
	if error_state__WebsocketConnection__maybe != nil {
		return error_state__WebsocketConnection__maybe
	}
	This.EgressMutex.Lock()
	_ = WebsocketConnection_captured.SetWriteDeadline(
		_TIME.Now().Add(This.DeadlineTimeout_Write__),
	)
	error_write__WebsocketConnection_captured__maybe := WebsocketConnection_captured.WriteMessage(
		_WEBSOCKET.BinaryMessage,
		payload_binaryMessage,
	)
	This.EgressMutex.Unlock()
	return error_write__WebsocketConnection_captured__maybe
}
