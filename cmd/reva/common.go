package main

import (
	"bufio"
	"encoding/json"
	"io/ioutil"
	gouser "os/user"
	"path"
	"strings"

	"golang.org/x/crypto/ssh/terminal"
)

func getConfigFile() string {
	user, err := gouser.Current()
	if err != nil {
		panic(err)
	}

	return path.Join(user.HomeDir, ".reva.config")
}

func getTokenFile() string {
	user, err := gouser.Current()
	if err != nil {
		panic(err)
	}

	return path.Join(user.HomeDir, ".reva-token")
}

func writeToken(token string) {
	ioutil.WriteFile(getTokenFile(), []byte(token), 0600)
}

func readToken() (string, error) {
	data, err := ioutil.ReadFile(getTokenFile())
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readConfig() (*config, error) {
	data, err := ioutil.ReadFile(getConfigFile())
	if err != nil {
		return nil, err
	}

	c := &config{}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}

	return c, nil
}

func writeConfig(c *config) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(getConfigFile(), data, 0600)
}

type config struct {
	Host string `json:"host"`
}

func read(r *bufio.Reader) (string, error) {
	text, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}
func readPassword(fd int) (string, error) {
	bytePassword, err := terminal.ReadPassword(fd)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(bytePassword)), nil
}
