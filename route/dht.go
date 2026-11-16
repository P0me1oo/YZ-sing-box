package route

import (
	"context"
	"slices"
	"sync/atomic"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/dht"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/option"
	R "github.com/sagernet/sing-box/route/rule"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// Deliberately has no reader unwrap method: fast copying must not bypass the
// packet check. Writers and lifecycle operations retain the original interface.
type dhtPacketConn struct {
	N.PacketConn
	router   *Router
	ctx      context.Context
	metadata adapter.InboundContext
	reported map[dhtReportKey]struct{}
	blockCtx atomic.Pointer[context.Context]
}

// The tracker may retain the authenticated session identity for observations.
// This does not change the context used to match routing rules.
func (c *dhtPacketConn) SetDHTBlockContext(ctx context.Context) {
	c.blockCtx.Store(&ctx)
}

type dhtReportKey struct {
	destination M.Socksaddr
	rule        string
	outbound    string
}

func (c *dhtPacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	start := buffer.Start()
	for {
		destination, err := c.PacketConn.ReadPacket(buffer)
		if err != nil {
			return destination, err
		}
		if !c.block(buffer.Bytes(), destination) {
			return destination, nil
		}
		buffer.Resize(start, 0)
	}
}

func (c *dhtPacketConn) ReaderMTU() int      { return N.CalculateMTU(c.PacketConn, nil) }
func (c *dhtPacketConn) ReaderOverhead() int { return N.CalculateReaderOverhead(c.PacketConn) }

func (c *dhtPacketConn) block(packet []byte, destination M.Socksaddr) bool {
	if !dht.IsMessage(packet) {
		return false
	}
	r := c.router
	r.rulesAccess.RLock()
	defer r.rulesAccess.RUnlock()
	if r.closed || len(r.dhtRuleIndices) == 0 {
		return false
	}
	metadata := c.metadata
	if destination.IsValid() && destination != metadata.Destination {
		metadata.Destination = destination
		metadata.Domain = ""
		metadata.DestinationAddresses = nil
	}
	metadata.Protocol = C.ProtocolBitTorrent
	// Reuse the normal matcher, including rule order and all other conditions.
	// Sniff actions see the known protocol and never read from the connection.
	// Block observations are deduplicated below; do not format a debug route
	// message for every repeated datagram, even when debug logging is disabled.
	rule, index, _, _, err := r.matchRuleWithLogging(c.ctx, &metadata, nil, nil, false)
	if err != nil || !r.isDHTBlockRule(rule, index) {
		return false
	}
	var outbound adapter.Outbound
	switch action := rule.Action().(type) {
	case *R.RuleActionRoute:
		outbound, _ = r.outbound.Outbound(action.Outbound)
		if outbound == nil || outbound.Type() != C.TypeBlock {
			return false
		}
	case *R.RuleActionReject:
	default:
		return false
	}
	metadata.RouteRule = r.dhtRuleNames[index]
	if outbound != nil {
		metadata.RouteOutbound = outbound.Tag()
	}
	key := dhtReportKey{
		destination: metadata.Destination,
		rule:        metadata.RouteRule,
		outbound:    metadata.RouteOutbound,
	}
	if c.reported == nil {
		c.reported = make(map[dhtReportKey]struct{})
	}
	if _, exists := c.reported[key]; !exists {
		// Bound per-session logging state even for many-target UDP transports.
		if len(c.reported) < 256 {
			c.reported[key] = struct{}{}
			blockCtx := c.ctx
			if captured := c.blockCtx.Load(); captured != nil {
				blockCtx = *captured
			}
			for _, tracker := range r.trackers {
				if observer, ok := tracker.(adapter.DHTBlockTracker); ok {
					observer.BlockedDHTPacket(blockCtx, metadata, rule, outbound)
				}
			}
		}
	}
	return true
}

// Called with rulesAccess held, so route reloads change both the gate and matcher
// atomically. Only explicit BT rules with a blocking action enable the guard.
func (r *Router) hasDHTBlockRule() bool {
	for _, index := range r.dhtRuleIndices {
		if r.isDHTBlockRule(r.rules[index], index) {
			return true
		}
	}
	return false
}

func (r *Router) isDHTBlockRule(rule adapter.Rule, index int) bool {
	if rule == nil || !r.dhtRuleFlags[index] {
		return false
	}
	switch action := rule.Action().(type) {
	case *R.RuleActionReject:
		return true
	case *R.RuleActionRoute:
		outbound, _ := r.outbound.Outbound(action.Outbound)
		if outbound != nil && outbound.Type() == C.TypeBlock {
			return true
		}
	}
	return false
}

// Build these alongside the rule snapshot. Outbound types remain dynamic and
// are checked when used, including when an outbound is replaced independently.
func (r *Router) prepareDHTRules() {
	r.dhtRuleIndices = make([]int, 0)
	r.dhtRuleNames = make([]string, len(r.rules))
	r.dhtRuleFlags = make([]bool, len(r.rules))
	for index, rule := range r.rules {
		options := r.ruleOptions[index]
		if options.Type != C.RuleTypeLogical && !options.DefaultOptions.Invert {
			networks := options.DefaultOptions.Network
			if len(networks) > 0 && !slices.Contains(networks, N.NetworkUDP) {
				continue
			}
		}
		if hasBTProtocol(options) {
			r.dhtRuleIndices = append(r.dhtRuleIndices, index)
			r.dhtRuleNames[index] = rule.String()
			r.dhtRuleFlags[index] = true
		}
	}
}

func hasBTProtocol(rule option.Rule) bool {
	if rule.Type == C.RuleTypeLogical {
		for _, child := range rule.LogicalOptions.Rules {
			if hasBTProtocol(child) {
				return true
			}
		}
		return false
	}
	for _, protocol := range rule.DefaultOptions.Protocol {
		if protocol == C.ProtocolBitTorrent {
			return true
		}
	}
	return false
}
