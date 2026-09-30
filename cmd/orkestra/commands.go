package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/orkestra/internal/client"
	"github.com/orkestra/internal/propagation"
)

// defaultServer is the control plane address used when neither --server nor
// ORKESTRA_SERVER is set.
const defaultServer = "http://localhost:8080"

// newFlagSet creates a FlagSet with the shared --server flag registered.
func newFlagSet(name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	server := os.Getenv("ORKESTRA_SERVER")
	if server == "" {
		server = defaultServer
	}
	addr := fs.String("server", server, "Orkestra server URL (env ORKESTRA_SERVER)")
	return fs, addr
}

// fail prints an error and exits with a non-zero status.
func fail(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "Error: "+format+"\n", args...)
	os.Exit(1)
}

// newTable returns a tabwriter for aligned column output.
func newTable() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
}

// formatTime renders a timestamp for tables, or "-" if it was never set.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func handleClusterCommand(args []string) {
	if len(args) == 0 {
		printClusterUsage()
		return
	}

	switch args[0] {
	case "register":
		fs, server := newFlagSet("cluster register")
		name := fs.String("name", "", "Cluster name (required)")
		kubeconfig := fs.String("kubeconfig", "", "Path to kubeconfig (required)")
		fs.Parse(args[1:])

		if *name == "" || *kubeconfig == "" {
			fmt.Fprintln(os.Stderr, "Error: --name and --kubeconfig are required")
			fs.Usage()
			os.Exit(1)
		}

		// The server opens the kubeconfig itself, so send an absolute path
		// rather than one relative to this shell's working directory.
		path, err := filepath.Abs(*kubeconfig)
		if err != nil {
			fail("invalid kubeconfig path: %v", err)
		}

		cluster, err := client.New(*server).RegisterCluster(*name, path)
		if err != nil {
			fail("registering cluster: %v", err)
		}
		fmt.Printf("✅ Cluster '%s' registered successfully (endpoint: %s)\n", cluster.Name, cluster.Endpoint)

	case "list":
		fs, server := newFlagSet("cluster list")
		fs.Parse(args[1:])

		clusters, err := client.New(*server).ListClusters()
		if err != nil {
			fail("listing clusters: %v", err)
		}
		if len(clusters) == 0 {
			fmt.Println("No clusters registered.")
			return
		}

		tw := newTable()
		fmt.Fprintln(tw, "NAME\tSTATUS\tNODES\tENDPOINT\tLAST CHECK")
		for _, c := range clusters {
			fmt.Fprintf(tw, "%s\t%s\t%d/%d\t%s\t%s\n",
				c.Name, c.Status, c.ReadyNodes, c.NodeCount, c.Endpoint, formatTime(c.LastHealthCheck))
		}
		tw.Flush()

	default:
		fmt.Fprintf(os.Stderr, "Unknown cluster subcommand: %s\n\n", args[0])
		printClusterUsage()
		os.Exit(1)
	}
}

func printClusterUsage() {
	fmt.Println(`Usage: orkestra cluster <subcommand>

Subcommands:
  register    Register a Kubernetes cluster
  list        List registered clusters and their health`)
}

func handleDeployCommand(args []string) {
	fs, server := newFlagSet("deploy")
	file := fs.String("file", "", "Deployment manifest file (required)")
	clusters := fs.String("clusters", "", "Comma-separated cluster names (required)")
	fs.Parse(args)

	if *file == "" || *clusters == "" {
		fmt.Fprintln(os.Stderr, "Error: --file and --clusters are required")
		fs.Usage()
		os.Exit(1)
	}

	manifest, err := os.ReadFile(*file)
	if err != nil {
		fail("reading manifest: %v", err)
	}

	var targets []string
	for _, name := range strings.Split(*clusters, ",") {
		if name = strings.TrimSpace(name); name != "" {
			targets = append(targets, name)
		}
	}

	resp, err := client.New(*server).Propagate(string(manifest), targets)
	if err != nil {
		fail("propagating deployment: %v", err)
	}

	fmt.Printf("Deployment %s/%s:\n", resp.Namespace, resp.Name)
	failed := 0
	tw := newTable()
	for _, r := range resp.Results {
		if r.Action == propagation.ActionFailed {
			failed++
			fmt.Fprintf(tw, "  ❌ %s\t%s\t%s\n", r.Cluster, r.Action, r.Error)
		} else {
			fmt.Fprintf(tw, "  ✅ %s\t%s\t\n", r.Cluster, r.Action)
		}
	}
	tw.Flush()

	if failed > 0 {
		fail("%d of %d clusters failed", failed, len(resp.Results))
	}
}

