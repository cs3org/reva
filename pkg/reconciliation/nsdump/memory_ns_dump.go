package nsdump

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

type EOSMemoryNSInspectConfig struct {
	maxDepth    int    `mapstructure:"maxdepth"`
	ignoreFiles bool   `mapstructure:"ignorefiles"`
	instance    string `mapstructure:"instance"`
}

type EOSMemoryNSInspect struct {
	cfg EOSMemoryNSInspectConfig
}

func (e *EOSMemoryNSInspect) Setup(config map[string]any) error {
	// TODO(jgeens): implement
	return nil
}

func (e *EOSMemoryNSInspect) Dump(rootPath string, maxDepth int) (*NamespaceDump, error) {

	noFilesFlag := ""
	if e.cfg.ignoreFiles {
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
		e.cfg.instance,
		e.cfg.instance,
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
