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

func getNextSessIdx(sessionsPerGroup SessionsPerGroup, group string) (int, error) {
	sessions := sessionsPerGroup[group]
	var idxs []int
	for s := range sessions {
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

func launchTermForSession(group, session, host string) error {
	groupSess := serializeGroupSess(group, session)
	hostGroupSess := serializeHostGroupSess(host, group, session)
	cmd := exec.Command(pref.Terminal.Bin,
		pref.Terminal.NameFlag, hostGroupSess,
		"-e", I3TMUX_BIN,
		"-host", host,
		"-shell",
		"-session", groupSess)
	log.Printf("%+v", cmd)
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("launching cmd %s: %w", cmd, err)
	}
	return nil
}

func parseSessionsPerGroup(lines []string) SessionsPerGroup {
	sessions := make(SessionsPerGroup)
	for _, l := range lines {
		group, session, err := deserializeGroupSessFromString(l)
		if err != nil {
			// Skip unrecognized format
			continue
		}
		if _, ok := sessions[group]; !ok {
			sessions[group] = make(map[string]bool)
		}
		sessions[group][session] = true
	}
	return sessions
}

func fetchSessionsPerGroup(sshClient *SSHClient) (SessionsPerGroup, int, string) {
	stdout, stderr, err := sshClient.Run(`tmux ls -F "#{session_name}"`)
	if err != nil {
		if strings.Contains(stderr, "no server running on ") ||
			strings.Contains(stderr, "No such file or directory") {
			// return nil, TmuxNoSessionsError
			return nil, TmuxNoSessionsError, stderr
		}
		return nil, UnknownError, fmt.Sprintf("%s: %s", stderr, err)
	}
	lines := strings.Split(stdout, "\n")
	return parseSessionsPerGroup(lines), ErrOk, ""
}

func createSession(group, session string, sshClient *SSHClient) (string, string, error) {
	sessionGroup := serializeGroupSess(group, session)
	cmd := fmt.Sprintf("tmux new -d -s %s", sessionGroup)
	return sshClient.Run(cmd)
}
