package protocol

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

const HeaderSize = 12

var Magic = []byte("SPIL")

const ProtocolVersion uint16 = 1
const DefaultMaxFrameBytes = 16 << 20

type FrameKind byte

const (
	Request  FrameKind = 1
	Response FrameKind = 2
	Event    FrameKind = 3
)

type ProtocolError struct {
	Message string
	Fatal   bool
}

func (e *ProtocolError) Error() string { return e.Message }

type Frame struct {
	Kind    FrameKind
	Payload Value
}

func EncodeFrame(k FrameKind, p Value) ([]byte, error) {
	return EncodeFrameLimit(k, p, DefaultMaxFrameBytes)
}

func EncodeFrameLimit(k FrameKind, p Value, maxBytes int) ([]byte, error) {
	if k < Request || k > Event {
		return nil, &ProtocolError{fmt.Sprintf("Unknown frame kind: %d", k), true}
	}
	body, e := EncodeMsgpack(p)
	if e != nil {
		return nil, e
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxFrameBytes
	}
	if len(body) > maxBytes {
		return nil, &ProtocolError{fmt.Sprintf("Frame payload exceeds %d bytes", maxBytes), true}
	}
	out := make([]byte, HeaderSize+len(body))
	copy(out, Magic)
	binary.LittleEndian.PutUint16(out[4:], ProtocolVersion)
	out[6] = byte(k)
	binary.LittleEndian.PutUint32(out[8:], uint32(len(body)))
	copy(out[HeaderSize:], body)
	return out, nil
}

type FrameDecoder struct {
	buf []byte
	max int
}

func NewFrameDecoder(max int) *FrameDecoder {
	if max <= 0 {
		max = DefaultMaxFrameBytes
	}
	return &FrameDecoder{max: max}
}
func (d *FrameDecoder) Push(c []byte) ([]Frame, error) {
	d.buf = append(d.buf, c...)
	var out []Frame
	for len(d.buf) >= HeaderSize {
		if !bytes.Equal(d.buf[:4], Magic) {
			return nil, &ProtocolError{"Invalid frame magic", true}
		}
		if binary.LittleEndian.Uint16(d.buf[4:]) != ProtocolVersion {
			return nil, &ProtocolError{"Unsupported protocol version", true}
		}
		k := FrameKind(d.buf[6])
		if k < Request || k > Event {
			return nil, &ProtocolError{"Unknown frame kind", true}
		}
		if d.buf[7] != 0 {
			return nil, &ProtocolError{"Non-zero frame flags are not supported", true}
		}
		n := binary.LittleEndian.Uint32(d.buf[8:])
		if n > uint32(d.max) {
			return nil, &ProtocolError{fmt.Sprintf("Frame payload exceeds %d bytes", d.max), true}
		}
		if len(d.buf) < HeaderSize+int(n) {
			break
		}
		v, e := DecodeMsgpack(d.buf[HeaderSize : HeaderSize+int(n)])
		if e != nil {
			return nil, &ProtocolError{fmt.Sprintf("Invalid MessagePack payload: %v", e), true}
		}
		out = append(out, Frame{k, v})
		d.buf = d.buf[HeaderSize+int(n):]
	}
	return out, nil
}
func (d *FrameDecoder) Finish() error {
	if len(d.buf) > 0 {
		return &ProtocolError{"Connection ended with a truncated frame", true}
	}
	return nil
}
