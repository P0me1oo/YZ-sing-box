package mieru

import "github.com/sagernet/sing-box/option"

func (h *Inbound) UpdateUsers(users []option.MieruUser) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	options := h.options
	options.Users = users
	if len(users) == 0 {
		h.server.UpdateUsers(nil)
		h.options, h.userNames = options, nil
		return nil
	}
	config, names, err := buildMieruServerConfig(h.ctx, options)
	if err != nil {
		return err
	}
	h.server.UpdateUsers(config.Config.GetUsers())
	h.options, h.userNames = options, names
	return nil
}
