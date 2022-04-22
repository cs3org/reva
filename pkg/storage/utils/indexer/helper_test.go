// Copyright 2018-2022 CERN
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

package indexer

import (
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
)

var _ = Describe("Helper", func() {
	Describe("dedup", func() {
		It("dedups reasonably fast", func() {
			experiment := gmeasure.NewExperiment("deduplicating string slices")
			AddReportEntry(experiment.Name, experiment)

			experiment.Sample(func(idx int) {
				slice := []string{}
				for i := 0; i < 900; i++ {
					slice = append(slice, strconv.Itoa(i))
				}
				for i := 0; i < 100; i++ {
					slice = append(slice, strconv.Itoa(i))
				}
				experiment.MeasureDuration("repagination", func() {
					dedupped := dedup(slice)
					Expect(len(dedupped)).To(Equal(900))
				})
			}, gmeasure.SamplingConfig{N: 100000, Duration: 10 * time.Second})

			repaginationStats := experiment.GetStats("repagination")
			medianDuration := repaginationStats.DurationFor(gmeasure.StatMedian)
			Expect(medianDuration).To(BeNumerically("<", 1200*time.Microsecond))
		})
	})
})
