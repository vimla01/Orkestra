package main

import (
	"flag"
	"fmt"
	"os"

	"k8s.io/client-go/kubernetes"

	"github.com/orkestra/internal/k8s"
	"github.com/orkestra/internal/registry"
)

func handleClusterCommand(args []string) {
	if len(args) == 0 {
		fmt.Println("Usage: orkestra cluster <subcommand>")
		fmt.Println()
		fmt.Println("Subcommands:")
		fmt.Println("  register    Register a Kubernetes cluster")
		return
	}

	switch args[0] {
	case "register":
		registerFlags := flag.NewFlagSet("cluster register", flag.ExitOnError)
		name := registerFlags.String("name", "", "Cluster name (required)")
		kubeconfig := registerFlags.String("kubeconfig", "", "Path to kubeconfig (required)")
		registerFlags.Parse(args[1:])

		if *name == "" || *kubeconfig == "" {
			fmt.Fprintln(os.Stderr, "Error: --name and --kubeconfig are required")
			registerFlags.Usage()
			os.Exit(1)
		}

		// Create Kubernetes client factory
		clientFactory := func(kubeconfigPath string) (kubernetes.Interface, error) {
			return k8s.NewClientFromKubeconfig(kubeconfigPath)
		}

		// Create registry and register the cluster
		reg := registry.NewRegistry(clientFactory)
		cluster, err := reg.Register(*name, *kubeconfig)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error registering cluster: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Cluster '%s' registered successfully (endpoint: %s)\n", *name, cluster.Endpoint)

	default:
		fmt.Fprintf(os.Stderr, "Unknown cluster subcommand: %s\n", args[0])
		fmt.Println("Usage: orkestra cluster <subcommand>")
		fmt.Println()
		fmt.Println("Subcommands:")
		fmt.Println("  register    Register a Kubernetes cluster")
	}
}

func handleDeployCommand(args []string) {
	deployFlags := flag.NewFlagSet("deploy", flag.ExitOnError)
	file := deployFlags.String("file", "", "Deployment manifest file (required)")
	clusters := deployFlags.String("clusters", "", "Comma-separated cluster names (required)")
	deployFlags.Parse(args)

	if *file == "" || *clusters == "" {
		fmt.Fprintln(os.Stderr, "Error: --file and --clusters are required")
		deployFlags.Usage()
		os.Exit(1)
	}

	fmt.Printf("⚠️  Propagation engine is not yet implemented (Phase 2). Deployment file: %s, Target clusters: %s\n", *file, *clusters)
}

func printUsage() {
	fmt.Println(`Usage: orkestra <command> [options]

Commands:
  serve                    Start the control plane server (default)
  cluster register         Register a Kubernetes cluster
  deploy                   Propagate a deployment (Phase 2)

Server Options:
  --config <path>          Path to config file (default: config.yaml)

Cluster Register Options:
  --name <name>            Cluster name (required)
  --kubeconfig <path>      Path to kubeconfig (required)

Deploy Options:
  --file <path>            Deployment manifest file (required)
  --clusters <list>        Comma-separated cluster names (required)`)
}
