//go:build !unix

package install

import (
	"context"
	"errors"
)

func acquireLock(context.Context, string) (func(), error) {
	return nil, errors.New("install locking is not supported on this platform")
}
