package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/eugene-panin/kctx/internal/kube"
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true)
	currentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	clientStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	cursorStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	helpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	sectionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true)
)

// Action is what the app should do after the TUI exits.
type Action int

const (
	ActionSelect Action = iota
	ActionPaste
	ActionNamespace
	ActionFavorite
)

// Result is returned after quit.
type Result struct {
	Action      Action
	Selected    string
	Canceled    bool
	Namespace   string
	PasteData   []byte
	PasteClient string
	PasteName   string
}

// Options configures the picker.
type Options struct {
	Entries   []kube.Entry
	Recent    []string
	Favorites []string
	KubePath  string
	Stdin     io.Reader
	Stderr    io.Writer
}

type mode int

const (
	modePick mode = iota
	modePasteClient
	modePasteName
	modeNamespace
)

type item struct {
	entry  kube.Entry
	header string
}

type model struct {
	opts        Options
	all         []kube.Entry
	byName      map[string]kube.Entry
	items       []item
	cursor      int
	filter      textinput.Model
	current     string
	mode        mode
	action      Action
	selected    string
	namespace   string
	pasteData   []byte
	pasteClient string
	pasteName   string
	quitting    bool
	canceled    bool
	height      int
	status      string
	favSet      map[string]bool
}

// Run starts the picker.
func Run(opts Options) (Result, error) {
	ti := textinput.New()
	ti.Placeholder = "фильтр · p вставить · n ns · f избранное"
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 56

	current := ""
	byName := map[string]kube.Entry{}
	for _, e := range opts.Entries {
		byName[e.Name] = e
		if e.Current {
			current = e.Name
		}
	}
	favSet := map[string]bool{}
	for _, f := range opts.Favorites {
		favSet[f] = true
	}

	m := model{
		opts:    opts,
		all:     opts.Entries,
		byName:  byName,
		filter:  ti,
		current: current,
		mode:    modePick,
		favSet:  favSet,
	}
	m.rebuild("")

	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		return Result{Canceled: true}, err
	}
	out := final.(model)
	if out.canceled {
		return Result{Canceled: true}, nil
	}
	if out.action == ActionSelect && out.selected == "" {
		return Result{Canceled: true}, nil
	}
	return Result{
		Action:      out.action,
		Selected:    out.selected,
		Namespace:   out.namespace,
		PasteData:   out.pasteData,
		PasteClient: out.pasteClient,
		PasteName:   out.pasteName,
	}, nil
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch m.mode {
		case modePasteClient, modePasteName, modeNamespace:
			return m.updatePrompt(msg)
		default:
			return m.updatePick(msg)
		}
	}
	return m, nil
}

