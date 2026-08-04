package common

import "time"

const KeyTransportAckReadMetrics = "transport_ack_read_metrics"

type TransportAckReadMetrics struct {
	RawBytes        int
	RawReadDuration time.Duration
}
