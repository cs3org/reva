package nsdump

import (
	"bytes"
	"fmt"
	"iter"
)

type EOSMemoryNSInspectConfig struct {
	IgnoreFiles bool   `mapstructure:"ignorefiles"`
	Instance    string `mapstructure:"instance"`
}

type EOSMemoryNSInspect struct {
	cfg EOSMemoryNSInspectConfig
}

func (e *EOSMemoryNSInspect) Setup(config any) error {
	if c, ok := config.(EOSMemoryNSInspectConfig); ok {
		e.cfg = c
		return nil
	}
	return fmt.Errorf("requires a config of type %T", EOSMemoryNSInspectConfig{})
}

func (e *EOSMemoryNSInspect) Dump(rootPath string, maxDepth int) iter.Seq2[NameSpaceEntry, error] {
	return func(yield func(NameSpaceEntry, error) bool) {
		cmd := nsInspectCommand(e.cfg.Instance, rootPath, maxDepth, e.cfg.IgnoreFiles)

		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			yield(NameSpaceEntry{}, err)
			return
		}

		parseNSInspect(&stdout)(yield)
	}
}
