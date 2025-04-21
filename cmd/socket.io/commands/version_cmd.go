package commands

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

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

			// Step 2: Update modules
			modules := []string{`parsers/engine`, `parsers/socket`, `servers/engine`, `servers/socket`, `adapters/adapter`, `adapters/redis`, `clients/engine`, `clients/socket`}
			for _, mod := range modules {
				if _, err := os.Stat(mod); os.IsNotExist(err) {
					terminal.Printf("[Warn] Skipped missing module: %s\n", mod)
					continue
				}

				terminal.Printf("[Version] Updating dependencies in %s...\n", mod)
				if err := runInDir(mod, func() error {
					if err := run("go", "mod", "tidy"); err != nil {
						return err
					}
					// Filter and update specific dependencies
					cmd := exec.Command("go", "list", "-f", "{{if and (not .Indirect) (not .Main)}}{{.Path}}@"+version+"{{end}}", "-m", "all")
					cmd.Env = os.Environ()
					cmd.Dir = mod
					output, err := cmd.Output()
					if err != nil {
						return err
					}
					lines := strings.Split(string(output), "\n")
					for _, line := range lines {
						if strings.HasPrefix(line, "github.com/zishang520/socket.io") {
							if err := run("go", "get", "-v", line); err != nil {
								return err
							}
						}
					}
					return run("go", "mod", "tidy")
				}); err != nil {
					return fmt.Errorf("error in module %s: %w", mod, err)
				}
			}

			terminal.Println("[Version] Done.")
			return nil
		},
	}
}
