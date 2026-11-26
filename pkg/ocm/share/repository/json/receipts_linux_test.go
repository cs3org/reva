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

//go:build linux

package json

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/cs3org/reva/v3/pkg/ocm/share"
	"golang.org/x/sys/unix"
)

func TestJSONStateNoticeRevokedOnUnacknowledgedSave(t *testing.T) {
	repo, cleanup := setup(t)
	defer cleanup()
	m := repo.(*mgr)
	ctx := share.WithStateNotice(t.Context())
	user, rs := receivedFixture(t, m)
	update := func(state ocm.ShareState) (*share.ReceivedShareUpdate, error) {
		return m.UpdateReceivedShare(ctx, user, &ocm.ReceivedShare{Id: rs.Id, State: state}, mask("state"))
	}
	const accepted, rejected = ocm.ShareState_SHARE_STATE_ACCEPTED, ocm.ShareState_SHARE_STATE_REJECTED
	earlier, err := update(accepted)
	if err != nil || earlier.TryStateNotice == nil {
		t.Fatalf("accepted receipt = %v, err = %v", earlier, err)
	}
	path := m.c.File
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	backup, readFIFO, saveFIFO := path+".original", path+".read", path+".save"
	for _, fifo := range []string{readFIFO, saveFIFO} {
		if err := unix.Mkfifo(fifo, 0600); err != nil {
			t.Fatal(err)
		}
	}

	var restoreErr error
	func() {
		type updateResult struct {
			receipt *share.ReceivedShareUpdate
			err     error
		}
		done := make(chan updateResult, 1)
		deadline, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		var result updateResult
		joined, readOpened := true, false
		feed, saveReader := -1, -1
		var saved []byte
		openReadWriter := func(ctx context.Context) error {
			for {
				select {
				case result = <-done:
					joined = true
					return errors.Join(errors.New("update finished before read FIFO rendezvous"), result.err)
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				fd, err := unix.Open(readFIFO, unix.O_WRONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
				if err == nil {
					feed = fd
					readOpened = true
					return nil
				}
				if !errors.Is(err, unix.ENXIO) {
					return err
				}
				select {
				case <-tick.C:
				case <-ctx.Done():
				}
			}
		}
		collect := func(ctx context.Context) error {
			var buf [4096]byte
			for {
				select {
				case result = <-done:
					joined = true
				default:
				}
				if saveReader >= 0 {
					for {
						if err := ctx.Err(); err != nil {
							return err
						}
						n, err := unix.Read(saveReader, buf[:])
						if errors.Is(err, unix.EINTR) {
							continue
						}
						if errors.Is(err, unix.EAGAIN) {
							break
						}
						if err != nil {
							return err
						}
						if n == 0 {
							break
						}
						saved = append(saved, buf[:n]...)
					}
				}
				if joined {
					return nil
				}
				select {
				case <-tick.C:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}

		if err := os.Rename(path, backup); err != nil {
			t.Fatal(err)
		}
		defer func() {
			// Late opens use the regular file. A parked reader still needs readFIFO.
			restoreErr = os.Rename(backup, path)
			if restoreErr != nil {
				t.Errorf("restore repository file: %v", restoreErr)
			}
			if saveReader >= 0 {
				defer func() {
					if err := unix.Close(saveReader); err != nil {
						t.Errorf("close save FIFO reader: %v", err)
					}
				}()
			}
			cleanup, stop := context.WithTimeout(t.Context(), 5*time.Second)
			defer stop()
			if !joined && !readOpened {
				_ = openReadWriter(cleanup)
			}
			if feed >= 0 {
				if err := unix.Close(feed); err != nil {
					t.Errorf("close read FIFO writer during cleanup: %v", err)
				}
			}
			if !joined {
				if err := collect(cleanup); err != nil {
					t.Errorf("join update during cleanup: %v", err)
				}
			}
		}()
		if err := os.Link(readFIFO, path); err != nil {
			t.Fatal(err)
		}
		joined = false
		go func() {
			receipt, err := update(rejected)
			done <- updateResult{receipt: receipt, err: err}
		}()
		if err := openReadWriter(deadline); err != nil {
			t.Fatalf("open read FIFO writer: %v", err)
		}

		// Keep a reader on the save inode from publication through the worker's exit.
		saveReader, err = unix.Open(saveFIFO, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Link(saveFIFO, path+".next"); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path+".next", path); err != nil {
			t.Fatal(err)
		}
		if n, err := unix.Write(feed, original); err != nil || n != len(original) {
			t.Fatalf("release repository read: wrote %d of %d bytes, err = %v", n, len(original), err)
		}
		err = unix.Close(feed)
		feed = -1
		if err != nil {
			t.Fatalf("close read FIFO writer: %v", err)
		}
		if err := collect(deadline); err != nil {
			t.Fatalf("observe save FIFO and update result: %v", err)
		}
		if len(saved) == 0 {
			t.Fatalf("update finished without attempted save bytes: err = %v", result.err)
		}
		var attempted shareModel
		if err := json.Unmarshal(saved, &attempted); err != nil {
			t.Fatalf("decode attempted save: %v", err)
		}
		if got := attempted.ReceivedShares[rs.Id.OpaqueId].GetState(); got != rejected {
			t.Fatalf("attempted state = %v, want %v", got, rejected)
		}
		if !errors.Is(result.err, unix.EINVAL) {
			t.Fatalf("save error = %v, want FIFO sync failure", result.err)
		}
		if result.receipt != nil {
			t.Fatal("unacknowledged update returned a receipt")
		}
	}()

	if restoreErr != nil {
		return
	}
	ref := &ocm.ShareReference{Spec: &ocm.ShareReference_Id{Id: rs.Id}}
	current, err := m.GetReceivedShare(ctx, user, ref)
	if err != nil || current.GetState() != accepted {
		t.Fatalf("restored state = %v, err = %v, want %v", current.GetState(), err, accepted)
	}
	if earlier.TryStateNotice() {
		t.Fatal("unacknowledged save left the earlier proof live")
	}
}
