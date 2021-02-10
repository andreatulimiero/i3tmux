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
  addMode = flag.Bool("add", false, "add window to current session group")
  detachMode = flag.Bool("detach", false, "detach current session group")
  resumeGroup = flag.String("resume", "", "resume session group")
  resumeLayoutPath = flag.String("layout", "", "use layout to resume session group")
  listMode = flag.Bool("list", false, "list sessions groups")
)

type SessionsPerGroup map[string]map[string]bool

func parseGroupSessFromCon(con *i3.Node) (string, string, error) {
  conName := con.WindowProperties.Instance
  split := strings.Split(conName, "_")
  if len(split) != 2 {
    return "", "", fmt.Errorf("name in unexpected format: %s", conName)
  }
  return split[0], split[1], nil
}

func fetchSessionsPerGroup(host string) (SessionsPerGroup, error) {
  cmd := exec.Command("ssh", host, `tmux ls -F "#{session_group},#{session_name},#{session_id}"`)
  out, err := cmd.Output()
  if err != nil {
    return nil, err
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
  return sessionsPerGroup, nil
}

func addWindow() error {
  tree, err := i3.GetTree()
	if err != nil {
		return err
	}
	con := tree.Root.FindFocused(func(n *i3.Node) bool {
    return len(n.Nodes) == 0 &&
           len(n.FloatingNodes) == 0 &&
           n.Type == i3.Con
	})
  group, session, err := parseGroupSessFromCon(con)
  if err != nil {
    return err
  }
  log.Println(group,session)
  return nil
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

func getFocusedCon(tree *i3.Tree) (*i3.Node, error) {
  var con *i3.Node
  tree.Root.FindFocused(func(n *i3.Node) bool { 
    con = n
    return false
  })
  if con == nil {
    return nil, fmt.Errorf("could not locate focused container")
  }
  return con, nil
}

func nodeIsLeaf(n *i3.Node) bool {
  return n.Type == i3.Con && len(n.Nodes) == 0
}

func getTreeOfGroupSess(u *i3.Node) map[string]interface{} {
  if nodeIsLeaf(u) {
    group,session,err := parseGroupSessFromCon(u)
    if err != nil {
      // We care about tmux session leaves only
      return nil
    }
    m := make(map[string]interface{})
    m["type"] = i3.Con
    m["swallows"] = []map[string]string{{"instance":fmt.Sprintf("%s_%s",group,session)}}
    return m
  } else {
    var nodes []map[string]interface{}
    for _,v := range u.Nodes {
      sessionNodes := getTreeOfGroupSess(v)
      if sessionNodes == nil {
        continue
      }
      nodes = append(nodes, sessionNodes)
    }
    switch len(nodes) {
    case 0:
      // No child contains a session, skip this
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

func detachSessionGroup() error {
  // TODO: Add killing of terminals running ssh sessions once layout is retrieved
  //       to simulate a proper detach
  tree, err := i3.GetTree()
  if err != nil {
    return err
  }
  con, err := getFocusedCon(&tree)
  if err != nil {
    return err
  }
  group, session, err := parseGroupSessFromCon(con)
  if err != nil {
    return err
  }
  ws, err := getFocusedWs(&tree)
  if err != nil {
    return err
  }
  groupSessLayout := getTreeOfGroupSess(ws)
  j, err := json.Marshal(groupSessLayout)
  if err != nil {
    return err
  }
  err = ioutil.WriteFile(fmt.Sprintf("%s_%s.json",group,session),j,0644)
  if err != nil {
    return err
  }
  log.Println("Saved layout for group:",group)
  return nil
}

func resumeSessionGroup() error {
  sessionsPerGroup, err := fetchSessionsPerGroup("RPi")
  if err != nil {
    return err
  }
  sessions, ok := sessionsPerGroup[*resumeGroup]
  if !ok {
    return fmt.Errorf("group not found")
  }
  if *resumeLayoutPath != "" {
    _, err := i3.RunCommand(fmt.Sprintf("append_layout %s",*resumeLayoutPath))
    if err != nil {
      log.Fatal(err)
    }
  }
  for s,_ := range sessions {
      sshCmd := fmt.Sprintf("ssh -t RPi tmux attach -t %s",s)
      i3cmd := fmt.Sprintf("exec %s %s '%s_%s' %s",terminalBin,terminalNameFlag,*resumeGroup,s,sshCmd)
      _, err := i3.RunCommand(i3cmd)
      if err != nil {
        log.Fatal(err)
      }
  }
  return nil
}

func listSessionsGroup() error {
  fmt.Println("Retrieving available sessions groups ...")
  sessionsPerGroup, err := fetchSessionsPerGroup("RPi")
  if err != nil {
    return err
  }
  if len(sessionsPerGroup) == 0 {
    fmt.Println("No available session")
  } else {
    for g, sessions := range sessionsPerGroup {
      fmt.Println(g+":")
      for s, _ := range sessions {
        fmt.Printf("- %s\n",s)
      }
    }
  }
  return nil
}

func main() {
  logwriter, err := syslog.New(syslog.LOG_DEBUG, "i3tmux")
  if err != nil {
    log.Fatal(err)
  }
  log.SetOutput(logwriter)

  flag.Parse()
  modsCount := 0
  if *addMode { modsCount++ }
  if *detachMode { modsCount++ }
  if *resumeGroup != "" { modsCount++ }
  if *listMode { modsCount++ }
  if modsCount != 1 {
    log.Println("You must specify one option among 'a', 'd' and 'r'")
    os.Exit(1)
  }

  if *addMode {
    if err := addWindow(); err != nil {
      log.Fatal(err)
    }
  }
  if *detachMode {
    if err := detachSessionGroup(); err != nil {
      log.Fatal(err)
    }
  }
  if *resumeGroup != "" {
    if err := resumeSessionGroup(); err != nil {
      log.Fatal(err)
    }
  }
  if *listMode {
    if err := listSessionsGroup(); err != nil {
      log.Fatal(err)
    }
  }
}
