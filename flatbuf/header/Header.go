// automatically generated by the FlatBuffers compiler, do not modify

package header

import (
	flatbuffers "github.com/google/flatbuffers/go"
)

type Header struct {
	_tab flatbuffers.Table
}

func GetRootAsHeader(buf []byte, offset flatbuffers.UOffsetT) *Header {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &Header{}
	x.Init(buf, n+offset)
	return x
}

func (rcv *Header) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Header) Event() int8 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.GetInt8(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Header) MutateEvent(n int8) bool {
	return rcv._tab.MutateInt8Slot(4, n)
}

func (rcv *Header) Opcode() int8 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.GetInt8(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Header) MutateOpcode(n int8) bool {
	return rcv._tab.MutateInt8Slot(6, n)
}

func (rcv *Header) Metadata() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func HeaderStart(builder *flatbuffers.Builder) {
	builder.StartObject(3)
}
func HeaderAddEvent(builder *flatbuffers.Builder, event int8) {
	builder.PrependInt8Slot(0, event, 0)
}
func HeaderAddOpcode(builder *flatbuffers.Builder, opcode int8) {
	builder.PrependInt8Slot(1, opcode, 0)
}
func HeaderAddMetadata(builder *flatbuffers.Builder, metadata flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(2, flatbuffers.UOffsetT(metadata), 0)
}
func HeaderEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT {
	return builder.EndObject()
}