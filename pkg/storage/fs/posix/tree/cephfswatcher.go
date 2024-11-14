// Copyright 2018-2024 CERN
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// In applying this license, CERN does not waive the privileges and immunities
// granted to it by virtue of its status as an Intergovernmental Organization
// or submit itself to any jurisdiction.

package tree

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	kafka "github.com/segmentio/kafka-go"
)

const (
	CEPH_MDS_NOTIFY_ACCESS        = 0x0000000000000001
	CEPH_MDS_NOTIFY_ATTRIB        = 0x0000000000000002
	CEPH_MDS_NOTIFY_CLOSE_WRITE   = 0x0000000000000004
	CEPH_MDS_NOTIFY_CLOSE_NOWRITE = 0x0000000000000008
	CEPH_MDS_NOTIFY_CREATE        = 0x0000000000000010
	CEPH_MDS_NOTIFY_DELETE        = 0x0000000000000020
	CEPH_MDS_NOTIFY_DELETE_SELF   = 0x0000000000000040
	CEPH_MDS_NOTIFY_MODIFY        = 0x0000000000000080
	CEPH_MDS_NOTIFY_MOVE_SELF     = 0x0000000000000100
	CEPH_MDS_NOTIFY_MOVED_FROM    = 0x0000000000000200
	CEPH_MDS_NOTIFY_MOVED_TO      = 0x0000000000000400
	CEPH_MDS_NOTIFY_OPEN          = 0x0000000000000800
	CEPH_MDS_NOTIFY_CLOSE         = 0x0000000000001000
	CEPH_MDS_NOTIFY_MOVE          = 0x0000000000002000
	CEPH_MDS_NOTIFY_ONESHOT       = 0x0000000000004000
	CEPH_MDS_NOTIFY_IGNORED       = 0x0000000000008000
	CEPH_MDS_NOTIFY_ONLYDIR       = 0x0000000000010000
)

type CephfsWatcher struct {
	tree        *Tree
	mount_point string
	brokers     []string
}

func NewCephfsWatcher(tree *Tree, kafkaBrokers []string) (*CephfsWatcher, error) {
	return &CephfsWatcher{
		tree:        tree,
		mount_point: tree.options.WatchMountPoint,
		brokers:     kafkaBrokers,
	}, nil
}

type event struct {
	Path string `json:"path"`
	Mask int    `json:"mask"`
}

func (w *CephfsWatcher) Watch(topic string) {
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: w.brokers,
		GroupID: "ocis-posixfs",
		Topic:   topic,
	})

	for {
		m, err := r.ReadMessage(context.Background())
		if err != nil {
			fmt.Println("error reading message", err)
			break
		}

		ev := &event{}
		err = json.Unmarshal(m.Value, ev)
		if err != nil {
			fmt.Println("error unmarshalling message", err)
			continue
		}

		if isLockFile(ev.Path) || isTrash(ev.Path) || w.tree.isUpload(ev.Path) {
			continue
		}

		isDir := ev.Mask&CEPH_MDS_NOTIFY_ONLYDIR > 0
		path := filepath.Join(w.mount_point, ev.Path)
		go func() {
			switch {
			case ev.Mask&CEPH_MDS_NOTIFY_DELETE > 0:
				_ = w.tree.HandleFileDelete(path)
			case ev.Mask&CEPH_MDS_NOTIFY_CREATE > 0:
				_ = w.tree.Scan(path, ActionCreate, isDir)
			case ev.Mask&CEPH_MDS_NOTIFY_CLOSE_WRITE > 0:
				_ = w.tree.Scan(path, ActionUpdate, isDir)
			case ev.Mask&CEPH_MDS_NOTIFY_MOVED_TO > 0:
				_ = w.tree.Scan(path, ActionMove, isDir)
			}
		}()
	}
	if err := r.Close(); err != nil {
		log.Fatal("failed to close reader:", err)
	}
}
