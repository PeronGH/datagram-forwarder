package forwarder

import (
	"encoding/binary"
)

// byte 0-3: channel id
// byte 4-end: data
type MultiplexDatagram []byte

func (p MultiplexDatagram) IsInvalid() bool {
	return len(p) < 4
}

func (p MultiplexDatagram) ChannelID() uint32 {
	return binary.BigEndian.Uint32(p[:4])
}

func (p MultiplexDatagram) Data() []byte {
	return p[4:]
}

func (p MultiplexDatagram) SendTo(conn DatagramConn) error {
	return conn.SendDatagram(p)
}

func NewMultiplexDatagram(channelID uint32, data []byte) MultiplexDatagram {
	p := make(MultiplexDatagram, 4+len(data))
	binary.BigEndian.PutUint32(p[:4], channelID)
	copy(p[4:], data)
	return p
}
