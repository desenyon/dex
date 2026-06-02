package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/desenyon/dex/internal/api"
	"github.com/desenyon/dex/internal/bench"
	"github.com/desenyon/dex/internal/jsonx"
	"github.com/desenyon/dex/internal/network"
	"github.com/desenyon/dex/internal/output"
	"github.com/desenyon/dex/internal/process"
	"github.com/desenyon/dex/internal/regexx"
	"github.com/desenyon/dex/internal/storage"
	"github.com/desenyon/dex/internal/system"
	"github.com/desenyon/dex/internal/tui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type globalOptions struct {
	format   output.Format
	interval time.Duration
	noColor  bool
	theme    string
	profile  string
	export   string
	watch    bool
	save     bool
	copy     bool
	quiet    bool
	verbose  bool
}

type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

const (
	httpMethodGet  = "GET"
	httpMethodHead = "HEAD"
)

func NewRoot(stdout io.Writer, stderr io.Writer, build ...BuildInfo) *cobra.Command {
	options := &globalOptions{format: output.Text, interval: time.Second, theme: "dark", profile: "default"}
	info := BuildInfo{Version: "dev", Commit: "local"}
	if len(build) > 0 {
		info = build[0]
	}
	root := &cobra.Command{
		Use:           "dex",
		Short:         "Dex is a local-first developer command center.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if options.format == output.Text && term.IsTerminal(int(syscall.Stdout)) {
				ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
				defer cancel()
				return tui.Run(tui.NewModel(dashboardSnapshot(ctx)))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			return writeRendered(cmd, options, system.Dashboard(ctx))
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		if err := ensureStorage(); err != nil {
			return err
		}
		return resolveFormat(cmd, options)
	}

	flags := root.PersistentFlags()
	flags.Bool("json", false, "output JSON")
	flags.Bool("csv", false, "output CSV")
	flags.Bool("markdown", false, "output Markdown")
	flags.Bool("raw", false, "output raw values")
	flags.BoolVar(&options.watch, "watch", false, "watch command output")
	flags.DurationVar(&options.interval, "interval", time.Second, "watch interval")
	flags.BoolVar(&options.noColor, "no-color", false, "disable color")
	flags.StringVar(&options.theme, "theme", "dark", "theme name")
	flags.StringVar(&options.profile, "profile", "default", "profile name")
	flags.BoolVar(&options.save, "save", false, "save command result when supported")
	flags.StringVar(&options.export, "export", "", "export output to path")
	flags.BoolVar(&options.copy, "copy", false, "copy output when supported")
	flags.BoolVar(&options.quiet, "quiet", false, "reduce nonessential output")
	flags.BoolVar(&options.verbose, "verbose", false, "show verbose output")

	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show Dex version.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeRendered(cmd, options, info)
		},
	})

	root.AddCommand(
		newNetworkCommand(options),
		newProcessCommand(options),
		newAPICommand(options),
		newJSONCommand(options),
		newSystemCommand(options),
		newRegexCommand(options),
		newBenchCommand(options),
		newFilesCommand(options),
		newClipboardCommand(options),
		newTerminalCommand(options),
		newSettingsCommand(options),
	)
	wrapWatch(root, options)
	return root
}

func resolveFormat(cmd *cobra.Command, options *globalOptions) error {
	selected := 0
	for _, item := range []struct {
		flag   string
		format output.Format
	}{
		{"json", output.JSON},
		{"csv", output.CSV},
		{"markdown", output.Markdown},
		{"raw", output.Raw},
	} {
		enabled, err := cmd.Flags().GetBool(item.flag)
		if err != nil {
			return err
		}
		if enabled {
			selected++
			options.format = item.format
		}
	}
	if selected > 1 {
		return fmt.Errorf("choose only one output format")
	}
	return nil
}

func writeRendered(cmd *cobra.Command, options *globalOptions, value any) error {
	rendered, err := output.Render(output.Options{Format: options.format, Quiet: options.quiet}, value)
	if err != nil {
		return err
	}
	if options.export != "" {
		if err := os.WriteFile(options.export, []byte(rendered), 0o600); err != nil {
			return err
		}
	}
	if options.copy {
		copyCmd := exec.Command("pbcopy")
		copyCmd.Stdin = strings.NewReader(rendered)
		if err := copyCmd.Run(); err != nil {
			return err
		}
	}
	if options.save {
		root, err := storage.Root()
		if err != nil {
			return err
		}
		if err := storage.AppendHistory(root, storage.HistoryEntry{
			Command:    strings.Join(os.Args, " "),
			Format:     string(options.format),
			OutputPath: options.export,
		}); err != nil {
			return err
		}
	}
	_, err = io.WriteString(cmd.OutOrStdout(), rendered)
	return err
}

func writeList(cmd *cobra.Command, options *globalOptions, structured any, records []output.Record) error {
	if options.format == output.JSON {
		return writeRendered(cmd, options, structured)
	}
	return writeRendered(cmd, options, records)
}

func wrapWatch(cmd *cobra.Command, options *globalOptions) {
	if cmd.RunE != nil {
		original := cmd.RunE
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			if !options.watch {
				return original(cmd, args)
			}
			for {
				if err := original(cmd, args); err != nil {
					return err
				}
				select {
				case <-cmd.Context().Done():
					return nil
				case <-time.After(options.interval):
				}
			}
		}
	}
	for _, child := range cmd.Commands() {
		wrapWatch(child, options)
	}
}

func ensureStorage() error {
	root, err := storage.Root()
	if err != nil {
		return err
	}
	config, err := storage.LoadConfig(root)
	if err != nil {
		return err
	}
	if config.Theme == "" {
		config.Theme = "dark"
	}
	if config.Profile == "" {
		config.Profile = "default"
	}
	return storage.SaveConfig(root, config)
}

