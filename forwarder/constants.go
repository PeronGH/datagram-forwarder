package forwarder

import "time"

const (
	UDPTimeout    = 5 * time.Minute
	UDPBufferSize = 8 * 1024
	UDPBacklog    = 128
)
