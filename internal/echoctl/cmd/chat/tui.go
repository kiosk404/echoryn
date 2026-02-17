package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/kiosk404/echoryn/pkg/version"
)

// --- Colors ---

const (
	colorOrange      = "208"
	colorOrangeDim   = "172"
	colorWhite       = "255"
	colorGray        = "241"
	colorDarkGray    = "238"
	colorUser        = "39"
	colorAssistant   = "252"
	colorAssistLabel = "212"
	colorError       = "196"
	colorPrompt      = "208"

	// Powerline segment colors
	colorPLModel   = "208" // orange bg
	colorPLSession = "33"  // blue bg
	colorPLStatus  = "236" // dark bg
	colorPLTokens  = "98"  // purple bg
)

// --- Styles ---

var (
	// Banner
	bannerBorderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorOrange))

	bannerTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("0")).
				Background(lipgloss.Color(colorOrange)).
				Padding(0, 1)

	welcomeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorWhite)).
			Background(lipgloss.Color("238")).
			Padding(0, 1)

	tipsHeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorOrange))

	tipsBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorAssistant))

	recentHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorOrange))

	recentBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGray))

	infoLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGray))

	// Message styles
	userLabelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(colorUser))

	userMsgStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorUser))

	assistantLabelStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorAssistLabel))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorError)).
			Bold(true)

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorDarkGray))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorGray))

	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPrompt)).
			Bold(true)
)

// --- Tea Messages ---

type streamDeltaMsg struct{ delta string }
type streamDoneMsg struct{ err error }

// --- Display message ---

type displayMessage struct {
	role     string // "user", "assistant", "error"
	content  string // raw content
	rendered string // glamour-rendered markdown (for assistant)
}

// --- TUI Model ---

type tuiModel struct {
	textarea textarea.Model
	spinner  spinner.Model
	viewport viewport.Model

	messages      []displayMessage
	history       []ChatMessage
	streaming     bool
	streamContent strings.Builder
	err           error
	width, height int
	ready         bool
	quitting      bool

	// glamour renderer
	mdRenderer *glamour.TermRenderer

	client  *HivemindClient
	program *tea.Program
}

func newTUIModel(client *HivemindClient) *tuiModel {
	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()
	ta.CharLimit = 4096
	ta.SetHeight(2)
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetKeys("shift+enter")
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(colorOrange))

	vp := viewport.New(80, 20)
	vp.YPosition = 0

	return &tuiModel{
		textarea: ta,
		spinner:  sp,
		viewport: vp,
		messages: []displayMessage{},
		history:  []ChatMessage{},
		client:   client,
	}
}

func (m *tuiModel) initMarkdownRenderer() {
	w := m.width - 4
	if w <= 0 {
		w = 76
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(w),
	)
	if err == nil {
		m.mdRenderer = r
	}
}

func (m *tuiModel) renderMarkdown(content string) string {
	if m.mdRenderer == nil {
		return content
	}
	rendered, err := m.mdRenderer.Render(content)
	if err != nil {
		return content
	}
	return strings.TrimRight(rendered, "\n")
}

