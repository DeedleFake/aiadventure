// Command aiadventure is an AI harness for open-ended adventure games.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"deedles.dev/aiadventure/internal/app"
	"deedles.dev/aiadventure/internal/config"
)

func main() {
	os.Exit(run())
}

func run() int {
	fs := flag.NewFlagSet("aiadventure", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var (
		showHelp    bool
		showVersion bool
		configPath  string
		sessionsDir string
	)
	fs.BoolVar(&showHelp, "help", false, "show help")
	fs.BoolVar(&showHelp, "h", false, "show help")
	fs.BoolVar(&showVersion, "version", false, "show version")
	fs.StringVar(&configPath, "config", "", "path to config.json (default: user config dir)")
	fs.StringVar(&sessionsDir, "sessions-dir", "", "directory for adventure sessions")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}

	if showHelp {
		printUsage(fs)
		return 0
	}
	if showVersion {
		fmt.Println("aiadventure 0.1.0")
		return 0
	}

	paths, err := config.DefaultPaths(configPath, sessionsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config paths: %v\n", err)
		return 1
	}
	cfg, err := config.Load(paths, config.Options{
		SessionsDirOverride: sessionsDir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		return 1
	}
	// When --sessions-dir was not set, paths.SessionsDir is default;
	// Load may have filled SessionsDir from file — keep file value.
	// When --sessions-dir was set, override is applied above.
	if err := config.EnsureDirs(cfg, paths); err != nil {
		fmt.Fprintf(os.Stderr, "ensure dirs: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	application := app.New(cfg, paths)
	if err := application.Run(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func printUsage(fs *flag.FlagSet) {
	fmt.Fprintf(os.Stdout, `AI Adventure — AI-powered open-ended adventure games

Usage:
  aiadventure [flags]

Flags:
`)
	fs.SetOutput(os.Stdout)
	fs.PrintDefaults()
	fmt.Fprintf(os.Stdout, `
Launches a keyboard-driven terminal UI (Bubble Tea) for xAI OAuth sign-in,
model/effort selection, session create/list/search, brainstorming then
adventure play, editing prior turns (manual or AI-assisted), branch
navigation, and out-of-band feedback for future AI responses.

Sessions are saved automatically under the configured sessions directory.
`)
}
