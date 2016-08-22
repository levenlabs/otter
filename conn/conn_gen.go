package conn

// NOTE: THIS FILE WAS PRODUCED BY THE
// MSGP CODE GENERATION TOOL (github.com/tinylib/msgp)
// DO NOT EDIT

import "github.com/tinylib/msgp/msgp"

// DecodeMsg implements msgp.Decodable
func (z *Conn) DecodeMsg(dc *msgp.Reader) (err error) {
	var field []byte
	_ = field
	var zxvk uint32
	zxvk, err = dc.ReadMapHeader()
	if err != nil {
		return
	}
	for zxvk > 0 {
		zxvk--
		field, err = dc.ReadMapKeyPtr()
		if err != nil {
			return
		}
		switch msgp.UnsafeString(field) {
		case "ID":
			{
				var zbzg string
				zbzg, err = dc.ReadString()
				z.ID = ID(zbzg)
			}
			if err != nil {
				return
			}
		case "Presence":
			z.Presence, err = dc.ReadString()
			if err != nil {
				return
			}
		case "IsBackend":
			z.IsBackend, err = dc.ReadBool()
			if err != nil {
				return
			}
		default:
			err = dc.Skip()
			if err != nil {
				return
			}
		}
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z Conn) EncodeMsg(en *msgp.Writer) (err error) {
	// map header, size 3
	// write "ID"
	err = en.Append(0x83, 0xa2, 0x49, 0x44)
	if err != nil {
		return err
	}
	err = en.WriteString(string(z.ID))
	if err != nil {
		return
	}
	// write "Presence"
	err = en.Append(0xa8, 0x50, 0x72, 0x65, 0x73, 0x65, 0x6e, 0x63, 0x65)
	if err != nil {
		return err
	}
	err = en.WriteString(z.Presence)
	if err != nil {
		return
	}
	// write "IsBackend"
	err = en.Append(0xa9, 0x49, 0x73, 0x42, 0x61, 0x63, 0x6b, 0x65, 0x6e, 0x64)
	if err != nil {
		return err
	}
	err = en.WriteBool(z.IsBackend)
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z Conn) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	// map header, size 3
	// string "ID"
	o = append(o, 0x83, 0xa2, 0x49, 0x44)
	o = msgp.AppendString(o, string(z.ID))
	// string "Presence"
	o = append(o, 0xa8, 0x50, 0x72, 0x65, 0x73, 0x65, 0x6e, 0x63, 0x65)
	o = msgp.AppendString(o, z.Presence)
	// string "IsBackend"
	o = append(o, 0xa9, 0x49, 0x73, 0x42, 0x61, 0x63, 0x6b, 0x65, 0x6e, 0x64)
	o = msgp.AppendBool(o, z.IsBackend)
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *Conn) UnmarshalMsg(bts []byte) (o []byte, err error) {
	var field []byte
	_ = field
	var zbai uint32
	zbai, bts, err = msgp.ReadMapHeaderBytes(bts)
	if err != nil {
		return
	}
	for zbai > 0 {
		zbai--
		field, bts, err = msgp.ReadMapKeyZC(bts)
		if err != nil {
			return
		}
		switch msgp.UnsafeString(field) {
		case "ID":
			{
				var zcmr string
				zcmr, bts, err = msgp.ReadStringBytes(bts)
				z.ID = ID(zcmr)
			}
			if err != nil {
				return
			}
		case "Presence":
			z.Presence, bts, err = msgp.ReadStringBytes(bts)
			if err != nil {
				return
			}
		case "IsBackend":
			z.IsBackend, bts, err = msgp.ReadBoolBytes(bts)
			if err != nil {
				return
			}
		default:
			bts, err = msgp.Skip(bts)
			if err != nil {
				return
			}
		}
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z Conn) Msgsize() (s int) {
	s = 1 + 3 + msgp.StringPrefixSize + len(string(z.ID)) + 9 + msgp.StringPrefixSize + len(z.Presence) + 10 + msgp.BoolSize
	return
}

// DecodeMsg implements msgp.Decodable
func (z *ID) DecodeMsg(dc *msgp.Reader) (err error) {
	{
		var zajw string
		zajw, err = dc.ReadString()
		(*z) = ID(zajw)
	}
	if err != nil {
		return
	}
	return
}

// EncodeMsg implements msgp.Encodable
func (z ID) EncodeMsg(en *msgp.Writer) (err error) {
	err = en.WriteString(string(z))
	if err != nil {
		return
	}
	return
}

// MarshalMsg implements msgp.Marshaler
func (z ID) MarshalMsg(b []byte) (o []byte, err error) {
	o = msgp.Require(b, z.Msgsize())
	o = msgp.AppendString(o, string(z))
	return
}

// UnmarshalMsg implements msgp.Unmarshaler
func (z *ID) UnmarshalMsg(bts []byte) (o []byte, err error) {
	{
		var zwht string
		zwht, bts, err = msgp.ReadStringBytes(bts)
		(*z) = ID(zwht)
	}
	if err != nil {
		return
	}
	o = bts
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z ID) Msgsize() (s int) {
	s = msgp.StringPrefixSize + len(string(z))
	return
}
