package socks

import (
	"github.com/sagernet/sing/common/auth"
)

func (h *Inbound) UpdateUsers(users []auth.User) error {
	authenticator := auth.NewAuthenticator(users)
	if authenticator == nil {
		// 删除最后一个用户后拒绝认证，不能退回匿名代理。
		authenticator = new(auth.Authenticator)
	}
	h.authenticator.Store(authenticator)
	return nil
}
