package main

import (
	_BYTES "bytes"
	_BINARY "encoding/binary"
	_FMT "fmt"
)

type _BinaryDecoder_WebsocketMessage_ struct {
	Label_MessageStruct__         string
	Buffer__Payload_BinaryMessage []byte
	Cursor_Buffer                 int
	MaybeError_Earliest           error
}

type _NewApi__BinaryDecoder_WebsocketMessage_ struct {
	Label_MessageStruct__         string
	Buffer__Payload_BinaryMessage []byte
	Cursor_Buffer                 int
}

func New__BinaryDecoder_WebsocketMessage(
	api _NewApi__BinaryDecoder_WebsocketMessage_,
) _BinaryDecoder_WebsocketMessage_ {
	return _BinaryDecoder_WebsocketMessage_{
		Label_MessageStruct__:         api.Label_MessageStruct__,
		Buffer__Payload_BinaryMessage: api.Buffer__Payload_BinaryMessage,
		Cursor_Buffer:                 api.Cursor_Buffer,
		MaybeError_Earliest:           nil,
	}
}

func (this *_BinaryDecoder_WebsocketMessage_) CanDecodeParameter(
	requiredByteCount int,
	label_expectedParameter string,
	label_targetType string,
) bool {
	if this.MaybeError_Earliest != nil {
		return false
	} else if remainingBytes := len(this.Buffer__Payload_BinaryMessage) - this.Cursor_Buffer; remainingBytes < requiredByteCount {
		this.MaybeError_Earliest = _FMT.Errorf(
			"[%s] failed to decode %s for '%s': unexpected end of payload at offset %d (need %d bytes, remaining %d)",
			this.Label_MessageStruct__,
			label_targetType,
			label_expectedParameter,
			this.Cursor_Buffer,
			requiredByteCount,
			remainingBytes,
		)
		return false
	}
	return true
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Uint8(
	label_expectedParameter string,
) uint8 {
	if false == this.CanDecodeParameter(1, label_expectedParameter, "uint8") {
		return 0
	}
	decodedValue_expectedParameter := this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer]
	this.Cursor_Buffer++
	return decodedValue_expectedParameter
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Uint16(
	label_expectedParameter string,
) uint16 {
	if false == this.CanDecodeParameter(2, label_expectedParameter, "uint16") {
		return 0
	}
	decodedValue_expectedParameter := _BINARY.BigEndian.Uint16(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer : this.Cursor_Buffer+2])
	this.Cursor_Buffer += 2
	return decodedValue_expectedParameter
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Uint32(
	label_expectedParameter string,
) uint32 {
	if false == this.CanDecodeParameter(4, label_expectedParameter, "uint32") {
		return 0
	}
	decodedValue_expectedParameter := _BINARY.BigEndian.Uint32(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer : this.Cursor_Buffer+4])
	this.Cursor_Buffer += 4
	return decodedValue_expectedParameter
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Int32(
	label_expectedParameter string,
) int32 {
	return int32(this.DecodeParameter_Uint32(label_expectedParameter))
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_String16(
	label_expectedParameter string,
) string {
	length_decodedString := int(this.DecodeParameter_Uint16(label_expectedParameter + ".Length"))
	if false == this.CanDecodeParameter(length_decodedString, label_expectedParameter, "string16 data") {
		return ""
	}
	decodedValue_expectedParameter := string(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer : this.Cursor_Buffer+length_decodedString])
	this.Cursor_Buffer += length_decodedString
	return decodedValue_expectedParameter
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Bytes16(
	label_expectedParameter string,
) []byte {
	length_decodedBytes := int(this.DecodeParameter_Uint16(label_expectedParameter + ".Length"))
	if false == this.CanDecodeParameter(length_decodedBytes, label_expectedParameter, "bytes16 data") {
		return nil
	}
	decodedValue_expectedParameter := _BYTES.Clone(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer : this.Cursor_Buffer+length_decodedBytes])
	this.Cursor_Buffer += length_decodedBytes
	return decodedValue_expectedParameter
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_TrailingBytes(
	label_expectedParameter string,
) []byte {
	if this.MaybeError_Earliest != nil {
		return nil
	} else if this.Cursor_Buffer > len(this.Buffer__Payload_BinaryMessage) {
		this.MaybeError_Earliest = _FMT.Errorf(
			"[%s] failed to decode trailing bytes for '%s': cursor offset %d exceeds payload length %d",
			this.Label_MessageStruct__,
			label_expectedParameter,
			this.Cursor_Buffer,
			len(this.Buffer__Payload_BinaryMessage),
		)
		return nil
	}
	decodedValue_expectedParameter := _BYTES.Clone(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer:])
	this.Cursor_Buffer = len(this.Buffer__Payload_BinaryMessage)
	return decodedValue_expectedParameter
}

func (this *_BinaryDecoder_WebsocketMessage_) AssertEndOfPayload() {
	if this.MaybeError_Earliest != nil {
		return
	} else if this.Cursor_Buffer != len(this.Buffer__Payload_BinaryMessage) {
		this.MaybeError_Earliest = _FMT.Errorf(
			"[%s] payload size mismatch: expected exactly %d bytes, got %d bytes (unconsumed %d trailing bytes at offset %d)",
			this.Label_MessageStruct__,
			this.Cursor_Buffer,
			len(this.Buffer__Payload_BinaryMessage),
			len(this.Buffer__Payload_BinaryMessage)-this.Cursor_Buffer,
			this.Cursor_Buffer,
		)
	}
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter__Slice16_String16(
	label_expectedParameter string,
) []string {
	count_decodedSlice := int(this.DecodeParameter_Uint16(label_expectedParameter + ".Count"))
	if this.MaybeError_Earliest != nil {
		return nil
	}
	decodedSliceResult_expectedParameter := make([]string, 0, count_decodedSlice)
	for currentIndex_decodedSlice := 0; currentIndex_decodedSlice < count_decodedSlice; currentIndex_decodedSlice++ {
		decodedSliceResult_expectedParameter = append(
			decodedSliceResult_expectedParameter,
			this.DecodeParameter_String16(
				_FMT.Sprintf(
					"%s[%d]",
					label_expectedParameter,
					currentIndex_decodedSlice,
				),
			),
		)
	}
	return decodedSliceResult_expectedParameter
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter__Slice16_Uint32(
	label_expectedParameter string,
) []uint32 {
	count_decodedSlice := int(this.DecodeParameter_Uint16(label_expectedParameter + ".Count"))
	if this.MaybeError_Earliest != nil {
		return nil
	}
	decodedSliceResult_expectedParameter := make([]uint32, 0, count_decodedSlice)
	for currentIndex_decodedSlice := 0; currentIndex_decodedSlice < count_decodedSlice; currentIndex_decodedSlice++ {
		decodedSliceResult_expectedParameter = append(
			decodedSliceResult_expectedParameter,
			this.DecodeParameter_Uint32(
				_FMT.Sprintf(
					"%s[%d]",
					label_expectedParameter,
					currentIndex_decodedSlice,
				),
			),
		)
	}
	return decodedSliceResult_expectedParameter
}

func DecodeParameter__Slice16__BinaryDecoder_WebsocketMessage[
	__Element__ any,
](
	binaryDecoder *_BinaryDecoder_WebsocketMessage_,
	label_expectedParameter string,
	decodeElement__ func(binaryDecoder *_BinaryDecoder_WebsocketMessage_, currentIndex_decodedSlice int) __Element__,
) []__Element__ {
	count_decodedSlice := int(binaryDecoder.DecodeParameter_Uint16(label_expectedParameter + ".Count"))
	if binaryDecoder.MaybeError_Earliest != nil {
		return nil
	}
	decodedSliceResult_expectedParameter := make([]__Element__, 0, count_decodedSlice)
	for currentIndex_decodedSlice := 0; currentIndex_decodedSlice < count_decodedSlice; currentIndex_decodedSlice++ {
		decodedElement := decodeElement__(
			binaryDecoder,
			currentIndex_decodedSlice,
		)
		if binaryDecoder.MaybeError_Earliest != nil {
			return nil
		}
		decodedSliceResult_expectedParameter = append(
			decodedSliceResult_expectedParameter,
			decodedElement,
		)
	}
	return decodedSliceResult_expectedParameter
}

func DecodeParameter__Map16__BinaryDecoder_WebsocketMessage[
	__Key__ comparable,
	__Value__ any,
](
	binaryDecoder *_BinaryDecoder_WebsocketMessage_,
	label_expectedParameter string,
	decodeEntry__ func(binaryDecoder *_BinaryDecoder_WebsocketMessage_, currentIndex_decodedMap int) (__Key__, __Value__),
) map[__Key__]__Value__ {
	count_decodedMap := int(binaryDecoder.DecodeParameter_Uint16(label_expectedParameter + ".Count"))
	if binaryDecoder.MaybeError_Earliest != nil {
		return nil
	}
	decodedMapResult_expectedParameter := make(map[__Key__]__Value__, count_decodedMap)
	for currentIndex_decodedMap := 0; currentIndex_decodedMap < count_decodedMap; currentIndex_decodedMap++ {
		decodedKey, decodedValue := decodeEntry__(
			binaryDecoder,
			currentIndex_decodedMap,
		)
		if binaryDecoder.MaybeError_Earliest != nil {
			return nil
		}
		decodedMapResult_expectedParameter[decodedKey] = decodedValue
	}
	return decodedMapResult_expectedParameter
}
