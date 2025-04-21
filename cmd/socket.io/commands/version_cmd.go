package commands

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/symfony-cli/console"
	"github.com/symfony-cli/terminal"
)

func runInDir(dir string, fn func() error) error {
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	if err := os.Chdir(dir); err != nil {
		return err
	}
	return fn()
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func versionCommand() *console.Command {
	return &console.Command{
		Category: "make",
		Name:     "version",
		Usage:    "Sets and propagates the module version",
		Args: []*console.Arg{
			{Name: "version", Optional: false, Description: "The version to set (e.g., v3.0.0[-alpha|beta|rc.x])"},
		},
		Action: func(ctx *console.Context) error {
			version := ctx.Args().Get("version")
			if version == "" {
				return fmt.Errorf("[Error] VERSION is required. Usage: make version v3.0.0[-alpha|beta|rc.x]")
			}

			terminal.Printf("[Version] Updating version to %s\n", version)

			// Step 1: Update version.go
			versionFile := "pkg/version/version.go"
			content, err := os.ReadFile(versionFile)
			if err != nil {
				return fmt.Errorf("failed to read %s: %w", versionFile, err)
			}

			updated := regexp.MustCompile(`VERSION\s*=\s*".*?"`).ReplaceAll(content, []byte(fmt.Sprintf(`VERSION = "%s"`, version)))
			if err := os.WriteFile(versionFile, updated, 0644); err != nil {
				return fmt.Errorf("failed to write %s: %w", versionFile, err)
			}

			terminal.Println("[Version] Done.")
			return nil
		},
	}
}
