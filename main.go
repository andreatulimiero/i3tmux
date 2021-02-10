package main

import (
        "fmt"
        "log"
        "log/syslog"
        "os/exec"
        "strings"
        "flag"
        "os"
        "encoding/json"
        "io/ioutil"

        "go.i3wm.org/i3/v4"
)

const (
  terminalBin = "kitty"
  terminalNameFlag = "--name"
)

var (
  addMode = flag.Bool("a", false, "add window to current session group")
  detachMode = flag.Bool("d", false, "detach current session group")
  resumeMode = flag.String("r", "", "resume targeted session group")
)

type SessionsPerGroup map[string]map[string]bool

func getContainerGroupSession(con *i3.Node) (string, string, error) {
  conName := con.WindowProperties.Instance
  split := strings.Split(conName, ":")
  if len(split) != 2 {
    return "", "", fmt.Errorf("name in unexpected format: %s", conName)
  }
  return split[0], split[1], nil
}

func fetchSessionsPerGroup(host string) SessionsPerGroup {
  cmd := exec.Command("ssh", host, `tmux ls -F "#{session_group},#{session_name},#{session_id}"`)
  out, err := cmd.Output()
  if err != nil {
    log.Fatal(err)
  }
  lines := strings.Split(string(out),"\n")
  sessionsPerGroup := make(map[string]map[string]bool)
  for _,l := range lines {
    split := strings.Split(l,",")
    if len(split) != 3 { continue }
    group, session, _ := split[0], split[1], split[2]
    if _,ok := sessionsPerGroup[group]; !ok {
      sessionsPerGroup[group] = make(map[string]bool)
    }
    sessionsPerGroup[group][session] = true
  }
  return sessionsPerGroup
}

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
  group, session, err := getContainerGroupSession(con)
  if err != nil {
    log.Fatal(err)
  }
  log.Println(group,session)
  return 0
}

func getFocusedWs(tree *i3.Tree) (*i3.Node, error) {
  ws := tree.Root.FindFocused(func(n *i3.Node) bool {
		return n.Type == i3.WorkspaceNode
	})
	if ws == nil {
    return nil, fmt.Errorf("could not locate focused workspace")
	}
  return ws, nil
}

func nodeIsLeaf(n *i3.Node) bool {
  return n.Type == i3.Con && len(n.Nodes) == 0
}

func getTreeOfSessions(u *i3.Node) map[string]interface{} {
  if nodeIsLeaf(u) {
    group,session,err := getContainerGroupSession(u)
    if err != nil {
      // We care about tmux session leaves only
      return nil
    }
    m := make(map[string]interface{})
    m["type"] = i3.Con
    m["swallows"] = []map[string]string{{"instance":fmt.Sprintf("%s\\:%s",group,session)}}
    return m
  } else {
    var nodes []map[string]interface{}
    for _,v := range u.Nodes {
      sessionNodes := getTreeOfSessions(v)
      if sessionNodes == nil {
        continue
      }
      nodes = append(nodes, sessionNodes)
    }
    switch len(nodes) {
    case 0:
      // No child is session-container, skip this
      return nil
    case 1:
      // Optimize out self and return the only child
      return nodes[0]
    default:
      m := make(map[string]interface{})
      m["layout"] = u.Layout
      m["type"] = u.Type
      m["percent"] = u.Percent
      m["nodes"] = nodes
      return m
    }
  }
}

func detachSessionGroup() int {
  tree, err := i3.GetTree()
  if err != nil {
    log.Fatal(err)
  }
  ws, err := getFocusedWs(&tree)
  if err != nil {
    log.Fatal(err)
  }
  wsSessMap := getTreeOfSessions(ws)
  j, err := json.Marshal(wsSessMap)
  if err != nil {
    log.Fatal(err)
  }
  fmt.Println(string(j))
  err = ioutil.WriteFile("layout.json",j,0644)
  if err != nil {
    log.Fatal(err)
  }
  return 0
}

func resumeSessionGroup() int {
  sessionsPerGroup := fetchSessionsPerGroup("RPi")
  var groups []string
  for k := range sessionsPerGroup {
    groups = append(groups,k)
  }
  for s,_ := range sessionsPerGroup[groups[0]] {
      sshCmd := fmt.Sprintf("ssh -t RPi tmux attach -t %s",s)
      cmd := fmt.Sprintf("exec %s %s '%s:%s' %s",terminalBin,terminalNameFlag,groups[0],s,sshCmd)
      log.Println(cmd)
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
