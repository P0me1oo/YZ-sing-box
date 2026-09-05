package route

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
)

func TestNetworkStartWaitsForInitialInterfaceUpdates(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		first := make(chan struct{})
		manager := &NetworkManager{ctx: context.Background(), interfaceUpdateDone: first}
		result := make(chan error, 1)
		go func() { result <- manager.waitInitialInterfaceUpdate() }()
		synctest.Wait()
		if manager.started.Load() {
			t.Fatal("首次接口更新未完成时，不能标记核心已启动")
		}

		// 启动期间出现后续通知时，要等待最新通知完成。
		second := make(chan struct{})
		manager.interfaceUpdateAccess.Lock()
		manager.interfaceUpdateDone = second
		manager.interfaceUpdateAccess.Unlock()
		close(first)
		synctest.Wait()
		if manager.started.Load() {
			t.Fatal("最新接口更新未完成时，不能标记核心已启动")
		}
		close(second)
		synctest.Wait()
		if err := <-result; err != nil || !manager.started.Load() {
			t.Fatalf("接口初始化完成后启动失败: %v", err)
		}
	})
}

func TestNetworkStartCanCancelInitialInterfaceUpdate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		manager := &NetworkManager{ctx: ctx, interfaceUpdateDone: make(chan struct{})}
		result := make(chan error, 1)
		go func() { result <- manager.waitInitialInterfaceUpdate() }()
		synctest.Wait()
		cancel()
		synctest.Wait()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("取消启动应返回上下文错误: %v", err)
		}
		if manager.started.Load() {
			t.Fatal("取消的初始化不能标记为启动成功")
		}
	})
}