func dashboardSnapshot(ctx context.Context) tui.Snapshot {
	health := system.SystemHealth(ctx)
	snapshot := tui.Snapshot{SystemStatus: health.Status}
	if ports, err := network.ListeningPorts(ctx); err == nil {
		snapshot.Connections = len(ports)
	}
	if processes, err := process.List(ctx); err == nil {
		snapshot.Processes = len(processes)
	}
	if root, err := storage.Root(); err == nil {
		if entries, err := storage.ListHistory(root, 500); err == nil {
			snapshot.RecentJSON = len(entries)
		}
		if items, err := os.ReadDir(filepath.Join(root, "api")); err == nil {
			snapshot.Collections = len(items)
		}
	}
	return snapshot
}

func newNetworkCommand(options *globalOptions) *cobra.Command {
	networkCmd := &cobra.Command{Use: "network", Short: "Inspect local network state."}

	var includeAll bool
	ipCmd := &cobra.Command{
		Use:   "ip",
		Short: "Show local IP addresses.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ips, err := network.IPs(includeAll)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, ipRecords(ips))
		},
	}
	ipCmd.Flags().BoolVar(&includeAll, "all", false, "include loopback and down interfaces")

	publicIPCmd := &cobra.Command{
		Use:   "public-ip",
		Short: "Show public IP address.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			ip, err := network.PublicIP(ctx)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, output.Record{"public_ip": ip})
		},
	}

	var interfacesAll bool
	interfacesCmd := &cobra.Command{
		Use:   "interfaces",
		Short: "Show network interfaces.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			items, err := network.Interfaces(interfacesAll)
			if err != nil {
				return err
			}
			return writeList(cmd, options, items, interfaceRecords(items))
		},
	}
	interfacesCmd.Flags().BoolVar(&interfacesAll, "all", false, "include down interfaces")

	interfaceCmd := &cobra.Command{
		Use:   "interface <name>",
		Short: "Show one network interface.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			item, err := network.InterfaceByName(args[0])
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, item)
		},
	}

	macCmd := &cobra.Command{
		Use:   "mac",
		Short: "Show interface MAC addresses.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			items, err := network.MACAddresses()
			if err != nil {
				return err
			}
			return writeList(cmd, options, items, interfaceRecords(items))
		},
	}

	hostnameCmd := &cobra.Command{
		Use:   "hostname",
		Short: "Show local hostname.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			hostname, err := network.Hostname()
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, output.Record{"hostname": hostname})
		},
	}

	portsCmd := &cobra.Command{
		Use:   "ports",
		Short: "Show listening TCP ports.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			ports, err := network.ListeningPorts(ctx)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, portRecords(ports))
		},
	}
	portsCmd.Flags().Bool("listening", true, "show listening ports")

	var tcpPort int
	var includeHTTP bool
	latencyCmd := &cobra.Command{
		Use:   "latency <host-or-url>",
		Short: "Measure DNS, TCP, and optional HTTP latency.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			port := tcpPort
			if port == 0 {
				port = network.DefaultTCPPort(args[0])
			}
			result, err := network.Latency(ctx, args[0], port, includeHTTP)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, result)
		},
	}
	latencyCmd.Flags().IntVar(&tcpPort, "tcp", 0, "TCP port to probe")
	latencyCmd.Flags().BoolVar(&includeHTTP, "http", false, "include HTTP HEAD timing")

	gatewayCmd := &cobra.Command{
		Use:   "gateway",
		Short: "Show default gateway.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			gateway, err := network.Gateway(ctx)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, output.Record{"gateway": gateway})
		},
	}

	routesCmd := &cobra.Command{
		Use:   "routes",
		Short: "Show routing table.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			routes, err := network.Routes(ctx)
			if err != nil {
				return err
			}
			return writeList(cmd, options, routes, routeRecords(routes))
		},
	}

	dnsConfigCmd := &cobra.Command{
		Use:   "dns-config",
		Short: "Show configured DNS resolvers.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			resolvers, err := network.DNSConfig(ctx)
			if err != nil {
				return err
			}
			return writeList(cmd, options, resolvers, dnsResolverRecords(resolvers))
		},
	}

	proxyCmd := &cobra.Command{
		Use:   "proxy",
		Short: "Show system proxy configuration.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			value, err := network.Proxy(ctx)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, value)
		},
	}

	vpnCmd := &cobra.Command{
		Use:   "vpn",
		Short: "Show configured VPN services.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			value, err := network.VPN(ctx)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, value)
		},
	}

	var dnsType string
	var dnsResolver string
	dnsCmd := &cobra.Command{
		Use:   "dns <domain>",
		Short: "Resolve DNS records.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			records, err := network.Lookup(ctx, args[0], dnsType, dnsResolver)
			if err != nil {
				return err
			}
			return writeList(cmd, options, records, dnsRecordRecords(records))
		},
	}
	dnsCmd.Flags().StringVar(&dnsType, "type", "A", "DNS record type: A, AAAA, MX, TXT, CNAME, NS")
	dnsCmd.Flags().StringVar(&dnsResolver, "resolver", "", "DNS resolver IP")

	headerCmd := networkHTTPCmd(options, "headers", "Show response headers.", httpMethodHead)
	statusCmd := networkHTTPCmd(options, "status", "Show HTTP status.", httpMethodHead)
	httpCmd := networkHTTPCmd(options, "http", "Probe an HTTP endpoint.", httpMethodGet)
	redirectCmd := networkHTTPCmd(options, "redirect-chain", "Show HTTP redirect chain.", httpMethodHead)
	cookiesCmd := networkHTTPCmd(options, "cookies", "Show response cookies.", httpMethodHead)
	compressionCmd := networkHTTPCmd(options, "compression", "Show response compression headers.", httpMethodHead)
	cacheCmd := networkHTTPCmd(options, "cache", "Show response cache headers.", httpMethodHead)
	corsCmd := networkHTTPCmd(options, "cors", "Show response CORS headers.", httpMethodHead)
	http2Cmd := networkHTTPCmd(options, "http2", "Show negotiated HTTP protocol.", httpMethodHead)

	tlsCmd := networkTLSCmd(options, "ssl", "Show TLS certificate summary.", false)
	certCmd := networkTLSCmd(options, "cert", "Show leaf certificate.", true)
	certChainCmd := networkTLSCmd(options, "cert-chain", "Show certificate chain summary.", false)
	certExpiryCmd := networkTLSCmd(options, "cert-expiry", "Show certificate expiry.", false)
	tlsVersionCmd := networkTLSCmd(options, "tls-version", "Show TLS version.", false)
	cipherCmd := networkTLSCmd(options, "cipher", "Show TLS cipher.", false)
	handshakeCmd := networkTLSCmd(options, "handshake", "Show TLS handshake result.", false)

	networkCmd.AddCommand(
		ipCmd, publicIPCmd, interfacesCmd, interfaceCmd, macCmd, hostnameCmd, gatewayCmd, routesCmd, dnsConfigCmd, proxyCmd, vpnCmd,
		portsCmd, latencyCmd, dnsCmd,
		httpCmd, headerCmd, statusCmd, redirectCmd, cookiesCmd, compressionCmd, cacheCmd, corsCmd, http2Cmd,
		tlsCmd, certCmd, certChainCmd, certExpiryCmd, tlsVersionCmd, cipherCmd, handshakeCmd,
	)
	return networkCmd
}

