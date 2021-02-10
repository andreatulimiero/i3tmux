package main

import (
        "fmt"
        "log"
        "log/syslog"
        "os/exec"
        "strings"
        "flag"
        "os"

        "go.i3wm.org/i3/v4"
)

var (
  addMode = flag.Bool("a", false, "add window to current session group")
  detachMode = flag.Bool("d", false, "detach current session group")
  resumeMode = flag.String("r", "", "resume targeted session group")
)

func addWindow() int {
  tree, err := i3.GetTree()
	if err != nil {
		log.Fatal(err)
	}
	con := tree.Root.FindFocused(func(n *i3.Node) bool {
    return len(n.Nodes) == 0 &&
           len(n.FloatingNodes) == 0 &&
           n.Type == i3.Con
	})
  conName := con.WindowProperties.Instance
  split := strings.Split(conName, ":")
  if len(split) != 2 {
    log.Println("Name in unexpected format:", conName)
    return 0
  }
  group, session := split[0], split[1]
  log.Println(group,session)
  return 0
}

func detachSessionGroup() int {
  return 0
}

func resumeSessionGroup() int {
  cmd := exec.Command("ssh", "RPi", `tmux ls -F "#{session_group},#{session_name},#{session_id}"`)
  out, err := cmd.Output()
  if err != nil {
    log.Fatal(err)
  }
  lines := strings.Split(string(out),"\n")
  var groups []string
  sessionsPerGroup := make(map[string]map[string]bool)
  for _,l := range lines {
    split := strings.Split(l,",")
    if len(split) != 3 { continue }
    group, session, _ := split[0], split[1], split[2]
    if _,ok := sessionsPerGroup[group]; !ok {
      sessionsPerGroup[group] = make(map[string]bool)
    }
    sessionsPerGroup[group][session] = true
    groups = append(groups, group)
  }
  g := groups[1]
  for s,_ := range sessionsPerGroup[g] {
      sshCmd := fmt.Sprintf("ssh -t RPi tmux attach -t %s",s)
      cmd := fmt.Sprintf("exec st -n '%s:%s' %s",g,s,sshCmd)
      out, err := i3.RunCommand(cmd)
      if err != nil {
        log.Fatal(err)
      }
      log.Println(out)
  }
  return 0
}

func main() {
  logwriter, err := syslog.New(syslog.LOG_DEBUG, "i3tmux")
  if err != nil {
    log.Fatal(err)
  }
  log.SetOutput(logwriter)

  flag.Parse()
  checkCount := 0
  if *resumeMode != "" { checkCount++ }
  if *addMode { checkCount++ }
  if *detachMode { checkCount++ }
  if checkCount != 1 {
    log.Println("You must specify an option among 'a', 'd' and 'r'")
    os.Exit(1)
  }

  if *resumeMode != "" {
    os.Exit(resumeSessionGroup())
  }
  if *addMode {
    os.Exit(addWindow())
  }
  if *detachMode {
    os.Exit(detachSessionGroup())
  }
}
