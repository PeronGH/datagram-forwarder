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

	// handle incoming datagrams from listener
	sourceToID := cache.New(
		cache.WithAge[string, uint32](int64(UDPTimeout.Seconds())),
		cache.WithUpdateAgeOnGet[string, uint32](),
	)

	group.Append("handle incoming datagram", func(ctx context.Context) error {
		var lastID uint32

		buf := make([]byte, UDPBufferSize)
		for {
			select {
			case <-ctx.Done():
				return nil
			default:
				n, addr, err := config.Listener.ReadFromUDP(buf)
				if err != nil {
					return errors.Wrap(err, "error when reading from udp")
				}
				data := buf[:n]

				sourceAddr := addr.String()
				channelID, _ := sourceToID.LoadOrStore(sourceAddr, func() uint32 {
					lastID++
					return lastID
				})
				err = NewMultiplexDatagram(channelID, data).SendTo(config.RelayConn)
				if err != nil {
					return errors.Wrap(err, "error when sending to relay")
				}

				// add reply handler
				idToHandler.LoadOrStore(channelID, func() replyHandler {
					return func(reply []byte) {
						_, err := config.Listener.WriteToUDP(reply, addr)
						if err != nil {
							log.Printf("error when writing to udp: %v", err)
						}
					}
				})
			}
		}
	})

	return group.Run(config.Ctx)
}

type replyHandler func([]byte)