func networkHTTPCmd(options *globalOptions, name string, short string, method string) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <url>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()
			probe, err := network.HTTP(ctx, args[0], method)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, probe)
		},
	}
}

func networkTLSCmd(options *globalOptions, name string, short string, includePEM bool) *cobra.Command {
	return &cobra.Command{
		Use:   name + " <host>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 15*time.Second)
			defer cancel()
			info, err := network.TLSInfo(ctx, args[0], includePEM)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, info)
		},
	}
}

func newProcessCommand(options *globalOptions) *cobra.Command {
	processCmd := &cobra.Command{Use: "process", Short: "Inspect local processes."}
	processCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List running processes.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			processes, err := process.List(ctx)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, processRecords(processes))
		},
	})
	processCmd.AddCommand(&cobra.Command{
		Use:   "inspect <pid>",
		Short: "Inspect one process.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid pid %q", args[0])
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			proc, err := process.Inspect(ctx, pid)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, processRecords([]process.Process{proc}))
		},
	})

	processCmd.AddCommand(&cobra.Command{
		Use:   "search <query>",
		Short: "Search running processes.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			processes, err := process.List(ctx)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, processRecords(process.Search(processes, args[0])))
		},
	})

	processCmd.AddCommand(&cobra.Command{
		Use:   "tree",
		Short: "Show process tree rows.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			processes, err := process.List(ctx)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, processRecords(processes))
		},
	})

	for _, spec := range []struct {
		name  string
		short string
		run   func([]process.Process, int) []process.Process
	}{
		{"children", "Show process children.", process.ChildrenOf},
		{"ancestry", "Show process ancestry.", process.AncestryOf},
		{"family", "Show process family.", process.FamilyOf},
	} {
		spec := spec
		processCmd.AddCommand(&cobra.Command{
			Use:   spec.name + " <pid>",
			Short: spec.short,
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				pid, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid pid %q", args[0])
				}
				ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
				defer cancel()
				processes, err := process.List(ctx)
				if err != nil {
					return err
				}
				return writeRendered(cmd, options, processRecords(spec.run(processes, pid)))
			},
		})
	}

	processCmd.AddCommand(&cobra.Command{
		Use:   "parent <pid>",
		Short: "Show process parent.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid pid %q", args[0])
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			processes, err := process.List(ctx)
			if err != nil {
				return err
			}
			parent, ok := process.ParentOf(processes, pid)
			if !ok {
				return fmt.Errorf("parent not found for pid %d", pid)
			}
			return writeRendered(cmd, options, processRecords([]process.Process{parent}))
		},
	})

	var topLimit int
	topCmd := &cobra.Command{
		Use:   "top",
		Short: "Show top processes by CPU.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProcessTop(cmd, options, "cpu", topLimit)
		},
	}
	topCmd.Flags().IntVar(&topLimit, "limit", 10, "number of processes")
	processCmd.AddCommand(topCmd)
	processCmd.AddCommand(&cobra.Command{
		Use:   "cpu",
		Short: "Show top processes by CPU.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProcessTop(cmd, options, "cpu", 10)
		},
	})
	processCmd.AddCommand(&cobra.Command{
		Use:   "memory",
		Short: "Show top processes by memory.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProcessTop(cmd, options, "memory", 10)
		},
	})

	for _, spec := range []struct {
		name string
		run  func(context.Context, int) (process.Detail, error)
	}{
		{"sockets", func(ctx context.Context, pid int) (process.Detail, error) { return process.Lsof(ctx, pid, "sockets") }},
		{"files", func(ctx context.Context, pid int) (process.Detail, error) { return process.Lsof(ctx, pid, "files") }},
		{"env", process.Env},
		{"cwd", process.CWD},
		{"command", process.Command},
		{"limits", func(_ context.Context, pid int) (process.Detail, error) {
			return process.Detail{PID: pid, Kind: "limits", Items: []string{"per-process limits are platform-dependent on macOS; use `launchctl limit` for system defaults"}}, nil
		}},
	} {
		spec := spec
		processCmd.AddCommand(&cobra.Command{
			Use:   spec.name + " <pid>",
			Short: "Show process " + spec.name + ".",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				pid, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("invalid pid %q", args[0])
				}
				ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
				defer cancel()
				detail, err := spec.run(ctx, pid)
				if err != nil {
					return err
				}
				return writeRendered(cmd, options, detail)
			},
		})
	}

	processCmd.AddCommand(&cobra.Command{
		Use:   "kill <pid>",
		Short: "Terminate a process with SIGTERM.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid pid %q", args[0])
			}
			if err := process.Kill(pid); err != nil {
				return err
			}
			return writeRendered(cmd, options, output.Record{"pid": pid, "signal": "SIGTERM"})
		},
	})
	processCmd.AddCommand(&cobra.Command{
		Use:   "kill-port <port>",
		Short: "Terminate the process listening on a port.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			port, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid port %q", args[0])
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			if err := process.KillPort(ctx, port); err != nil {
				return err
			}
			return writeRendered(cmd, options, output.Record{"port": port, "signal": "SIGTERM"})
		},
	})
	processCmd.AddCommand(&cobra.Command{
		Use:   "explain-port <port>",
		Short: "Show the process that owns a listening port.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			port, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid port %q", args[0])
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			explanation, err := process.ExplainPort(ctx, port)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, explanation)
		},
	})

	processCmd.AddCommand(&cobra.Command{
		Use:   "heat",
		Short: "Show process resource heat.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runProcessTop(cmd, options, "cpu", 15)
		},
	})
	processCmd.AddCommand(&cobra.Command{
		Use:   "ghost",
		Short: "Show suspicious orphan-like processes.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			processes, err := process.List(ctx)
			if err != nil {
				return err
			}
			var ghosts []process.Process
			for _, proc := range processes {
				if proc.PPID == 1 && proc.PID != 1 {
					ghosts = append(ghosts, proc)
				}
			}
			return writeRendered(cmd, options, processRecords(ghosts))
		},
	})
	return processCmd
}

