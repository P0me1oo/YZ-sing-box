package vmess

import (
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common"
)

// 认证结果携带身份值，用户列表更新后既有多路复用连接仍归属原用户。
type userIdentity struct {
	index int
	name  string
}

func (h *Inbound) UpdateUsers(users []option.VMessUser) error {
	h.userAccess.Lock()
	defer h.userAccess.Unlock()
	return h.updateUsersLocked(users)
}

func (h *Inbound) updateUsersLocked(users []option.VMessUser) error {
	err := h.service.UpdateUsers(common.MapIndexed(users, func(index int, user option.VMessUser) userIdentity {
		return userIdentity{index: index, name: user.Name}
	}), common.Map(users, func(user option.VMessUser) string {
		return user.UUID
	}), common.Map(users, func(user option.VMessUser) int {
		return user.AlterId
	}))
	if err != nil {
		return err
	}
	h.users = append([]option.VMessUser(nil), users...)
	return nil
}

func (h *Inbound) DelUsers(uuids []string) error {
	h.userAccess.Lock()
	defer h.userAccess.Unlock()
	toDelete := make(map[string]struct{}, len(uuids))
	for _, uuid := range uuids {
		toDelete[uuid] = struct{}{}
	}
	remaining := make([]option.VMessUser, 0, len(h.users))
	for _, user := range h.users {
		if _, found := toDelete[user.UUID]; !found {
			remaining = append(remaining, user)
		}
	}
	return h.updateUsersLocked(remaining)
}
