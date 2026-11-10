package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"kctx/internal/kube"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true)
	currentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	clientStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)

// Result is what the TUI returns after quit.
type Result struct {
	Selected string
	Canceled bool
}

type item struct {
	entry  kube.Entry
	header string // non-empty => client header row, not selectable
}

type model struct {
	all      []kube.Entry
	items    []item
	cursor   int
	filter   textinput.Model
	current  string
	selected string
	quitting bool
	canceled bool
	height   int
}

// Run starts the picker. Selected context name is in Result.Selected.
func Run(entries []kube.Entry) (Result, error) {
	ti := textinput.New()
	ti.Placeholder = "фильтр: клиент или кластер…"
	ti.Focus()
	ti.CharLimit = 128
	ti.Width = 48

	current := ""
	for _, e := range entries {
		if e.Current {
			current = e.Name
			break
		}
	}

	m := model{
		all:     entries,
		filter:  ti,
		current: current,
	}
	m.rebuild("")

	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return Result{Canceled: true}, err
	}
	out := final.(model)
	if out.canceled || out.selected == "" {
		return Result{Canceled: true}, nil
	}
	return Result{Selected: out.selected}, nil
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.canceled = true
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			if e, ok := m.selectedEntry(); ok {
				m.selected = e.Name
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		case tea.KeyUp, tea.KeyCtrlP:
			m.move(-1)
			return m, nil
		case tea.KeyDown, tea.KeyCtrlN:
			m.move(1)
			return m, nil
		case tea.KeyRunes:
			if len(msg.Runes) == 1 && msg.Runes[0] == 'q' && m.filter.Value() == "" {
				m.canceled = true
				m.quitting = true
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	prev := m.filter.Value()
	m.filter, cmd = m.filter.Update(msg)
	if m.filter.Value() != prev {
		m.rebuild(m.filter.Value())
	}
	return m, cmd
}

func (m *model) rebuild(query string) {
	filtered := kube.Filter(m.all, query)
	items := make([]item, 0, len(filtered)+8)
	prev := ""
	for _, e := range filtered {
		if e.Client != prev {
			items = append(items, item{header: e.Client})
			prev = e.Client
		}
		items = append(items, item{entry: e})
	}
	m.items = items
	m.cursor = firstSelectable(items)
}

func firstSelectable(items []item) int {
	for i, it := range items {
		if it.header == "" {
			return i
		}
	}
	return 0
}

func (m *model) move(delta int) {
	if len(m.items) == 0 {
		return
	}
	for range m.items {
		m.cursor += delta
		if m.cursor < 0 {
			m.cursor = len(m.items) - 1
		}
		if m.cursor >= len(m.items) {
			m.cursor = 0
		}
		if m.items[m.cursor].header == "" {
			return
		}
	}
}

func (m model) selectedEntry() (kube.Entry, bool) {
	if len(m.items) == 0 || m.cursor < 0 || m.cursor >= len(m.items) {
		return kube.Entry{}, false
	}
	it := m.items[m.cursor]
	if it.header != "" {
		return kube.Entry{}, false
	}
	return it.entry, true
}

func (m model) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	cur := m.current
	if cur == "" {
		cur = "(не задан)"
	}
	fmt.Fprintf(&b, "%s  текущий: %s\n\n", titleStyle.Render("kctx"), currentStyle.Render(cur))
	b.WriteString(m.filter.View())
	b.WriteString("\n\n")

	if len(m.items) == 0 {
		b.WriteString(mutedStyle.Render("  ничего не найдено"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("enter выбрать · esc отмена · ↑↓ навигация"))
		return b.String()
	}

	maxRows := m.height - 8
	if maxRows < 8 {
		maxRows = 16
	}
	start, end := window(m.cursor, len(m.items), maxRows)
	for i := start; i < end; i++ {
		it := m.items[i]
		if it.header != "" {
			fmt.Fprintf(&b, "%s\n", clientStyle.Render(it.header))
			continue
		}
		prefix := "  "
		name := it.entry.Short
		if i == m.cursor {
			prefix = cursorStyle.Render("▸ ")
			name = cursorStyle.Render(it.entry.Short)
		}
		extra := ""
		if it.entry.Namespace != "" {
			extra += mutedStyle.Render("  ns/" + it.entry.Namespace)
		}
		if it.entry.Current {
			extra += " " + currentStyle.Render("★")
		}
		fmt.Fprintf(&b, "%s%s%s\n", prefix, name, extra)
	}

	b.WriteByte('\n')
	b.WriteString(helpStyle.Render("enter выбрать · esc отмена · ↑↓ навигация"))
	return b.String()
}

func window(cursor, total, maxRows int) (start, end int) {
	if total <= maxRows {
		return 0, total
	}
	start = cursor - maxRows/2
	if start < 0 {
		start = 0
	}
	end = start + maxRows
	if end > total {
		end = total
		start = end - maxRows
		if start < 0 {
			start = 0
		}
	}
	return start, end
}