func handleDeploymentCommand(args []string) {
	if len(args) == 0 {
		printDeploymentUsage()
		return
	}

	switch args[0] {
	case "list":
		fs, server := newFlagSet("deployment list")
		fs.Parse(args[1:])

		records, err := client.New(*server).ListDeployments()
		if err != nil {
			fail("listing deployments: %v", err)
		}
		if len(records) == 0 {
			fmt.Println("No deployments propagated.")
			return
		}

		tw := newTable()
		fmt.Fprintln(tw, "NAMESPACE\tNAME\tCLUSTERS\tPROPAGATED")
		for _, r := range records {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				r.Namespace, r.Name, strings.Join(r.Clusters, ","), formatTime(r.PropagatedAt))
		}
		tw.Flush()

	case "status":
		fs, server := newFlagSet("deployment status")
		namespace := fs.String("namespace", "default", "Deployment namespace")
		// Accept the name before or after flags: `status web -namespace x`
		// and `status -namespace x web` both work.
		var name string
		rest := args[1:]
		if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
			name, rest = rest[0], rest[1:]
		}
		fs.Parse(rest)
		if name == "" {
			name = fs.Arg(0)
		}
		if name == "" {
			fmt.Fprintln(os.Stderr, "Error: deployment name is required")
			fmt.Fprintln(os.Stderr, "Usage: orkestra deployment status <name> [--namespace <ns>]")
			os.Exit(1)
		}

		status, err := client.New(*server).DeploymentStatus(*namespace, name)
		if err != nil {
			fail("getting deployment status: %v", err)
		}

		overall := "Progressing"
		if status.Ready {
			overall = "Ready"
		}
		fmt.Printf("Deployment %s/%s: %s\n\n", status.Namespace, status.Name, overall)

		tw := newTable()
		fmt.Fprintln(tw, "CLUSTER\tREADY\tUP-TO-DATE\tAVAILABLE\tSTATUS")
		for _, s := range status.Statuses {
			state := "Progressing"
			switch {
			case s.Error != "":
				state = "Error: " + s.Error
			case s.Ready:
				state = "Ready"
			}
			fmt.Fprintf(tw, "%s\t%d/%d\t%d\t%d\t%s\n",
				s.Cluster, s.ReadyReplicas, s.DesiredReplicas, s.UpdatedReplicas, s.AvailableReplicas, state)
		}
		tw.Flush()

		if len(status.Failovers) > 0 {
			fmt.Println("\nFailovers:")
			for _, f := range status.Failovers {
				fmt.Printf("  %s  %s -> %s (%s)\n", formatTime(f.At), f.From, f.To, f.Reason)
				if f.CleanupError != "" {
					fmt.Printf("    ⚠️  could not remove from %s: %s\n", f.From, f.CleanupError)
				}
			}
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown deployment subcommand: %s\n\n", args[0])
		printDeploymentUsage()
		os.Exit(1)
	}
}

func printDeploymentUsage() {
	fmt.Println(`Usage: orkestra deployment <subcommand>

Subcommands:
  list                     List propagated deployments
  status <name>            Show live rollout status on each cluster`)
}

func printUsage() {
	fmt.Println(`Usage: orkestra <command> [options]

Commands:
  serve                    Start the control plane server (default)
  cluster register         Register a Kubernetes cluster
  cluster list             List registered clusters and their health
  deploy                   Propagate a deployment to member clusters
  deployment list          List propagated deployments
  deployment status <name> Show live rollout status on each cluster

Server Options:
  --config <path>          Path to config file (default: config.yaml)

Client Options (all commands except serve):
  --server <url>           Orkestra server URL (default: http://localhost:8080,
                           or env ORKESTRA_SERVER)

Cluster Register Options:
  --name <name>            Cluster name (required)
  --kubeconfig <path>      Path to kubeconfig (required)

Deploy Options:
  --file <path>            Deployment manifest file (required)
  --clusters <list>        Comma-separated cluster names (required)

Deployment Status Options:
  --namespace <ns>         Deployment namespace (default: default)`)
}
