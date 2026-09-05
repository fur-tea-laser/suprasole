package main

import (
	_BINARY "encoding/binary"
)

type _BinaryEncoder_WebsocketMessage_ struct {
	Buffer__Payload_BinaryMessage []byte
}

func New__BinaryEncoder_WebsocketMessage() _BinaryEncoder_WebsocketMessage_ {
	return _BinaryEncoder_WebsocketMessage_{
		Buffer__Payload_BinaryMessage: make([]byte, 0),
	}
}

func (this *_BinaryEncoder_WebsocketMessage_) EncodeParameter_Uint8(
	parameterValue uint8,
) {
	this.Buffer__Payload_BinaryMessage = append(
		this.Buffer__Payload_BinaryMessage,
		parameterValue,
	)
}

func (this *_BinaryEncoder_WebsocketMessage_) EncodeParameter_Bool(
	parameterValue bool,
) {
	if parameterValue {
		this.EncodeParameter_Uint8(0x01)
	} else {
		this.EncodeParameter_Uint8(0x00)
	}
}

func (this *_BinaryEncoder_WebsocketMessage_) EncodeParameter_Uint16(
	parameterValue uint16,
) {
	bytesResult_parameterValue := make([]byte, 2)
	_BINARY.BigEndian.PutUint16(bytesResult_parameterValue, parameterValue)
	this.Buffer__Payload_BinaryMessage = append(
		this.Buffer__Payload_BinaryMessage,
		bytesResult_parameterValue...,
	)
}

func (this *_BinaryEncoder_WebsocketMessage_) EncodeParameter_Uint32(
	parameterValue uint32,
) {
	bytesResult_parameterValue := make([]byte, 4)
	_BINARY.BigEndian.PutUint32(bytesResult_parameterValue, parameterValue)
	this.Buffer__Payload_BinaryMessage = append(
		this.Buffer__Payload_BinaryMessage,
		bytesResult_parameterValue...,
	)
}

func (this *_BinaryEncoder_WebsocketMessage_) EncodeParameter_Int32(
	parameterValue int32,
) {
	this.EncodeParameter_Uint32(uint32(parameterValue))
}

func (this *_BinaryEncoder_WebsocketMessage_) EncodeParameter_String16(
	parameterValue string,
) {
	this.EncodeParameter_Uint16(uint16(len(parameterValue)))
	this.Buffer__Payload_BinaryMessage = append(
		this.Buffer__Payload_BinaryMessage,
		[]byte(parameterValue)...,
	)
}

func (this *_BinaryEncoder_WebsocketMessage_) EncodeParameter_Bytes16(
	parameterValue []byte,
) {
	this.EncodeParameter_Uint16(uint16(len(parameterValue)))
	this.Buffer__Payload_BinaryMessage = append(
		this.Buffer__Payload_BinaryMessage,
		parameterValue...,
	)
}

func (this *_BinaryEncoder_WebsocketMessage_) EncodeParameter_TrailingBytes(
	parameterValue []byte,
) {
	this.Buffer__Payload_BinaryMessage = append(
		this.Buffer__Payload_BinaryMessage,
		parameterValue...,
	)
}

func (this *_BinaryEncoder_WebsocketMessage_) EncodeParameter__Slice16_String16(
	parameterSlice []string,
) {
	this.EncodeParameter_Uint16(uint16(len(parameterSlice)))
	for _, currentValue_parameterSlice := range parameterSlice {
		this.EncodeParameter_String16(currentValue_parameterSlice)
	}
}

func (this *_BinaryEncoder_WebsocketMessage_) EncodeParameter__Slice16_Uint32(
	parameterSlice []uint32,
) {
	this.EncodeParameter_Uint16(uint16(len(parameterSlice)))
	for _, currentValue_parameterSlice := range parameterSlice {
		this.EncodeParameter_Uint32(currentValue_parameterSlice)
	}
}

func (this *_BinaryEncoder_WebsocketMessage_) Bytes() []byte {
	return this.Buffer__Payload_BinaryMessage
}

func EncodeParameter__Slice16__BinaryEncoder_WebsocketMessage[
	__Element__ any,
](
	binaryEncoder *_BinaryEncoder_WebsocketMessage_,
	parameterSlice []__Element__,
	encodeElement__ func(binaryEncoder *_BinaryEncoder_WebsocketMessage_, currentIndex_parameterSlice int, currentElement_parameterSlice __Element__),
) {
	binaryEncoder.EncodeParameter_Uint16(uint16(len(parameterSlice)))
	for currentIndex_parameterSlice, currentElement_parameterSlice := range parameterSlice {
		encodeElement__(
			binaryEncoder,
			currentIndex_parameterSlice,
			currentElement_parameterSlice,
		)
	}
}