func (m *tuiModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, m.spinner.Tick)
}

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			if m.streaming {
				return m, nil
			}
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if m.streaming {
				return m, nil
			}
			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}
			switch input {
			case "/quit", "/exit":
				m.quitting = true
				return m, tea.Quit
			case "/clear":
				m.messages = []displayMessage{}
				m.history = []ChatMessage{}
				m.textarea.Reset()
				m.rebuildViewport()
				return m, nil
			}

			m.messages = append(m.messages, displayMessage{role: "user", content: input})
			m.history = append(m.history, ChatMessage{Role: "user", Content: input})
			m.textarea.Reset()
			m.streaming = true
			m.streamContent.Reset()
			m.err = nil

			m.rebuildViewport()

			go m.runStream()

			return m, nil
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.textarea.SetWidth(m.width - 4)
		m.initMarkdownRenderer()
		// Re-render all existing assistant messages with new width
		for i := range m.messages {
			if m.messages[i].role == "assistant" {
				m.messages[i].rendered = m.renderMarkdown(m.messages[i].content)
			}
		}
		m.rebuildViewport()
		return m, nil

	case streamDeltaMsg:
		m.streamContent.WriteString(msg.delta)
		m.rebuildViewport()
		return m, nil

	case streamDoneMsg:
		m.streaming = false
		content := m.streamContent.String()
		if msg.err != nil {
			m.err = msg.err
			if content != "" {
				m.messages = append(m.messages, displayMessage{
					role:     "assistant",
					content:  content,
					rendered: m.renderMarkdown(content),
				})
				m.history = append(m.history, ChatMessage{Role: "assistant", Content: content})
			}
			m.messages = append(m.messages, displayMessage{role: "error", content: msg.err.Error()})
		} else {
			m.messages = append(m.messages, displayMessage{
				role:     "assistant",
				content:  content,
				rendered: m.renderMarkdown(content),
			})
			m.history = append(m.history, ChatMessage{Role: "assistant", Content: content})
		}
		m.streamContent.Reset()
		m.rebuildViewport()
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Forward key/mouse events to viewport for scrolling
	if !m.streaming {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		cmds = append(cmds, cmd)
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

// rebuildViewport regenerates the viewport content from all messages.
func (m *tuiModel) rebuildViewport() {
	w := m.width
	if w <= 0 {
		w = 80
	}

	var sb strings.Builder

	// Welcome banner
	sb.WriteString(m.renderWelcomeBanner())

	// All conversation messages
	for _, msg := range m.messages {
		sb.WriteString(m.renderMessage(msg))
	}

	// Currently streaming content (show raw text, render markdown when done)
	if m.streaming {
		sep := separatorStyle.Render(strings.Repeat("─", max(w-2, 20)))
		sb.WriteString("\n" + sep + "\n")
		sb.WriteString(assistantLabelStyle.Render("eidolon") + "\n")
		content := m.streamContent.String()
		if content != "" {
			sb.WriteString(wrapContent(content, w-2) + "\n")
		}
		sb.WriteString(m.spinner.View() + "\n")
	}

	m.viewport.SetContent(sb.String())

	// Calculate viewport height: total height minus statusbar, input, help
	vpH := m.height - 6 // statusbar(1) + gap(1) + input(2) + help(1) + gap(1)
	if vpH < 5 {
		vpH = 5
	}
	m.viewport.Width = w
	m.viewport.Height = vpH

	// Auto-scroll to bottom
	m.viewport.GotoBottom()
}

// runStream performs the streaming HTTP call and sends deltas to the TUI.
func (m *tuiModel) runStream() {
	history := make([]ChatMessage, len(m.history))
	copy(history, m.history)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	_, err := m.client.ChatStream(ctx, history, func(delta string) {
		if m.program != nil {
			m.program.Send(streamDeltaMsg{delta: delta})
		}
	})

	if m.program != nil {
		m.program.Send(streamDoneMsg{err: err})
	}
}

func (m *tuiModel) View() string {
	if m.quitting {
		return "\n  Goodbye!\n\n"
	}
	if !m.ready {
		return "\n  Initializing...\n"
	}

	var sb strings.Builder

	// Scrollable message area
	sb.WriteString(m.viewport.View())
	sb.WriteString("\n")

	// Powerline status bar
	sb.WriteString(m.renderPowerline())
	sb.WriteString("\n")

	// Input prompt
	sb.WriteString(promptStyle.Render("> ") + m.textarea.View())
	sb.WriteString("\n")

	// Help line
	sb.WriteString(helpStyle.Render("Enter: send │ Shift+Enter: newline │ ↑↓/PgUp/PgDn: scroll │ /clear: reset │ Esc: quit"))
	sb.WriteString("\n")

	return sb.String()
}

// --- Powerline Status Bar ---

const powerlineArrow = "\ue0b0"

func plSegment(text string, fg, bg lipgloss.Color) string {
	return lipgloss.NewStyle().
		Foreground(fg).
		Background(bg).
		Padding(0, 1).
		Render(text)
}

func plArrow(leftBg, rightBg lipgloss.Color) string {
	return lipgloss.NewStyle().
		Foreground(leftBg).
		Background(rightBg).
		Render(powerlineArrow)
}

func (m *tuiModel) renderPowerline() string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	bgModel := lipgloss.Color(colorPLModel)
	bgSession := lipgloss.Color(colorPLSession)
	bgStatus := lipgloss.Color(colorPLStatus)
	bgTokens := lipgloss.Color(colorPLTokens)
	fgDark := lipgloss.Color("0")
	fgLight := lipgloss.Color(colorWhite)

	// Segment 1: Model
	seg1 := plSegment(" "+m.client.Model, fgDark, bgModel)
	arrow1 := plArrow(bgModel, bgSession)

	// Segment 2: Session
	sessionDisplay := m.client.SessionKey
	if sessionDisplay == "" {
		sessionDisplay = "new"
	} else if len(sessionDisplay) > 12 {
		sessionDisplay = sessionDisplay[:12] + "…"
	}
	seg2 := plSegment("⚡ "+sessionDisplay, fgLight, bgSession)
	arrow2 := plArrow(bgSession, bgStatus)

	// Segment 3: Status
	var statusText string
	if m.streaming {
		statusText = m.spinner.View() + " Generating..."
	} else if m.err != nil {
		errMsg := m.err.Error()
		if len(errMsg) > 30 {
			errMsg = errMsg[:30] + "…"
		}
		statusText = "✗ " + errMsg
	} else {
		statusText = "● Ready"
	}
	seg3 := plSegment(statusText, fgLight, bgStatus)
	arrow3 := plArrow(bgStatus, bgTokens)

	// Segment 4: Message count
	msgCount := len(m.history)
	seg4 := plSegment(fmt.Sprintf("✉ %d msgs", msgCount), fgLight, bgTokens)
	arrow4 := plArrow(bgTokens, lipgloss.Color("0"))

	bar := seg1 + arrow1 + seg2 + arrow2 + seg3 + arrow3 + seg4 + arrow4

	// Pad to full width
	barW := lipgloss.Width(bar)
	if barW < w {
		bar += lipgloss.NewStyle().Background(lipgloss.Color("0")).Render(strings.Repeat(" ", w-barW))
	}

	return bar
}

