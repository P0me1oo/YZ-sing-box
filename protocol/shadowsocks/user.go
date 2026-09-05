package shadowsocks

import (
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
)

type userIdentity struct {
	index int
	name  string
}

func (h *MultiInbound) UpdateUsersByOptions(users []option.ShadowsocksUser) error {
	return h.service.UpdateUsersWithPasswords(common.MapIndexed(users, func(index int, user option.ShadowsocksUser) userIdentity {
		return userIdentity{index: index, name: user.Name}
	}), common.Map(users, func(user option.ShadowsocksUser) string {
		return user.Password
	}))
}
