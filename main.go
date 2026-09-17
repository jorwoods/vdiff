package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── commit item ───────────────────────────────────────────────────────────────

type commitItem struct {
	id      string
	display string
}

func (c commitItem) Title() string       { return c.display }
func (c commitItem) Description() string { return "" }
func (c commitItem) FilterValue() string { return c.display }

// ── styles ────────────────────────────────────────────────────────────────────

var (
	activeBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205"))
	dimBorder      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238"))
	statusStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cmdPromptStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	infoStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("248"))
	errStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	// Diff syntax
	addStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	delStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	hunkStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	metaStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	fileStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11"))

	// Diff overlays (applied on top of syntax styles)
	visualSelStyle     = lipgloss.NewStyle().Background(lipgloss.Color("237"))                                  // dim gray bg
	searchMatchStyle   = lipgloss.NewStyle().Background(lipgloss.Color("58"))                                   // dark amber
	searchCurrentStyle = lipgloss.NewStyle().Background(lipgloss.Color("228")).Foreground(lipgloss.Color("16")) // bright yellow, black fg
)

// ── model ─────────────────────────────────────────────────────────────────────

type appMode int

const (
	normalMode  appMode = iota
	commandMode         // typing a new git command
	searchMode          // typing a search term in the diff
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

	mode  appMode
	focus focusedPane

	gitCmd string
	files  string

	// Raw diff lines — needed to rebuild content on search/visual state changes.
	rawLines []string

	// Search state
	searchInput   textinput.Model
	searchTerm    string
	searchMatches []int
	searchIdx     int

	// Visual-select state
	visualActive bool
	visualAnchor int // line where v was pressed
	cursorLine   int // current cursor in visual mode

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

// diffMsg carries raw (un-highlighted) lines so the model can re-render with
// search and visual overlays applied.
type diffMsg struct{ rawLines []string }
type errMsg struct{ err error }

// ── init ──────────────────────────────────────────────────────────────────────

func newModel() model {
	d := list.NewDefaultDelegate()
	d.ShowDescription = false
	d.SetHeight(1)
	d.SetSpacing(0)
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.
		Foreground(lipgloss.Color("205")).BorderForeground(lipgloss.Color("205"))
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(lipgloss.Color("252"))

	l := list.New(nil, d, 0, 0)
	l.Title = "git log"
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.SetShowStatusBar(true)
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	l.KeyMap.Quit.SetEnabled(false)
	l.KeyMap.ForceQuit.SetEnabled(false)

	cmd := textinput.New()
	cmd.Placeholder = "e.g. git log --oneline -50"
	cmd.Prompt = ""

	srch := textinput.New()
	srch.Placeholder = "search…"
	srch.Prompt = ""

	return model{
		commits:     l,
		cmdInput:    cmd,
		searchInput: srch,
		gitCmd:      "git log",
		mode:        normalMode,
		focus:       listFocus,
	}
}

func (m model) Init() tea.Cmd {
	return runGitCmd("git log")
}

// ── sizing ────────────────────────────────────────────────────────────────────

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
	return m.height - 3 // border top/bottom (2) + status bar (1)
}

// ── update ────────────────────────────────────────────────────────────────────

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.cmdInput.Width = m.width - 8
		m.searchInput.Width = m.width - 8
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
		m.rawLines = msg.rawLines
		m.searchMatches = findMatches(m.rawLines, m.searchTerm)
		m.searchIdx = 0
		m.visualActive = false
		m.cursorLine = 0
		m.diff.GotoTop()
		m.diff.SetContent(buildDiffContent(m))

	case errMsg:
		m.err = msg.err
		m.status = ""

	default:
		// Forward internal list messages (e.g. FilterMatchesMsg, spinner ticks)
		// so async filtering completes correctly.
		var cmd tea.Cmd
		m.commits, cmd = m.commits.Update(msg)
		cmds = append(cmds, cmd)

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		// ── command mode ──────────────────────────────────────────────────────
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

		// ── search mode ───────────────────────────────────────────────────────
		if m.mode == searchMode {
			switch msg.String() {
			case "enter":
				m.mode = normalMode
				m.searchInput.Blur()
				// Scroll to first match if any.
				if len(m.searchMatches) > 0 {
					scrollToLine(&m.diff, m.searchMatches[m.searchIdx])
				}
			case "esc":
				m.mode = normalMode
				m.searchInput.Blur()
				m.searchTerm = ""
				m.searchMatches = nil
				m.searchIdx = 0
				m.diff.SetContent(buildDiffContent(m))
			default:
				var cmd tea.Cmd
				m.searchInput, cmd = m.searchInput.Update(msg)
				cmds = append(cmds, cmd)
				// Live search: recompute matches and re-render as the user types.
				m.searchTerm = m.searchInput.Value()
				m.searchMatches = findMatches(m.rawLines, m.searchTerm)
				m.searchIdx = 0
				m.diff.SetContent(buildDiffContent(m))
				if len(m.searchMatches) > 0 {
					scrollToLine(&m.diff, m.searchMatches[0])
				}
			}
			return m, tea.Batch(cmds...)
		}

		// ── normal mode ───────────────────────────────────────────────────────
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
				if m.focus == listFocus {
					m.status = "Running…"
					m.err = nil
					cmds = append(cmds, runGitCmd("git stash list"))
					handled = true
				}
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
				m, cmds = handleDiffKey(m, msg.String(), cmds)
			}
		}
	}

	return m, tea.Batch(cmds...)
}

