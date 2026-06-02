package process

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

type Process struct {
	PID     int    `json:"pid"`
	PPID    int    `json:"ppid"`
	Command string `json:"command"`
	CPU     string `json:"cpu"`
	Memory  string `json:"memory"`
	Elapsed string `json:"elapsed,omitempty"`
}

type Detail struct {
	PID   int      `json:"pid"`
	Kind  string   `json:"kind"`
	Items []string `json:"items"`
}

type PortExplanation struct {
	Port    int     `json:"port"`
	Owner   Process `json:"owner"`
	Message string  `json:"message"`
}

func List(ctx context.Context) ([]Process, error) {
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,pcpu=,pmem=,comm=")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return parsePSList(string(output)), nil
}

func Inspect(ctx context.Context, pid int) (Process, error) {
	cmd := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "pid=,ppid=,pcpu=,pmem=,etime=,comm=")
	output, err := cmd.Output()
	if err != nil {
		return Process{}, err
	}
	processes := parsePSList(string(output))
	if len(processes) == 0 {
		return Process{}, exec.ErrNotFound
	}
	return processes[0], nil
}

func Search(processes []Process, query string) []Process {
	query = strings.ToLower(query)
	var out []Process
	for _, proc := range processes {
		if strings.Contains(strings.ToLower(proc.Command), query) || strconv.Itoa(proc.PID) == query {
			out = append(out, proc)
		}
	}
	return out
}

func ChildrenOf(processes []Process, pid int) []Process {
	var out []Process
	for _, proc := range processes {
		if proc.PPID == pid {
			out = append(out, proc)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PID < out[j].PID })
	return out
}

func ParentOf(processes []Process, pid int) (Process, bool) {
	byPID := indexByPID(processes)
	proc, ok := byPID[pid]
	if !ok {
		return Process{}, false
	}
	parent, ok := byPID[proc.PPID]
	return parent, ok
}

func AncestryOf(processes []Process, pid int) []Process {
	byPID := indexByPID(processes)
	var out []Process
	seen := map[int]bool{}
	for {
		proc, ok := byPID[pid]
		if !ok || seen[pid] {
			break
		}
		out = append(out, proc)
		seen[pid] = true
		if proc.PPID == 0 {
			break
		}
		pid = proc.PPID
	}
	return out
}

func FamilyOf(processes []Process, pid int) []Process {
	out := AncestryOf(processes, pid)
	out = append(out, ChildrenOf(processes, pid)...)
	return out
}

func Top(processes []Process, field string, limit int) []Process {
	if limit <= 0 {
		limit = 10
	}
	sort.Slice(processes, func(i, j int) bool {
		left := parseMetric(processes[i].CPU)
		right := parseMetric(processes[j].CPU)
		if field == "memory" {
			left = parseMetric(processes[i].Memory)
			right = parseMetric(processes[j].Memory)
		}
		return left > right
	})
	if len(processes) < limit {
		return processes
	}
	return processes[:limit]
}

func Kill(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}

func PortOwner(ctx context.Context, port int) (Process, error) {
	cmd := exec.CommandContext(ctx, "lsof", "-tiTCP:"+strconv.Itoa(port), "-sTCP:LISTEN")
	out, err := cmd.Output()
	if err != nil {
		return Process{}, err
	}
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return Process{}, fmt.Errorf("no process is listening on port %d", port)
	}
	pid, err := strconv.Atoi(fields[0])
	if err != nil {
		return Process{}, err
	}
	return Inspect(ctx, pid)
}

func ExplainPort(ctx context.Context, port int) (PortExplanation, error) {
	owner, err := PortOwner(ctx, port)
	if err != nil {
		return PortExplanation{}, err
	}
	return PortExplanation{Port: port, Owner: owner, Message: fmt.Sprintf("port %d is owned by pid %d (%s)", port, owner.PID, owner.Command)}, nil
}

func KillPort(ctx context.Context, port int) error {
	owner, err := PortOwner(ctx, port)
	if err != nil {
		return err
	}
	return Kill(owner.PID)
}

func Lsof(ctx context.Context, pid int, kind string) (Detail, error) {
	args := []string{"-p", strconv.Itoa(pid), "-nP"}
	if kind == "sockets" {
		args = []string{"-a", "-p", strconv.Itoa(pid), "-i", "-nP"}
	}
	cmd := exec.CommandContext(ctx, "lsof", args...)
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return Detail{PID: pid, Kind: kind, Items: []string{}}, nil
	}
	return Detail{PID: pid, Kind: kind, Items: nonemptyLines(string(out))}, nil
}

func Command(ctx context.Context, pid int) (Detail, error) {
	cmd := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-ww", "-o", "command=")
	out, err := cmd.Output()
	if err != nil {
		return Detail{}, err
	}
	return Detail{PID: pid, Kind: "command", Items: []string{strings.TrimSpace(string(out))}}, nil
}

func Env(ctx context.Context, pid int) (Detail, error) {
	cmd := exec.CommandContext(ctx, "ps", "eww", "-p", strconv.Itoa(pid), "-o", "command=")
	out, err := cmd.Output()
	if err != nil {
		return Detail{}, err
	}
	return Detail{PID: pid, Kind: "env", Items: strings.Fields(strings.TrimSpace(string(out)))}, nil
}

func CWD(ctx context.Context, pid int) (Detail, error) {
	cmd := exec.CommandContext(ctx, "lsof", "-a", "-p", strconv.Itoa(pid), "-d", "cwd", "-Fn")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return Detail{}, err
	}
	var items []string
	for _, line := range nonemptyLines(string(out)) {
		if strings.HasPrefix(line, "n") {
			items = append(items, strings.TrimPrefix(line, "n"))
		}
	}
	return Detail{PID: pid, Kind: "cwd", Items: items}, nil
}

func parsePSList(input string) []Process {
	scanner := bufio.NewScanner(strings.NewReader(input))
	var processes []Process
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		ppid, _ := strconv.Atoi(fields[1])
		proc := Process{
			PID:    pid,
			PPID:   ppid,
			CPU:    fields[2],
			Memory: fields[3],
		}
		if len(fields) > 5 && isElapsed(fields[4]) {
			proc.Elapsed = fields[4]
			proc.Command = strings.Join(fields[5:], " ")
		} else {
			proc.Command = strings.Join(fields[4:], " ")
		}
		processes = append(processes, proc)
	}
	return processes
}

func isElapsed(value string) bool {
	return strings.Contains(value, ":") || strings.Contains(value, "-")
}

func indexByPID(processes []Process) map[int]Process {
	byPID := make(map[int]Process, len(processes))
	for _, proc := range processes {
		byPID[proc.PID] = proc
	}
	return byPID
}

func parseMetric(value string) float64 {
	metric, _ := strconv.ParseFloat(value, 64)
	return metric
}

func nonemptyLines(value string) []string {
	var out []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
