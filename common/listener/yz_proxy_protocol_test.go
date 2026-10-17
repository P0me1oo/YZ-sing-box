package listener

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/log"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
	N "github.com/sagernet/sing/common/network"

	"github.com/pires/go-proxyproto"
)

type proxyObservation struct {
	source string
	data   string
	err    error
}

type proxyTestHandler struct {
	observed chan proxyObservation
}

func (h *proxyTestHandler) NewConnection(ctx context.Context, conn net.Conn, metadata adapter.InboundContext, _ N.CloseHandlerFunc) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	data, err := io.ReadAll(conn)
	select {
	case h.observed <- proxyObservation{metadata.Source.String(), string(data), err}:
	case <-ctx.Done():
	}
	if err == nil {
		_, _ = conn.Write(data)
	}
}

func proxyTestListener(t *testing.T, trust *ProxyProtocolTrust, listenAddress ...string) (*Listener, <-chan proxyObservation) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ctx = ContextWithProxyProtocolTrust(ctx, trust)
	address := badoption.Addr(netip.MustParseAddr("127.0.0.1"))
	if len(listenAddress) > 0 {
		address = badoption.Addr(netip.MustParseAddr(listenAddress[0]))
	}
	handler := &proxyTestHandler{observed: make(chan proxyObservation, 64)}
	l := New(Options{
		Context: ctx, Logger: log.NewNOPFactory().Logger(), Network: []string{N.NetworkTCP},
		Listen: option.ListenOptions{Listen: &address}, ConnectionHandler: handler,
	})
	if err := l.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = l.Close() })
	return l, handler.observed
}