// handleDiffKey processes key events when the diff pane is focused.
func handleDiffKey(m model, key string, cmds []tea.Cmd) (model, []tea.Cmd) {
	switch key {

	case "/":
		m.mode = searchMode
		m.searchInput.SetValue("")
		cmds = append(cmds, m.searchInput.Focus())

	case "n":
		if len(m.searchMatches) > 0 {
			m.searchIdx = (m.searchIdx + 1) % len(m.searchMatches)
			m.diff.SetContent(buildDiffContent(m))
			scrollToLine(&m.diff, m.searchMatches[m.searchIdx])
		}

	case "N":
		if len(m.searchMatches) > 0 {
			m.searchIdx = (m.searchIdx - 1 + len(m.searchMatches)) % len(m.searchMatches)
			m.diff.SetContent(buildDiffContent(m))
			scrollToLine(&m.diff, m.searchMatches[m.searchIdx])
		}

	case "d":
		m.diff.HalfViewDown()

	case "u":
		m.diff.HalfViewUp()

	case "v":
		m.visualActive = !m.visualActive
		if m.visualActive {
			m.cursorLine = m.diff.YOffset
			m.visualAnchor = m.cursorLine
		}
		m.diff.SetContent(buildDiffContent(m))

	case "y":
		if m.visualActive {
			lo, hi := visualRange(m.visualAnchor, m.cursorLine, len(m.rawLines))
			text := strings.Join(m.rawLines[lo:hi+1], "\n")
			if err := clipboard.WriteAll(text); err != nil {
				m.err = err
			} else {
				m.status = fmt.Sprintf("Copied %d lines", hi-lo+1)
			}
			m.visualActive = false
			m.diff.SetContent(buildDiffContent(m))
		}

	case "esc":
		if m.visualActive {
			m.visualActive = false
			m.diff.SetContent(buildDiffContent(m))
		} else if m.searchTerm != "" {
			m.searchTerm = ""
			m.searchMatches = nil
			m.searchIdx = 0
			m.diff.SetContent(buildDiffContent(m))
		}

	case "j", "down":
		if m.visualActive {
			if m.cursorLine < len(m.rawLines)-1 {
				m.cursorLine++
				if m.cursorLine >= m.diff.YOffset+m.diff.Height {
					m.diff.LineDown(1)
				}
				m.diff.SetContent(buildDiffContent(m))
			}
		} else {
			var cmd tea.Cmd
			m.diff, cmd = m.diff.Update(tea.KeyMsg{Type: tea.KeyDown})
			cmds = append(cmds, cmd)
		}

	case "k", "up":
		if m.visualActive {
			if m.cursorLine > 0 {
				m.cursorLine--
				if m.cursorLine < m.diff.YOffset {
					m.diff.LineUp(1)
				}
				m.diff.SetContent(buildDiffContent(m))
			}
		} else {
			var cmd tea.Cmd
			m.diff, cmd = m.diff.Update(tea.KeyMsg{Type: tea.KeyUp})
			cmds = append(cmds, cmd)
		}

	default:
		var cmd tea.Cmd
		m.diff, cmd = m.diff.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		cmds = append(cmds, cmd)
	}

	return m, cmds
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
	listBox := listBorder.Width(m.listInnerW()).Height(m.innerH()).Render(m.commits.View())

	diffBorder := dimBorder
	if m.focus == diffFocus && m.mode == normalMode {
		diffBorder = activeBorder
	}
	diffBox := diffBorder.Width(m.diffInnerW()).Height(m.innerH()).Render(m.diff.View())

	panels := lipgloss.JoinHorizontal(lipgloss.Top, listBox, diffBox)
	return lipgloss.JoinVertical(lipgloss.Left, panels, m.statusBar())
}

