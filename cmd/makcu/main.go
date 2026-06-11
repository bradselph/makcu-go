package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	makcu "github.com/bradselph/makcu-go"
)

func usage() {
	fmt.Println("Usage:")
	fmt.Println("  makcu --debug")
	fmt.Println("  makcu --testPort COM3")
	fmt.Println("  makcu --runtest")
	fmt.Println()
	fmt.Println("  (or: go run ./cmd/makcu --debug)")
}

func debugConsole() {
	controller := makcu.NewMakcuController(makcu.WithDebug(true))

	err := controller.Connect()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to connect: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Makcu Debug Console")
	fmt.Println("Type a raw command (e.g., km.version()) and press Enter.")
	fmt.Println("Type 'exit' or 'quit' to leave.")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print(">>> ")
		if !scanner.Scan() {
			break
		}
		cmd := strings.TrimSpace(scanner.Text())

		if strings.ToLower(cmd) == "exit" || strings.ToLower(cmd) == "quit" {
			break
		}
		if cmd == "" {
			continue
		}

		response, err := controller.Transport.SendCommand(cmd, true, 0)
		if err != nil {

			fmt.Printf("Error: %v\n", err)
			continue
		}

		if response != "" && strings.TrimSpace(response) != "" {
			if strings.TrimSpace(response) == cmd {
				fmt.Println(cmd)
			} else {
				fmt.Println(response)
			}
		} else {
			fmt.Println("(no response)")
		}
	}

	_ = controller.Disconnect()
	fmt.Println("Disconnected.")
}

func testPort(port string) {
	fmt.Printf("Trying to connect to %s...\n", port)

	controller := makcu.NewMakcuController(
		makcu.WithFallbackPort(port),
		makcu.WithSendInit(false),
		makcu.WithOverridePort(true),
	)

	err := controller.Connect()
	if err != nil {
		if strings.Contains(err.Error(), "FileNotFoundError") || strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
			fmt.Printf("Port %s does not exist. Please check the port name.\n", port)
		} else {

			fmt.Printf("Failed to connect to %s: \n", port)
		}
		return
	}

	if controller.IsConnected() {
		fmt.Printf("Successfully connected to %s.\n", port)
	}
	_ = controller.Disconnect()
}

func runTests() {
	cmd := exec.Command("go", "test", "./...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error running tests: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		usage()
		return
	}

	switch {
	case args[0] == "--debug":
		debugConsole()
	case args[0] == "--testPort" && len(args) == 2:
		testPort(args[1])
	case args[0] == "--runtest":
		runTests()
	default:
		fmt.Printf("Unknown command: %s\n", strings.Join(args, " "))
		usage()
	}
}
