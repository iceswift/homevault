package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func key(t *testing.T, m Model, key string) Model {
	t.Helper()
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		msg = tea.KeyMsg{Type: tea.KeyTab}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "pgdown":
		msg = tea.KeyMsg{Type: tea.KeyPgDown}
	case "backspace":
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	}
	next, cmd := m.Update(msg)
	if cmd != nil {
		t.Fatalf("unexpected command for %s", key)
	}
	return next.(Model)
}
func TestFramesStayWithinTerminal(t *testing.T) {
	for _, size := range [][2]int{{120, 32}, {110, 24}, {100, 26}, {80, 24}, {60, 30}, {40, 18}, {32, 12}, {18, 6}} {
		for _, ascii := range []bool{false, true} {
			for _, detail := range []bool{false, true} {
				t.Run(fmt.Sprint(size, ascii, detail), func(t *testing.T) {
					m, err := New(Options{Width: size[0], Height: size[1], ASCII: ascii, Details: detail, Project: "notes"})
					if err != nil {
						t.Fatal(err)
					}
					views := []string{m.View()}
					m = key(t, m, "?")
					views = append(views, m.View())
					m = key(t, m, "esc")
					m = key(t, m, "/")
					m = key(t, m, "missing")
					views = append(views, m.View())
					for _, view := range views {
						lines := strings.Split(view, "\n")
						if len(lines) > size[1] {
							t.Fatalf("height overflow: %d", len(lines))
						}
						for _, line := range lines {
							if ansi.StringWidth(line) > size[0] {
								t.Fatalf("width overflow: %q", line)
							}
						}
						if !strings.Contains(ansi.Strip(view), "DEMO") && !strings.Contains(ansi.Strip(view), "DESIGN PREVIEW") {
							t.Fatal("missing synthetic label")
						}
					}
				})
			}
		}
	}
}
func TestSearchDetailsAndBackPreserveContext(t *testing.T) {
	m, _ := New(Options{NoColor: true})
	m = key(t, m, "2")
	m = key(t, m, "/")
	m = key(t, m, "notes")
	m = key(t, m, "enter")
	if m.page != 1 || len(m.filtered()) != 1 || m.filtered()[0].id != "notes" {
		t.Fatal("filter or page lost")
	}
	m = key(t, m, "enter")
	if !m.details || !strings.Contains(m.View(), "Stale source") {
		t.Fatal("missing filtered details")
	}
	m = key(t, m, "pgdown")
	m = key(t, m, "esc")
	if m.details || m.query != "notes" || m.page != 1 {
		t.Fatal("back lost context")
	}
	m = key(t, m, "/")
	m = key(t, m, "esc")
	if m.query != "" || len(m.filtered()) != 4 {
		t.Fatal("escape did not clear filter")
	}
	m = key(t, m, "/")
	m = key(t, m, "q")
	if !m.searching || m.query != "q" {
		t.Fatal("q quit while filtering")
	}
	m = key(t, m, "backspace")
	if m.query != "" {
		t.Fatal("backspace failed")
	}
}
func TestOperationsAreInertAndStatusesStayDistinct(t *testing.T) {
	m, _ := New(Options{Width: 120, Height: 32, NoColor: true, Project: "notes", Details: true})
	view := m.View()
	for _, text := range []string{"Stale source", "Backup + check passed", "50h ago", "1h ago"} {
		if !strings.Contains(view, text) {
			t.Fatalf("missing %s", text)
		}
	}
	for _, operation := range []string{"b", "c", "r", "d", "a", "s", "p"} {
		m = key(t, m, operation)
		if !strings.Contains(m.View(), "No job started") {
			t.Fatal("operation did not explain its inert state")
		}
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("Ctrl+C did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("unexpected quit command")
	}
}
func TestSelectionRemainsVisibleAndASCII(t *testing.T) {
	m, _ := New(Options{Width: 40, Height: 20, ASCII: true, NoColor: true})
	for i := 0; i < 20; i++ {
		m = key(t, m, "down")
	}
	if m.filtered()[m.selected].id != "photos" || !strings.Contains(m.View(), "> Photos demo") {
		t.Fatal("selected row not visible")
	}
	for _, r := range m.View() {
		if r > 127 {
			t.Fatal("non-ASCII output in fallback")
		}
	}
	if strings.Contains(m.View(), "\x1b") {
		t.Fatal("color in monochrome view")
	}
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	m = next.(Model)
	if !strings.Contains(m.View(), "Photos demo") || m.selected != 3 {
		t.Fatal("resize lost selection")
	}
}
func TestInvalidOptionsFailClosed(t *testing.T) {
	for _, options := range []Options{{Width: -1}, {Height: 201}, {Page: "snapshots"}, {Project: "production"}} {
		if _, err := New(options); err == nil {
			t.Fatal("invalid demo option accepted")
		}
	}
}

func TestDetailsDoNotAccumulateHiddenScroll(t *testing.T) {
	m, _ := New(Options{Width: 60, Height: 22, Details: true, NoColor: true, Project: "notes"})
	for i := 0; i < 100; i++ {
		m = key(t, m, "pgdown")
	}
	end := m.offset
	if end == 0 {
		t.Fatal("detail fixture should scroll")
	}
	m = key(t, m, "up")
	if m.offset != end-1 {
		t.Fatal("hidden offset delayed upward scrolling")
	}
	m = key(t, m, "?")
	for i := 0; i < 100; i++ {
		m = key(t, m, "pgdown")
	}
	end = m.offset
	m = key(t, m, "up")
	if m.offset >= end {
		t.Fatal("help did not scroll back immediately")
	}
}

func TestTableHeaderAlignsWithValues(t *testing.T) {
	m, _ := New(Options{Width: 120, Height: 32, NoColor: true, ASCII: true})
	var heading, row string
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "SOURCE AGE") {
			heading = line
		}
		if strings.Contains(line, "Shop demo") {
			row = line
		}
	}
	if heading == "" || row == "" || strings.Index(heading, "RESULT") != strings.Index(row, "Passed") || strings.Index(heading, "SOURCE AGE") != strings.Index(row, "2h") {
		t.Fatal("headers and values are misaligned")
	}
}

func TestPastedSearchCannotEmitTerminalControls(t *testing.T) {
	m, _ := New(Options{Width: 60, Height: 26, NoColor: true})
	m = key(t, m, "/")
	m = key(t, m, "notes\x1b[31m\n\r\t\x00"+strings.Repeat("x", 100))
	if strings.ContainsAny(m.query, "\x1b\n\r\t\x00") || len([]rune(m.query)) > 80 {
		t.Fatal("unsafe or oversized pasted filter")
	}
	if strings.Contains(m.View(), "\x1b") {
		t.Fatal("search emitted a terminal escape")
	}
	if !m.searching {
		t.Fatal("pasted control characters triggered navigation")
	}
}
