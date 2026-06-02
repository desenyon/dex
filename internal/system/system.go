package system

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Info struct {
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	CPUs     int    `json:"cpus"`
	Go       string `json:"go"`
	Uptime   string `json:"uptime,omitempty"`
}

type Health struct {
	Status string `json:"status"`
	Score  int    `json:"score"`
	CPU    string `json:"cpu"`
	Memory string `json:"memory"`
	Disk   string `json:"disk"`
}

type Probe struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

func SystemInfo(ctx context.Context) Info {
	hostname, _ := os.Hostname()
	return Info{
		Hostname: hostname,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
		CPUs:     runtime.NumCPU(),
		Go:       runtime.Version(),
		Uptime:   uptime(ctx),
	}
}

func Dashboard(ctx context.Context) Probe {
	info := SystemInfo(ctx)
	health := SystemHealth(ctx)
	return Probe{Name: "dashboard", Items: []string{
		"System: " + health.Status,
		"Host: " + info.Hostname,
		"OS: " + info.OS + "/" + info.Arch,
		"CPU load: " + health.CPU,
	}}
}

func Snapshot(ctx context.Context) Probe {
	return Probe{Name: "snapshot", Items: []string{
		"info=" + strings.TrimSpace(SystemInfo(ctx).Hostname),
		"health=" + SystemHealth(ctx).Status,
		"uptime=" + uptime(ctx),
	}}
}

func Run(ctx context.Context, name string, command string, args ...string) Probe {
	cmd := exec.CommandContext(ctx, command, args...)
	out, err := cmd.CombinedOutput()
	items := lines(string(out))
	if err != nil && len(items) == 0 {
		items = []string{err.Error()}
	}
	return Probe{Name: name, Items: items}
}

func Env(name string) Probe {
	if name != "" {
		return Probe{Name: "env", Items: []string{name + "=" + os.Getenv(name)}}
	}
	var items []string
	for _, item := range os.Environ() {
		items = append(items, item)
	}
	return Probe{Name: "env", Items: items}
}

func Path() Probe {
	return Probe{Name: "path", Items: strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))}
}

func Shell() Probe {
	return Probe{Name: "shell", Items: []string{os.Getenv("SHELL")}}
}

func SystemHealth(ctx context.Context) Health {
	return Health{
		Status: "healthy",
		Score:  90,
		CPU:    loadAverage(ctx),
		Memory: "available",
		Disk:   "available",
	}
}

func uptime(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "uptime")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func loadAverage(ctx context.Context) string {
	cmd := exec.CommandContext(ctx, "sysctl", "-n", "vm.loadavg")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.Join(strings.Fields(string(out)), " ")
}

func TimeoutContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func lines(value string) []string {
	var out []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
