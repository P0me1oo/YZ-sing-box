package route

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/option"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

func TestUpdateRulesDoesNotWaitForDetourConnection(t *testing.T) {
	for _, network := range []string{"tcp", "udp"} {
		t.Run(network, func(t *testing.T) {
			inbound := &waitingDetourInbound{entered: make(chan struct{}), release: make(chan struct{})}
			router := &Router{ctx: service.ContextWithDefaultRegistry(context.Background()), inbound: &detourInboundManager{target: inbound}}
			routed := make(chan error, 1)
			go func() {
				metadata := adapter.InboundContext{Inbound: "source", InboundDetour: "target"}
				if network == "tcp" {
					routed <- router.routeConnection(context.Background(), nil, metadata, nil)
				} else {
					routed <- router.routePacketConnection(context.Background(), nil, metadata, nil)
				}
			}()
			<-inbound.entered
			updated := make(chan error, 1)
			go func() { updated <- router.UpdateRules([]option.Rule{}, nil) }()
			select {
			case err := <-updated:
				if err != nil {
					t.Error(err)
				}
				close(inbound.release)
			case <-time.After(2 * time.Second):
				t.Error("入站转交仍在等待连接时，路由热更新被阻塞")
				close(inbound.release)
				<-updated
			}
			if err := <-routed; err != nil {
				t.Fatal(err)
			}
		})
	}
}

type detourInboundManager struct {
	adapter.InboundManager
	target adapter.Inbound
}

func (m *detourInboundManager) Get(string) (adapter.Inbound, bool) { return m.target, true }

type waitingDetourInbound struct {
	adapter.Inbound
	entered chan struct{}
	release chan struct{}
}

func (i *waitingDetourInbound) NewConnection(context.Context, net.Conn, adapter.InboundContext, N.CloseHandlerFunc) {
	close(i.entered)
	<-i.release
}

func (i *waitingDetourInbound) NewPacketConnection(context.Context, N.PacketConn, adapter.InboundContext, N.CloseHandlerFunc) {
	close(i.entered)
	<-i.release
}