func newAPICommand(options *globalOptions) *cobra.Command {
	apiCmd := &cobra.Command{Use: "api", Short: "Send, inspect, and shape API requests."}
	for _, spec := range []struct {
		name   string
		method string
		body   bool
	}{
		{"get", "GET", false},
		{"post", "POST", true},
		{"put", "PUT", true},
		{"patch", "PATCH", true},
		{"delete", "DELETE", false},
	} {
		spec := spec
		var bodyPath string
		cmd := &cobra.Command{
			Use:   spec.name + " <url>",
			Short: spec.method + " an API endpoint.",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
				defer cancel()
				response, err := api.Request(ctx, spec.method, args[0], bodyPath)
				if err != nil {
					return err
				}
				return writeRendered(cmd, options, response)
			},
		}
		if spec.body {
			cmd.Flags().StringVar(&bodyPath, "body", "", "JSON request body file")
		}
		apiCmd.AddCommand(cmd)
	}
	apiCmd.AddCommand(&cobra.Command{
		Use:   "schema <response.json>",
		Short: "Infer a simple response schema.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer file.Close()
			schema, err := api.InferSchema(file)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, pathStringValueRecords(schema))
		},
	})
	apiCmd.AddCommand(&cobra.Command{
		Use:   "assert status <code>",
		Short: "Represent an API status assertion.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "status" {
				return fmt.Errorf("only status assertions are implemented in this pass")
			}
			return writeRendered(cmd, options, output.Record{"assert": "status", "expected": args[1]})
		},
	})
	apiCmd.AddCommand(localStateCommand(options, "collection", "Manage local API collections.", "ls", "-la", os.ExpandEnv("$HOME/.dex/api")))
	apiCmd.AddCommand(&cobra.Command{
		Use:   "record <url> <session.dexapi>",
		Short: "Record one API response to a local session file.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			defer cancel()
			response, err := api.Request(ctx, "GET", args[0], "")
			if err != nil {
				return err
			}
			session := api.Session{Name: filepath.Base(args[1]), CreatedAt: time.Now().Format(time.RFC3339), Responses: []api.Response{response}}
			if err := api.SaveSession(args[1], session); err != nil {
				return err
			}
			return writeRendered(cmd, options, session)
		},
	})
	apiCmd.AddCommand(&cobra.Command{
		Use:   "replay <session.dexapi>",
		Short: "Replay a saved local API session.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			session, err := api.LoadSession(args[0])
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, session)
		},
	})
	apiCmd.AddCommand(localStateCommand(options, "test-report", "Show API test report location.", "ls", "-la", os.ExpandEnv("$HOME/.dex/api")))
	return apiCmd
}

func runProcessTop(cmd *cobra.Command, options *globalOptions, field string, limit int) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
	defer cancel()
	processes, err := process.List(ctx)
	if err != nil {
		return err
	}
	return writeRendered(cmd, options, processRecords(process.Top(processes, field, limit)))
}

func newBenchCommand(options *globalOptions) *cobra.Command {
	benchCmd := &cobra.Command{Use: "bench", Short: "Run local command benchmarks."}
	benchCmd.AddCommand(&cobra.Command{
		Use:   "run <command>",
		Short: "Time a command.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()
			result, err := bench.RunCommand(ctx, args[0])
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, result)
		},
	})
	benchCmd.AddCommand(localStateCommand(options, "history", "Show benchmark history.", "ls", "-la", os.ExpandEnv("$HOME/.dex/benchmarks")))
	benchCmd.AddCommand(localStateCommand(options, "export", "Show benchmark export directory.", "mkdir", "-p", os.ExpandEnv("$HOME/.dex/benchmarks")))
	benchCmd.AddCommand(localStateCommand(options, "trend", "Show benchmark trend data.", "ls", "-la", os.ExpandEnv("$HOME/.dex/benchmarks")))
	benchCmd.AddCommand(localStateCommand(options, "lab", "Show benchmark lab state.", "mkdir", "-p", os.ExpandEnv("$HOME/.dex/benchmarks")))
	return benchCmd
}

