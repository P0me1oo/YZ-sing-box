package tuic

import (
	"github.com/gofrs/uuid/v5"
	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/exceptions"
)

// 已认证会话保存用户身份，不依赖可变化的用户切片下标。
type userIdentity struct {
	index int
	name  string
}

func (h *Inbound) UpdateUsers(users []option.TUICUser) error {
	var userList []userIdentity
	var userUUIDList [][16]byte
	var userPasswordList []string
	for index, user := range users {
		if user.UUID == "" {
			return exceptions.New("missing uuid for user ", index)
		}
		userUUID, err := uuid.FromString(user.UUID)
		if err != nil {
			return exceptions.Cause(err, "invalid uuid for user ", index)
		}
		userList = append(userList, userIdentity{index: index, name: user.Name})
		userUUIDList = append(userUUIDList, userUUID)
		userPasswordList = append(userPasswordList, user.Password)
	}
	h.server.UpdateUsers(userList, userUUIDList, userPasswordList)
	return nil
}
