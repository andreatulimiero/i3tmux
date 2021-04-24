package main

import (
	"fmt"
	"go.i3wm.org/i3/v4"
)

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

// getTreeOfGroupSess traverses the tree of nodes u
// looking for i3tmux sessions
// FIXME: look for windows beloning to a host only
func getTreeOfGroupSess(u *i3.Node) map[string]interface{} {
	if nodeIsLeaf(u) {
		host, group, session, err := deserializeHostGroupSessFromCon(u)
		if err != nil {
			// We care about tmux session leaves only
			return nil
		}
		m := make(map[string]interface{})
		m["type"] = i3.Con
		m["swallows"] = []map[string]string{{"instance": serializeHostGroupSess(host, group, session)}}
		return m
	} else {
		var nodes []map[string]interface{}
		for _, v := range u.Nodes {
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
			// TODO: Make this an option. If optimization is not done it should
			//       be easier to recreate an entire workspace layout with other
			//       applications (e.g., browser)
			return nodes[0]
		default:
			m := make(map[string]interface{})
			m["layout"] = u.Layout
			m["type"] = i3.Con
			m["percent"] = u.Percent
			m["nodes"] = nodes
			return m
		}
	}
}

func closeGroupSessWindows(u *i3.Node, group *string) error {
	for _, v := range u.Nodes {
		err := closeGroupSessWindows(v, group)
		if err != nil {
			return err
		}
	}
	_, g, _, err := deserializeHostGroupSessFromCon(u)
	if err != nil || g != *group {
		return nil
		// Just skip container since not targeted
	}
	_, err = i3.RunCommand(fmt.Sprintf("[con_id=%d] kill", u.ID))
	if err != nil {
		return err
	}
	return nil
}
