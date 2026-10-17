package listener

import (
	"context"
	"net"
	"net/netip"
	"sync/atomic"
	"time"

	"github.com/pires/go-proxyproto"
)

// ProxyProtocolTrust holds the trusted TCP forwarders for one embedded instance.
// An empty list never enables PROXY header parsing.
type ProxyProtocolTrust struct {
	trusted atomic.Pointer[[]netip.Prefix]
}

type proxyProtocolTrustContextKey struct{}

func ContextWithProxyProtocolTrust(ctx context.Context, trust *ProxyProtocolTrust) context.Context {
	return context.WithValue(ctx, proxyProtocolTrustContextKey{}, trust)
}

// Set replaces the policy for subsequent connections without replacing listeners.
func (t *ProxyProtocolTrust) Set(prefixes []netip.Prefix) {
	list := make([]netip.Prefix, 0, len(prefixes))
	for _, prefix := range prefixes {
		if !prefix.IsValid() {
			continue
		}
		addr, bits := prefix.Addr(), prefix.Bits()
		if addr.Is4In6() && bits >= 96 {
			addr, bits = addr.Unmap(), bits-96
		}
		list = append(list, netip.PrefixFrom(addr, bits).Masked())
	}
	if len(list) == 0 {
		t.trusted.Store(nil)
		return
	}
	t.trusted.Store(&list)
}

func (t *ProxyProtocolTrust) policy(upstream net.Addr) proxyproto.Policy {
	list := t.trusted.Load()
	if list == nil {
		return proxyproto.SKIP
	}
	address, ok := upstream.(*net.TCPAddr)
	if !ok {
		return proxyproto.SKIP
	}
	addr := address.AddrPort().Addr().Unmap()
	for _, prefix := range *list {
		if prefix.Contains(addr) {
			return proxyproto.USE
		}
	}
	return proxyproto.SKIP
}

func wrapProxyProtocolListener(ctx context.Context, listener net.Listener) net.Listener {
	trust, _ := ctx.Value(proxyProtocolTrustContextKey{}).(*ProxyProtocolTrust)
	if trust == nil {
		return listener
	}
	return &proxyproto.Listener{
		Listener:          listener,
		ReadHeaderTimeout: 5 * time.Second,
		ConnPolicy: func(options proxyproto.ConnPolicyOptions) (proxyproto.Policy, error) {
			return trust.policy(options.Upstream), nil
		},
	}
}