func newFilesCommand(options *globalOptions) *cobra.Command {
	filesCmd := &cobra.Command{Use: "files", Short: "Inspect local files."}
	filesCmd.AddCommand(localStateCommand(options, "tree", "Show file tree.", "find", ".", "-maxdepth", "3", "-print"))
	filesCmd.AddCommand(localStateCommand(options, "size", "Show directory size.", "du", "-sh", "."))
	filesCmd.AddCommand(localStateCommand(options, "largest", "Show largest files.", "sh", "-lc", "find . -type f -print0 | xargs -0 ls -lh 2>/dev/null | sort -k5 -hr | head -20"))
	filesCmd.AddCommand(localStateCommand(options, "duplicates", "Find duplicate file hashes.", "sh", "-lc", "find . -type f -maxdepth 4 -print0 | xargs -0 shasum 2>/dev/null | sort | uniq -w 40 -d"))
	filesCmd.AddCommand(localStateCommand(options, "search", "Search file names.", "find", ".", "-maxdepth", "5", "-type", "f"))
	filesCmd.AddCommand(localStateCommand(options, "recent", "Show recent files.", "sh", "-lc", "find . -type f -maxdepth 5 -print0 | xargs -0 ls -lt 2>/dev/null | head -25"))
	filesCmd.AddCommand(localStateCommand(options, "old", "Show old files.", "sh", "-lc", "find . -type f -maxdepth 5 -print0 | xargs -0 ls -ltr 2>/dev/null | head -25"))
	filesCmd.AddCommand(localStateCommand(options, "type-map", "Show file type counts.", "sh", "-lc", "find . -type f | sed 's/.*\\.//' | sort | uniq -c | sort -nr | head -30"))
	filesCmd.AddCommand(localStateCommand(options, "clean-preview", "Preview common disposable files.", "sh", "-lc", "find . \\( -name '.DS_Store' -o -name '*.tmp' -o -name '*.log' \\) -print"))
	return filesCmd
}

func newClipboardCommand(options *globalOptions) *cobra.Command {
	clipboardCmd := &cobra.Command{Use: "clipboard", Short: "Inspect the macOS clipboard."}
	clipboardCmd.AddCommand(localStateCommand(options, "history", "Show saved clipboard history.", "ls", "-la", os.ExpandEnv("$HOME/.dex/clipboard")))
	clipboardCmd.AddCommand(localStateCommand(options, "save", "Save current clipboard.", "sh", "-lc", "mkdir -p ~/.dex/clipboard && pbpaste > ~/.dex/clipboard/latest.txt && ls -l ~/.dex/clipboard/latest.txt"))
	clipboardCmd.AddCommand(localStateCommand(options, "search", "Search saved clipboard.", "sh", "-lc", "grep -R . ~/.dex/clipboard 2>/dev/null || true"))
	clipboardCmd.AddCommand(localStateCommand(options, "clear", "Clear saved clipboard history.", "sh", "-lc", "rm -rf ~/.dex/clipboard && mkdir -p ~/.dex/clipboard"))
	clipboardCmd.AddCommand(localStateCommand(options, "export", "Show clipboard export path.", "sh", "-lc", "mkdir -p ~/.dex/clipboard && printf '%s\n' ~/.dex/clipboard"))
	return clipboardCmd
}

func newTerminalCommand(options *globalOptions) *cobra.Command {
	terminalCmd := &cobra.Command{Use: "terminal", Short: "Inspect terminal sessions and history."}
	terminalCmd.AddCommand(localStateCommand(options, "history", "Show shell history tail.", "sh", "-lc", "tail -50 ~/.zsh_history 2>/dev/null || true"))
	terminalCmd.AddCommand(localStateCommand(options, "stats", "Show command history stats.", "sh", "-lc", "cut -d';' -f2- ~/.zsh_history 2>/dev/null | awk '{print $1}' | sort | uniq -c | sort -nr | head -20"))
	terminalCmd.AddCommand(localStateCommand(options, "aliases", "Show shell aliases.", "sh", "-lc", "zsh -ic alias"))
	terminalCmd.AddCommand(localStateCommand(options, "profile", "Show terminal profile env.", "sh", "-lc", "printf 'TERM=%s\\nTERM_PROGRAM=%s\\nSHELL=%s\\n' \"$TERM\" \"$TERM_PROGRAM\" \"$SHELL\""))
	terminalCmd.AddCommand(localStateCommand(options, "sessions", "Show terminal session storage.", "ls", "-la", os.ExpandEnv("$HOME/.dex/terminal")))
	terminalCmd.AddCommand(localStateCommand(options, "record", "Create terminal record directory.", "mkdir", "-p", os.ExpandEnv("$HOME/.dex/terminal")))
	terminalCmd.AddCommand(localStateCommand(options, "replay", "Show terminal recordings.", "ls", "-la", os.ExpandEnv("$HOME/.dex/terminal")))
	return terminalCmd
}

