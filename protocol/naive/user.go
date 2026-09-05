package naive

import (
	"github.com/sagernet/sing/common/auth"
)

func (n *Inbound) UpdateUsers(users []auth.User) error {
	authenticator := auth.NewAuthenticator(users)
	if authenticator == nil {
		authenticator = new(auth.Authenticator)
	}
	n.authenticator.Store(authenticator)
	return nil
}
