package route

import (
	"context"
	"io"
	"testing"

	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

func dhtRouterForTest(tb testing.TB, preceding int) *Router {
	tb.Helper()
	r := &Router{ctx: service.ContextWithDefaultRegistry(context.Background()), logger: log.NewNOPFactory().NewLogger("test"),
		outbound: dhtTestOutbounds{}, dns: dhtTestDNS{}, dnsTransport: dhtTestTransports{}}
	rules := make([]option.Rule, 0, preceding+1)
	for range preceding {
		rules = append(rules, option.Rule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{
			RawDefaultRule: option.RawDefaultRule{Protocol: []string{"http"}},
			RuleAction:     option.RuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.RouteActionOptions{Outbound: "direct"}},
		}})
	}
	rules = append(rules, option.Rule{Type: C.RuleTypeDefault, DefaultOptions: option.DefaultRule{
		RawDefaultRule: option.RawDefaultRule{Protocol: []string{C.ProtocolBitTorrent}, Port: []uint16{6881}},
		RuleAction:     option.RuleAction{Action: C.RuleActionTypeRoute, RouteOptions: option.RouteActionOptions{Outbound: "block"}},
	}})
	if err := r.Initialize(rules, nil); err != nil {
		tb.Fatal(err)
	}
	return r
}

type dhtBatchFixture struct {
	dhtTestConn
	batches     [][]dhtTestPacket
	finalError  error
	initialized bool
}

func (f *dhtBatchFixture) InitializeReadWaiter(N.ReadWaitOptions) bool {
	f.initialized = true
	return false
}
func (f *dhtBatchFixture) WaitReadPackets() ([]*buf.Buffer, []M.Socksaddr, error) {
	if len(f.batches) == 0 {
		return nil, nil, io.EOF
	}
	batch := f.batches[0]
	f.batches = f.batches[1:]
	buffers := make([]*buf.Buffer, 0, len(batch))
	destinations := make([]M.Socksaddr, 0, len(batch))
	for _, packet := range batch {
		buffers = append(buffers, buf.As([]byte(packet.payload)))
		destinations = append(destinations, packet.destination)
	}
	if len(f.batches) == 0 {
		return buffers, destinations, f.finalError
	}
	return buffers, destinations, nil
}
func (f *dhtBatchFixture) WaitReadConnectedPackets() ([]*buf.Buffer, M.Socksaddr, error) {
	buffers, destinations, err := f.WaitReadPackets()
	if len(destinations) == 0 {
		return buffers, M.Socksaddr{}, err
	}
	return buffers, destinations[0], err
}
func (f *dhtBatchFixture) WaitReadPacket() (*buf.Buffer, M.Socksaddr, error) {
	buffers, destinations, err := f.WaitReadPackets()
	if len(buffers) == 0 {
		return nil, M.Socksaddr{}, err
	}
	return buffers[0], destinations[0], err
}

func TestDHTOptimizedReaders(t *testing.T) {
	destination := M.ParseSocksaddr("192.0.2.1:6881")
	for _, mode := range []string{"single", "batch", "connected_batch"} {
		t.Run(mode, func(t *testing.T) {
			underlying := &dhtBatchFixture{batches: [][]dhtTestPacket{
				{{payload: testDHTPing, destination: destination}},
				{{payload: "normal", destination: destination}},
			}}
			guard := &dhtPacketConn{PacketConn: underlying, router: dhtRouterForTest(t, 0), ctx: context.Background(),
				metadata: adapter.InboundContext{Network: N.NetworkUDP, Destination: destination}}
			switch mode {
			case "single":
				reader, ok := bufio.CreatePacketReadWaiter(guard)
				if !ok {
					t.Fatal("single reader lost")
				}
				reader.InitializeReadWaiter(N.ReadWaitOptions{})
				buffer, got, err := reader.WaitReadPacket()
				if err != nil || buffer == nil || string(buffer.Bytes()) != "normal" || got != destination {
					t.Fatalf("incorrect filtered packet: %v", err)
				}
				buffer.Release()
			case "batch":
				reader, ok := bufio.CreatePacketBatchReadWaiter(guard)
				if !ok {
					t.Fatal("batch reader lost")
				}
				reader.InitializeReadWaiter(N.ReadWaitOptions{})
				buffers, targets, err := reader.WaitReadPackets()
				defer buf.ReleaseMulti(buffers)
				if err != nil || len(buffers) != 1 || string(buffers[0].Bytes()) != "normal" || targets[0] != destination {
					t.Fatalf("incorrect batch: %v", err)
				}
			case "connected_batch":
				reader, ok := bufio.CreateConnectedPacketBatchReadWaiter(guard)
				if !ok {
					t.Fatal("connected reader lost")
				}
				reader.InitializeReadWaiter(N.ReadWaitOptions{})
				buffers, target, err := reader.WaitReadConnectedPackets()
				defer buf.ReleaseMulti(buffers)
				if err != nil || len(buffers) != 1 || string(buffers[0].Bytes()) != "normal" || target != destination {
					t.Fatalf("incorrect connected batch: %v", err)
				}
			}
			if !underlying.initialized {
				t.Fatal("read options not forwarded")
			}
			if N.UnwrapPacketReader(guard) != guard {
				t.Fatal("reader may bypass filtering")
			}
			if N.UnwrapPacketWriter(guard) != underlying {
				t.Fatal("writer cannot use original fast path")
			}
		})
	}
}

