package main

import (
	_BINARY "encoding/binary"
	_FMT "fmt"
)

func (This *_WorkspaceController_) HandleConnected_PtyWebsocket(
	id_WebsocketConnection_new uint64,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Connect__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: id_WebsocketConnection_new,
	}
}

func (This *_WorkspaceController_) HandleConnected_Takeover__PtyWebsocket(
	id_WebsocketConnection_new uint64,
) {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Connect__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: id_WebsocketConnection_new,
	}
}

func (This *_WorkspaceController_) HandleDisconnected_PtyWebsocket() {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Disconnect__WorkspaceOrder_LifecycleCoordinator_{}
}

func (This *_WorkspaceController_) HandleDisconnected_Takeover__PtyWebsocket() {
	This.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Disconnect__WorkspaceOrder_LifecycleCoordinator_{}
}

func (This *_WorkspaceController_) HandlePayload_BinaryMessage__PtyWebsocket(
	id_WebsocketConnection_expected uint64,
	payload_binaryMessage []byte,
) {
	__decodeWebsocketPayload_binaryMessage(
		MAP__DECODE_PAYLOAD___PTY_MESSAGE__INGRESS,
		"pty websocket client",
		This,
		id_WebsocketConnection_expected,
		payload_binaryMessage,
	)
}

func __decodeWebsocketPayload_binaryMessage[
	__Code__Message_Ingress__ ~uint16,
	__Message_Ingress__ interface {
		Execute(
			WorkspaceController_forwarded *_WorkspaceController_,
			id_WebsocketConnection_expected uint64,
		)
	},
](
	map_decodePayloadToMessage__ map[__Code__Message_Ingress__]func(payload_binaryMessage []byte) (__Message_Ingress__, error),
	label_messageSource__ErrorLog__ string,
	WorkspaceController_this *_WorkspaceController_,
	id_WebsocketConnection_expected uint64,
	payload_binaryMessage []byte,
) {
	if len(payload_binaryMessage) < 2 {
		_FMT.Printf(
			"%s message error: payload too short: %d bytes\n",
			label_messageSource__ErrorLog__,
			len(payload_binaryMessage),
		)
		return
	}
	code_messagePayload := __Code__Message_Ingress__(_BINARY.BigEndian.Uint16(payload_binaryMessage[:2]))
	decodePayloadToMessage := map_decodePayloadToMessage__[code_messagePayload]
	if nil == decodePayloadToMessage {
		_FMT.Printf(
			"%s decode message error: unrecognized message opcode: 0x%04x\n",
			label_messageSource__ErrorLog__,
			code_messagePayload,
		)
		return
	}
	message_decoded, error_decodePayloadToMessage__maybe := decodePayloadToMessage(payload_binaryMessage)
	if error_decodePayloadToMessage__maybe != nil {
		_FMT.Printf(
			"%s decode message error: %v\n",
			label_messageSource__ErrorLog__,
			error_decodePayloadToMessage__maybe,
		)
		return
	}
	message_decoded.Execute(
		WorkspaceController_this,
		id_WebsocketConnection_expected,
	)
}
