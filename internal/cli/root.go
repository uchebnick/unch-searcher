package cli

// @filectx: Command-line composition root that wires clean-architecture services to concrete adapters.

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/uchebnick/unch-searcher/internal/indexing"
	"github.com/uchebnick/unch-searcher/internal/project"
	"github.com/uchebnick/unch-searcher/internal/runtime"
	"github.com/uchebnick/unch-searcher/internal/termui"
)

// @search: Run is the clean-architecture replacement for the old RunCLI entrypoint and dispatches to index by default or to search when requested.
func Run(program string, args []string) (err error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working dir: %w", err)
	}

	paths, err := project.PreparePaths(cwd)
	if err != nil {
		return err
	}

	s, err := termui.NewSession(paths.LocalDir)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			s.Logf("fatal error: %v", err)
		}
		_ = s.Close()
	}()
	s.Logf("program=%s", program)
	s.Logf("args=%q", args)
	s.Logf("cwd=%s", cwd)

	command, commandArgs, err := detectCommand(args)
	if err != nil {
		return err
	}
	s.Logf("command=%s", command)

	scanner := indexing.FileScanner{}
	models := runtime.ModelCache{}
	runtimes := runtime.YzmaResolver{}

	switch command {
	case "search":
		return runSearch(ctx, program, commandArgs, paths, s, scanner, runtimes, models)
	default:
		return runIndex(ctx, program, commandArgs, paths, s, scanner, runtimes, models)
	}
}

func detectCommand(args []string) (string, []string, error) {
	if len(args) == 0 {
		return "index", args, nil
	}

	switch args[0] {
	case "index", "search":
		return args[0], args[1:], nil
	default:
		if len(args[0]) > 0 && args[0][0] == '-' {
			return "index", args, nil
		}
		return "", nil, fmt.Errorf("unknown command %q", args[0])
	}
}
