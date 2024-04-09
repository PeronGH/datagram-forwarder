package forwarder

import "time"

const (
	UDPTimeout    = 5 * time.Minute
	UDPBufferSize = 16 * 1024
	UDPBacklog    = 128
)
