package route

import (
	"context"
	"fmt"
	"github.com/sagernet/sing-box/adapter"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"testing"
)

func BenchmarkDHTGuard(b *testing.B) {
	for _, preceding := range []int{0, 100, 1000} {
		b.Run(fmt.Sprintf("rules_%d", preceding+1), func(b *testing.B) {
			destination := M.ParseSocksaddr("192.0.2.1:6881")
			guard := &dhtPacketConn{router: dhtRouterForTest(b, preceding), ctx: context.Background(),
				metadata: adapter.InboundContext{Network: N.NetworkUDP, Destination: destination}}
			for _, tc := range []struct{ name, payload string }{
				{name: "ordinary", payload: "normal UDP payload"}, {name: "d_prefix", payload: "data from an ordinary application"}, {name: "ping", payload: testDHTPing},
			} {
				b.Run(tc.name, func(b *testing.B) {
					packet := []byte(tc.payload)
					guard.block(packet, destination)
					b.ReportAllocs()
					for b.Loop() {
						guard.block(packet, destination)
					}
				})
			}
		})
	}
}
