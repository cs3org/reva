// Copyright 2018-2021 CERN
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

package decomposedfs_test

import (
	"sync"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/owncloud/reva/v2/pkg/errtypes"
	helpers "github.com/owncloud/reva/v2/pkg/storage/utils/decomposedfs/testhelpers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("MarkProcessing", func() {
	var (
		env *helpers.TestEnv

		ref *provider.Reference
	)

	JustBeforeEach(func() {
		var err error
		env, err = helpers.NewTestEnv(nil)
		Expect(err).ToNot(HaveOccurred())

		ref = &provider.Reference{
			ResourceId: env.SpaceRootRes,
			Path:       "/dir1/file1",
		}
	})

	AfterEach(func() {
		if env != nil {
			env.Cleanup()
		}
	})

	// isProcessing reads the processing flag directly from the resolved node.
	isProcessing := func() bool {
		n, err := env.Lookup.NodeFromResource(env.Ctx, ref)
		Expect(err).ToNot(HaveOccurred())
		return n.IsProcessing(env.Ctx)
	}

	Context("on an unmarked node", func() {
		It("sets the processing flag", func() {
			Expect(isProcessing()).To(BeFalse())

			err := env.Fs.MarkProcessing(env.Ctx, ref, true, "test-session")

			Expect(err).ToNot(HaveOccurred())
			Expect(isProcessing()).To(BeTrue())
		})

		It("is a no-op when clearing", func() {
			Expect(isProcessing()).To(BeFalse())

			err := env.Fs.MarkProcessing(env.Ctx, ref, false, "test-session")

			Expect(err).ToNot(HaveOccurred())
			Expect(isProcessing()).To(BeFalse())
		})

		It("allows concurrent marks from different sessions (last writer wins)", func() {
			sessions := []string{"session-a", "session-b"}
			results := make([]error, 2)
			ready := make(chan struct{})
			var wg sync.WaitGroup
			wg.Add(2)
			for i := 0; i < 2; i++ {
				i := i
				go func() {
					defer wg.Done()
					<-ready
					results[i] = env.Fs.MarkProcessing(env.Ctx, ref, true, sessions[i])
				}()
			}
			close(ready)
			wg.Wait()

			Expect(results[0]).ToNot(HaveOccurred())
			Expect(results[1]).ToNot(HaveOccurred())

			n, err := env.Lookup.NodeFromResource(env.Ctx, ref)
			Expect(err).ToNot(HaveOccurred())
			id, err := n.ProcessingID(env.Ctx)
			Expect(err).ToNot(HaveOccurred())
			Expect(id).To(BeElementOf(sessions))
		})
	})

	Context("on an already-marked node", func() {
		JustBeforeEach(func() {
			Expect(env.Fs.MarkProcessing(env.Ctx, ref, true, "test-session")).To(Succeed())
		})

		It("overwrites the processing flag with the new session ID", func() {
			err := env.Fs.MarkProcessing(env.Ctx, ref, true, "session-b")

			Expect(err).ToNot(HaveOccurred())
			n, nerr := env.Lookup.NodeFromResource(env.Ctx, ref)
			Expect(nerr).ToNot(HaveOccurred())
			id, nerr := n.ProcessingID(env.Ctx)
			Expect(nerr).ToNot(HaveOccurred())
			Expect(id).To(Equal("session-b"))
		})

		It("clears the processing flag", func() {
			Expect(isProcessing()).To(BeTrue())

			err := env.Fs.MarkProcessing(env.Ctx, ref, false, "test-session")

			Expect(err).ToNot(HaveOccurred())
			Expect(isProcessing()).To(BeFalse())
		})

		It("does not clear when session ID does not match", func() {
			Expect(isProcessing()).To(BeTrue())

			err := env.Fs.MarkProcessing(env.Ctx, ref, false, "other-session")

			Expect(err).ToNot(HaveOccurred())
			Expect(isProcessing()).To(BeTrue())
		})
	})

	Context("on a non-existent node", func() {
		It("returns NotFound", func() {
			missingRef := &provider.Reference{
				ResourceId: env.SpaceRootRes,
				Path:       "/dir1/does-not-exist",
			}

			err := env.Fs.MarkProcessing(env.Ctx, missingRef, true, "test-session")

			Expect(err).To(HaveOccurred())
			_, ok := err.(errtypes.IsNotFound)
			Expect(ok).To(BeTrue(), "expected errtypes.NotFound, got %T: %v", err, err)
		})
	})
})
