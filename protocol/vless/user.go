package vless

import (
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
)

type userIdentity struct {
	index int
	name  string
}

func (h *Inbound) UpdateUsers(users []option.VLESSUser) error {
	h.service.UpdateUsers(common.MapIndexed(users, func(index int, user option.VLESSUser) userIdentity {
		return userIdentity{index: index, name: user.Name}
	}), common.Map(users, func(user option.VLESSUser) string {
		return user.UUID
	}), common.Map(users, func(user option.VLESSUser) string {
		return user.Flow
	}))
	return nil
}