func newSettingsCommand(options *globalOptions) *cobra.Command {
	settingsCmd := &cobra.Command{Use: "settings", Short: "Manage Dex local settings."}
	settingsCmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show Dex config.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := storage.Root()
			if err != nil {
				return err
			}
			config, err := storage.LoadConfig(root)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, config)
		},
	})
	settingsCmd.AddCommand(&cobra.Command{
		Use:   "theme <name>",
		Short: "Set Dex theme.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := storage.Root()
			if err != nil {
				return err
			}
			config, err := storage.LoadConfig(root)
			if err != nil {
				return err
			}
			config.Theme = args[0]
			if err := storage.SaveConfig(root, config); err != nil {
				return err
			}
			return writeRendered(cmd, options, config)
		},
	})
	settingsCmd.AddCommand(&cobra.Command{
		Use:   "profile <name>",
		Short: "Set Dex profile.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := storage.Root()
			if err != nil {
				return err
			}
			config, err := storage.LoadConfig(root)
			if err != nil {
				return err
			}
			config.Profile = args[0]
			if err := storage.SaveConfig(root, config); err != nil {
				return err
			}
			return writeRendered(cmd, options, config)
		},
	})
	settingsCmd.AddCommand(&cobra.Command{
		Use:   "history",
		Short: "Show saved command history.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := storage.Root()
			if err != nil {
				return err
			}
			entries, err := storage.ListHistory(root, 50)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, entries)
		},
	})
	settingsCmd.AddCommand(&cobra.Command{
		Use:   "storage",
		Short: "Show Dex storage root.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := storage.Root()
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, output.Record{"storage": root})
		},
	})
	return settingsCmd
}

func localStateCommand(options *globalOptions, name string, short string, command string, args ...string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
			defer cancel()
			return writeRendered(cmd, options, system.Run(ctx, name, command, args...))
		},
	}
}

func newJSONCommand(options *globalOptions) *cobra.Command {
	jsonCmd := &cobra.Command{Use: "json", Short: "Explore and compare JSON."}
	viewCmd := &cobra.Command{
		Use:   "view <file>",
		Short: "Pretty-print JSON.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer file.Close()
			formatted, err := jsonx.View(file)
			if err != nil {
				return err
			}
			if options.format == output.Text {
				options.format = output.Raw
			}
			return writeRendered(cmd, options, formatted)
		},
	}
	formatCmd := *viewCmd
	formatCmd.Use = "format <file>"
	formatCmd.Short = "Format JSON."
	jsonCmd.AddCommand(viewCmd, &formatCmd)
	jsonCmd.AddCommand(&cobra.Command{
		Use:   "minify <file>",
		Short: "Minify JSON.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := withOpenFile(args[0], jsonx.Minify)
			if err != nil {
				return err
			}
			if options.format == output.Text {
				options.format = output.Raw
			}
			return writeRendered(cmd, options, value+"\n")
		},
	})
	jsonCmd.AddCommand(&cobra.Command{
		Use:   "validate <file>",
		Short: "Validate JSON.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			file, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer file.Close()
			if err := jsonx.Validate(file); err != nil {
				return err
			}
			return writeRendered(cmd, options, output.Record{"valid": true, "file": args[0]})
		},
	})
	jsonCmd.AddCommand(&cobra.Command{
		Use:   "query <file> <path>",
		Short: "Read a value at a dot path.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := readJSONFile(args[0])
			if err != nil {
				return err
			}
			result, err := jsonx.Query(value, args[1])
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, result)
		},
	})
	jsonCmd.AddCommand(&cobra.Command{
		Use:   "flatten <file>",
		Short: "Flatten JSON into dot paths.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := readJSONFile(args[0])
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, pathValueRecords(jsonx.Flatten(value)))
		},
	})
	jsonCmd.AddCommand(&cobra.Command{
		Use:   "keys <file>",
		Short: "List top-level object keys.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := readJSONFile(args[0])
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, stringRecords("key", jsonx.Keys(value)))
		},
	})
	jsonCmd.AddCommand(&cobra.Command{
		Use:   "paths <file>",
		Short: "List JSON dot paths.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := readJSONFile(args[0])
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, stringRecords("path", jsonx.Paths(value)))
		},
	})
	jsonCmd.AddCommand(&cobra.Command{
		Use:   "types <file>",
		Short: "List JSON path types.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := readJSONFile(args[0])
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, pathStringValueRecords(jsonx.Types(value)))
		},
	})
	jsonCmd.AddCommand(&cobra.Command{
		Use:   "redact <file>",
		Short: "Redact sensitive-looking JSON values.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := readJSONFile(args[0])
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, jsonx.Redact(value))
		},
	})
	jsonCmd.AddCommand(&cobra.Command{
		Use:   "fingerprint <file>",
		Short: "Create a stable structural fingerprint.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			value, err := readJSONFile(args[0])
			if err != nil {
				return err
			}
			fingerprint, err := jsonx.Fingerprint(value)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, output.Record{"fingerprint": fingerprint})
		},
	})
	jsonCmd.AddCommand(&cobra.Command{
		Use:   "diff <before> <after>",
		Short: "Show semantic JSON differences.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			before, err := os.Open(args[0])
			if err != nil {
				return err
			}
			defer before.Close()
			after, err := os.Open(args[1])
			if err != nil {
				return err
			}
			defer after.Close()
			changes, err := jsonx.Diff(before, after)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, changeRecords(changes))
		},
	})
	return jsonCmd
}

func withOpenFile(path string, fn func(io.Reader) (string, error)) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	return fn(file)
}

func readJSONFile(path string) (any, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return jsonx.Decode(file)
}

