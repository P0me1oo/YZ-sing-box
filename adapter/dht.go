package adapter

import "context"

// DHTBlockTracker is an optional observer for packets blocked after UDP routing.
// It does not register another connection or alter the main tracker interface.
type DHTBlockTracker interface {
	BlockedDHTPacket(ctx context.Context, metadata InboundContext, rule Rule, outbound Outbound)
}
