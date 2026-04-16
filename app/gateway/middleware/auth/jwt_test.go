package auth

import (
	"common/util"
	"testing"
)

func TestJwt(t *testing.T) {
	salt := util.RandStringRunes(64)
	t.Logf("Salt: %s", salt)
}