func TestDHTBatchPreservesTargetsAndPartialError(t *testing.T) {
	destination := M.ParseSocksaddr("192.0.2.1:6881")
	other := M.ParseSocksaddr("192.0.2.1:6882")
	underlying := &dhtBatchFixture{batches: [][]dhtTestPacket{{
		{payload: testDHTPing, destination: destination}, {payload: "normal", destination: destination},
		{payload: testDHTPing, destination: other}, {payload: testDHTPing, destination: destination},
	}}, finalError: io.EOF}
	guard := &dhtPacketConn{PacketConn: underlying, router: dhtRouterForTest(t, 0), ctx: context.Background(),
		metadata: adapter.InboundContext{Network: N.NetworkUDP, Destination: destination}}
	reader, ok := guard.CreatePacketBatchReadWaiter()
	if !ok {
		t.Fatal("missing batch reader")
	}
	buffers, targets, err := reader.WaitReadPackets()
	defer buf.ReleaseMulti(buffers)
	if err != io.EOF || len(buffers) != 2 || string(buffers[0].Bytes()) != "normal" || string(buffers[1].Bytes()) != testDHTPing || targets[1] != other {
		t.Fatalf("mixed batch or error changed: %v %v", targets, err)
	}
}

func TestDHTCachedPacketsCannotBypassGuard(t *testing.T) {
	destination := M.ParseSocksaddr("192.0.2.1:6881")
	base := &dhtBatchFixture{}
	cached := bufio.NewCachedPacketConn(base, buf.As([]byte("normal")), destination)
	cached = bufio.NewCachedPacketConn(cached, buf.As([]byte(testDHTPing)), destination)
	guard := &dhtPacketConn{PacketConn: cached, router: dhtRouterForTest(t, 0), ctx: context.Background(),
		metadata: adapter.InboundContext{Network: N.NetworkUDP, Destination: destination}}
	if _, ok := guard.CreatePacketBatchReadWaiter(); ok {
		t.Fatal("unconsumed cache bypassed")
	}
	packet := guard.ReadCachedPacket()
	if packet == nil || string(packet.Buffer.Bytes()) != "normal" {
		t.Fatal("cached DHT passed, or normal packet lost")
	}
	packet.Buffer.Release()
	N.PutPacketBuffer(packet)
	if guard.ReadCachedPacket() != nil {
		t.Fatal("cache consumed twice")
	}
	if _, ok := guard.CreatePacketBatchReadWaiter(); !ok {
		t.Fatal("batch path not restored after cache")
	}
}

func TestDHTGateSkipsTCPOnlyRules(t *testing.T) {
	r := dhtRouterForTest(t, 0)
	rules := append([]option.Rule{}, r.ruleOptions...)
	rules[0].DefaultOptions.Network = []string{N.NetworkTCP}
	if err := r.UpdateRules(rules, nil); err != nil {
		t.Fatal(err)
	}
	if r.hasDHTBlockRule() {
		t.Fatal("TCP-only rule changed the UDP copy path")
	}
	rules = append([]option.Rule{}, rules...)
	rules[0].DefaultOptions.Network = []string{N.NetworkUDP}
	if err := r.UpdateRules(rules, nil); err != nil {
		t.Fatal(err)
	}
	if !r.hasDHTBlockRule() {
		t.Fatal("UDP gate not restored on reload")
	}
}

func TestDHTAllBlockedBatchPreservesError(t *testing.T) {
	destination := M.ParseSocksaddr("192.0.2.1:6881")
	source := &dhtBatchFixture{batches: [][]dhtTestPacket{{{payload: testDHTPing, destination: destination}}}, finalError: io.EOF}
	guard := &dhtPacketConn{PacketConn: source, router: dhtRouterForTest(t, 0), ctx: context.Background(), metadata: adapter.InboundContext{Network: N.NetworkUDP, Destination: destination}}
	reader, _ := guard.CreatePacketBatchReadWaiter()
	buffers, targets, err := reader.WaitReadPackets()
	if len(buffers) != 0 || len(targets) != 0 || err != io.EOF {
		t.Fatalf("error lost after dropping final batch: %v", err)
	}
}
