package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"homevault/internal/jobops"
	"homevault/internal/tui"
)

func runTUIJobs(ctx context.Context, configuration string, timeout time.Duration, args []string) error {
	fs := flag.NewFlagSet("tui-jobs", flag.ContinueOnError)
	render := fs.Bool("render", false, "render local native-job records; never starts a job")
	width := fs.Int("width", 100, "initial/render columns")
	height := fs.Int("height", 30, "initial/render rows")
	ascii := fs.Bool("ascii", false, "ASCII separators")
	noColor := fs.Bool("no-color", false, "monochrome")
	zone := fs.String("timezone", "Local", "display timezone")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if configuration == "" || fs.NArg() != 0 {
		return fmt.Errorf("usage: homevault --config ABSOLUTE_CONFIG.json tui-jobs; explicit manual file jobs only")
	}
	location, err := time.LoadLocation(*zone)
	if err != nil {
		return fmt.Errorf("display timezone unavailable")
	}
	backend, err := jobops.NewWithTimeout(configuration, timeout)
	if err != nil {
		return err
	}
	defer func() { backend.Discard(); backend.Cancel(); backend.Wait() }()
	data, err := backend.Load()
	if err != nil {
		return err
	}
	model, err := tui.NewManualJobs(tui.Options{Width: *width, Height: *height, ASCII: *ascii || os.Getenv("TERM") == "dumb", NoColor: *noColor || *render || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb", Location: location}, data, backend.Load, backend)
	if err != nil {
		return err
	}
	if *render {
		fmt.Println(model.View())
		return nil
	}
	_, err = tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	return err
}