func proxyTestExchange(t *testing.T, l *Listener, observed <-chan proxyObservation, data string) proxyObservation {
	t.Helper()
	conn, err := net.Dial("tcp", l.TCPListener().Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.WriteString(conn, data); err != nil {
		t.Fatal(err)
	}
	if err := conn.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(conn)
	select {
	case result := <-observed:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("connection handler did not finish")
		return proxyObservation{}
	}
}

func proxyTestHeader(t *testing.T, version byte, source string) string {
	t.Helper()
	address := netip.MustParseAddr(source)
	transport := proxyproto.TCPv4
	if address.Is6() {
		transport = proxyproto.TCPv6
	}
	header := &proxyproto.Header{
		Version: version, Command: proxyproto.PROXY, TransportProtocol: transport,
		SourceAddr:      &net.TCPAddr{IP: net.IP(address.AsSlice()), Port: 32123},
		DestinationAddr: &net.TCPAddr{IP: net.IP(address.AsSlice()), Port: 443},
	}
	data, err := header.Format()
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestProxyProtocolTrustedHeader(t *testing.T) {
	for _, version := range []byte{1, 2} {
		for _, source := range []string{"203.0.113.9", "2001:db8::9"} {
			t.Run(fmt.Sprintf("v%d/%s", version, source), func(t *testing.T) {
				trust := &ProxyProtocolTrust{}
				trust.Set([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
				l, observed := proxyTestListener(t, trust)
				got := proxyTestExchange(t, l, observed, proxyTestHeader(t, version, source)+"payload")
				want := net.JoinHostPort(source, "32123")
				if got.err != nil || got.source != want || got.data != "payload" {
					t.Fatalf("header was not stripped before dispatch: got=%+v, source=%s", got, want)
				}
			})
		}
	}
}

func TestProxyProtocolTrustReloadAndDirectConnections(t *testing.T) {
	trust := &ProxyProtocolTrust{}
	l, observed := proxyTestListener(t, trust)
	header := proxyTestHeader(t, 2, "203.0.113.9")
	check := func(trusted bool) {
		t.Helper()
		got := proxyTestExchange(t, l, observed, header+"payload")
		if trusted {
			if got.err != nil || got.source != "203.0.113.9:32123" || got.data != "payload" {
				t.Fatalf("trusted source: %+v", got)
			}
		} else if got.err != nil || got.data != header+"payload" || netip.MustParseAddrPort(got.source).Addr().String() != "127.0.0.1" {
			t.Fatalf("untrusted header changed source or data: %+v", got)
		}
	}
	check(false)
	for range 2 {
		prefixes := []netip.Prefix{netip.MustParsePrefix("::ffff:127.0.0.1/128"), {}}
		trust.Set(prefixes)
		prefixes[0] = netip.MustParsePrefix("192.0.2.0/24")
		check(true)
		got := proxyTestExchange(t, l, observed, "ordinary direct connection")
		if got.err != nil || got.data != "ordinary direct connection" || netip.MustParseAddrPort(got.source).Addr().String() != "127.0.0.1" {
			t.Fatalf("trusted peer without a header stopped working: %+v", got)
		}
		trust.Set(nil)
		check(false)
	}
}

func TestProxyProtocolInstancesAreIsolated(t *testing.T) {
	first, second := &ProxyProtocolTrust{}, &ProxyProtocolTrust{}
	first.Set([]netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")})
	a, aObserved := proxyTestListener(t, first)
	b, bObserved := proxyTestListener(t, second)
	header := proxyTestHeader(t, 1, "203.0.113.9")
	if got := proxyTestExchange(t, a, aObserved, header+"payload"); got.err != nil || got.data != "payload" {
		t.Fatalf("first instance did not use its trust list: %+v", got)
	}
	if got := proxyTestExchange(t, b, bObserved, header+"payload"); got.err != nil || got.data != header+"payload" {
		t.Fatalf("second instance inherited the first trust list: %+v", got)
	}
}

func TestProxyProtocolSlowPeerDoesNotBlockAccept(t *testing.T) {
	trust := &ProxyProtocolTrust{}
	trust.Set([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	l, observed := proxyTestListener(t, trust)
	slow, err := net.Dial("tcp4", l.TCPListener().Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer slow.Close()
	got := proxyTestExchange(t, l, observed, proxyTestHeader(t, 2, "203.0.113.9")+"fast")
	if got.err != nil || got.data != "fast" || got.source != "203.0.113.9:32123" {
		t.Fatalf("idle peer blocked another connection: %+v", got)
	}
}

func TestProxyProtocolMalformedHeaderDoesNotStopListener(t *testing.T) {
	trust := &ProxyProtocolTrust{}
	trust.Set([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	l, observed := proxyTestListener(t, trust)
	got := proxyTestExchange(t, l, observed, "PROXY TCP4 invalid addresses\r\npayload")
	if got.err == nil {
		t.Fatalf("malformed header reached the protocol as valid data: %+v", got)
	}
	got = proxyTestExchange(t, l, observed, proxyTestHeader(t, 1, "203.0.113.9")+"recovered")
	if got.err != nil || got.data != "recovered" {
		t.Fatalf("listener did not recover after malformed header: %+v", got)
	}
}

func TestProxyProtocolIPv6Forwarder(t *testing.T) {
	trust := &ProxyProtocolTrust{}
	trust.Set([]netip.Prefix{netip.MustParsePrefix("::1/128")})
	l, observed := proxyTestListener(t, trust, "::1")
	got := proxyTestExchange(t, l, observed, proxyTestHeader(t, 2, "2001:db8::9")+"ipv6")
	if got.err != nil || got.source != "[2001:db8::9]:32123" || got.data != "ipv6" {
		t.Fatalf("IPv6 forwarder was not recognized: %+v", got)
	}
}

func TestProxyProtocolConcurrentTrustUpdates(t *testing.T) {
	trust := &ProxyProtocolTrust{}
	l, observed := proxyTestListener(t, trust)
	header := proxyTestHeader(t, 2, "203.0.113.9")
	stop, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		for {
			select {
			case <-stop:
				return
			default:
				trust.Set([]netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
				trust.Set(nil)
			}
		}
	}()
	defer func() { close(stop); <-stopped }()
	var workers sync.WaitGroup
	for range 20 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			got := proxyTestExchange(t, l, observed, header+"payload")
			trusted := got.data == "payload" && got.source == "203.0.113.9:32123"
			untrusted := got.data == header+"payload" && netip.MustParseAddrPort(got.source).Addr().String() == "127.0.0.1"
			if got.err != nil || (!trusted && !untrusted) {
				t.Errorf("concurrent update mixed source and stream state: %+v", got)
			}
		}()
	}
	workers.Wait()
}
