package main

import (
	"fmt"
	"log"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"go.i3wm.org/i3/v4"
)

func serializeGroupSess(group string, session string) string {
	return fmt.Sprintf("%s%s%s", group, GROUP_SESS_DELIM, session)
}

func serializeHostGroupSess(host, group, session string) string {
	return fmt.Sprintf("%s@%s", serializeGroupSess(group, session), host)
}

func deserializeHostGroupSessFromString(s string) (string, string, string, error) {
	split := strings.Split(s, HOST_DELIM)
	if len(split) != 2 {
		return "", "", "", fmt.Errorf("name not in GROUP%sSESSION%sHOST format: %s",
			GROUP_SESS_DELIM,
			HOST_DELIM,
			s)
	}
	host := split[1]
	group, sess, err := deserializeGroupSessFromString(split[0])
	if err != nil {
		return "", "", "", err
	}
	return host, group, sess, nil
}

func deserializeGroupSessFromString(s string) (string, string, error) {
	split := strings.Split(s, GROUP_SESS_DELIM)
	if len(split) != 2 {
		return "", "", fmt.Errorf("name not in GROUP%sSESSION format: %s",
			GROUP_SESS_DELIM,
			s)
	}
	return split[0], split[1], nil
}

func deserializeHostGroupSessFromCon(con *i3.Node) (string, string, string, error) {
	return deserializeHostGroupSessFromString(con.WindowProperties.Instance)
}

func deserializeGroupSessFromCon(con *i3.Node) (string, string, error) {
	return deserializeGroupSessFromString(con.WindowProperties.Instance)
}

func fetchSessionsPerGroup(host string) (SessionsPerGroup, error) {
	cmd := exec.Command("ssh", host, `tmux ls -F "#{session_name}"`)
	out, err := cmd.CombinedOutput()
	// For simplicity's sake we assume that if the command succeeds
	// stderr messages do not pollute stdout
	outStr := string(out)
	if err != nil {
		if strings.HasPrefix(outStr, "no server running on ") {
			return nil, TmuxNoSessionsError
		}
		return nil, err
	}
	lines := strings.Split(outStr, "\n")
	sessionsPerGroup := make(map[string]map[string]bool)
	for _, l := range lines {
		group, session, err := deserializeGroupSessFromString(l)
		if err != nil {
			// Skip unrecognized format
			continue
		}
		if _, ok := sessionsPerGroup[group]; !ok {
			sessionsPerGroup[group] = make(map[string]bool)
		}
		sessionsPerGroup[group][session] = true
	}
	return sessionsPerGroup, nil
}

func getNextSessIdx(sessionsPerGroup SessionsPerGroup, group string) (int, error) {
	sessions := sessionsPerGroup[group]
	var idxs []int
	for s, _ := range sessions {
		res := sessionFmtRe.FindStringSubmatch(s)
		if len(res) != 2 {
			return -1, fmt.Errorf("malformed session '%s'", s)
		}
		i, err := strconv.Atoi(res[1])
		if err != nil {
			return -1, err
		}
		idxs = append(idxs, i)
	}
	sort.Ints(idxs)
	for i, idx := range idxs {
		if i < idx {
			return i, nil
		}
	}
	return len(sessions), nil
}

func launchTermForSession(host, group, session, terminalBin, nameFlag string) error {
	sshCmd := fmt.Sprintf("ssh -t %s tmux attach -t %s", host, serializeGroupSess(group, session))
	log.Println(sshCmd)
	i3cmd := fmt.Sprintf("exec %s %s '%s' %s",
		terminalBin,
		nameFlag,
		serializeHostGroupSess(host, group, session),
		sshCmd)
	_, err := i3.RunCommand(i3cmd)
	return err
}
