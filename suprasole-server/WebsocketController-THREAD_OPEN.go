package main

import (
	_HTTP "net/http"
	_TIME "time"

	_WEBSOCKET "github.com/gorilla/websocket"
)

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
					messageType_Gorilla, payload_binaryMessage, error_ReadMessage_maybe := This.WebsocketConnection_state.ReadMessage()
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
						panic("invalid path: _WebsocketController_ RunWorker__Submission_GetWebsocketConnection read loop")
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
	if This.Status_WebsocketConnection_state != TAKEOVER_CONNECTING__Status_WebsocketConnection {
		This.Status_WebsocketConnection_state = CONNECTING__Status_WebsocketConnection
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
	status_WebsocketConnection_captured := This.Status_WebsocketConnection_state
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
		This.Status_WebsocketConnection_state = TAKEOVER_UPGRADE_FAILED__Status_WebsocketConnection
		This.Mutex.Unlock()
		return false, 0
	} else if error_UpgradeRequest_maybe != nil && CONNECTING__Status_WebsocketConnection == status_WebsocketConnection_captured {
		This.Mutex.Lock()
		This.Status_WebsocketConnection_state = UPGRADE_FAILED__Status_WebsocketConnection
		This.Mutex.Unlock()
		return false, 0
	} else {
		panic("invalid path: _WebsocketController_ Update_WebsocketConnection")
	}
}

func (This *_WebsocketController_) Attach_WebsocketConnection(
	onConnected__ func(id_WebsocketConnection_new uint64),
	websocketConnection_new *_WEBSOCKET.Conn,
) uint64 {
	This.Mutex.Lock()
	This.Id_WebsocketConnection_state++
	id_WebsocketConnection_new := This.Id_WebsocketConnection_state
	This.WebsocketConnection_state = websocketConnection_new
	_ = This.WebsocketConnection_state.SetReadDeadline(
		_TIME.Now().Add(This.DeadlineTimeout_Read__),
	)
	This.Status_WebsocketConnection_state = CONNECTED__Status_WebsocketConnection
	This.Mutex.Unlock()
	onConnected__(id_WebsocketConnection_new)
	return id_WebsocketConnection_new
}

func (This *_WebsocketController_) Teardown_WebsocketConnection() {
	var takeoverStatus_WebsocketConnection_captured _TakeoverStatus_WebsocketConnection_
	var WebsocketConnection_closing *_WEBSOCKET.Conn
	This.Mutex.Lock()
	takeoverStatus_WebsocketConnection_captured = This.TakeoverStatus_WebsocketConnection_state
	if PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection_captured {
		This.Status_WebsocketConnection_state = TAKEOVER_CONNECTING__Status_WebsocketConnection
		This.TakeoverStatus_WebsocketConnection_state = NOT_PENDING__TakeoverStatus_WebsocketConnection
	} else if NOT_PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection_captured {
		This.Status_WebsocketConnection_state = DISCONNECTED__Status_WebsocketConnection
		WebsocketConnection_closing = This.WebsocketConnection_state
	} else {
		panic("invalid path: _WebsocketController_ Teardown_WebsocketConnection [STATE_TRANSITION]")
	}
	This.WebsocketConnection_state = nil
	This.Mutex.Unlock()
	if WebsocketConnection_closing != nil {
		_ = WebsocketConnection_closing.Close()
	}
	if PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection_captured {
		This.OnDisconnected_Takeover__()
	} else if NOT_PENDING__TakeoverStatus_WebsocketConnection == takeoverStatus_WebsocketConnection_captured {
		This.OnDisconnected__()
	} else {
		panic("invalid path: _WebsocketController_ Teardown_WebsocketConnection [CALLBACK_DISPATCH]")
	}
}
