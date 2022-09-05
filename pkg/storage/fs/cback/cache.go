package cback

import (
	"context"
	"time"

	"github.com/cs3org/reva/pkg/storage/utils/cback"
)

func (f *cbackfs) listBackups(ctx context.Context, username string) ([]*cback.Backup, error) {
	if d, err := f.cache.Get(username); err == nil {
		return d.([]*cback.Backup), nil
	}
	b, err := f.client.ListBackups(ctx, username)
	if err != nil {
		return nil, err
	}
	_ = f.cache.SetWithExpire(username, b, time.Duration(f.conf.Expiration)*time.Second)
	return b, nil
}
