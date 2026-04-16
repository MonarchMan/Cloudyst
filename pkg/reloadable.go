package common

import (
	"context"
)

type (
	Reloadable interface {
		Reload(ctx context.Context) error
	}
)
