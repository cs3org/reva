package nsdump

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type EOSMemoryNSInspectConfig struct {
	MaxDepth    int    `mapstructure:"maxdepth"`
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
	return fmt.Errorf("requires a config of type %t", EOSMemoryNSInspectConfig{})
}

func (e *EOSMemoryNSInspect) Dump(rootPath string, maxDepth int) (*NamespaceDump, error) {

	noFilesFlag := ""
	if e.cfg.IgnoreFiles {
		noFilesFlag = " --no-files"
	}

	maxDepthFlag := ""
	if maxDepth > 0 {
		maxDepthFlag = fmt.Sprintf(" --maxdepth %d", maxDepth)
	}

	args := fmt.Sprintf(
		"scan --path %s %s --members %s-qdb:7777 --password-file /keytabs/%s_keytab --json%s",
		rootPath,
		maxDepthFlag,
		e.cfg.Instance,
		e.cfg.Instance,
		noFilesFlag,
	)

	cmd := exec.Command("/usr/bin/eos-ns-inspect", strings.Split(args, " ")...)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	err := cmd.Run()
	if err != nil {
		return nil, err
	}

	return parseNSInspectOutput(stdout.Bytes())
}
