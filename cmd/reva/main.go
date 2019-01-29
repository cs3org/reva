package main

import (
	"fmt"
	"os"
	"strings"
)

var (
	conf *config
)

func main() {

	cmds := []*command{
		configureCommand(),
		loginCommand(),
		whoamiCommand(),
		lsCommand(),
		statCommand(),
		uploadCommand(),
		downloadCommand(),
		rmCommand(),
		moveCommand(),
		mkdirCommand(),
		brokerFindCommand(),
		appRegistryFindCommand(),
		appProviderGetIFrameCommand(),
	}

	mainUsage := createMainUsage(cmds)

	// Verify that a subcommand has been provided
	// os.Arg[0] is the main command
	// os.Arg[1] will be the subcommand
	if len(os.Args) < 2 {
		fmt.Println(mainUsage)
		os.Exit(1)
	}

	// Verify a configuration file exists.
	// If if does not, create one
	c, err := readConfig()
	if err != nil && os.Args[1] != "configure" {
		fmt.Println("reva is not initialized, run \"reva configure\"")
		os.Exit(1)
	} else {
		if os.Args[1] != "configure" {
			conf = c
		}
	}

	// Run command
	action := os.Args[1]
	for _, v := range cmds {
		if v.Name == action {
			v.Parse(os.Args[2:])
			err := v.Action()
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			os.Exit(0)
		}
	}

	fmt.Println(mainUsage)
	os.Exit(1)
}

func createMainUsage(cmds []*command) string {
	n := 0
	for _, cmd := range cmds {
		l := len(cmd.Name)
		if l > n {
			n = l
		}
	}

	usage := "Command line interface to REVA\n\n"
	for _, cmd := range cmds {
		usage += fmt.Sprintf("%s%s%s\n", cmd.Name, strings.Repeat(" ", 4+(n-len(cmd.Name))), cmd.Description())
	}
	usage += "\nAuthors: hugo.gonzalez.labrador@cern.ch"
	usage += "\nCopyright: CERN IT Storage Group"
	return usage
}
