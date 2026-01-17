package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mpoegel/pine/pkg/arborist"
)

func main() {
	endpoint := flag.String("e", "/var/run/pine.sock", "pine daemon endpoint")
	timeout := flag.Duration("t", 10*time.Second, "command timeout")

	flag.Parse()

	command := flag.Arg(0)
	treeName := flag.Arg(1)

	client := arborist.NewClient(*endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if err := run(ctx, client, command, treeName); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, client arborist.Client, command, treeName string) error {
	switch command {
	default:
		return fmt.Errorf("unknown command '%s'", command)
	case "start":
		return client.StartTree(ctx, treeName)
	case "stop":
		return client.StopTree(ctx, treeName)
	case "restart":
		return client.RestartTree(ctx, treeName)
	case "status":
		if status, err := client.GetTreeStatus(ctx, treeName); err != nil {
			return err
		} else {
			fmt.Printf("Tree:%s State:%s Uptime:%d LastChange:%d\n", status.TreeName, status.State, status.Uptime, status.LastChange)
		}
	case "list":
		if statusList, err := client.ListTrees(ctx); err != nil {
			return err
		} else {
			for _, status := range statusList.Trees {
				fmt.Printf("Tree:%s State:%s Uptime:%d LastChange:%d\n", status.TreeName, status.State, status.Uptime, status.LastChange)
			}
		}
	case "logrotate":
		return client.RotateTreeLog(ctx, treeName)
	}

	return nil
}
