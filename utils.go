package main

import (
	"fmt"
	"log"
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

func fetchSessionsPerGroup(host string, conf *Conf) (SessionsPerGroup, error) {
	client, err := NewClient(host, conf)
	if err != nil {
		return nil, fmt.Errorf("unable to create client: %w", err)
	}
	stdout, stderr, err := client.Run(`tmux ls -F "#{session_name}"`)
	if err != nil {
		if strings.Contains(stderr, "no server running on ") ||
			strings.Contains(stderr, "No such file or directory") {
			return nil, TmuxNoSessionsError
		}
		return nil, fmt.Errorf("%s: %w", stderr, err)
	}
	lines := strings.Split(stdout, "\n")
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

func addSessionToGroup(host, group string, conf *Conf) (string, error) {
	sessionsPerGroup, err := fetchSessionsPerGroup(host, conf)
	if err != nil {
		return "", err
	}
	nextSessIdx, err := getNextSessIdx(sessionsPerGroup, group)
	if err != nil {
		return "", err
	}
	nextSess := fmt.Sprintf("session%d", nextSessIdx)
	log.Println("Adding session to group", group, nextSess)
	err = createSession(host, group, nextSess, conf)
	if err != nil {
		return "", err
	}
	return nextSess, nil
}

func createSession(host, group, sess string, conf *Conf) error {
	client, err := NewClient(host, conf)
	if err != nil {
		return fmt.Errorf("unable to create client: %w", err)
	}
	cmd := fmt.Sprintf("tmux new -d -s %s", serializeGroupSess(group, sess))
	_, _, err = client.Run(cmd)
	return err
}

func killSession(host, group, sess string, conf *Conf) error {
	client, err := NewClient(host, conf)
	if err != nil {
		return fmt.Errorf("unable to create client: %w", err)
	}
	groupSess := serializeGroupSess(group, sess)
	cmd := fmt.Sprintf("tmux kill-session -t %s", groupSess)
	_, stderr, err := client.Run(cmd)
	if err != nil {
		return fmt.Errorf("unable to execute remote cmd: %s, %s, %w", cmd, stderr, err)
	}
	return nil
}
