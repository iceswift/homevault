package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"homevault/internal/tuidemo"
)

func runTUIDemo(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("tui-demo", flag.ContinueOnError)
	render := fs.Bool("render", false, "print one deterministic frame instead of opening a terminal")
	width := fs.Int("width", 100, "initial/render width; terminal resize overrides this interactively")
	height := fs.Int("height", 30, "initial/render height")
	ascii := fs.Bool("ascii", false, "use ASCII separators and selection cursor")
	noColor := fs.Bool("no-color", false, "disable ANSI color")
	page := fs.String("page", "overview", "sample page: overview or projects")
	project := fs.String("project", "", "sample selection: shop, notes, files or photos")
	details := fs.Bool("details", false, "start in full sample project details")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("tui-demo accepts flags only; it never opens a real project")
	}
	m, err := tuidemo.New(tuidemo.Options{Width: *width, Height: *height, ASCII: *ascii || os.Getenv("TERM") == "dumb", NoColor: *noColor || *render || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb", Page: *page, Project: *project, Details: *details})
	if err != nil {
		return err
	}
	if *render {
		fmt.Println(m.View())
		return nil
	}
	_, err = tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	return err
}
