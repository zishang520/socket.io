package commands

import (
	"github.com/symfony-cli/console"
)

func CommonCommands() []*console.Command {
	cmds := []*console.Command{
		versionCommand(),
	}

	return cmds
}