func (m model) statusBar() string {
	switch m.mode {
	case commandMode:
		return cmdPromptStyle.Render("cmd: ") + m.cmdInput.View()
	case searchMode:
		suffix := ""
		if m.searchTerm != "" {
			if len(m.searchMatches) == 0 {
				suffix = statusStyle.Render("  [no matches]")
			} else {
				suffix = statusStyle.Render(fmt.Sprintf("  [%d/%d]", m.searchIdx+1, len(m.searchMatches)))
			}
		}
		return cmdPromptStyle.Render("/") + m.searchInput.View() + suffix
	}

	if m.err != nil {
		return errStyle.Render("Error: " + m.err.Error())
	}
	if m.status != "" {
		return infoStyle.Render(m.status)
	}

	if m.focus == listFocus {
		return statusStyle.Render("j/k: navigate  /: filter  s: stash  c: command  tab: diff  q: quit")
	}
	if m.visualActive {
		lo, hi := visualRange(m.visualAnchor, m.cursorLine, len(m.rawLines))
		return statusStyle.Render(fmt.Sprintf("VISUAL  %d lines  j/k: extend  y: copy  esc: cancel", hi-lo+1))
	}
	if m.searchTerm != "" && len(m.searchMatches) > 0 {
		return statusStyle.Render(fmt.Sprintf("/%s  [%d/%d]  n/N: next/prev  esc: clear", m.searchTerm, m.searchIdx+1, len(m.searchMatches)))
	}
	return statusStyle.Render("j/k: scroll  d/u: half-page  /: search  v: visual  tab: list  q: quit")
}

// ── diff rendering ────────────────────────────────────────────────────────────

func buildDiffContent(m model) string {
	return renderDiff(m.rawLines, m.searchTerm, m.searchMatches, m.searchIdx,
		m.visualActive, m.visualAnchor, m.cursorLine)
}

func renderDiff(lines []string, searchTerm string, matches []int, matchIdx int,
	visualActive bool, visualAnchor, cursorLine int) string {

	matchSet := make(map[int]struct{}, len(matches))
	for _, i := range matches {
		matchSet[i] = struct{}{}
	}
	currentMatch := -1
	if len(matches) > 0 {
		currentMatch = matches[matchIdx]
	}

	visualLo, visualHi := visualAnchor, cursorLine
	if visualLo > visualHi {
		visualLo, visualHi = visualHi, visualLo
	}

	var sb strings.Builder
	for i, line := range lines {
		style := diffLineStyle(line)

		_, inMatchSet := matchSet[i]
		switch {
		case visualActive && i == cursorLine:
			style = style.Reverse(true)
		case visualActive && i >= visualLo && i <= visualHi:
			style = visualSelStyle
		case i == currentMatch:
			style = searchCurrentStyle
		case inMatchSet:
			style = style.Inherit(searchMatchStyle)
		}

		sb.WriteString(style.Render(line))
		sb.WriteByte('\n')
	}
	return sb.String()
}

func diffLineStyle(line string) lipgloss.Style {
	switch {
	case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
		return fileStyle
	case strings.HasPrefix(line, "+"):
		return addStyle
	case strings.HasPrefix(line, "-"):
		return delStyle
	case strings.HasPrefix(line, "@@"):
		return hunkStyle
	case strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") ||
		strings.HasPrefix(line, "commit ") || strings.HasPrefix(line, "Author:") ||
		strings.HasPrefix(line, "Date:") || strings.HasPrefix(line, "Merge:"):
		return metaStyle
	default:
		return lipgloss.NewStyle()
	}
}

func findMatches(lines []string, term string) []int {
	if term == "" {
		return nil
	}
	lower := strings.ToLower(term)
	var out []int
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), lower) {
			out = append(out, i)
		}
	}
	return out
}

// scrollToLine centres lineIdx in the viewport.
func scrollToLine(vp *viewport.Model, lineIdx int) {
	offset := lineIdx - vp.Height/2
	if offset < 0 {
		offset = 0
	}
	vp.YOffset = offset
}

// visualRange returns the clamped [lo, hi] line indices of the current selection.
func visualRange(anchor, cursor, nLines int) (int, int) {
	lo, hi := anchor, cursor
	if lo > hi {
		lo, hi = hi, lo
	}
	if hi >= nLines {
		hi = nLines - 1
	}
	if lo < 0 {
		lo = 0
	}
	return lo, hi
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
		return diffMsg{strings.Split(patch, "\n")}
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
