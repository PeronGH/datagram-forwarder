package forwarder

import (
	"context"
	"errors"
	"sync"
)

type DatagramConn interface {
	SendDatagram(payload []byte) error
	ReceiveDatagram(context.Context) ([]byte, error)
}

type datagramPipeConn struct {
	sendCh    chan []byte
	receiveCh chan []byte
	closeCh   chan struct{}
}

func (c *datagramPipeConn) SendDatagram(payload []byte) error {
	select {
	case <-c.closeCh:
		return errors.New("connection closed")
	case c.sendCh <- payload:
		return nil
	}
}

func (c *datagramPipeConn) ReceiveDatagram(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.closeCh:
		return nil, errors.New("connection closed")
	case payload := <-c.receiveCh:
		return payload, nil
	}
}

// debug purpose only
func DatagramConnPipe() (DatagramConn, DatagramConn, func()) {
	ch1 := make(chan []byte, UDPBacklog)
	ch2 := make(chan []byte, UDPBacklog)
	closeCh := make(chan struct{})

	conn1 := &datagramPipeConn{
		sendCh:    ch1,
		receiveCh: ch2,
		closeCh:   closeCh,
	}
	conn2 := &datagramPipeConn{
		sendCh:    ch2,
		receiveCh: ch1,
		closeCh:   closeCh,
	}

	var once sync.Once
	close := func() {
		once.Do(func() {
			close(closeCh)
		})
	}

	return conn1, conn2, close
}