func newSystemCommand(options *globalOptions) *cobra.Command {
	systemCmd := &cobra.Command{Use: "system", Short: "Inspect machine health."}
	systemCmd.AddCommand(&cobra.Command{
		Use:   "info",
		Short: "Show system information.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			return writeRendered(cmd, options, system.SystemInfo(ctx))
		},
	})
	systemCmd.AddCommand(&cobra.Command{
		Use:   "health",
		Short: "Show local health score.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			return writeRendered(cmd, options, system.SystemHealth(ctx))
		},
	})
	systemCmd.AddCommand(&cobra.Command{
		Use:   "dashboard",
		Short: "Show a compact system dashboard.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			return writeRendered(cmd, options, system.Dashboard(ctx))
		},
	})
	systemCmd.AddCommand(&cobra.Command{
		Use:   "uptime",
		Short: "Show uptime.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			return writeRendered(cmd, options, system.Run(ctx, "uptime", "uptime"))
		},
	})
	systemCmd.AddCommand(&cobra.Command{
		Use:   "profile",
		Short: "Show local profile summary.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeRendered(cmd, options, output.Record{"profile": options.profile, "theme": options.theme})
		},
	})
	systemCmd.AddCommand(&cobra.Command{
		Use:   "snapshot",
		Short: "Capture a lightweight system snapshot.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			return writeRendered(cmd, options, system.Snapshot(ctx))
		},
	})
	systemCmd.AddCommand(&cobra.Command{
		Use:   "report",
		Short: "Show a local system report.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			return writeRendered(cmd, options, system.Dashboard(ctx))
		},
	})
	systemCmd.AddCommand(systemProbeCmd(options, "cpu", "Show CPU details.", "sysctl", "-n", "machdep.cpu.brand_string", "hw.ncpu", "vm.loadavg"))
	systemCmd.AddCommand(systemProbeCmd(options, "memory", "Show memory pressure.", "memory_pressure"))
	systemCmd.AddCommand(systemProbeCmd(options, "disk", "Show disk usage.", "df", "-h"))
	systemCmd.AddCommand(systemProbeCmd(options, "battery", "Show battery status.", "pmset", "-g", "batt"))
	systemCmd.AddCommand(systemProbeCmd(options, "power", "Show power assertions.", "pmset", "-g", "assertions"))
	systemCmd.AddCommand(systemProbeCmd(options, "thermal", "Show thermal state.", "pmset", "-g", "therm"))
	systemCmd.AddCommand(systemProbeCmd(options, "fan", "Show fan/thermal state.", "pmset", "-g", "therm"))
	systemCmd.AddCommand(systemProbeCmd(options, "startup", "Show launch agents.", "launchctl", "print", "gui/"+strconv.Itoa(os.Getuid())))
	systemCmd.AddCommand(systemProbeCmd(options, "services", "Show services.", "launchctl", "list"))
	systemCmd.AddCommand(systemProbeCmd(options, "launch-agents", "Show user launch agents.", "ls", os.ExpandEnv("$HOME/Library/LaunchAgents")))
	systemCmd.AddCommand(systemProbeCmd(options, "launch-daemons", "Show system launch daemons.", "ls", "/Library/LaunchDaemons"))
	systemCmd.AddCommand(systemProbeCmd(options, "permissions", "Show app privacy database location.", "ls", "-l", os.ExpandEnv("$HOME/Library/Application Support/com.apple.TCC/TCC.db")))
	systemCmd.AddCommand(&cobra.Command{
		Use:   "env [name]",
		Short: "Show environment variables.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			return writeRendered(cmd, options, system.Env(name))
		},
	})
	systemCmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Show PATH entries.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeRendered(cmd, options, system.Path())
		},
	})
	systemCmd.AddCommand(&cobra.Command{
		Use:   "shell",
		Short: "Show current shell.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeRendered(cmd, options, system.Shell())
		},
	})
	systemCmd.AddCommand(&cobra.Command{
		Use:   "terminal",
		Short: "Show terminal environment.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeRendered(cmd, options, output.Record{"term": os.Getenv("TERM"), "program": os.Getenv("TERM_PROGRAM")})
		},
	})
	systemCmd.AddCommand(&cobra.Command{
		Use:   "score",
		Short: "Show local health score.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
			defer cancel()
			return writeRendered(cmd, options, system.SystemHealth(ctx))
		},
	})
	return systemCmd
}

func systemProbeCmd(options *globalOptions, name string, short string, command string, args ...string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: short,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), 8*time.Second)
			defer cancel()
			return writeRendered(cmd, options, system.Run(ctx, name, command, args...))
		},
	}
}

