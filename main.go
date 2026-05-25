package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── commit item ───────────────────────────────────────────────────────────────

type commitItem struct {
	id      string // hash or stash ref
	display string // full oneline string shown in the list
}

func (c commitItem) Title() string       { return c.display }
func (c commitItem) Description() string { return "" }
func (c commitItem) FilterValue() string { return c.display }

// ── styles ────────────────────────────────────────────────────────────────────

var (
	activeBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205"))
	dimBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("238"))

	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cmdPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	infoStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	addStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	delStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	hunkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	metaStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	fileStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))
)

// ── model ─────────────────────────────────────────────────────────────────────

type appMode int

const (
	normalMode appMode = iota
	commandMode
)

type focusedPane int

const (
	listFocus focusedPane = iota
	diffFocus
)

type model struct {
	commits  list.Model
	diff     viewport.Model
	cmdInput textinput.Model

	mode   appMode
	focus  focusedPane
	gitCmd string
	files  string

	width  int
	height int
	err    error
	status string
	ready  bool
}

// ── messages ──────────────────────────────────────────────────────────────────

type commitsMsg struct {
	ids      []string
	displays []string
	files    string
	gitCmd   string
}

type diffMsg struct{ content string }
type errMsg struct{ err error }

// ── init ──────────────────────────────────────────────────────────────────────

func newModel() model {
	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	d.SetSpacing(0)
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(lipgloss.Color("205")).
		BorderForeground(lipgloss.Color("205"))
	d.Styles.NormalTitle = d.Styles.NormalTitle.
		Foreground(lipgloss.Color("252"))

	l := list.New(nil, d, 0, 0)
	l.Title = "git log"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.SetShowStatusBar(true)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	// Let our global handler own q and ctrl+c.
	l.KeyMap.Quit.SetEnabled(false)
	l.KeyMap.ForceQuit.SetEnabled(false)

	ti := textinput.New()
	ti.Placeholder = "e.g. git log --oneline -50"
	ti.Prompt = ""

	return model{
		commits:  l,
		cmdInput: ti,
		gitCmd:   "git log",
		mode:     normalMode,
		focus:    listFocus,
	}
}

func (m model) Init() tea.Cmd {
	return runGitCmd("git log")
}

// ── sizing helpers ────────────────────────────────────────────────────────────

func (m model) listOuterW() int {
	if m.width == 0 {
		return 40
	}
	return max(m.width*33/100, 20)
}

func (m model) diffOuterW() int { return m.width - m.listOuterW() }
func (m model) listInnerW() int { return max(m.listOuterW()-2, 1) }
func (m model) diffInnerW() int { return max(m.diffOuterW()-2, 1) }

func (m model) innerH() int {
	if m.height < 5 {
		return 1
	}
	// Terminal height minus: border top/bottom (2) + status bar (1).
	return m.height - 3
}

