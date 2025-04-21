package main

import (
	"fmt"
	"os"
	"time"

	"github.com/symfony-cli/console"
	"github.com/symfony-cli/terminal"
	"github.com/zishang520/socket.io/v3/pkg/cmd/socket.io/commands"
)

var (
	version   = "1.0.0"
	channel   = "stable"
	buildDate = time.Now().Format("2006-01-02")
)

func main() {
	if os.Getenv("SC_DEBUG") == "1" {
		terminal.SetLogLevel(5)
	}

	args := os.Args

	app := &console.Application{
		Name:          "Socket.io Tools",
		Usage:         "Socket.io building tools",
		Copyright:     fmt.Sprintf("(c) 2022-%d Luoyy", time.Now().Year()),
		FlagEnvPrefix: []string{"SOCKET_IO"},
		Commands:      commands.CommonCommands(),
		Version:       version,
		Channel:       channel,
		BuildDate:     buildDate,
	}

	app.Run(args)
}
