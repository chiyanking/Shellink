package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

type Value any
type KV struct {
	Key string
	Val Value
}

var ErrOversized = errors.New("messagepack value too large")

func EncodeMsgpack(v Value) ([]byte, error) {
	var b bytes.Buffer
	if err := encode(&b, v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func encode(b *bytes.Buffer, v Value) error {
	switch x := v.(type) {
	case nil:
		b.WriteByte(0xc0)
	case bool:
		if x {
			b.WriteByte(0xc3)
		} else {
			b.WriteByte(0xc2)
		}
	case int:
		return encodeInt(b, int64(x))
	case int8:
		return encodeInt(b, int64(x))
	case int16:
		return encodeInt(b, int64(x))
	case int32:
		return encodeInt(b, int64(x))
	case int64:
		return encodeInt(b, x)
	case uint:
		return encodeUint(b, uint64(x))
	case uint8:
		return encodeUint(b, uint64(x))
	case uint16:
		return encodeUint(b, uint64(x))
	case uint32:
		return encodeUint(b, uint64(x))
	case uint64:
		return encodeUint(b, x)
	case float32:
		b.WriteByte(0xcb)
		return writeU64(b, math.Float64bits(float64(x)))
	case float64:
		b.WriteByte(0xcb)
		return writeU64(b, math.Float64bits(x))
	case string:
		return encodeString(b, x)
	case []byte:
		return encodeBin(b, x)
	case []Value:
		if len(x) > 65535 {
			return ErrOversized
		}
		writeArrayHeader(b, len(x))
		for _, e := range x {
			if err := encode(b, e); err != nil {
				return err
			}
		}
		return nil
	case []KV:
		if len(x) > 65535 {
			return ErrOversized
		}
		writeMapHeader(b, len(x))
		for _, e := range x {
			if err := encodeString(b, e.Key); err != nil {
				return err
			}
			if err := encode(b, e.Val); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported messagepack value %T", v)
	}
	return nil
}
func encodeInt(b *bytes.Buffer, n int64) error {
	if n >= 0 {
		return encodeUint(b, uint64(n))
	}
	if n >= -32 {
		b.WriteByte(byte(int8(n)))
		return nil
	}
	if n >= -128 {
		b.WriteByte(0xd0)
		b.WriteByte(byte(int8(n)))
		return nil
	}
	if n >= -32768 {
		b.WriteByte(0xd1)
		return writeU16(b, uint16(int16(n)))
	}
	if n >= -2147483648 {
		b.WriteByte(0xd2)
		return writeU32(b, uint32(int32(n)))
	}
	b.WriteByte(0xd3)
	return writeU64(b, uint64(n))
}
func encodeUint(b *bytes.Buffer, n uint64) error {
	switch {
	case n < 128:
		b.WriteByte(byte(n))
	case n <= 255:
		b.Write([]byte{0xcc, byte(n)})
	case n <= 65535:
		b.WriteByte(0xcd)
		return writeU16(b, uint16(n))
	case n <= 4294967295:
		b.WriteByte(0xce)
		return writeU32(b, uint32(n))
	default:
		b.WriteByte(0xcf)
		return writeU64(b, n)
	}
	return nil
}
func encodeString(b *bytes.Buffer, s string) error {
	n := len(s)
	if n < 32 {
		b.WriteByte(byte(0xa0 | n))
	} else if n <= 255 {
		b.Write([]byte{0xd9, byte(n)})
	} else if n <= 65535 {
		b.WriteByte(0xda)
		if err := writeU16(b, uint16(n)); err != nil {
			return err
		}
	} else {
		b.WriteByte(0xdb)
		if err := writeU32(b, uint32(n)); err != nil {
			return err
		}
	}
	b.WriteString(s)
	return nil
}
func encodeBin(b *bytes.Buffer, v []byte) error {
	n := len(v)
	if n <= 255 {
		b.Write([]byte{0xc4, byte(n)})
	} else if n <= 65535 {
		b.WriteByte(0xc5)
		writeU16(b, uint16(n))
	} else {
		b.WriteByte(0xc6)
		writeU32(b, uint32(n))
	}
	b.Write(v)
	return nil
}
func writeArrayHeader(b *bytes.Buffer, n int) {
	if n < 16 {
		b.WriteByte(byte(0x90 | n))
	} else {
		b.WriteByte(0xdc)
		writeU16(b, uint16(n))
	}
}
func writeMapHeader(b *bytes.Buffer, n int) {
	if n < 16 {
		b.WriteByte(byte(0x80 | n))
	} else {
		b.WriteByte(0xde)
		writeU16(b, uint16(n))
	}
}
func writeU16(b *bytes.Buffer, v uint16) error { return binary.Write(b, binary.BigEndian, v) }
func writeU32(b *bytes.Buffer, v uint32) error { return binary.Write(b, binary.BigEndian, v) }
func writeU64(b *bytes.Buffer, v uint64) error { return binary.Write(b, binary.BigEndian, v) }

func DecodeMsgpack(data []byte) (Value, error) {
	d := decoder{b: data}
	v, err := d.value()
	if err != nil {
		return nil, err
	}
	if d.i != len(data) {
		return nil, fmt.Errorf("trailing messagepack data")
	}
	return v, nil
}

type decoder struct {
	b     []byte
	i     int
	depth int
}

func (d *decoder) take(n int) ([]byte, error) {
	if n < 0 || n > len(d.b)-d.i {
		return nil, errors.New("truncated messagepack")
	}
	v := d.b[d.i : d.i+n]
	d.i += n
	return v, nil
}
func (d *decoder) value() (Value, error) {
	if d.depth >= 128 {
		return nil, errors.New("messagepack nesting limit exceeded")
	}
	d.depth++
	defer func() { d.depth-- }()
	p, err := d.take(1)
	if err != nil {
		return nil, err
	}
	c := p[0]
	switch {
	case c <= 0x7f:
		return uint64(c), nil
	case c >= 0xe0:
		return int64(int8(c)), nil
	case c >= 0xa0 && c <= 0xbf:
		n := int(c & 31)
		v, e := d.take(n)
		return string(v), e
	case c >= 0x90 && c <= 0x9f:
		n := int(c & 15)
		return d.array(n)
	case c >= 0x80 && c <= 0x8f:
		n := int(c & 15)
		return d.mapv(n)
	}
	switch c {
	case 0xc0:
		return nil, nil
	case 0xc2:
		return false, nil
	case 0xc3:
		return true, nil
	}
	widths := map[byte]int{0xcc: 1, 0xcd: 2, 0xce: 4, 0xcf: 8, 0xd0: 1, 0xd1: 2, 0xd2: 4, 0xd3: 8, 0xca: 4, 0xcb: 8, 0xd9: 1, 0xda: 2, 0xdb: 4, 0xc4: 1, 0xc5: 2, 0xc6: 4, 0xdc: 2, 0xdd: 4, 0xde: 2, 0xdf: 4}
	width, ok := widths[c]
	if !ok {
		return nil, fmt.Errorf("unsupported messagepack tag 0x%x", c)
	}
	data, e := d.take(width)
	if e != nil {
		return nil, e
	}
	var n uint64
	switch width {
	case 1:
		n = uint64(data[0])
	case 2:
		n = uint64(binary.BigEndian.Uint16(data))
	case 4:
		n = uint64(binary.BigEndian.Uint32(data))
	case 8:
		n = binary.BigEndian.Uint64(data)
	}
	switch c {
	case 0xcc, 0xcd, 0xce, 0xcf:
		return n, nil
	case 0xd0:
		return int64(int8(n)), nil
	case 0xd1:
		return int64(int16(n)), nil
	case 0xd2:
		return int64(int32(n)), nil
	case 0xd3:
		return int64(n), nil
	case 0xca:
		return float64(math.Float32frombits(uint32(n))), nil
	case 0xcb:
		return math.Float64frombits(n), nil
	}
	if n > uint64(len(d.b)-d.i) {
		return nil, errors.New("truncated messagepack")
	}
	switch c {
	case 0xd9, 0xda, 0xdb:
		return d.str(int(n))
	case 0xc4, 0xc5, 0xc6:
		return d.bin(int(n))
	case 0xdc, 0xdd:
		return d.array(int(n))
	case 0xde, 0xdf:
		return d.mapv(int(n))
	}
	return nil, errors.New("invalid messagepack")
}
func (d *decoder) str(n int) (Value, error) { v, e := d.take(n); return string(v), e }
func (d *decoder) bin(n int) (Value, error) { v, e := d.take(n); return append([]byte(nil), v...), e }
func (d *decoder) array(n int) (Value, error) {
	if n > len(d.b)-d.i {
		return nil, errors.New("truncated messagepack array")
	}
	a := make([]Value, n)
	for i := range a {
		v, e := d.value()
		if e != nil {
			return nil, e
		}
		a[i] = v
	}
	return a, nil
}
func (d *decoder) mapv(n int) (Value, error) {
	if n > (len(d.b)-d.i)/2 {
		return nil, errors.New("truncated messagepack map")
	}
	m := make([]KV, 0, n)
	for i := 0; i < n; i++ {
		k, e := d.value()
		if e != nil {
			return nil, e
		}
		s, ok := k.(string)
		if !ok {
			return nil, errors.New("map key is not string")
		}
		v, e := d.value()
		if e != nil {
			return nil, e
		}
		m = append(m, KV{s, v})
	}
	return m, nil
}
