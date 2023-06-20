package spaceidindex

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/cs3org/reva/v2/pkg/storage/utils/decomposedfs/mtimesyncedcache"
	"github.com/pkg/errors"
	"github.com/rogpeppe/go-internal/lockedfile"
	"github.com/shamaton/msgpack/v2"
)

type Index struct {
	root  string
	index string
	cache mtimesyncedcache.Cache[string, map[string][]byte]
}

func New(root, index string) *Index {
	return &Index{
		root:  root,
		index: index,
	}
}

func (i *Index) Load(key string) (map[string][]byte, error) {
	indexPath := filepath.Join(i.root, i.index, key+".mpk")
	fi, err := os.Stat(indexPath)
	if err != nil {
		return nil, err
	}
	return i.readSpaceIndex(indexPath, i.index+":"+key, fi.ModTime())
}

func (i *Index) readSpaceIndex(indexPath, cacheKey string, mtime time.Time) (map[string][]byte, error) {
	return i.cache.LoadOrStore(cacheKey, mtime, func() (map[string][]byte, error) {
		// Acquire a read log on the index file
		f, err := lockedfile.Open(indexPath)
		if err != nil {
			return nil, errors.Wrap(err, "unable to lock index to read")
		}
		defer func() {
			rerr := f.Close()

			// if err is non nil we do not overwrite that
			if err == nil {
				err = rerr
			}
		}()

		// Read current state
		msgBytes, err := io.ReadAll(f)
		if err != nil {
			return nil, errors.Wrap(err, "unable to read index")
		}
		links := map[string][]byte{}
		if len(msgBytes) > 0 {
			err = msgpack.Unmarshal(msgBytes, &links)
			if err != nil {
				return nil, errors.Wrap(err, "unable to parse index")
			}
		}
		return links, nil
	})
}
