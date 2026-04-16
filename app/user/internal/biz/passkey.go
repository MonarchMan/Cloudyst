package biz

import (
	"common/hashid"
	"user/ent"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/samber/lo"
)

type AuthnUser struct {
	Hasher      hashid.Encoder
	User        *ent.User
	Credentials []*ent.Passkey
}

func (au *AuthnUser) WebAuthnID() []byte {
	return []byte(hashid.EncodeUserID(au.Hasher, au.User.ID))
}

func (au *AuthnUser) WebAuthnName() string {
	return au.User.Email
}

func (au *AuthnUser) WebAuthnDisplayName() string {
	return au.User.Nick
}

func (au *AuthnUser) WebAuthnCredentials() []webauthn.Credential {
	if au.Credentials == nil {
		return nil
	}

	return lo.Map(au.Credentials, func(item *ent.Passkey, index int) webauthn.Credential {
		return *item.Credential
	})
}
