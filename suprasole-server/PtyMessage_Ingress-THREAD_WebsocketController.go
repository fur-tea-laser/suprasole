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

func (this _SpawnPty__PtyMessage_Ingress_) Execute(
	WorkspaceController_forwarded *_WorkspaceController_,
	id_WebsocketConnection_expected uint64,
) {
	WorkspaceController_forwarded.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _SpawnPty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: id_WebsocketConnection_expected,
		Message_SpawnPty:                this,
	}
}

func (this _Batch_ResizePty__PtyMessage_Ingress_) Execute(
	WorkspaceController_forwarded *_WorkspaceController_,
	id_WebsocketConnection_expected uint64,
) {
	WorkspaceController_forwarded.MessageDebouncer__GeometryUpdate_PtyProxy.QueueChannel <- _Resize___MessageOrder__GeometryUpdate_PtyProxy_{
		Id_WebsocketConnection_expected: id_WebsocketConnection_expected,
		Message__Batch_ResizePty:        this,
	}
}

func (this _WriteInput_Pty__PtyMessage_Ingress_) Execute(
	WorkspaceController_forwarded *_WorkspaceController_,
	id_WebsocketConnection_expected uint64,
) {
	WorkspaceController_forwarded.Mutex.Lock()
	var State__WorkspacePty_target__captured _State_WorkspacePty_
	if WorkspacePty_target := WorkspaceController_forwarded.PtyPool[this.Id_WorkspacePty]; WorkspacePty_target != nil {
		State__WorkspacePty_target__captured = WorkspacePty_target.State_state
	}
	WorkspaceController_forwarded.Mutex.Unlock()
	if State__WorkspacePty_target__captured != nil {
		State__WorkspacePty_target__captured.HandleWriteInput_Ingress(this.InputOrder_PtyWriter)
	}
}

func (This *_State_Spawning__WorkspacePty_) HandleWriteInput_Ingress(inputOrder_PtyWriter _InputOrder_PtyWriter_) {
	panic("invalid path: _State_Spawning__WorkspacePty_ HandleWriteInput_Ingress")
}

func (this *_State_Active__WorkspacePty_) HandleWriteInput_Ingress(inputOrder_PtyWriter _InputOrder_PtyWriter_) {
	select {
	case <-this.PtyProxy.PtyWriter.WorkerContext.Done():
	default:
		select {
		case this.PtyProxy.PtyWriter.QueueChannel_InputOrder <- inputOrder_PtyWriter:
		default:
		}
	}
}

func (this *_State_Exited__WorkspacePty_) HandleWriteInput_Ingress(inputOrder_PtyWriter _InputOrder_PtyWriter_) {
	// Valid invocation: Client keystrokes transmitted prior to receiving process exit status cross in-flight across the network.
	// No action required: The process has already exited and the input writer is closed. In-flight input is safely dropped.
}

func (this _TerminatePty__PtyMessage_Ingress_) Execute(
	WorkspaceController_forwarded *_WorkspaceController_,
	id_WebsocketConnection_expected uint64,
) {
	WorkspaceController_forwarded.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _TerminatePty__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: id_WebsocketConnection_expected,
		Id_WorkspacePty:                 this.Id_WorkspacePty,
		TerminalSignal_PtyProcess:       this.TerminalSignal_PtyProcess,
	}
}

func (this _Batch_RemovePty__PtyMessage_Ingress_) Execute(
	WorkspaceController_forwarded *_WorkspaceController_,
	id_WebsocketConnection_expected uint64,
) {
	WorkspaceController_forwarded.Mutex.Lock()
	for _, order_RemovePty_current := range this.OrderBatch_RemovePty {
		if WorkspacePty_target := WorkspaceController_forwarded.PtyPool[order_RemovePty_current.Id_WorkspacePty]; WorkspacePty_target != nil {
			State__WorkspacePty__target_exited_maybe, _ := WorkspacePty_target.State_state.(*_State_Exited__WorkspacePty_)
			if State__WorkspacePty__target_exited_maybe != nil {
				WorkspacePty_target.PtyResizer.WorkerCancel()
				delete(
					WorkspaceController_forwarded.PtyPool,
					order_RemovePty_current.Id_WorkspacePty,
				)
			}
		}
	}
	WorkspaceController_forwarded.Mutex.Unlock()
}

func (this _Batch_SyncPty__PtyMessage_Ingress_) Execute(
	WorkspaceController_forwarded *_WorkspaceController_,
	id_WebsocketConnection_expected uint64,
) {
	WorkspaceController_forwarded.MessageDebouncer__GeometryUpdate_PtyProxy.QueueChannel <- _Sync___MessageOrder__GeometryUpdate_PtyProxy_{
		Id_WebsocketConnection_expected: id_WebsocketConnection_expected,
		Message__Batch_SyncPty:          this,
	}
}
