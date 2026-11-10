package nsdump

import (
	"bytes"
	"fmt"
	"iter"
)

type EOSMemoryNSInspect struct {
	cfg NSInspectConfig
}

func (e *EOSMemoryNSInspect) Setup(config any) error {
	if c, ok := config.(NSInspectConfig); ok {
		e.cfg = c
		return nil
	}
	return fmt.Errorf("requires a config of type %T", NSInspectConfig{})
}

func (e *EOSMemoryNSInspect) Dump(rootPath string, maxDepth int) iter.Seq2[NameSpaceEntry, error] {
	return func(yield func(NameSpaceEntry, error) bool) {
		cmd := nsInspectCommand(e.cfg, rootPath, maxDepth)

		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			yield(NameSpaceEntry{}, err)
			return
		}

		parseNSInspect(&stdout)(yield)
	}
}
