package route_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	C "github.com/sagernet/sing-box/constant"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/service"
)

func newReloadRouter(t *testing.T) (context.Context, adapter.Router, adapter.RuleUpdater) {
	t.Helper()
	ctx := include.Context(context.Background())
	options, err := json.UnmarshalExtendedContext[option.Options](ctx, []byte(`{
		"log": {"disabled": true},
		"outbounds": [{"type": "direct", "tag": "direct"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	instance, err := box.New(box.Options{Context: ctx, Options: options})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })
	if err := instance.Start(); err != nil {
		t.Fatal(err)
	}
	router := service.FromContext[adapter.Router](ctx)
	updater, ok := router.(adapter.RuleUpdater)
	if !ok {
		t.Fatal("缺少嵌入式路由更新接口")
	}
	return ctx, router, updater
}

func reloadOptions(t *testing.T, ctx context.Context, data string) option.RouteOptions {
	t.Helper()
	options, err := json.UnmarshalExtendedContext[option.RouteOptions](ctx, []byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return options
}

func TestUpdateRulesCommitsRuleSetsAndPreservesStateOnFailure(t *testing.T) {
	ctx, router, updater := newReloadRouter(t)
	initial := reloadOptions(t, ctx, `{
		"rules": [{"rule_set": "primary", "action": "reject"}],
		"rule_set": [{"type": "inline", "tag": "primary", "rules": [{"domain": "first.test"}]}]
	}`)
	if err := updater.UpdateRules(initial.Rules, initial.RuleSet); err != nil {
		t.Fatal(err)
	}
	before := router.Rules()[0]
	if !before.Match(&adapter.InboundContext{Domain: "first.test"}) {
		t.Fatal("新规则没有绑定到本次创建的规则集")
	}
	if err := updater.UpdateRules(initial.Rules, initial.RuleSet); err != nil {
		t.Fatal(err)
	}
	if router.Rules()[0] != before {
		t.Fatal("相同配置不应重复构建和关闭规则")
	}

	invalid := reloadOptions(t, ctx, `{"rules": [{"rule_set": "missing", "action": "reject"}]}`)
	for range 2 {
		if err := updater.UpdateRules(invalid.Rules, invalid.RuleSet); err == nil {
			t.Fatal("缺失规则集应导致更新失败")
		}
		if router.Rules()[0] != before || !before.Match(&adapter.InboundContext{Domain: "first.test"}) {
			t.Fatal("失败更新破坏了旧规则")
		}
	}
	duplicate := reloadOptions(t, ctx, `{
		"rule_set": [
			{"type":"inline","tag":"duplicate","rules":[]},
			{"type":"inline","tag":"duplicate","rules":[]}
		]
	}`)
	if err := updater.UpdateRules(duplicate.Rules, duplicate.RuleSet); err == nil {
		t.Fatal("重复规则集标签应被拒绝")
	}

	replacement := reloadOptions(t, ctx, `{
		"rules": [{"rule_set": "primary", "action": "reject"}],
		"rule_set": [{"type": "inline", "tag": "primary", "rules": [{"domain": "second.test"}]}]
	}`)
	if err := updater.UpdateRules(replacement.Rules, replacement.RuleSet); err != nil {
		t.Fatal(err)
	}
	after := router.Rules()[0]
	if after.Match(&adapter.InboundContext{Domain: "first.test"}) || !after.Match(&adapter.InboundContext{Domain: "second.test"}) {
		t.Fatal("同名规则集更新后仍在使用旧内容")
	}
}

func TestUpdateRulesSupportsLocalRuleSetMultipleTags(t *testing.T) {
	ctx, router, updater := newReloadRouter(t)
	directory := t.TempDir()
	for _, tag := range []string{"primary", "alias"} {
		if err := os.WriteFile(filepath.Join(directory, tag+".json"), []byte(`{"version":3,"rules":[{"domain":"local.test"}]}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := json.Marshal(map[string]any{
		"rules": []any{map[string]any{"rule_set": []string{"primary", "alias"}, "action": "reject"}},
		"rule_set": []any{map[string]any{
			"type": "local", "tag": []string{"primary", "alias"}, "format": "source",
			"path": filepath.Join(directory, C.RuleSetTagPlaceholder+".json"),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	options := reloadOptions(t, ctx, string(data))
	if err := updater.UpdateRules(options.Rules, options.RuleSet); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"primary", "alias"} {
		if _, ok := router.RuleSet(tag); !ok {
			t.Fatalf("未注册本地规则集标签: %s", tag)
		}
	}
	if !router.Rules()[0].Match(&adapter.InboundContext{Domain: "local.test"}) {
		t.Fatal("多标签本地规则集未生效")
	}
}

func TestUpdateRulesConcurrentPreMatch(t *testing.T) {
	ctx, router, updater := newReloadRouter(t)
	first := reloadOptions(t, ctx, `{"rules":[{"domain":"first.test","action":"reject"}]}`)
	second := reloadOptions(t, ctx, `{"rules":[{"domain":"second.test","action":"reject"}]}`)
	var workers sync.WaitGroup
	done := make(chan struct{})
	for range 4 {
		workers.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
					router.PreMatch(adapter.InboundContext{Network: "tcp", Destination: M.ParseSocksaddr("first.test:443")}, nil)
				}
			}
		})
	}
	defer func() { close(done); workers.Wait() }()
	for range 100 {
		for _, options := range []option.RouteOptions{first, second} {
			if err := updater.UpdateRules(options.Rules, options.RuleSet); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestUpdateRulesDoesNotWaitForIdleDNSConnection(t *testing.T) {
	ctx, router, updater := newReloadRouter(t)
	initial := reloadOptions(t, ctx, `{"rules":[{"action":"hijack-dns"}]}`)
	if err := updater.UpdateRules(initial.Rules, initial.RuleSet); err != nil {
		t.Fatal(err)
	}
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	reading := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		router.RouteConnectionEx(ctx, &dnsReadSignalConn{Conn: server, reading: reading}, adapter.InboundContext{
			Destination: M.ParseSocksaddr("192.0.2.1:53"),
		}, nil)
	}()
	select {
	case <-reading:
	case <-time.After(2 * time.Second):
		t.Fatal("DNS 连接没有进入读取状态")
	}
	updated := make(chan error, 1)
	go func() { updated <- updater.UpdateRules(nil, nil) }()
	select {
	case err := <-updated:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(2 * time.Second):
		t.Error("路由热更新被已有的空闲 DNS 连接阻塞")
		_ = client.Close()
		<-updated
	}
	_ = client.Close()
	<-done
}

type dnsReadSignalConn struct {
	net.Conn
	reading chan struct{}
	once    sync.Once
}

func (c *dnsReadSignalConn) Read(buffer []byte) (int, error) {
	c.once.Do(func() { close(c.reading) })
	return c.Conn.Read(buffer)
}
