package anytls

import (
	anytls "github.com/anytls/sing-anytls"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
)

func (h *Inbound) UpdateUsers(users []option.AnyTLSUser) error {
	service, err := anytls.NewService(anytls.ServiceConfig{
		Users:         common.Map(users, func(it option.AnyTLSUser) anytls.User { return anytls.User(it) }),
		PaddingScheme: h.paddingScheme,
		Handler:       (*inboundHandler)(h),
		Logger:        h.logger,
	})
	if err != nil {
		return err
	}
	// 既有会话保留原认证快照；新连接只读取一次完整的新服务。
	h.service.Store(service)
	return nil
}
