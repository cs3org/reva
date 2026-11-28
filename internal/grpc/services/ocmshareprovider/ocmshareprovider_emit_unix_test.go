// Copyright 2018-2026 CERN
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

//go:build unix

package ocmshareprovider

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"golang.org/x/sys/unix"
	"google.golang.org/protobuf/proto"
)

func TestJSONReceivedStateNoticeWhileFileReadHeld(t *testing.T) {
	testJSONReceivedStateNoticeWhileRepositoryBlocked(t, false,
		func(path string, get func() (*ocm.ReceivedShare, error), accepted *ocm.ReceivedShare, notify func()) {
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			backup, fifo := path+".original", path+".read"
			if err := unix.Mkfifo(fifo, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(path, backup); err != nil {
				t.Fatal(err)
			}

			type readResult struct {
				share *ocm.ReceivedShare
				err   error
			}
			done := make(chan readResult, 1)
			deadline, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			tick := time.NewTicker(time.Millisecond)
			defer tick.Stop()
			feed := -1
			joined := true
			openWriter := func(ctx context.Context) (int, error) {
				for {
					select {
					case result := <-done:
						joined = true
						return -1, errors.Join(errors.New("repository read finished before FIFO rendezvous"), result.err)
					case <-ctx.Done():
						return -1, ctx.Err()
					default:
					}
					fd, err := unix.Open(fifo, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
					if !errors.Is(err, unix.ENXIO) {
						return fd, err
					}
					select {
					case <-tick.C:
					case <-ctx.Done():
					}
				}
			}
			defer func() {
				// Late opens need a regular file. A parked open still needs the FIFO inode.
				if err := os.Rename(backup, path); err != nil {
					t.Errorf("restore repository file before releasing read: %v", err)
				}
				cleanup, stop := context.WithTimeout(t.Context(), 5*time.Second)
				defer stop()
				if !joined && feed < 0 {
					feed, _ = openWriter(cleanup)
				}
				if feed >= 0 {
					if err := unix.Close(feed); err != nil {
						t.Errorf("close FIFO writer during cleanup: %v", err)
					}
				}
				if joined {
					return
				}
				select {
				case <-done:
				case <-cleanup.Done():
					t.Error("repository read did not exit during cleanup")
				}
			}()
			if err := os.Link(fifo, path); err != nil {
				t.Fatal(err)
			}
			joined = false
			go func() {
				got, err := get()
				done <- readResult{share: got, err: err}
			}()
			feed, err = openWriter(deadline)
			if err != nil {
				t.Fatalf("open held-read FIFO writer: %v", err)
			}

			notify()
			select {
			case result := <-done:
				joined = true
				t.Fatalf("repository read finished before release: %v", result.err)
			default:
			}
			if n, err := unix.Write(feed, original); err != nil || n != len(original) {
				t.Fatalf("release repository read: wrote %d of %d bytes, err = %v", n, len(original), err)
			}
			err = unix.Close(feed)
			feed = -1
			if err != nil {
				t.Fatalf("close held-read FIFO writer: %v", err)
			}
			drain, stop := context.WithTimeout(t.Context(), 5*time.Second)
			defer stop()
			select {
			case result := <-done:
				joined = true
				if result.err != nil || !proto.Equal(result.share, accepted) {
					t.Fatalf("released read = %v, err = %v, want %v", result.share, result.err, accepted)
				}
			case <-drain.Done():
				t.Fatal("repository read did not finish after release")
			}
		})
}
