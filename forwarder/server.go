package forwarder

import (
	"context"
	"net"

	"github.com/pkg/errors"
	"github.com/sagernet/sing/common/cache"
	"github.com/charmbracelet/log"
)

type Server struct {
	destination *net.UDPAddr

	ctx    context.Context
	cancel context.CancelFunc

	logger *log.Logger
}

func NewServer(ctx context.Context, destination *net.UDPAddr, logger *log.Logger) *Server {
	if logger == nil {
		logger = log.Default()
	}

	ctx, cancel := context.WithCancel(ctx)
	return &Server{
		destination: destination,
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger,
	}
}

func (s *Server) Close() {
	s.cancel()
}

func (s *Server) Handle(relayConn DatagramConn) error {
	idToConn := cache.New(
		cache.WithAge[uint32, *net.UDPConn](int64(UDPTimeout.Seconds())),
		cache.WithUpdateAgeOnGet[uint32, *net.UDPConn](),
		cache.WithEvict(func(id uint32, conn *net.UDPConn) {
			conn.Close()
		}),
	)
	defer idToConn.Clear()

	for {
		select {
		case <-s.ctx.Done():
			return nil
		default:
		}
		// read from relay
		data, err := relayConn.ReceiveDatagram(s.ctx)
		if err != nil {
			return errors.Wrap(err, "server error when receiving from relay")
		}
		s.logger.Debugf("server received %d bytes from relay", len(data))

		p := MultiplexDatagram(data)
		if p.IsInvalid() {
			continue
		}

		channelID := p.ChannelID()
		conn, _ := idToConn.LoadOrStore(channelID, func() *net.UDPConn {
			ln, err := net.ListenUDP("udp", nil)
			if err != nil {
				s.logger.Warnf("server error when dialing udp: %v", err)
			}

			// handle incoming datagrams from destination
			go func() {
				defer idToConn.Delete(channelID)

				buf := make([]byte, UDPBufferSize)
				for {
					select {
					case <-s.ctx.Done():
						return
					default:
					}
					n, addr, err := ln.ReadFromUDP(buf)
					if err != nil {
						s.logger.Warnf("server error when reading from udp: %v", err)
						return
					}
					s.logger.Debugf("server received %d bytes from %s", n, addr)

					p := NewMultiplexDatagram(channelID, buf[:n])
					if err := p.SendTo(relayConn); err != nil {
						s.logger.Warnf("server error when sending to relay: %v", err)
						return
					}
				}
			}()

			return ln
		})

		n, err := conn.WriteToUDP(p.Data(), s.destination)
		if err != nil {
			return errors.Wrap(err, "server error when writing to udp")
		}
		s.logger.Debugf("server sent %d bytes to %s", n, s.destination)
	}
}

func (s *Server) Wait() {
	<-s.ctx.Done()
}