func (m model) updatePick(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.canceled = true
		m.quitting = true
		return m, tea.Quit
	case tea.KeyEnter:
		if e, ok := m.selectedEntry(); ok {
			m.action = ActionSelect
			m.selected = e.Name
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	case tea.KeyUp:
		m.move(-1)
		return m, nil
	case tea.KeyDown:
		m.move(1)
		return m, nil
	case tea.KeyRunes:
		if m.filter.Value() != "" {
			break
		}
		if len(msg.Runes) != 1 {
			break
		}
		switch msg.Runes[0] {
		case 'q':
			m.canceled = true
			m.quitting = true
			return m, tea.Quit
		case 'p':
			return m.startPaste()
		case 'n':
			return m.startNamespace()
		case 'f':
			if e, ok := m.selectedEntry(); ok {
				m.action = ActionFavorite
				m.selected = e.Name
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

func (m model) updatePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.mode = modePick
		m.status = ""
		m.filter.Placeholder = "фильтр · p вставить · n ns · f избранное"
		m.filter.SetValue("")
		m.rebuild("")
		return m, nil
	case tea.KeyEnter:
		val := strings.TrimSpace(m.filter.Value())
		switch m.mode {
		case modePasteClient:
			if val == "" || strings.Contains(val, "/") {
				m.status = "нужен client без '/'"
				return m, nil
			}
			m.pasteClient = val
			m.mode = modePasteName
			m.filter.SetValue("")
			m.filter.Placeholder = "короткое имя кластера (Enter = из файла)"
			m.status = "клиент: " + val
			return m, nil
		case modePasteName:
			m.pasteName = val
			m.action = ActionPaste
			m.quitting = true
			return m, tea.Quit
		case modeNamespace:
			if val == "-" {
				val = ""
			}
			m.namespace = val
			m.action = ActionNamespace
			m.quitting = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(msg)
	return m, cmd
}

func (m model) startPaste() (tea.Model, tea.Cmd) {
	data, err := clipboard.ReadAll()
	if err != nil || strings.TrimSpace(data) == "" {
		m.status = "буфер пуст — скопируйте kubeconfig и нажмите p"
		return m, nil
	}
	if !looksLikeKubeconfig(data) {
		m.status = "в буфере не похоже на kubeconfig"
		return m, nil
	}
	m.pasteData = []byte(data)
	m.mode = modePasteClient
	m.filter.SetValue("")
	m.filter.Placeholder = "имя клиента для вставки"
	m.status = "вставка из буфера"
	return m, nil
}

func (m model) startNamespace() (tea.Model, tea.Cmd) {
	e, ok := m.selectedEntry()
	if !ok {
		return m, nil
	}
	m.selected = e.Name
	m.mode = modeNamespace
	m.filter.SetValue(e.Namespace)
	m.filter.Placeholder = "namespace (− чтобы сбросить)"
	m.status = "ns для " + e.Name
	return m, nil
}

func looksLikeKubeconfig(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "clusters:") || strings.Contains(low, "contexts:") || strings.Contains(low, "users:")
}

func (m *model) rebuild(query string) {
	filtered := kube.Filter(m.all, query)
	items := make([]item, 0, len(filtered)+16)

	if query == "" {
		if favs := m.sectionEntries(m.opts.Favorites); len(favs) > 0 {
			items = append(items, item{header: "★ избранное"})
			for _, e := range favs {
				items = append(items, item{entry: e})
			}
		}
		if recents := m.sectionEntries(m.opts.Recent); len(recents) > 0 {
			items = append(items, item{header: "недавние"})
			for _, e := range recents {
				items = append(items, item{entry: e})
			}
		}
		if len(items) > 0 {
			items = append(items, item{header: "все"})
		}
	}

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

func (m model) sectionEntries(names []string) []kube.Entry {
	out := make([]kube.Entry, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		e, ok := m.byName[name]
		if !ok {
			continue
		}
		seen[name] = true
		out = append(out, e)
	}
	return out
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
	fmt.Fprintf(&b, "%s  текущий: %s\n", titleStyle.Render("kctx"), currentStyle.Render(cur))
	if m.status != "" {
		fmt.Fprintf(&b, "%s\n", mutedStyle.Render(m.status))
	}
	b.WriteByte('\n')
	b.WriteString(m.filter.View())
	b.WriteString("\n\n")

	if m.mode != modePick {
		b.WriteString(helpStyle.Render("enter подтвердить · esc назад"))
		return b.String()
	}

	if len(m.items) == 0 {
		b.WriteString(mutedStyle.Render("  ничего не найдено"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("enter выбрать · p вставить · esc отмена"))
		return b.String()
	}

	maxRows := m.height - 9
	if maxRows < 8 {
		maxRows = 16
	}
	start, end := window(m.cursor, len(m.items), maxRows)
	for i := start; i < end; i++ {
		it := m.items[i]
		if it.header != "" {
			style := clientStyle
			if it.header == "★ избранное" || it.header == "недавние" || it.header == "все" {
				style = sectionStyle
			}
			fmt.Fprintf(&b, "%s\n", style.Render(it.header))
			continue
		}
		prefix := "  "
		name := it.entry.Short
		if i == m.cursor {
			prefix = cursorStyle.Render("▸ ")
			name = cursorStyle.Render(it.entry.Short)
		}
		extra := ""
		if m.favSet[it.entry.Name] {
			extra += " " + currentStyle.Render("★")
		}
		if it.entry.Namespace != "" {
			extra += mutedStyle.Render("  ns/" + it.entry.Namespace)
		}
		if it.entry.Current {
			extra += " " + currentStyle.Render("●")
		}
		fmt.Fprintf(&b, "%s%s%s\n", prefix, name, extra)
	}
	b.WriteByte('\n')
	b.WriteString(helpStyle.Render("enter выбрать · p буфер · n ns · f избранное · esc"))
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
