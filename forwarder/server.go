package forwarder

import (
	"context"
	"log"
	"net"

	"github.com/pkg/errors"
	"github.com/sagernet/sing/common/cache"
)

type Server struct {
	destination *net.UDPAddr

	idToConn      *cache.LruCache[uint32, *net.UDPConn]
	localAddrToID *cache.LruCache[string, uint32]

	ctx    context.Context
	cancel context.CancelFunc
}

func NewServer(ctx context.Context, destination *net.UDPAddr) *Server {
	ctx, cancel := context.WithCancel(ctx)
	return &Server{
		destination: destination,
		idToConn: cache.New(
			cache.WithAge[uint32, *net.UDPConn](int64(UDPTimeout.Seconds())),
			cache.WithUpdateAgeOnGet[uint32, *net.UDPConn](),
			cache.WithEvict(func(id uint32, conn *net.UDPConn) {
				conn.Close()
			}),
		),
		localAddrToID: cache.New(
			cache.WithAge[string, uint32](int64(UDPTimeout.Seconds())),
			cache.WithUpdateAgeOnGet[string, uint32](),
		),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *Server) Close() {
	s.cancel()
	s.idToConn.Clear()
}

func (s *Server) Handle(relayConn DatagramConn) error {
	for {
		// read from relay
		data, err := relayConn.ReceiveDatagram(s.ctx)
		if err != nil {
			return errors.Wrap(err, "error when receiving from relay")
		}
		log.Printf("server received %d bytes from relay", len(data))

		p := MultiplexDatagram(data)
		if p.IsInvalid() {
			continue
		}

		channelID := p.ChannelID()
		conn, _ := s.idToConn.LoadOrStore(channelID, func() *net.UDPConn {
			conn, err := net.ListenUDP("udp", nil)
			if err != nil {
				log.Printf("error when dialing udp: %v", err)
			}

			// handle incoming datagrams from destination
			go func() {
				buf := make([]byte, UDPBufferSize)
				for {
					select {
					case <-s.ctx.Done():
						return
					default:
						n, _, err := conn.ReadFromUDP(buf)
						if err != nil {
							log.Printf("error when reading from udp: %v", err)
							return
						}

						p := NewMultiplexDatagram(channelID, buf[:n])
						if err := p.SendTo(relayConn); err != nil {
							log.Printf("error when sending to relay: %v", err)
							return
						}
					}
				}
			}()

			return conn
		})

		n, err := conn.WriteToUDP(p.Data(), s.destination)
		if err != nil {
			return errors.Wrap(err, "error when writing to udp")
		}
		if n != len(p.Data()) {
			log.Printf("short write: %d/%d", n, len(p.Data()))
		}
		return nil
	}
}

func (s *Server) Wait() {
	<-s.ctx.Done()
}