// eidolonASCII is the mascot displayed in the welcome banner.
var eidolonASCII = []string{
	`     ▄▄████▄▄     `,
	`   ██▀      ▀██   `,
	`  █▀  ◉    ◉  ▀█  `,
	`  █    ╰──╯    █   `,
	`  █            █   `,
	`   █          █    `,
	`  █ ▀▄  ▄▀▄ █     `,
	`   ▀▄ ▀▀ ▄▀       `,
}

func (m *tuiModel) renderWelcomeBanner() string {
	w := m.width
	if w <= 0 {
		w = 80
	}

	orange := bannerBorderStyle

	var sb strings.Builder

	// ── Top border with title tag ──
	titleTag := bannerTitleStyle.Render(fmt.Sprintf(" Eidolon Chat %s ", version.GitVersion))
	titleTagW := lipgloss.Width(titleTag)
	lineAfter := max(w-2-titleTagW, 0)
	topLine := orange.Render("─ ") + titleTag + orange.Render(" "+strings.Repeat("─", lineAfter))
	sb.WriteString(topLine + "\n")

	// Calculate column widths: left ~40%, right ~60%
	leftW := max(w*2/5, 20)
	rightW := max(w-leftW-1, 20) // -1 for gap

	// ── Row 1: Welcome + Tips ──

	// Left: Welcome badge
	leftRow1Lines := []string{
		welcomeStyle.Render(" Welcome! "),
	}

	// Right: Tips
	rightRow1Lines := []string{
		tipsHeaderStyle.Render("Tips for getting started"),
		tipsBodyStyle.Render("Type a message and press Enter to chat with the AI agent"),
		tipsBodyStyle.Render("Use ↑↓ or PgUp/PgDn to scroll through messages"),
		tipsBodyStyle.Render("Use /clear to reset the conversation"),
		tipsBodyStyle.Render("Use Esc or /quit to exit"),
	}

	// Pad to same height
	row1H := max(len(leftRow1Lines), len(rightRow1Lines))
	for len(leftRow1Lines) < row1H {
		leftRow1Lines = append(leftRow1Lines, "")
	}
	for len(rightRow1Lines) < row1H {
		rightRow1Lines = append(rightRow1Lines, "")
	}

	for i := 0; i < row1H; i++ {
		left := padRight(leftRow1Lines[i], leftW)
		right := truncOrPad(rightRow1Lines[i], rightW)
		sb.WriteString(left + " " + right + "\n")
	}

	sb.WriteString("\n")

	// ── Row 2: ASCII mascot + Recent activity ──

	// Right: session info as "recent activity"
	rightRow2Lines := []string{
		recentHeaderStyle.Render("Session info"),
		recentBodyStyle.Render(fmt.Sprintf("Model:   %s", m.client.Model)),
		recentBodyStyle.Render(fmt.Sprintf("Server:  %s", m.client.BaseURL)),
		recentBodyStyle.Render(fmt.Sprintf("Session: %s", m.client.SessionKey)),
	}

	// Left: ASCII art
	leftRow2Lines := make([]string, len(eidolonASCII))
	copy(leftRow2Lines, eidolonASCII)

	// Add model name under ASCII art
	modelLabel := infoLabelStyle.Render(fmt.Sprintf("  %s", m.client.Model))
	leftRow2Lines = append(leftRow2Lines, modelLabel)
	serverLabel := infoLabelStyle.Render(fmt.Sprintf("  %s", m.client.BaseURL))
	leftRow2Lines = append(leftRow2Lines, serverLabel)

	// Colorize ASCII art with orange
	for i, line := range leftRow2Lines {
		if i < len(eidolonASCII) {
			leftRow2Lines[i] = orange.Render(line)
		}
	}

	row2H := max(len(leftRow2Lines), len(rightRow2Lines))
	for len(leftRow2Lines) < row2H {
		leftRow2Lines = append(leftRow2Lines, "")
	}
	for len(rightRow2Lines) < row2H {
		rightRow2Lines = append(rightRow2Lines, "")
	}

	for i := 0; i < row2H; i++ {
		left := padRight(leftRow2Lines[i], leftW)
		right := truncOrPad(rightRow2Lines[i], rightW)
		sb.WriteString(left + " " + right + "\n")
	}

	// ── Bottom border ──
	botLine := orange.Render(strings.Repeat("─", w))
	sb.WriteString(botLine + "\n")

	return sb.String()
}

