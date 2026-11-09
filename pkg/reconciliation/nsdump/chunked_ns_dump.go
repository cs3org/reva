package nsdump

import (
	"fmt"
	"io"
	"iter"
	"os"

	"github.com/pkg/errors"
)

type EOSChunkedNSInspectConfig struct {
	IgnoreFiles bool   `mapstructure:"ignorefiles"`
	Instance    string `mapstructure:"instance"`
	// TmpDir holds the dump while it is parsed. It takes about 800 MB per
	// million nodes. Empty means os.TempDir().
	TmpDir string `mapstructure:"tmp_dir"`
}

// EOSChunkedNSInspect writes the scan to a file and parses it from there, so
// the output is never in memory as a whole.
type EOSChunkedNSInspect struct {
	cfg EOSChunkedNSInspectConfig
}

func (e *EOSChunkedNSInspect) Setup(config any) error {
	if c, ok := config.(EOSChunkedNSInspectConfig); ok {
		e.cfg = c
		return nil
	}
	return fmt.Errorf("requires a config of type %T", EOSChunkedNSInspectConfig{})
}

func (e *EOSChunkedNSInspect) Dump(rootPath string, maxDepth int) iter.Seq2[NameSpaceEntry, error] {
	return func(yield func(NameSpaceEntry, error) bool) {
		f, err := os.CreateTemp(e.cfg.TmpDir, "nsdump-*.json")
		if err != nil {
			yield(NameSpaceEntry{}, errors.Wrap(err, "failed to create the dump file"))
			return
		}
		// also when the caller stops early or the parse fails: one dump can
		// take gigabytes of disk.
		defer os.Remove(f.Name())
		defer f.Close()

		cmd := nsInspectCommand(e.cfg.Instance, rootPath, maxDepth, e.cfg.IgnoreFiles)
		cmd.Stdout = f
		if err := cmd.Run(); err != nil {
			yield(NameSpaceEntry{}, errors.Wrap(err, "failed to run eos-ns-inspect"))
			return
		}

		if _, err := f.Seek(0, io.SeekStart); err != nil {
			yield(NameSpaceEntry{}, errors.Wrap(err, "failed to rewind the dump file"))
			return
		}
		parseNSInspect(f)(yield)
	}
}
