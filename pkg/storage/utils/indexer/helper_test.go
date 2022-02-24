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
			Expect(medianDuration).To(BeNumerically("~", 100*time.Microsecond, 200*time.Microsecond))
		})
	})
})
