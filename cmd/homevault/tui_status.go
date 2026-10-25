package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"homevault/internal/statusdata"
	"homevault/internal/statusview"
	"homevault/internal/tui"
)

func runTUIStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("tui-status", flag.ContinueOnError)
	plan := fs.String("plan", "", "explicit coordinator plan; reads metadata only")
	render := fs.Bool("render", false, "print one local-record frame without opening a terminal")
	width := fs.Int("width", 100, "initial/render columns")
	height := fs.Int("height", 30, "initial/render rows")
	ascii := fs.Bool("ascii", false, "ASCII separators and cursor")
	noColor := fs.Bool("no-color", false, "disable ANSI color")
	page := fs.String("page", "overview", "overview or projects")
	project := fs.String("project", "", "select a workflow ID from the plan")
	details := fs.Bool("details", false, "open project details")
	displayContext := fs.String("context", "unknown", "operator label: lab, production or unknown; never inferred")
	zone := fs.String("timezone", "Local", "display timezone (Local, UTC or IANA name)")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *plan == "" || fs.NArg() != 0 {
		return fmt.Errorf("usage: tui-status --plan ABSOLUTE_PLAN.json [--render]; read-only local records, no engine actions")
	}
	location, err := time.LoadLocation(*zone)
	if err != nil {
		return fmt.Errorf("display timezone unavailable; use Local, UTC or an installed IANA timezone")
	}
	load := func() (statusdata.Snapshot, error) {
		if ctx.Err() != nil {
			return statusdata.Snapshot{}, fmt.Errorf("local read cancelled")
		}
		return statusview.Read(*plan, time.Now().UTC())
	}
	data, err := load()
	if err != nil {
		return err
	}
	model, err := tui.NewReadOnly(tui.Options{Workflows: true, Context: *displayContext, Width: *width, Height: *height, ASCII: *ascii || os.Getenv("TERM") == "dumb", NoColor: *noColor || *render || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb", Page: *page, Project: *project, Details: *details, Location: location}, data, load)
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
