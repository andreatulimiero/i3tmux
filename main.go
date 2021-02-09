package main

import (
        "log"
        // "log/syslog"
        "os/exec"
        "strings"

        "go.i3wm.org/i3/v4"
)

func main() {
  // logwriter, err := syslog.New(syslog.LOG_DEBUG, "i3tmux")
  // if err != nil {
    // log.Fatal(err)
  // }
  // log.SetOutput(logwriter)

  cmd := exec.Command("ssh", "ubs7", `tmux ls -F "#{session_group}, #{session_name},#{session_id}"`)
  out, err := cmd.Output()
  if err != nil {
    log.Fatal(err)
  }
  log.Println(strings.Split(string(out),","))

  tree, err := i3.GetTree()
	if err != nil {
		log.Fatal(err)
	}
	focusedCon := tree.Root.FindFocused(func(n *i3.Node) bool {
    return len(n.Nodes) == 0 &&
           len(n.FloatingNodes) == 0 &&
           n.Type == i3.Con
	})
  log.Println(focusedCon)
}
