package main

import (
	"bufio"
	"fmt"
	"os"
)

var configureCommand = func() *command {
	cmd := newCommand("configure")
	cmd.Description = func() string { return "configure the reva client" }
	cmd.Action = func() error {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("host: ")
		text, err := read(reader)
		if err != nil {
			return err
		}

		c := &config{Host: text}
		writeConfig(c)
		fmt.Println("config saved in ", getConfigFile())
		return nil
	}
	return cmd
}
