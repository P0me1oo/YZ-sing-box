package trojan

import (
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
)

type userIdentity struct {
	index int
	name  string
}

func (h *Inbound) UpdateUsers(users []option.TrojanUser) error {
	return h.service.UpdateUsers(common.MapIndexed(users, func(index int, user option.TrojanUser) userIdentity {
		return userIdentity{index: index, name: user.Name}
	}), common.Map(users, func(user option.TrojanUser) string {
		return user.Password
	}))
}
