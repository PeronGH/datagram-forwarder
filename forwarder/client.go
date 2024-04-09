package forwarder

import (
	"context"
	"log"
	"net"

	"github.com/pkg/errors"
	"github.com/sagernet/sing/common/cache"
	"github.com/sagernet/sing/common/task"
)

type ClientConfig struct {
	Ctx       context.Context
	RelayConn DatagramConn
	Listener  *net.UDPConn
}

func RunClient(config ClientConfig) error {
	var group task.Group

	// read from relay
	idToHandler := cache.New(
		cache.WithAge[uint32, replyHandler](int64(UDPTimeout.Seconds())),
		cache.WithUpdateAgeOnGet[uint32, replyHandler](),
	)

	group.Append("read from relay", func(ctx context.Context) error {
		for {
			data, err := config.RelayConn.ReceiveDatagram(ctx)
			if err != nil {
				return errors.Wrap(err, "error when receiving from relay")
			}
			log.Printf("received %d bytes from relay", len(data))

			p := MultiplexDatagram(data)
			if p.IsInvalid() {
				continue
			}

			channelID := p.ChannelID()
			handler, ok := idToHandler.Load(channelID)
			if ok {
				handler(p.Data())
			}
		}
	})

	// read from listener
	incomingCh := make(chan inboundDatagram, UDPBacklog)

	group.Append("read from listener", func(ctx context.Context) error {
		defer close(incomingCh)
		buf := make([]byte, UDPBufferSize)
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				n, addr, err := config.Listener.ReadFromUDP(buf)
				if err != nil {
					return errors.Wrap(err, "error when reading from udp")
				}
				log.Printf("received %d bytes from %s", n, addr)
				incomingCh <- inboundDatagram{buf[:n], addr}
			}
		}
	})

	// handle incoming datagrams from listener
	sourceToID := cache.New(
		cache.WithAge[string, uint32](int64(UDPTimeout.Seconds())),
		cache.WithUpdateAgeOnGet[string, uint32](),
	)

	group.Append("handle incoming datagram", func(ctx context.Context) error {
		var lastID uint32

		for datagram := range incomingCh {
			sourceAddr := datagram.addr.String()
			channelID, _ := sourceToID.LoadOrStore(sourceAddr, func() uint32 {
				lastID++
				return lastID
			})
			err := NewMultiplexDatagram(channelID, datagram.b).SendTo(config.RelayConn)
			if err != nil {
				return errors.Wrap(err, "error when sending to relay")
			}

			// add reply handler
			idToHandler.LoadOrStore(channelID, func() replyHandler {
				return func(reply []byte) {
					_, err := config.Listener.WriteToUDP(reply, datagram.addr)
					if err != nil {
						log.Printf("error when writing to udp: %v", err)
					}
				}
			})
		}

		return nil
	})

	return group.Run(config.Ctx)
}

type inboundDatagram struct {
	b    []byte
	addr *net.UDPAddr
}

type replyHandler func([]byte)
