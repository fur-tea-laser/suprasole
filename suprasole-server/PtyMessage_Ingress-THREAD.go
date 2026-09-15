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
	WorkspaceController_forwarded.MessageDebouncer__Batch_ResizePty.QueueChannel <- this
}

func (this _WriteInput_Pty__PtyMessage_Ingress_) Execute(
	WorkspaceController_forwarded *_WorkspaceController_,
	id_WebsocketConnection_expected uint64,
) {
	WorkspaceController_forwarded.Mutex.Lock()
	WorkspacePty_target := WorkspaceController_forwarded.PtyPool[this.Id_WorkspacePty]
	WorkspaceController_forwarded.Mutex.Unlock()
	if WorkspacePty_target != nil {
		WorkspacePty_target.State_current.HandleWriteInput_Ingress(this.InputOrder_PtyWriter)
	}
}

func (this *_State_Spawning__WorkspacePty_) HandleWriteInput_Ingress(inputOrder_PtyWriter _InputOrder_PtyWriter_) {
	_FMT.Println("invalid path: _State_Spawning__WorkspacePty_ HandleWriteInput_Ingress")
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
	WorkspaceController_forwarded.Mutex.Lock()
	WorkspacePty_target := WorkspaceController_forwarded.PtyPool[this.Id_WorkspacePty]
	WorkspaceController_forwarded.Mutex.Unlock()
	if WorkspacePty_target != nil {
		WorkspacePty_target.State_current.HandleTerminate_Ingress(this.TerminalSignal_PtyProcess)
	}
}

func (this *_State_Spawning__WorkspacePty_) HandleTerminate_Ingress(terminalSignal int) {
	this.Mutex.Lock()
	this.CancellationStatus = CANCELED____CancellationStatus___State_Spawning__WorkspacePty
	this.Mutex.Unlock()
}

func (this *_State_Active__WorkspacePty_) HandleTerminate_Ingress(terminalSignal int) {
	this.PtyProxy.Terminate_PtyProcess(terminalSignal)
}

func (this *_State_Exited__WorkspacePty_) HandleTerminate_Ingress(terminalSignal int) {
	// Valid invocation: A client termination request crosses in-flight across the network with natural process exit on the server.
	// No action required: The process has already exited and its exit outcome is finalized.
}

func (this _Batch_RemovePty__PtyMessage_Ingress_) Execute(
	WorkspaceController_forwarded *_WorkspaceController_,
	id_WebsocketConnection_expected uint64,
) {
	WorkspaceController_forwarded.Mutex.Lock()
	for _, order_RemovePty_current := range this.OrderBatch_RemovePty {
		WorkspacePty_target := WorkspaceController_forwarded.PtyPool[order_RemovePty_current.Id_WorkspacePty]
		if WorkspacePty_target != nil {
			WorkspacePty_target.State_current.HandleRemove_Ingress(
				WorkspaceController_forwarded.PtyPool,
				WorkspacePty_target.Id,
			)
		}
	}
	WorkspaceController_forwarded.Mutex.Unlock()
}

func (this *_State_Spawning__WorkspacePty_) HandleRemove_Ingress(ptyPool map[uint32]*_WorkspacePty_, id_WorkspacePty uint32) {
	// Valid invocation: Client requested removal, but spawning sessions must remain in PtyPool until the OS spawn worker completes.
	// No action required: Session is retained in PtyPool; coordinator will purge upon spawn failure or cancellation.
}

func (this *_State_Active__WorkspacePty_) HandleRemove_Ingress(ptyPool map[uint32]*_WorkspacePty_, id_WorkspacePty uint32) {
	// Valid invocation: Client requested removal, but active running sessions cannot be purged.
	// No action required: Running sessions remain protected in PtyPool until process termination.
}

func (this *_State_Exited__WorkspacePty_) HandleRemove_Ingress(ptyPool map[uint32]*_WorkspacePty_, id_WorkspacePty uint32) {
	delete(ptyPool, id_WorkspacePty)
}

func (this _Batch_SyncPty__PtyMessage_Ingress_) Execute(
	WorkspaceController_forwarded *_WorkspaceController_,
	id_WebsocketConnection_expected uint64,
) {
	WorkspaceController_forwarded.LifecycleCoordinator_WorkspacePty.QueueChannel_WorkspaceOrder <- _Sync__WorkspaceOrder_LifecycleCoordinator_{
		Id_WebsocketConnection_expected: id_WebsocketConnection_expected,
		Message__Batch_SyncPty:          this,
	}
}

