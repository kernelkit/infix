package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kernelkit/infix/src/yangerd/internal/ipc"
)

const defaultSocket = "/run/yangerd.sock"
const defaultTimeout = 5 * time.Second

func main() {
	socket := defaultSocket
	timeout := defaultTimeout

	args := os.Args[1:]
	for len(args) > 0 && len(args[0]) > 0 && args[0][0] == '-' {
		switch args[0] {
		case "--socket":
			if len(args) < 2 {
				die("--socket requires an argument")
			}
			socket = args[1]
			args = args[2:]
		case "--timeout":
			if len(args) < 2 {
				die("--timeout requires an argument")
			}
			d, err := time.ParseDuration(args[1])
			if err != nil {
				die("invalid duration: %v", err)
			}
			timeout = d
			args = args[2:]
		default:
			die("unknown flag: %s", args[0])
		}
	}

	if len(args) == 0 {
		usage()
	}

	client := ipc.NewClient(socket, timeout)

	switch args[0] {
	case "get":
		if len(args) < 2 {
			die("get requires a path argument")
		}
		resp, err := client.Get(args[1])
		if err != nil {
			die("get: %v", err)
		}
		printResponse(resp)
	case "dump":
		resp, err := client.Get("/")
		if err != nil {
			die("dump: %v", err)
		}
		printResponse(resp)
	case "health":
		resp, err := client.Health()
		if err != nil {
			die("health: %v", err)
		}
		printResponse(resp)
	default:
		die("unknown command: %s", args[0])
	}
}

func printResponse(resp *ipc.Response) {
	if resp.Code == 503 {
		fmt.Fprintf(os.Stderr, "yangerd is starting up\n")
		os.Exit(3)
	}
	if resp.Status == "error" {
		fmt.Fprintf(os.Stderr, "error %d: %s\n", resp.Code, resp.Message)
		if resp.Code == 404 {
			os.Exit(2)
		}
		os.Exit(1)
	}

	var out []byte
	if resp.Data != nil {
		out, _ = json.MarshalIndent(json.RawMessage(resp.Data), "", "  ")
	} else {
		out, _ = json.MarshalIndent(resp, "", "  ")
	}
	fmt.Println(string(out))
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: yangerctl [--socket path] [--timeout dur] <command> [args]\n\n")
	fmt.Fprintf(os.Stderr, "Commands:\n")
	fmt.Fprintf(os.Stderr, "  get <path>   Query a YANG subtree\n")
	fmt.Fprintf(os.Stderr, "  dump         Dump entire tree\n")
	fmt.Fprintf(os.Stderr, "  health       Show daemon health\n")
	os.Exit(1)
}

func die(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "yangerctl: "+format+"\n", args...)
	os.Exit(1)
}
