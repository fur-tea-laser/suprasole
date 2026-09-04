package main

import (
	_BYTES "bytes"
	_BINARY "encoding/binary"
	_FMT "fmt"
)

type _BinaryDecoder_WebsocketMessage_ struct {
	Buffer__Payload_BinaryMessage []byte
	Cursor_Buffer                 int
	Label_MessageStruct           string
	MaybeError_Earliest           error
}

type _NewApi__BinaryDecoder_WebsocketMessage_ struct {
	Buffer__Payload_BinaryMessage []byte
	Cursor_Buffer                 int
	Label_MessageStruct           string
}

func New__BinaryDecoder_WebsocketMessage(
	api _NewApi__BinaryDecoder_WebsocketMessage_,
) _BinaryDecoder_WebsocketMessage_ {
	return _BinaryDecoder_WebsocketMessage_{
		Buffer__Payload_BinaryMessage: api.Buffer__Payload_BinaryMessage,
		Cursor_Buffer:                 api.Cursor_Buffer,
		Label_MessageStruct:           api.Label_MessageStruct,
		MaybeError_Earliest:           nil,
	}
}

func (this *_BinaryDecoder_WebsocketMessage_) CanDecodeParameter(
	requiredByteCount int,
	expectedParameterLabel string,
	targetTypeLabel string,
) bool {
	if this.MaybeError_Earliest != nil {
		return false
	} else if remainingBytes := len(this.Buffer__Payload_BinaryMessage) - this.Cursor_Buffer; remainingBytes < requiredByteCount {
		this.MaybeError_Earliest = _FMT.Errorf(
			"[%s] failed to decode %s for '%s': unexpected end of payload at offset %d (need %d bytes, remaining %d)",
			this.Label_MessageStruct,
			targetTypeLabel,
			expectedParameterLabel,
			this.Cursor_Buffer,
			requiredByteCount,
			remainingBytes,
		)
		return false
	}
	return true
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Uint8(
	expectedParameterLabel string,
) uint8 {
	if false == this.CanDecodeParameter(1, expectedParameterLabel, "uint8") {
		return 0
	}
	decodedParameterValue := this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer]
	this.Cursor_Buffer++
	return decodedParameterValue
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Uint16(
	expectedParameterLabel string,
) uint16 {
	if false == this.CanDecodeParameter(2, expectedParameterLabel, "uint16") {
		return 0
	}
	decodedParameterValue := _BINARY.BigEndian.Uint16(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer : this.Cursor_Buffer+2])
	this.Cursor_Buffer += 2
	return decodedParameterValue
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Uint32(
	expectedParameterLabel string,
) uint32 {
	if false == this.CanDecodeParameter(4, expectedParameterLabel, "uint32") {
		return 0
	}
	decodedParameterValue := _BINARY.BigEndian.Uint32(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer : this.Cursor_Buffer+4])
	this.Cursor_Buffer += 4
	return decodedParameterValue
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Int32(
	expectedParameterLabel string,
) int32 {
	return int32(this.DecodeParameter_Uint32(expectedParameterLabel))
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_String16(
	expectedParameterLabel string,
) string {
	decodedStringLength := int(this.DecodeParameter_Uint16(expectedParameterLabel + ".Length"))
	if false == this.CanDecodeParameter(decodedStringLength, expectedParameterLabel, "string16 data") {
		return ""
	}
	decodedParameterValue := string(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer : this.Cursor_Buffer+decodedStringLength])
	this.Cursor_Buffer += decodedStringLength
	return decodedParameterValue
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Bytes16(
	expectedParameterLabel string,
) []byte {
	decodedBytesLength := int(this.DecodeParameter_Uint16(expectedParameterLabel + ".Length"))
	if false == this.CanDecodeParameter(decodedBytesLength, expectedParameterLabel, "bytes16 data") {
		return nil
	}
	decodedParameterValue := _BYTES.Clone(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer : this.Cursor_Buffer+decodedBytesLength])
	this.Cursor_Buffer += decodedBytesLength
	return decodedParameterValue
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_TrailingBytes(
	expectedParameterLabel string,
) []byte {
	if this.MaybeError_Earliest != nil {
		return nil
	} else if this.Cursor_Buffer > len(this.Buffer__Payload_BinaryMessage) {
		this.MaybeError_Earliest = _FMT.Errorf(
			"[%s] failed to decode trailing bytes for '%s': cursor offset %d exceeds payload length %d",
			this.Label_MessageStruct,
			expectedParameterLabel,
			this.Cursor_Buffer,
			len(this.Buffer__Payload_BinaryMessage),
		)
		return nil
	}
	decodedParameterValue := _BYTES.Clone(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer:])
	this.Cursor_Buffer = len(this.Buffer__Payload_BinaryMessage)
	return decodedParameterValue
}

func (this *_BinaryDecoder_WebsocketMessage_) AssertEndOfPayload() {
	if this.MaybeError_Earliest != nil {
		return
	} else if this.Cursor_Buffer != len(this.Buffer__Payload_BinaryMessage) {
		this.MaybeError_Earliest = _FMT.Errorf(
			"[%s] payload size mismatch: expected exactly %d bytes, got %d bytes (unconsumed %d trailing bytes at offset %d)",
			this.Label_MessageStruct,
			this.Cursor_Buffer,
			len(this.Buffer__Payload_BinaryMessage),
			len(this.Buffer__Payload_BinaryMessage)-this.Cursor_Buffer,
			this.Cursor_Buffer,
		)
	}
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter__Slice16_String16(
	expectedParameterLabel string,
) []string {
	decodedSliceCount := int(this.DecodeParameter_Uint16(expectedParameterLabel + ".Count"))
	if this.MaybeError_Earliest != nil {
		return nil
	}
	decodedParameterValueResult := make([]string, 0, decodedSliceCount)
	for currentSliceIndex := 0; currentSliceIndex < decodedSliceCount; currentSliceIndex++ {
		decodedParameterValueResult = append(
			decodedParameterValueResult,
			this.DecodeParameter_String16(
				_FMT.Sprintf(
					"%s[%d]",
					expectedParameterLabel,
					currentSliceIndex,
				),
			),
		)
	}
	return decodedParameterValueResult
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter__Slice16_Uint32(
	expectedParameterLabel string,
) []uint32 {
	decodedSliceCount := int(this.DecodeParameter_Uint16(expectedParameterLabel + ".Count"))
	if this.MaybeError_Earliest != nil {
		return nil
	}
	decodedParameterValueResult := make([]uint32, 0, decodedSliceCount)
	for currentSliceIndex := 0; currentSliceIndex < decodedSliceCount; currentSliceIndex++ {
		decodedParameterValueResult = append(
			decodedParameterValueResult,
			this.DecodeParameter_Uint32(
				_FMT.Sprintf(
					"%s[%d]",
					expectedParameterLabel,
					currentSliceIndex,
				),
			),
		)
	}
	return decodedParameterValueResult
}

func DecodeParameter__Slice16__BinaryDecoder_WebsocketMessage[
	__Element__ any,
](
	decoder *_BinaryDecoder_WebsocketMessage_,
	expectedParameterLabel string,
	decodeElement__ func(decoder *_BinaryDecoder_WebsocketMessage_, currentSliceIndex int) __Element__,
) []__Element__ {
	decodedSliceCount := int(decoder.DecodeParameter_Uint16(expectedParameterLabel + ".Count"))
	if decoder.MaybeError_Earliest != nil {
		return nil
	}
	decodedParameterValueResult := make([]__Element__, 0, decodedSliceCount)
	for currentSliceIndex := 0; currentSliceIndex < decodedSliceCount; currentSliceIndex++ {
		decodedElement := decodeElement__(
			decoder,
			currentSliceIndex,
		)
		if decoder.MaybeError_Earliest != nil {
			return nil
		}
		decodedParameterValueResult = append(
			decodedParameterValueResult,
			decodedElement,
		)
	}
	return decodedParameterValueResult
}

func DecodeParameter__Map16__BinaryDecoder_WebsocketMessage[
	__Key__ comparable,
	__Value__ any,
](
	decoder *_BinaryDecoder_WebsocketMessage_,
	expectedParameterLabel string,
	decodeEntry__ func(decoder *_BinaryDecoder_WebsocketMessage_, currentMapIndex int) (__Key__, __Value__),
) map[__Key__]__Value__ {
	decodedMapCount := int(decoder.DecodeParameter_Uint16(expectedParameterLabel + ".Count"))
	if decoder.MaybeError_Earliest != nil {
		return nil
	}
	decodedParameterValueResult := make(map[__Key__]__Value__, decodedMapCount)
	for currentMapIndex := 0; currentMapIndex < decodedMapCount; currentMapIndex++ {
		decodedKey, decodedValue := decodeEntry__(
			decoder,
			currentMapIndex,
		)
		if decoder.MaybeError_Earliest != nil {
			return nil
		}
		decodedParameterValueResult[decodedKey] = decodedValue
	}
	return decodedParameterValueResult
}