func (m *tuiModel) renderMessage(msg displayMessage) string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	sep := separatorStyle.Render(strings.Repeat("─", max(w-2, 20)))

	switch msg.role {
	case "user":
		label := userLabelStyle.Render("you")
		content := wrapContent(msg.content, w-2)
		return fmt.Sprintf("\n%s\n%s\n%s\n",
			sep,
			label,
			userMsgStyle.Render(content),
		)
	case "assistant":
		label := assistantLabelStyle.Render("eidolon")
		// Use glamour-rendered markdown if available
		content := msg.rendered
		if content == "" {
			content = wrapContent(msg.content, w-2)
		}
		return fmt.Sprintf("\n%s\n%s\n%s\n",
			sep,
			label,
			content,
		)
	case "error":
		return fmt.Sprintf("\n%s\n%s\n",
			sep,
			errorStyle.Render("Error: "+msg.content),
		)
	}
	return ""
}

// RunTUI starts the interactive chat TUI.
func RunTUI(client *HivemindClient) error {
	m := newTUIModel(client)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	m.program = p

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}

// RunOnce performs a single chat request (non-interactive mode) with streaming output to stdout.
func RunOnce(client *HivemindClient, message string, out func(string)) error {
	messages := []ChatMessage{{Role: "user", Content: message}}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	_, err := client.ChatStream(ctx, messages, func(delta string) {
		if out != nil {
			out(delta)
		}
	})
	return err
}

// --- Helpers ---

// wrapContent wraps long lines to fit within maxWidth.
func wrapContent(s string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = 80
	}
	lines := strings.Split(s, "\n")
	var result []string
	for _, line := range lines {
		if lipgloss.Width(line) <= maxWidth {
			result = append(result, line)
			continue
		}
		words := strings.Fields(line)
		if len(words) == 0 {
			result = append(result, line)
			continue
		}
		var current strings.Builder
		for _, word := range words {
			if current.Len() == 0 {
				current.WriteString(word)
			} else if lipgloss.Width(current.String()+" "+word) <= maxWidth {
				current.WriteString(" " + word)
			} else {
				result = append(result, current.String())
				current.Reset()
				current.WriteString(word)
			}
		}
		if current.Len() > 0 {
			result = append(result, current.String())
		}
	}
	return strings.Join(result, "\n")
}

// padRight pads a string with spaces to the given visible width.
func padRight(s string, w int) string {
	sw := lipgloss.Width(s)
	if sw >= w {
		return s
	}
	return s + strings.Repeat(" ", w-sw)
}

// truncOrPad truncates or pads a string to the given visible width.
func truncOrPad(s string, w int) string {
	sw := lipgloss.Width(s)
	if sw <= w {
		return s + strings.Repeat(" ", w-sw)
	}
	// Simple rune-based truncation
	runes := []rune(s)
	for i := len(runes); i > 0; i-- {
		candidate := string(runes[:i])
		if lipgloss.Width(candidate) <= w {
			return candidate
		}
	}
	return ""
}
