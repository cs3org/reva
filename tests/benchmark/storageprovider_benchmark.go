package benchmark

import (
	"fmt"

	provider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	posixhelpers "github.com/opencloud-eu/reva/v2/pkg/storage/fs/posix/testhelpers"
	"github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/node"
	decomposedhelpers "github.com/opencloud-eu/reva/v2/pkg/storage/pkg/decomposedfs/testhelpers"
	"github.com/test-go/testify/mock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gmeasure"
)

var _NFac = 1

var _ = Describe("FS Benchmark", func() {
	var (
		decomposedEnv *decomposedhelpers.TestEnv
		posixEnv      *posixhelpers.TestEnv
	)

	BeforeEach(func() {
		var err error

		posixEnv, err = posixhelpers.NewTestEnv(map[string]interface{}{
			"watch_fs": false,
		})
		Expect(err).ToNot(HaveOccurred())
		posixEnv.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(node.OwnerPermissions(), nil)

		decomposedEnv, err = decomposedhelpers.NewTestEnv(map[string]interface{}{})
		Expect(err).ToNot(HaveOccurred())
		decomposedEnv.Permissions.On("AssemblePermissions", mock.Anything, mock.Anything, mock.Anything).Return(node.OwnerPermissions(), nil)
	})

	AfterEach(func() {
		posixEnv.Cleanup()
	})

	It("measures fs.TouchFile", func() {
		N := 10000 * _NFac
		experiment := gmeasure.NewExperiment("fs.Touch")
		AddReportEntry(experiment.Name, experiment)

		By("testing posix fs")
		ref := &provider.Reference{
			ResourceId: posixEnv.SpaceRootRes,
		}
		experiment.SampleDuration("runtime: posix fs", func(idx int) {
			ref.Path = fmt.Sprintf("%d.txt", idx)
			Expect(posixEnv.Fs.TouchFile(posixEnv.Ctx, ref, false, "")).To(Succeed())
		}, gmeasure.SamplingConfig{N: N})

		By("testing decomposed fs")
		ref.ResourceId = decomposedEnv.SpaceRootRes
		experiment.SampleDuration("runtime: decomposed fs", func(idx int) {
			ref.Path = fmt.Sprintf("%d.txt", idx)
			Expect(decomposedEnv.Fs.TouchFile(decomposedEnv.Ctx, ref, false, "")).To(Succeed())
		}, gmeasure.SamplingConfig{N: N})
	})

	It("measures fs.GetMD", func() {
		N := 10000 * _NFac
		experiment := gmeasure.NewExperiment("fs.GetMD")
		AddReportEntry(experiment.Name, experiment)

		By("testing posix fs")
		ref := &provider.Reference{
			ResourceId: posixEnv.SpaceRootRes,
		}
		for i := range N {
			ref.Path = fmt.Sprintf("%d.txt", i)
			Expect(posixEnv.Fs.TouchFile(posixEnv.Ctx, ref, false, "")).To(Succeed())
		}
		experiment.SampleDuration("runtime: posix fs", func(idx int) {
			ref.Path = fmt.Sprintf("%d.txt", idx)
			_, err := posixEnv.Fs.GetMD(posixEnv.Ctx, ref, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
		}, gmeasure.SamplingConfig{N: int(ReportEntryVisibilityNever)})

		By("testing decomposed fs")
		ref = &provider.Reference{
			ResourceId: decomposedEnv.SpaceRootRes,
		}
		for i := range N {
			ref.Path = fmt.Sprintf("%d.txt", i)
			Expect(decomposedEnv.Fs.TouchFile(decomposedEnv.Ctx, ref, false, "")).To(Succeed())
		}
		experiment.SampleDuration("runtime: decomposed fs", func(idx int) {
			ref.Path = fmt.Sprintf("%d.txt", idx)
			_, err := decomposedEnv.Fs.GetMD(decomposedEnv.Ctx, ref, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
		}, gmeasure.SamplingConfig{N: int(ReportEntryVisibilityNever)})

	})

	It("measures fs.CreateDir", func() {
		N := 10000 * _NFac
		experiment := gmeasure.NewExperiment("fs.CreateDir")
		AddReportEntry(experiment.Name, experiment)
		ref := &provider.Reference{
			ResourceId: posixEnv.SpaceRootRes,
		}

		By("testing posix fs")
		experiment.SampleDuration("runtime: posix fs", func(idx int) {
			ref.Path = fmt.Sprintf("%d", idx)
			Expect(posixEnv.Fs.CreateDir(posixEnv.Ctx, ref)).To(Succeed())
		}, gmeasure.SamplingConfig{N: N})

		By("testing decomposed fs")
		ref.ResourceId = decomposedEnv.SpaceRootRes
		experiment.SampleDuration("runtime: decomposed fs", func(idx int) {
			ref.Path = fmt.Sprintf("%d", idx)
			Expect(decomposedEnv.Fs.CreateDir(decomposedEnv.Ctx, ref)).To(Succeed())
		}, gmeasure.SamplingConfig{N: N})
	})

	It("measures fs.ListFolder", func() {
		N := 300 * _NFac
		NFiles := 1000
		experiment := gmeasure.NewExperiment("fs.ListFolder")
		AddReportEntry(experiment.Name, experiment)

		By("testing posix fs")
		ref := &provider.Reference{
			ResourceId: posixEnv.SpaceRootRes,
		}
		for i := range NFiles {
			ref.Path = fmt.Sprintf("%d.txt", i)
			Expect(posixEnv.Fs.TouchFile(posixEnv.Ctx, ref, false, "")).To(Succeed())
		}
		experiment.SampleDuration("runtime: posix fs", func(idx int) {
			ref.Path = ""
			_, err := posixEnv.Fs.ListFolder(posixEnv.Ctx, ref, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
		}, gmeasure.SamplingConfig{N: N})

		By("testing decomposed fs")
		ref.ResourceId = decomposedEnv.SpaceRootRes
		for i := range NFiles {
			ref.Path = fmt.Sprintf("%d.txt", i)
			Expect(decomposedEnv.Fs.TouchFile(decomposedEnv.Ctx, ref, false, "")).To(Succeed())
		}
		experiment.SampleDuration("runtime: decomposed fs", func(idx int) {
			ref.Path = ""
			_, err := decomposedEnv.Fs.ListFolder(decomposedEnv.Ctx, ref, []string{}, []string{})
			Expect(err).ToNot(HaveOccurred())
		}, gmeasure.SamplingConfig{N: N})
	})

	It("measures fs.Move across directories", func() {
		N := 5000 * _NFac
		experiment := gmeasure.NewExperiment("fs.Move")
		AddReportEntry(experiment.Name, experiment)

		Expect(posixEnv.Fs.TouchFile(posixEnv.Ctx, &provider.Reference{
			ResourceId: posixEnv.SpaceRootRes,
			Path:       "file.txt",
		}, false, "")).To(Succeed())
		Expect(decomposedEnv.Fs.TouchFile(decomposedEnv.Ctx, &provider.Reference{
			ResourceId: decomposedEnv.SpaceRootRes,
			Path:       "file.txt",
		}, false, "")).To(Succeed())
		for i := range N {
			posixEnv.Fs.CreateDir(posixEnv.Ctx, &provider.Reference{
				ResourceId: posixEnv.SpaceRootRes,
				Path:       fmt.Sprintf("%d", i),
			})
			decomposedEnv.Fs.CreateDir(decomposedEnv.Ctx, &provider.Reference{
				ResourceId: decomposedEnv.SpaceRootRes,
				Path:       fmt.Sprintf("%d", i),
			})
		}

		By("testing posix fs")
		ref := &provider.Reference{
			ResourceId: posixEnv.SpaceRootRes,
			Path:       "file.txt",
		}
		targetRef := &provider.Reference{
			ResourceId: posixEnv.SpaceRootRes,
			Path:       "file.txt",
		}
		experiment.SampleDuration("runtime: posix fs", func(idx int) {
			targetRef.Path = fmt.Sprintf("%d/file.txt", idx)
			Expect(posixEnv.Fs.Move(posixEnv.Ctx, ref, targetRef)).To(Succeed())
			ref.Path = targetRef.Path
		}, gmeasure.SamplingConfig{N: N})

		ref.ResourceId = decomposedEnv.SpaceRootRes

		By("testing decomposed fs")
		ref = &provider.Reference{
			ResourceId: decomposedEnv.SpaceRootRes,
			Path:       "file.txt",
		}
		targetRef = &provider.Reference{
			ResourceId: decomposedEnv.SpaceRootRes,
			Path:       "file.txt",
		}
		experiment.SampleDuration("runtime: decomposed fs", func(idx int) {
			targetRef.Path = fmt.Sprintf("%d/file.txt", idx)
			Expect(decomposedEnv.Fs.Move(decomposedEnv.Ctx, ref, targetRef)).To(Succeed())
			ref.Path = targetRef.Path
		}, gmeasure.SamplingConfig{N: N})
	})
})