// ── update ────────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.cmdInput.Width = m.width - 8
		m.commits.SetSize(m.listInnerW(), m.innerH())
		if !m.ready {
			m.diff = viewport.New(m.diffInnerW(), m.innerH())
			m.ready = true
		} else {
			m.diff.Width = m.diffInnerW()
			m.diff.Height = m.innerH()
		}

	case commitsMsg:
		items := make([]list.Item, len(msg.ids))
		for i := range msg.ids {
			items[i] = commitItem{id: msg.ids[i], display: msg.displays[i]}
		}
		m.commits.SetItems(items)
		m.commits.Title = msg.gitCmd
		m.commits.ResetFilter()
		m.gitCmd = msg.gitCmd
		m.files = msg.files
		m.err = nil
		m.status = ""
		m.focus = listFocus
		if len(msg.ids) == 0 {
			m.status = "No commits found"
			m.diff.SetContent("")
		} else {
			cmds = append(cmds, loadDiff(msg.ids[0], msg.files))
		}

	case diffMsg:
		m.diff.SetContent(msg.content)
		m.diff.GotoTop()

	case errMsg:
		m.err = msg.err
		m.status = ""

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		// ── command mode: all keys go to the input ─────────────────────────
		if m.mode == commandMode {
			switch msg.String() {
			case "enter":
				cmd := strings.TrimSpace(m.cmdInput.Value())
				m.mode = normalMode
				m.cmdInput.Blur()
				m.status = "Running…"
				m.err = nil
				cmds = append(cmds, runGitCmd(cmd))
			case "esc":
				m.mode = normalMode
				m.cmdInput.Blur()
			default:
				var cmd tea.Cmd
				m.cmdInput, cmd = m.cmdInput.Update(msg)
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		// ── normal mode ────────────────────────────────────────────────────
		filtering := m.focus == listFocus && m.commits.FilterState() == list.Filtering
		handled := false

		if !filtering {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "tab":
				if m.focus == listFocus {
					m.focus = diffFocus
				} else {
					m.focus = listFocus
				}
				handled = true
			case "c":
				m.mode = commandMode
				m.cmdInput.SetValue(m.gitCmd)
				cmds = append(cmds, m.cmdInput.Focus())
				handled = true
			case "s":
				m.status = "Running…"
				m.err = nil
				cmds = append(cmds, runGitCmd("git stash list"))
				handled = true
			}
		}

		if !handled {
			switch m.focus {
			case listFocus:
				prevIdx := m.commits.Index()
				var cmd tea.Cmd
				m.commits, cmd = m.commits.Update(msg)
				cmds = append(cmds, cmd)
				if newIdx := m.commits.Index(); newIdx != prevIdx {
					if item, ok := m.commits.SelectedItem().(commitItem); ok {
						cmds = append(cmds, loadDiff(item.id, m.files))
					}
				}
			case diffFocus:
				var cmd tea.Cmd
				m.diff, cmd = m.diff.Update(msg)
				cmds = append(cmds, cmd)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// ── view ──────────────────────────────────────────────────────────────────────

func (m model) View() string {
	if !m.ready {
		return "\n  Loading…"
	}

	listBorder := dimBorder
	if m.focus == listFocus && m.mode == normalMode {
		listBorder = activeBorder
	}
	listBox := listBorder.
		Width(m.listInnerW()).Height(m.innerH()).
		Render(m.commits.View())

	diffBorder := dimBorder
	if m.focus == diffFocus {
		diffBorder = activeBorder
	}
	diffBox := diffBorder.
		Width(m.diffInnerW()).Height(m.innerH()).
		Render(m.diff.View())

	panels := lipgloss.JoinHorizontal(lipgloss.Top, listBox, diffBox)

	bar := m.statusBar()
	return lipgloss.JoinVertical(lipgloss.Left, panels, bar)
}

func (m model) statusBar() string {
	if m.mode == commandMode {
		return cmdPromptStyle.Render("cmd: ") + m.cmdInput.View()
	}
	if m.err != nil {
		return errStyle.Render("Error: " + m.err.Error())
	}
	if m.status != "" {
		return infoStyle.Render(m.status)
	}
	if m.focus == listFocus {
		return statusStyle.Render("j/k: navigate  /: filter  s: stash  c: command  tab: diff pane  q: quit")
	}
	return statusStyle.Render("j/k: scroll  tab: list pane  q: quit")
}

// ── diff highlighting ─────────────────────────────────────────────────────────

func highlightDiff(patch string) string {
	var sb strings.Builder
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			sb.WriteString(fileStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			sb.WriteString(addStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			sb.WriteString(delStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			sb.WriteString(hunkStyle.Render(line))
		case strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "commit ") || strings.HasPrefix(line, "Author:") ||
			strings.HasPrefix(line, "Date:") || strings.HasPrefix(line, "Merge:"):
			sb.WriteString(metaStyle.Render(line))
		default:
			sb.WriteString(line)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ── tea.Cmd factories ─────────────────────────────────────────────────────────

func runGitCmd(cmd string) tea.Cmd {
	return func() tea.Msg {
		ids, displays, files, finalCmd, err := processGitCommand(cmd)
		if err != nil {
			return errMsg{err}
		}
		return commitsMsg{ids: ids, displays: displays, files: files, gitCmd: finalCmd}
	}
}

func loadDiff(commit, files string) tea.Cmd {
	return func() tea.Msg {
		patch, err := getPatch(commit, files)
		if err != nil {
			return errMsg{err}
		}
		return diffMsg{highlightDiff(patch)}
	}
}

// ── main ──────────────────────────────────────────────────────────────────────

func main() {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
