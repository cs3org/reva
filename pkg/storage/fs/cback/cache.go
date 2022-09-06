package cback

import (
	"context"
	"fmt"
	"time"

	"github.com/cs3org/reva/pkg/storage/utils/cback"
)

func (f *cbackfs) listBackups(ctx context.Context, username string) ([]*cback.Backup, error) {
	key := "backups:" + username
	if d, err := f.cache.Get(key); err == nil {
		return d.([]*cback.Backup), nil
	}
	b, err := f.client.ListBackups(ctx, username)
	if err != nil {
		return nil, err
	}
	_ = f.cache.SetWithExpire(key, b, time.Duration(f.conf.Expiration)*time.Second)
	return b, nil
}

func (f *cbackfs) stat(ctx context.Context, username string, id int, snapshot, path string) (*cback.Resource, error) {
	key := fmt.Sprintf("stat:%s:%d:%s:%s", username, id, snapshot, path)
	if s, err := f.cache.Get(key); err == nil {
		return s.(*cback.Resource), nil
	}
	s, err := f.client.Stat(ctx, username, id, snapshot, path)
	if err != nil {
		return nil, err
	}
	_ = f.cache.SetWithExpire(key, s, time.Duration(f.conf.Expiration)*time.Second)
	return s, nil
}

func (f *cbackfs) listFolder(ctx context.Context, username string, id int, snapshot, path string) ([]*cback.Resource, error) {
	key := fmt.Sprintf("list:%s:%d:%s:%s", username, id, snapshot, path)
	if l, err := f.cache.Get(key); err == nil {
		return l.([]*cback.Resource), nil
	}
	l, err := f.client.ListFolder(ctx, username, id, snapshot, path)
	if err != nil {
		return nil, err
	}
	_ = f.cache.SetWithExpire(key, l, time.Duration(f.conf.Expiration)*time.Second)
	return l, nil
}
