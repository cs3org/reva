package mount

import (
	"github.com/cernbox/reva/pkg/storage"
)

type mount struct {
	name, dir string
	fs        storage.FS
	opts      *storage.MountOptions
}

func New(name, dir string, fs storage.FS) storage.Mount {
	return &mount{
		name: name,
		dir:  dir,
		fs:   fs,
		opts: nil,
	}
}

func (m *mount) GetName() string                   { return m.name }
func (m *mount) GetDir() string                    { return m.dir }
func (m *mount) GetFS() storage.FS                 { return m.fs }
func (m *mount) GetOptions() *storage.MountOptions { return m.opts }
