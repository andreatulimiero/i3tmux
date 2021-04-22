package main

import (
	"fmt"
	"log"
	"os"
	"os/user"
	"path"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"
	"gopkg.in/yaml.v2"
)

type Conf struct {
	host         string
	hostname     string
	portNo       int
	user         string
	identityFile string
}

func getConfForHost(host string) (*Conf, error) {
	sshConfFile, err := os.Open(path.Join(os.Getenv("HOME"), ".ssh", "config"))
	if err != nil {
		return nil, err
	}
	sshConf, err := ssh_config.Decode(sshConfFile)
	if err != nil {
		return nil, err
	}

	user, err := user.Current()
	if err != nil {
		return nil, err
	}
	conf := &Conf{
		host:   host,
		user:   user.Username,
		portNo: 22,
	}

	hostname, err := sshConf.Get(host, "Hostname")
	if err != nil {
		return nil, err
	}
	if hostname == "" {
		return nil, fmt.Errorf("Hostname must not be empty.\nHint: is %s present in config?", host)
	}
	conf.hostname = hostname

	portNoStr, err := sshConf.Get(host, "Port")
	if err != nil {
		return nil, err
	}
	if portNoStr != "" {
		portNo, err := strconv.Atoi(portNoStr)
		if err != nil {
			return nil, err
		}
		conf.portNo = portNo
	}

	userName, err := sshConf.Get(host, "User")
	if err != nil {
		return nil, err
	}
	if userName != "" {
		conf.user = userName
	}

	identityFile, err := sshConf.Get(host, "IdentityFile")
	if err != nil {
		return nil, err
	}
	if identityFile == "" {
		return nil, fmt.Errorf("IdentityFile must not be empty")
	}
	if strings.HasPrefix(identityFile, "~/") {
		identityFile = path.Join(user.HomeDir, identityFile[2:])
	}
	conf.identityFile = identityFile

	return conf, nil
}

// Pref struct holds user preferences
type Pref struct {
	Terminal struct {
		Bin      string
		NameFlag string `yaml:"nameFlag"`
	}
}

func getUserPreferences() *Pref {
	pref := Pref{}
	dotFilePath := path.Join(os.Getenv("HOME"), ".config", "i3tmux", "config.yaml")
	dotFile, err := os.ReadFile(dotFilePath)
	if err == nil {
		err := yaml.Unmarshal(dotFile, &pref)
		if err == nil {
		} else {
			log.Println("Dotfile is invalid, using flags or default values")
		}
	} else {
		log.Println(fmt.Errorf("Couldn't open dotfile, using flags or default values: %w", err))
	}
	if *terminalBinFlag != "" {
		pref.Terminal.Bin = *terminalBinFlag
	}
	if *terminalNameFlag != "" {
		pref.Terminal.NameFlag = *terminalNameFlag
	}
	return &pref
}
