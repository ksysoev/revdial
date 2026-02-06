package pool

// Selector defines the interface for selecting a connection from the pool.
// Different strategies can be implemented (least streams, round-robin, random, etc.)
type Selector interface {
	Select(conns []*MuxConn) *MuxConn
}

// LeastStreamsSelector selects the connection with the fewest active streams.
// This provides natural load balancing across connections.
type LeastStreamsSelector struct{}

// NewLeastStreamsSelector creates a new LeastStreamsSelector.
func NewLeastStreamsSelector() *LeastStreamsSelector {
	return &LeastStreamsSelector{}
}

// Select returns the connection with the fewest active streams.
// It takes a slice of MuxConn pointers and returns the best candidate.
// It returns nil if the slice is empty.
func (s *LeastStreamsSelector) Select(conns []*MuxConn) *MuxConn {
	if len(conns) == 0 {
		return nil
	}

	best := conns[0]
	minStreams := best.NumStreams()

	for _, conn := range conns[1:] {
		n := conn.NumStreams()
		if n < minStreams {
			minStreams = n
			best = conn
		}
	}

	return best
}
