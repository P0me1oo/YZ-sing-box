package mieru

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/enfein/mieru/v3/apis/common"
	"github.com/enfein/mieru/v3/apis/model"
	api "github.com/enfein/mieru/v3/apis/server"
	"github.com/enfein/mieru/v3/apis/trafficpattern"
	"github.com/enfein/mieru/v3/pkg/appctl/appctlcommon"
	pb "github.com/enfein/mieru/v3/pkg/appctl/appctlpb"
	transport "github.com/enfein/mieru/v3/pkg/common"
	"github.com/enfein/mieru/v3/pkg/sockopts"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing-box/third_party/mieru/pkg/protocol"
)

// 使用 Mieru 已有的 Mux 用户更新入口，保留监听和已经建立的会话。
type server struct {
	mux            *protocol.Mux
	running        atomic.Bool
	listenerAccess sync.Mutex
	listeners      []io.Closer
	listenConfig   net.ListenConfig
}

func newServer(config *api.ServerConfig, options option.MieruInboundOptions) (*server, error) {
	if err := api.NewServer().Store(config); err != nil {
		return nil, fmt.Errorf("validate mieru server: %w", err)
	}
	pattern, err := trafficpattern.NewConfig(config.Config.TrafficPattern)
	if err != nil {
		return nil, err
	}
	mux := protocol.NewMux(false)
	mux.SetTrafficPattern(pattern)
	mux.SetServerUsers(appctlcommon.UserListToMap(config.Config.GetUsers()))
	listenIP := net.IPv6zero
	if options.Listen != nil {
		listenIP = net.IP(netip.Addr(*options.Listen).AsSlice())
	}
	port := int(options.ListenPort)
	var endpoint protocol.UnderlayProperties
	if options.Transport == "UDP" {
		endpoint = protocol.NewUnderlayProperties(transport.DefaultMTU, transport.PacketTransport, &net.UDPAddr{IP: listenIP, Port: port}, nil)
	} else {
		endpoint = protocol.NewUnderlayProperties(transport.DefaultMTU, transport.StreamTransport, &net.TCPAddr{IP: listenIP, Port: port}, nil)
	}
	mux.SetEndpoints([]protocol.UnderlayProperties{endpoint})
	s := &server{mux: mux, listenConfig: net.ListenConfig{Control: sockopts.DefaultListenerControl()}}
	mux.SetStreamListenerFactory(s)
	mux.SetPacketListenerFactory(s)
	return s, nil
}

func (s *server) Start() error {
	if err := s.mux.Start(); err != nil {
		_ = s.Stop()
		return err
	}
	s.running.Store(true)
	return nil
}

func (s *server) Stop() error {
	s.running.Store(false)
	err := s.mux.Close()
	// Mux 取消接收协程后才异步释放监听；Stop 返回前必须真正归还端口。
	s.listenerAccess.Lock()
	defer s.listenerAccess.Unlock()
	for _, listener := range s.listeners {
		if closeErr := listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
	}
	s.listeners = nil
	return err
}

func (s *server) Listen(ctx context.Context, network, address string) (net.Listener, error) {
	listener, err := s.listenConfig.Listen(ctx, network, address)
	if err == nil {
		s.listenerAccess.Lock()
		s.listeners = append(s.listeners, listener)
		s.listenerAccess.Unlock()
	}
	return listener, err
}

func (s *server) ListenPacket(ctx context.Context, network, address string) (net.PacketConn, error) {
	listener, err := s.listenConfig.ListenPacket(ctx, network, address)
	if err == nil {
		s.listenerAccess.Lock()
		s.listeners = append(s.listeners, listener)
		s.listenerAccess.Unlock()
	}
	return listener, err
}

func (s *server) IsRunning() bool { return s.running.Load() }

func (s *server) UpdateUsers(users []*pb.User) {
	s.mux.SetServerUsers(appctlcommon.UserListToMap(users))
}

func (s *server) Accept() (net.Conn, *model.Request, error) {
	conn, err := s.mux.Accept()
	if err != nil {
		return nil, nil, err
	}
	if _, ok := conn.(common.UserContext); !ok {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("mieru connection has no user context")
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	request := new(model.Request)
	if err := request.ReadFromSocks5(conn); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	_ = conn.SetReadDeadline(time.Time{})
	return conn, request, nil
}
