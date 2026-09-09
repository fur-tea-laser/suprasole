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
	Error_Earliest_maybe          error
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
		Error_Earliest_maybe:          nil,
	}
}

func (this *_BinaryDecoder_WebsocketMessage_) CanDecodeParameter(
	byteCount_required int,
	label_expectedParameter string,
	label_targetType string,
) bool {
	if this.Error_Earliest_maybe != nil {
		return false
	} else if byteCount_remaining := len(this.Buffer__Payload_BinaryMessage) - this.Cursor_Buffer; byteCount_remaining < byteCount_required {
		this.Error_Earliest_maybe = _FMT.Errorf(
			"[%s] failed to decode %s for '%s': unexpected end of payload at offset %d (need %d bytes, remaining %d)",
			this.Label_MessageStruct__,
			label_targetType,
			label_expectedParameter,
			this.Cursor_Buffer,
			byteCount_required,
			byteCount_remaining,
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
	value_expectedParameter_result := this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer]
	this.Cursor_Buffer++
	return value_expectedParameter_result
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Uint16(
	label_expectedParameter string,
) uint16 {
	if false == this.CanDecodeParameter(2, label_expectedParameter, "uint16") {
		return 0
	}
	value_expectedParameter_result := _BINARY.BigEndian.Uint16(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer : this.Cursor_Buffer+2])
	this.Cursor_Buffer += 2
	return value_expectedParameter_result
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Uint32(
	label_expectedParameter string,
) uint32 {
	if false == this.CanDecodeParameter(4, label_expectedParameter, "uint32") {
		return 0
	}
	value_expectedParameter_result := _BINARY.BigEndian.Uint32(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer : this.Cursor_Buffer+4])
	this.Cursor_Buffer += 4
	return value_expectedParameter_result
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Int32(
	label_expectedParameter string,
) int32 {
	return int32(this.DecodeParameter_Uint32(label_expectedParameter))
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_String16(
	label_expectedParameter string,
) string {
	return __decodeParameter__Bytes16(
		this,
		label_expectedParameter,
		"string16 data",
		func(bytes_expectedParameter []byte) string {
			return string(bytes_expectedParameter)
		},
	)
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_Bytes16(
	label_expectedParameter string,
) []byte {
	return __decodeParameter__Bytes16(
		this,
		label_expectedParameter,
		"bytes16 data",
		_BYTES.Clone,
	)
}

func __decodeParameter__Bytes16[
	__Result__ any,
](
	binaryDecoder *_BinaryDecoder_WebsocketMessage_,
	label_expectedParameter string,
	label_targetType string,
	convertBytes_expectedParameter__ func(bytes_expectedParameter []byte) __Result__,
) __Result__ {
	length_expectedParameter := int(binaryDecoder.DecodeParameter_Uint16(label_expectedParameter + ".Length"))
	if false == binaryDecoder.CanDecodeParameter(length_expectedParameter, label_expectedParameter, label_targetType) {
		var expectedParameter_result __Result__
		return expectedParameter_result
	}
	bytes_expectedParameter := binaryDecoder.Buffer__Payload_BinaryMessage[binaryDecoder.Cursor_Buffer : binaryDecoder.Cursor_Buffer+length_expectedParameter]
	binaryDecoder.Cursor_Buffer += length_expectedParameter
	return convertBytes_expectedParameter__(bytes_expectedParameter)
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter_TrailingBytes(
	label_expectedParameter string,
) []byte {
	if this.Error_Earliest_maybe != nil {
		return nil
	} else if this.Cursor_Buffer > len(this.Buffer__Payload_BinaryMessage) {
		this.Error_Earliest_maybe = _FMT.Errorf(
			"[%s] failed to decode trailing bytes for '%s': cursor offset %d exceeds payload length %d",
			this.Label_MessageStruct__,
			label_expectedParameter,
			this.Cursor_Buffer,
			len(this.Buffer__Payload_BinaryMessage),
		)
		return nil
	}
	bytes_expectedParameter_result := _BYTES.Clone(this.Buffer__Payload_BinaryMessage[this.Cursor_Buffer:])
	this.Cursor_Buffer = len(this.Buffer__Payload_BinaryMessage)
	return bytes_expectedParameter_result
}

func (this *_BinaryDecoder_WebsocketMessage_) AssertEndOfPayload() {
	if this.Error_Earliest_maybe != nil {
		return
	} else if this.Cursor_Buffer != len(this.Buffer__Payload_BinaryMessage) {
		this.Error_Earliest_maybe = _FMT.Errorf(
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
	return __decodeParameter__Slice16(
		this,
		label_expectedParameter,
		func(binaryDecoder *_BinaryDecoder_WebsocketMessage_, index_expectedParameter_current int) string {
			return binaryDecoder.DecodeParameter_String16(
				_FMT.Sprintf(
					"%s[%d]",
					label_expectedParameter,
					index_expectedParameter_current,
				),
			)
		},
	)
}

func (this *_BinaryDecoder_WebsocketMessage_) DecodeParameter__Slice16_Uint32(
	label_expectedParameter string,
) []uint32 {
	return __decodeParameter__Slice16(
		this,
		label_expectedParameter,
		func(binaryDecoder *_BinaryDecoder_WebsocketMessage_, index_expectedParameter_current int) uint32 {
			return binaryDecoder.DecodeParameter_Uint32(
				_FMT.Sprintf(
					"%s[%d]",
					label_expectedParameter,
					index_expectedParameter_current,
				),
			)
		},
	)
}

func DecodeParameter__Slice16__BinaryDecoder_WebsocketMessage[
	__Element__ any,
](
	binaryDecoder *_BinaryDecoder_WebsocketMessage_,
	label_expectedParameter string,
	decodeElement_expectedParameter__ func(binaryDecoder *_BinaryDecoder_WebsocketMessage_, index_expectedParameter_current int) __Element__,
) []__Element__ {
	return __decodeParameter__Slice16(
		binaryDecoder,
		label_expectedParameter,
		decodeElement_expectedParameter__,
	)
}

func __decodeParameter__Slice16[
	__Element__ any,
](
	binaryDecoder *_BinaryDecoder_WebsocketMessage_,
	label_expectedParameter string,
	decodeElement_expectedParameter__ func(binaryDecoder *_BinaryDecoder_WebsocketMessage_, index_expectedParameter_current int) __Element__,
) []__Element__ {
	count_expectedParameter := int(binaryDecoder.DecodeParameter_Uint16(label_expectedParameter + ".Count"))
	if binaryDecoder.Error_Earliest_maybe != nil {
		return nil
	}
	slice_expectedParameter_result := make([]__Element__, count_expectedParameter)
	for index_expectedParameter_current := 0; index_expectedParameter_current < count_expectedParameter; index_expectedParameter_current++ {
		element_expected := decodeElement_expectedParameter__(
			binaryDecoder,
			index_expectedParameter_current,
		)
		if binaryDecoder.Error_Earliest_maybe != nil {
			return nil
		}
		slice_expectedParameter_result[index_expectedParameter_current] = element_expected
	}
	return slice_expectedParameter_result
}

func DecodeParameter__Map16__BinaryDecoder_WebsocketMessage[
	__Key__ comparable,
	__Value__ any,
](
	binaryDecoder *_BinaryDecoder_WebsocketMessage_,
	label_expectedParameter string,
	decodeEntry_expectedParameter__ func(binaryDecoder *_BinaryDecoder_WebsocketMessage_, index_expectedParameter_current int) (__Key__, __Value__),
) map[__Key__]__Value__ {
	count_expectedParameter := int(binaryDecoder.DecodeParameter_Uint16(label_expectedParameter + ".Count"))
	if binaryDecoder.Error_Earliest_maybe != nil {
		return nil
	}
	map_expectedParameter_result := make(map[__Key__]__Value__, count_expectedParameter)
	for index_expectedParameter_current := 0; index_expectedParameter_current < count_expectedParameter; index_expectedParameter_current++ {
		entryKey_expected, entryValue_expected := decodeEntry_expectedParameter__(
			binaryDecoder,
			index_expectedParameter_current,
		)
		if binaryDecoder.Error_Earliest_maybe != nil {
			return nil
		}
		map_expectedParameter_result[entryKey_expected] = entryValue_expected
	}
	return map_expectedParameter_result
}
