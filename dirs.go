package main

import (
	"fmt"
	"os"
	"path"
)

var (
	RUNTIME_BASE_DIR_DEFAULT = path.Join("/", "var", "run")
	RUNTIME_DIR              = path.Join(RUNTIME_BASE_DIR_DEFAULT, I3TMUX)

	CONF_BASE_DIR_DEFAULT = path.Join(os.Getenv("HOME"), ".config")
	CONF_DIR              = path.Join(CONF_BASE_DIR_DEFAULT, I3TMUX)

	DATA_BASE_DIR_DEFAULT = path.Join(os.Getenv("HOME"), ".local", "share")
	DATA_DIR              = path.Join(DATA_BASE_DIR_DEFAULT, I3TMUX)

	SSH_CONF = path.Join(os.Getenv("HOME"), ".ssh", "config")
)

var (
	SERVER_SOCK = ""
	LOG_FILE    = ""
)

func ensureBaseDirs() error {
	dirs := []struct {
		EnvDir string
		Name   *string
		Perm   os.FileMode
	}{
		{"XDG_RUNTIME_DIR", &RUNTIME_DIR, 0700},
		{"XDG_CONFIG_HOME", &CONF_DIR, 0755},
		{"XDG_DATA_HOME", &DATA_DIR, 0755},
	}
	for _, d := range dirs {
		if envDir := os.Getenv(d.EnvDir); envDir != "" {
			*d.Name = path.Join(envDir, I3TMUX)
		}
		if err := os.MkdirAll(*d.Name, d.Perm); err != nil {
			return fmt.Errorf("ensuring existence of %s: %e", *d.Name, err)
		}
	}

	SERVER_SOCK = path.Join(RUNTIME_DIR, I3TMUX+".sock")
	LOG_FILE = path.Join(RUNTIME_DIR, I3TMUX+".log")
	return nil
}
