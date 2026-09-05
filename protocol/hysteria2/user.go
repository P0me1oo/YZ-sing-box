package hysteria2

import (
	"github.com/sagernet/sing-box/option"
)

// 已认证会话保存用户身份，不依赖可变化的用户切片下标。
type userIdentity struct {
	index int
	name  string
}

func (h *Inbound) UpdateUsers(users []option.Hysteria2User) error {
	userList := make([]userIdentity, 0, len(users))
	userPasswordList := make([]string, 0, len(users))
	for index, user := range users {
		userList = append(userList, userIdentity{index: index, name: user.Name})
		userPasswordList = append(userPasswordList, user.Password)
	}
	h.service.UpdateUsers(userList, userPasswordList)
	return nil
}