func newRegexCommand(options *globalOptions) *cobra.Command {
	regexCmd := &cobra.Command{Use: "regex", Short: "Test and inspect regular expressions."}
	regexCmd.AddCommand(&cobra.Command{
		Use:   "test <pattern> <input-or-file>",
		Short: "Run a regex against input.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			input := args[1]
			if data, err := os.ReadFile(args[1]); err == nil {
				input = string(data)
			}
			matches, err := regexx.TestPattern(args[0], input)
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, matches)
		},
	})
	regexCmd.AddCommand(&cobra.Command{
		Use:   "find <pattern> <input-or-file>",
		Short: "Find regex matches.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			matches, err := regexx.Find(args[0], readLiteralOrFile(args[1]))
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, matches)
		},
	})
	regexCmd.AddCommand(&cobra.Command{
		Use:   "groups <pattern> <input-or-file>",
		Short: "Show regex capture groups.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			matches, err := regexx.Find(args[0], readLiteralOrFile(args[1]))
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, matches)
		},
	})
	regexCmd.AddCommand(&cobra.Command{
		Use:   "replace <pattern> <replacement> <input-or-file>",
		Short: "Replace regex matches.",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			replaced, err := regexx.Replace(args[0], args[1], readLiteralOrFile(args[2]))
			if err != nil {
				return err
			}
			if options.format == output.Text {
				options.format = output.Raw
			}
			return writeRendered(cmd, options, replaced)
		},
	})
	regexCmd.AddCommand(&cobra.Command{
		Use:   "explain <pattern>",
		Short: "Explain regex parse structure.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			explanation, err := regexx.Explain(args[0])
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, explanation)
		},
	})
	regexCmd.AddCommand(&cobra.Command{
		Use:   "visual <pattern>",
		Short: "Show a visual regex parse tree.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			visual, err := regexx.Visual(args[0])
			if err != nil {
				return err
			}
			if options.format == output.Text {
				options.format = output.Raw
			}
			return writeRendered(cmd, options, visual)
		},
	})
	regexCmd.AddCommand(&cobra.Command{
		Use:   "examples <pattern>",
		Short: "Generate simple matching and non-matching examples.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			examples, err := regexx.Examples(args[0])
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, examples)
		},
	})
	regexCmd.AddCommand(&cobra.Command{
		Use:   "danger <pattern>",
		Short: "Detect risky regex structure.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := regexx.Danger(args[0])
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, report)
		},
	})
	regexCmd.AddCommand(&cobra.Command{
		Use:   "escape <text>",
		Short: "Escape text for literal regex use.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if options.format == output.Text {
				options.format = output.Raw
			}
			return writeRendered(cmd, options, regexx.Escape(args[0]))
		},
	})
	regexCmd.AddCommand(&cobra.Command{
		Use:   "unescape <text>",
		Short: "Unescape simple regex escapes.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if options.format == output.Text {
				options.format = output.Raw
			}
			return writeRendered(cmd, options, regexx.Unescape(args[0]))
		},
	})
	regexCmd.AddCommand(&cobra.Command{
		Use:   "benchmark <pattern> <input-or-file>",
		Short: "Benchmark regex matching.",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := regexx.Benchmark(args[0], readLiteralOrFile(args[1]))
			if err != nil {
				return err
			}
			return writeRendered(cmd, options, result)
		},
	})
	regexCmd.AddCommand(&cobra.Command{
		Use:   "cheatsheet",
		Short: "Show regex syntax cheatsheet.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if options.format == output.Text {
				options.format = output.Raw
			}
			return writeRendered(cmd, options, "Anchors: ^ $\nClasses: \\d \\w \\s .\nQuantifiers: * + ? {m,n}\nGroups: (...) (?:...) | alternation\n")
		},
	})
	return regexCmd
}

func readLiteralOrFile(value string) string {
	if data, err := os.ReadFile(value); err == nil {
		return string(data)
	}
	return value
}

func ipRecords(items []network.IPAddress) []output.Record {
	records := make([]output.Record, 0, len(items))
	for _, item := range items {
		records = append(records, output.Record{"interface": item.Interface, "address": item.Address, "network": item.Network})
	}
	return records
}

func portRecords(items []network.Port) []output.Record {
	records := make([]output.Record, 0, len(items))
	for _, item := range items {
		records = append(records, output.Record{"command": item.Command, "pid": item.PID, "user": item.User, "address": item.Address, "state": item.State})
	}
	return records
}

func interfaceRecords(items []network.Interface) []output.Record {
	records := make([]output.Record, 0, len(items))
	for _, item := range items {
		records = append(records, output.Record{
			"name":          item.Name,
			"index":         item.Index,
			"mtu":           item.MTU,
			"hardware_addr": item.HardwareAddr,
			"flags":         item.Flags,
			"addresses":     fmt.Sprint(item.Addresses),
		})
	}
	return records
}

func routeRecords(items []network.Route) []output.Record {
	records := make([]output.Record, 0, len(items))
	for _, item := range items {
		records = append(records, output.Record{
			"destination": item.Destination,
			"gateway":     item.Gateway,
			"flags":       item.Flags,
			"interface":   item.Interface,
		})
	}
	return records
}

func dnsResolverRecords(items []network.DNSResolver) []output.Record {
	records := make([]output.Record, 0, len(items))
	for _, item := range items {
		records = append(records, output.Record{"resolver": item.Resolver, "nameserver": item.Nameserver})
	}
	return records
}

func dnsRecordRecords(items []network.DNSRecord) []output.Record {
	records := make([]output.Record, 0, len(items))
	for _, item := range items {
		records = append(records, output.Record{"name": item.Name, "type": item.Type, "value": item.Value})
	}
	return records
}

func stringRecords(key string, items []string) []output.Record {
	records := make([]output.Record, 0, len(items))
	for _, item := range items {
		records = append(records, output.Record{key: item})
	}
	return records
}

func pathValueRecords(items map[string]any) []output.Record {
	records := make([]output.Record, 0, len(items))
	for path, value := range items {
		records = append(records, output.Record{"path": path, "value": value})
	}
	sortRecords(records, "path")
	return records
}

func pathStringValueRecords(items map[string]string) []output.Record {
	records := make([]output.Record, 0, len(items))
	for path, value := range items {
		records = append(records, output.Record{"path": path, "value": value})
	}
	sortRecords(records, "path")
	return records
}

func sortRecords(records []output.Record, key string) {
	sort.Slice(records, func(i, j int) bool {
		return fmt.Sprint(records[i][key]) < fmt.Sprint(records[j][key])
	})
}

func processRecords(items []process.Process) []output.Record {
	records := make([]output.Record, 0, len(items))
	for _, item := range items {
		records = append(records, output.Record{
			"pid":     item.PID,
			"ppid":    item.PPID,
			"command": item.Command,
			"cpu":     item.CPU,
			"memory":  item.Memory,
			"elapsed": item.Elapsed,
		})
	}
	return records
}

func changeRecords(items []jsonx.Change) []output.Record {
	records := make([]output.Record, 0, len(items))
	for _, item := range items {
		records = append(records, output.Record{"path": item.Path, "type": item.Type, "before": item.Before, "after": item.After})
	}
	return records
}
