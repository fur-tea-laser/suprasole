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
