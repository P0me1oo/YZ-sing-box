package route

import (
	"context"
	"io"
	"net/netip"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

const testDHTPing = "d1:ad2:id20:01234567890123456789e1:q4:ping1:t2:aa1:y1:qe"

type dhtTestPacket struct {
	payload     string
	destination M.Socksaddr
}
type dhtTestConn struct {
	N.PacketConn
	packets []dhtTestPacket
}

func (c *dhtTestConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	if len(c.packets) == 0 {
		return M.Socksaddr{}, io.EOF
	}
	packet := c.packets[0]
	c.packets = c.packets[1:]
	_, err := buffer.Write([]byte(packet.payload))
	return packet.destination, err
}
func (*dhtTestConn) FrontHeadroom() int { return 10 }
func (*dhtTestConn) RearHeadroom() int  { return 4 }

type dhtTestOutbound struct{ adapter.Outbound }

func (dhtTestOutbound) Type() string { return C.TypeBlock }
func (dhtTestOutbound) Tag() string  { return "block" }

type dhtTestOutbounds struct{ adapter.OutboundManager }

func (dhtTestOutbounds) Outbound(tag string) (adapter.Outbound, bool) {
	if tag == "block" {
		return dhtTestOutbound{}, true
	}
	return nil, false
}

type dhtTestDNS struct{ adapter.DNSRouter }

func (dhtTestDNS) LookupReverseMapping(netip.Addr) (string, bool) { return "", false }

type dhtTestTransports struct{ adapter.DNSTransportManager }

func (dhtTestTransports) FakeIP() adapter.FakeIPTransport { return nil }

type dhtTestTracker struct {
	adapter.ConnectionTracker
	metadata []adapter.InboundContext
}

func (t *dhtTestTracker) BlockedDHTPacket(_ context.Context, metadata adapter.InboundContext, _ adapter.Rule, _ adapter.Outbound) {
	t.metadata = append(t.metadata, metadata)
}

func TestDHTPacketConnPreservesPacketsTargetsAndHeadroom(t *testing.T) {
	logger := log.NewNOPFactory().NewLogger("test")
	r := &Router{
		ctx: service.ContextWithDefaultRegistry(context.Background()), logger: logger, outbound: dhtTestOutbounds{},
		dns: dhtTestDNS{}, dnsTransport: dhtTestTransports{},
	}
	rule := option.Rule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{
		RawDefaultRule: option.RawDefaultRule{
			Protocol: []string{C.ProtocolBitTorrent}, Port: []uint16{6881},
			Network: []string{N.NetworkUDP}, Inbound: []string{"user-in"},
		},
		RuleAction: option.RuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.RouteActionOptions{Outbound: "block"}},
	}}
	if err := r.Initialize([]option.Rule{rule}, nil); err != nil {
		t.Fatal(err)
	}
	tracker := &dhtTestTracker{}
	r.trackers = []adapter.ConnectionTracker{tracker}
	destination := M.ParseSocksaddr("127.0.0.1:6881")
	other := M.ParseSocksaddr("127.0.0.1:6882")
	underlying := &dhtTestConn{packets: []dhtTestPacket{
		{payload: "normal", destination: destination}, {payload: testDHTPing, destination: destination},
		{payload: testDHTPing, destination: destination}, {payload: "after", destination: destination},
		{payload: testDHTPing, destination: other}, {payload: testDHTPing + "tail", destination: destination},
	}}
	conn := &dhtPacketConn{
		PacketConn: underlying, router: r, ctx: context.Background(),
		metadata: adapter.InboundContext{Inbound: "user-in", Network: N.NetworkUDP, Destination: destination, Protocol: "dns", User: "authenticated"},
	}
	if result := r.PreMatch(conn.metadata, []byte("ordinary UDP")); result.Action != adapter.PreMatchContinue {
		t.Fatal("L3 forwarding bypassed the subsequent-packet guard")
	}
	if N.CalculateFrontHeadroom(conn) != 10 || N.CalculateRearHeadroom(conn) != 4 {
		t.Fatal("transport headroom was hidden")
	}
	for i, want := range []string{"normal", "after", testDHTPing, testDHTPing + "tail"} {
		buffer := N.NewReadWaitOptions(nil, conn).NewPacketBuffer()
		target, err := conn.ReadPacket(buffer)
		if err != nil || string(buffer.Bytes()) != want {
			t.Fatalf("packet %d changed: %q %v", i, buffer.Bytes(), err)
		}
		if buffer.Start() != 10 || buffer.FreeLen() < 4 {
			t.Fatal("discarding a packet lost protocol headroom")
		}
		if i == 2 && target != other {
			t.Fatal("per-packet target changed")
		}
		buffer.Release()
	}
	if len(tracker.metadata) != 1 || tracker.metadata[0].User != "authenticated" || tracker.metadata[0].Destination != destination {
		t.Fatalf("incorrect block log or identity: %+v", tracker.metadata)
	}
	buffer := buf.NewPacket()
	defer buffer.Release()
	if _, err := conn.ReadPacket(buffer); err != io.EOF {
		t.Fatalf("reader error lost: %v", err)
	}
	if conn.metadata.Protocol != "dns" {
		t.Fatal("session protocol was changed")
	}
	// Removing the rule changes the gate and matcher together, including sessions
	// that already carry the guard; malformed reloads cannot broaden it.
	if err := r.UpdateRules(nil, nil); err != nil {
		t.Fatal(err)
	}
	underlying.packets = []dhtTestPacket{{payload: testDHTPing, destination: destination}}
	if _, err := conn.ReadPacket(buffer); err != nil || string(buffer.Bytes()) != testDHTPing {
		t.Fatalf("removed rule still blocks existing session: %v", err)
	}
}
