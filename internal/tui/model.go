package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/qzhello/code-review/internal/agent"
	"github.com/qzhello/code-review/internal/model"
	"github.com/qzhello/code-review/internal/review"
)

// Action represents what the user decided for a finding.
type Action int

const (
	ActionPending Action = iota
	ActionAccepted
	ActionDismissed
	ActionFixed
	ActionFixing // AI fix in progress
)

func (a Action) String() string {
	switch a {
	case ActionAccepted:
		return "accepted"
	case ActionDismissed:
		return "dismissed"
	case ActionFixed:
		return "fixed"
	case ActionFixing:
		return "fixing..."
	default:
		return "pending"
	}
}

// FindingItem wraps a finding with user action state.
type FindingItem struct {
	Finding     model.Finding
	Action      Action
	Note        string
	FixError    string // error message if fix failed
	Explanation string // explanation from AI after successful fix
}

// chatMsg represents a message in the conversation history.
type chatMsg struct {
	role    string // "user", "assistant", "system"
	content string
}

// chatResponseMsg is sent when an async chat response completes.
type chatResponseMsg struct {
	resp *agent.ChatResponse
	err  error
}

// fixProposalMsg is sent when an async fix proposal completes (browse or chat mode).
type fixProposalMsg struct {
	index    int
	proposal *agent.FixProposal
	err      error
}

// pendingFix holds a proposed fix awaiting user confirmation.
type pendingFix struct {
	index    int
	proposal *agent.FixProposal
}

// chatPendingFix holds a proposed fix from chat mode awaiting confirmation.
type chatPendingFix struct {
	filePath     string
	fixedContent string
	original     string
	explanation  string
}

// uiMode represents the current interaction mode.
type uiMode int

const (
	modeBrowse uiMode = iota // Navigate findings with hotkeys
	modeChat                 // Conversational AI chat
)

// Model is the bubbletea model for the interactive review TUI.
type Model struct {
	findings []FindingItem
	diff     *model.DiffResult
	result   *review.Result
	agentCfg *model.AgentConfig

	cursor   int
	viewport viewport.Model
	width    int
	height   int
	ready    bool
	quit     bool
	detail   bool // showing detail view in browse mode

	// Chat mode
	mode        uiMode
	input       textinput.Model
	chatHistory []chatMsg          // conversation history for display
	session     *agent.ChatSession // multi-turn LLM session
	chatWaiting bool               // waiting for AI response
	chatCtxIdx  int                // which finding index the session is contextualized for (-1 = none)

	// Pending fix confirmation
	browsePendingFix *pendingFix     // pending fix from browse mode (f key)
	chatPending      *chatPendingFix // pending fix from chat mode
}

// NewModel creates a new TUI model.
func NewModel(result *review.Result, diff *model.DiffResult, agentCfg *model.AgentConfig) Model {
	items := make([]FindingItem, len(result.Findings))
	for i, f := range result.Findings {
		items[i] = FindingItem{Finding: f}
	}

	ti := textinput.New()
	ti.Placeholder = "Type a message... (e.g. 'fix this', 'explain why', 'dismiss')"
	ti.CharLimit = 500
	ti.Width = 80

	return Model{
		findings:   items,
		diff:       diff,
		result:     result,
		agentCfg:   agentCfg,
		mode:       modeBrowse,
		input:      ti,
		chatCtxIdx: -1,
	}
}

// Findings returns the findings with their actions after the TUI exits.
func (m Model) Findings() []FindingItem {
	return m.findings
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerH := 2
		footerH := 2
		inputH := 0
		if m.mode == modeChat {
			inputH = 2
		}
		vpHeight := msg.Height - headerH - footerH - inputH
		if vpHeight < 1 {
			vpHeight = 1
		}
		if !m.ready {
			m.viewport = viewport.New(msg.Width, vpHeight)
			m.viewport.SetContent(m.renderBody())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = vpHeight
		}
		m.input.Width = msg.Width - 4
		return m, nil

	case chatResponseMsg:
		m.chatWaiting = false
		if msg.err != nil {
			m.chatHistory = append(m.chatHistory, chatMsg{role: "system", content: "Error: " + msg.err.Error()})
		} else {
			m.chatHistory = append(m.chatHistory, chatMsg{role: "assistant", content: msg.resp.Message})
			m.handleChatAction(msg.resp)
		}
		m.viewport.SetContent(m.renderBody())
		m.viewport.GotoBottom()
		return m, nil

	case fixProposalMsg:
		if msg.index >= 0 && msg.index < len(m.findings) {
			if msg.err != nil {
				m.findings[msg.index].Action = ActionPending
				m.findings[msg.index].FixError = msg.err.Error()
			} else {
				// Store as pending — user must confirm
				m.findings[msg.index].Action = ActionPending
				m.findings[msg.index].FixError = ""
				m.browsePendingFix = &pendingFix{index: msg.index, proposal: msg.proposal}
			}
			m.viewport.SetContent(m.renderBody())
		}
		return m, nil

	case tea.KeyMsg:
		if m.mode == modeChat {
			return m.updateChat(msg)
		}
		return m.updateBrowse(msg)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// updateBrowse handles keys in browse mode.
func (m Model) updateBrowse(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.quit = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
			m.viewport.SetContent(m.renderBody())
			m.viewport.GotoTop()
		}

	case "down", "j":
		if m.cursor < len(m.findings)-1 {
			m.cursor++
			m.viewport.SetContent(m.renderBody())
			m.viewport.GotoTop()
		}

	case "a":
		m.findings[m.cursor].Action = ActionAccepted
		m.viewport.SetContent(m.renderBody())

	case "d":
		m.findings[m.cursor].Action = ActionDismissed
		m.viewport.SetContent(m.renderBody())

	case "f":
		// AI fix proposal (non-conversational)
		if m.browsePendingFix != nil {
			break // already has a pending fix to confirm
		}
		item := &m.findings[m.cursor]
		if item.Action == ActionFixing {
			break
		}
		if m.agentCfg == nil || m.agentCfg.APIKey == "" {
			item.FixError = "agent not configured (no API key)"
			m.viewport.SetContent(m.renderBody())
			break
		}
		item.Action = ActionFixing
		item.FixError = ""
		m.viewport.SetContent(m.renderBody())
		idx := m.cursor
		finding := item.Finding
		cfg := *m.agentCfg
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			fixer, err := agent.NewFixer(cfg)
			if err != nil {
				return fixProposalMsg{index: idx, err: err}
			}
			proposal, err := fixer.Propose(ctx, finding)
			return fixProposalMsg{index: idx, proposal: proposal, err: err}
		}

	case "y":
		// Accept pending fix
		if m.browsePendingFix != nil {
			pf := m.browsePendingFix
			if err := pf.proposal.Apply(); err != nil {
				m.findings[pf.index].FixError = err.Error()
			} else {
				m.findings[pf.index].Action = ActionFixed
				m.findings[pf.index].Explanation = pf.proposal.Explanation
			}
			m.browsePendingFix = nil
			m.viewport.SetContent(m.renderBody())
		}

	case "n":
		// Reject pending fix
		if m.browsePendingFix != nil {
			m.browsePendingFix = nil
			m.viewport.SetContent(m.renderBody())
		}

	case "r":
		m.findings[m.cursor].Action = ActionPending
		m.findings[m.cursor].FixError = ""
		m.findings[m.cursor].Explanation = ""
		m.viewport.SetContent(m.renderBody())

	case "enter":
		m.detail = !m.detail
		m.viewport.SetContent(m.renderBody())
		m.viewport.GotoTop()

	case "tab":
		for i := 1; i <= len(m.findings); i++ {
			idx := (m.cursor + i) % len(m.findings)
			if m.findings[idx].Action == ActionPending {
				m.cursor = idx
				m.viewport.SetContent(m.renderBody())
				m.viewport.GotoTop()
				break
			}
		}

	case "c", "/":
		// Enter chat mode
		if m.agentCfg == nil || m.agentCfg.APIKey == "" {
			break
		}
		m.mode = modeChat
		m.input.Focus()
		// Resize viewport to make room for input
		m.viewport.Height = m.height - 6
		m.ensureChatSession()
		m.viewport.SetContent(m.renderBody())
		m.viewport.GotoBottom()
		return m, textinput.Blink
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// updateChat handles keys in chat mode.
func (m Model) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit

	case "esc":
		// Return to browse mode
		m.mode = modeBrowse
		m.input.Blur()
		m.viewport.Height = m.height - 4
		m.viewport.SetContent(m.renderBody())
		m.viewport.GotoTop()
		return m, nil

	case "ctrl+p":
		// Previous finding in chat mode
		if m.cursor > 0 {
			m.cursor--
			m.ensureChatSession()
			m.viewport.SetContent(m.renderBody())
			m.viewport.GotoBottom()
		}
		return m, nil

	case "ctrl+n":
		// Next finding in chat mode
		if m.cursor < len(m.findings)-1 {
			m.cursor++
			m.ensureChatSession()
			m.viewport.SetContent(m.renderBody())
			m.viewport.GotoBottom()
		}
		return m, nil

	case "enter":
		// Send message
		text := strings.TrimSpace(m.input.Value())
		if text == "" || m.chatWaiting {
			return m, nil
		}
		m.input.SetValue("")

		// Handle pending fix confirmation locally (don't send to AI)
		if m.chatPending != nil {
			lower := strings.ToLower(text)
			m.chatHistory = append(m.chatHistory, chatMsg{role: "user", content: text})
			if lower == "accept" || lower == "apply" || lower == "yes" || lower == "y" || lower == "ok" {
				if err := agent.ApplyFix(m.chatPending.filePath, m.chatPending.fixedContent); err != nil {
					m.chatHistory = append(m.chatHistory, chatMsg{role: "system", content: "Failed to apply fix: " + err.Error()})
				} else {
					m.findings[m.cursor].Action = ActionFixed
					m.findings[m.cursor].Explanation = m.chatPending.explanation
					m.chatHistory = append(m.chatHistory, chatMsg{role: "system", content: "Fix applied to " + m.chatPending.filePath})
				}
			} else if lower == "cancel" || lower == "reject" || lower == "no" || lower == "n" {
				m.chatHistory = append(m.chatHistory, chatMsg{role: "system", content: "Fix discarded."})
			} else {
				// Not a confirm/cancel — keep pending and send to AI for clarification
				m.chatHistory = append(m.chatHistory, chatMsg{role: "system", content: "Pending fix — type 'accept' to apply or 'cancel' to discard."})
			}
			if lower == "accept" || lower == "apply" || lower == "yes" || lower == "y" || lower == "ok" ||
				lower == "cancel" || lower == "reject" || lower == "no" || lower == "n" {
				m.chatPending = nil
			}
			m.viewport.SetContent(m.renderBody())
			m.viewport.GotoBottom()
			return m, nil
		}

		m.chatHistory = append(m.chatHistory, chatMsg{role: "user", content: text})
		m.chatWaiting = true
		m.viewport.SetContent(m.renderBody())
		m.viewport.GotoBottom()

		session := m.session
		return m, func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			resp, err := session.Send(ctx, text)
			return chatResponseMsg{resp: resp, err: err}
		}
	}

	// Forward other keys to text input
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)

	// Also update viewport for scrolling
	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)

	return m, tea.Batch(cmd, vpCmd)
}

// handleChatAction processes an action returned by the AI in chat mode.
func (m *Model) handleChatAction(resp *agent.ChatResponse) {
	target := resp.Target
	if target == "" {
		target = "current"
	}

	switch resp.Action {
	case "fix":
		if resp.FixedContent != "" && target == "current" {
			filePath := m.findings[m.cursor].Finding.FilePath
			// Read original content for diff preview
			original, err := os.ReadFile(filePath)
			if err != nil {
				m.chatHistory = append(m.chatHistory, chatMsg{role: "system", content: "Failed to read file: " + err.Error()})
				return
			}
			m.chatPending = &chatPendingFix{
				filePath:     filePath,
				fixedContent: resp.FixedContent,
				original:     string(original),
				explanation:  resp.Message,
			}
			m.chatHistory = append(m.chatHistory, chatMsg{
				role:    "system",
				content: "Fix proposed. Type 'accept' to apply or 'cancel' to discard.\n\n" + m.renderFixDiff(string(original), resp.FixedContent),
			})
		}

	case "dismiss":
		count := m.applyActionToTargets(ActionDismissed, target)
		m.chatHistory = append(m.chatHistory, chatMsg{role: "system", content: fmt.Sprintf("Dismissed %d finding(s)", count)})

	case "accept":
		count := m.applyActionToTargets(ActionAccepted, target)
		m.chatHistory = append(m.chatHistory, chatMsg{role: "system", content: fmt.Sprintf("Accepted %d finding(s)", count)})

	case "next":
		if m.cursor < len(m.findings)-1 {
			m.cursor++
			m.ensureChatSession()
			m.chatHistory = append(m.chatHistory, chatMsg{
				role:    "system",
				content: fmt.Sprintf("Now discussing: %s — %s", m.findings[m.cursor].Finding.Location(), m.findings[m.cursor].Finding.Message),
			})
		} else {
			m.chatHistory = append(m.chatHistory, chatMsg{role: "system", content: "Already at the last finding"})
		}

	case "prev":
		if m.cursor > 0 {
			m.cursor--
			m.ensureChatSession()
			m.chatHistory = append(m.chatHistory, chatMsg{
				role:    "system",
				content: fmt.Sprintf("Now discussing: %s — %s", m.findings[m.cursor].Finding.Location(), m.findings[m.cursor].Finding.Message),
			})
		} else {
			m.chatHistory = append(m.chatHistory, chatMsg{role: "system", content: "Already at the first finding"})
		}
	}
}

// applyActionToTargets applies an action to findings matching the target scope.
// Returns the number of findings affected.
func (m *Model) applyActionToTargets(action Action, target string) int {
	count := 0
	for i := range m.findings {
		if m.findings[i].Action != ActionPending {
			continue
		}
		match := false
		switch target {
		case "current":
			match = (i == m.cursor)
		case "all":
			match = true
		case "errors":
			match = m.findings[i].Finding.Severity == model.SeverityError
		case "warnings":
			match = m.findings[i].Finding.Severity == model.SeverityWarn
		case "infos":
			match = m.findings[i].Finding.Severity == model.SeverityInfo
		}
		if match {
			m.findings[i].Action = action
			count++
		}
	}
	return count
}

// ensureChatSession creates or updates the chat session for the current finding.
func (m *Model) ensureChatSession() {
	if m.agentCfg == nil {
		return
	}

	// Create session if needed
	if m.session == nil {
		session, err := agent.NewChatSession(*m.agentCfg)
		if err != nil {
			m.chatHistory = append(m.chatHistory, chatMsg{role: "system", content: "Failed to start chat: " + err.Error()})
			return
		}
		m.session = session
	}

	// Update context if finding changed
	if m.chatCtxIdx != m.cursor {
		if err := m.session.SetFindingContext(m.findings[m.cursor].Finding); err != nil {
			m.chatHistory = append(m.chatHistory, chatMsg{role: "system", content: "Failed to load file: " + err.Error()})
			return
		}
		m.chatCtxIdx = m.cursor
		// Clear chat history when switching findings
		m.chatHistory = nil
		m.chatHistory = append(m.chatHistory, chatMsg{
			role:    "system",
			content: fmt.Sprintf("Now discussing: %s — %s", m.findings[m.cursor].Finding.Location(), m.findings[m.cursor].Finding.Message),
		})
	}
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	header := m.renderHeader()
	body := m.viewport.View()
	footer := m.renderFooter()

	if m.mode == modeChat {
		inputView := m.renderInput()
		return header + "\n" + body + "\n" + inputView + "\n" + footer
	}

	return header + "\n" + body + "\n" + footer
}

func (m Model) renderHeader() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("15")).
		Background(lipgloss.Color("62")).
		Padding(0, 1)

	countStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	pending := 0
	fixing := 0
	for _, f := range m.findings {
		if f.Action == ActionPending {
			pending++
		}
		if f.Action == ActionFixing {
			fixing++
		}
	}

	title := titleStyle.Render(" Code Review ")

	modeTag := ""
	if m.mode == modeChat {
		modeStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("33")).
			Padding(0, 1)
		modeTag = " " + modeStyle.Render(" CHAT ")
	}

	status := fmt.Sprintf(" %d findings | %d pending", len(m.findings), pending)
	if fixing > 0 {
		status += fmt.Sprintf(" | %d fixing", fixing)
	}
	status += fmt.Sprintf(" | %d/%d ", m.cursor+1, len(m.findings))

	return title + modeTag + countStyle.Render(status)
}

func (m Model) renderBody() string {
	if m.mode == modeChat {
		return m.renderChatView()
	}
	return m.renderDetail()
}

func (m Model) renderChatView() string {
	if len(m.findings) == 0 {
		return "No findings to review."
	}

	var sb strings.Builder
	item := m.findings[m.cursor]
	f := item.Finding

	// Compact finding header
	sevStyle := m.severityStyle(f.Severity)
	sb.WriteString(sevStyle.Render(fmt.Sprintf(" %s ", strings.ToUpper(f.Severity.String()))))
	sb.WriteString(fmt.Sprintf("  %s  [%s]", f.Location(), m.sourceLabel(f)))
	if item.Action != ActionPending {
		actionStyle := lipgloss.NewStyle().
			Foreground(m.actionColor(item.Action)).
			Bold(true)
		sb.WriteString("  " + actionStyle.Render("["+item.Action.String()+"]"))
	}
	sb.WriteString("\n")

	msgStyle := lipgloss.NewStyle().Bold(true)
	sb.WriteString(msgStyle.Render(f.Message))
	sb.WriteString("\n")
	if f.Suggestion != "" {
		sugStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
		sb.WriteString(sugStyle.Render("Suggestion: " + f.Suggestion))
		sb.WriteString("\n")
	}

	// Separator
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	sb.WriteString(sepStyle.Render(strings.Repeat("─", min(m.width, 80))))
	sb.WriteString("\n\n")

	// Chat history
	userStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("33")).
		Bold(true)

	assistantStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("78")).
		Bold(true)

	systemStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Italic(true)

	for _, msg := range m.chatHistory {
		switch msg.role {
		case "user":
			sb.WriteString(userStyle.Render("You: "))
			sb.WriteString(msg.content)
		case "assistant":
			sb.WriteString(assistantStyle.Render("AI: "))
			sb.WriteString(msg.content)
		case "system":
			sb.WriteString(systemStyle.Render("● " + msg.content))
		}
		sb.WriteString("\n\n")
	}

	// Waiting indicator
	if m.chatWaiting {
		waitStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)
		sb.WriteString(waitStyle.Render("AI is thinking..."))
		sb.WriteString("\n")
	}

	return sb.String()
}

func (m Model) renderInput() string {
	promptStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("33")).
		Bold(true)

	return promptStyle.Render("> ") + m.input.View()
}

// renderFixDiff produces a simple line-by-line diff between original and fixed content.
func (m Model) renderFixDiff(original, fixed string) string {
	origLines := strings.Split(original, "\n")
	fixedLines := strings.Split(fixed, "\n")

	var sb strings.Builder
	addStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	delStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	// Find changed region to keep diff compact
	// Find first differing line
	start := 0
	minLen := len(origLines)
	if len(fixedLines) < minLen {
		minLen = len(fixedLines)
	}
	for start < minLen && origLines[start] == fixedLines[start] {
		start++
	}
	// Find last differing line (from end)
	endOrig := len(origLines) - 1
	endFixed := len(fixedLines) - 1
	for endOrig > start && endFixed > start && origLines[endOrig] == fixedLines[endFixed] {
		endOrig--
		endFixed--
	}

	// Show context: 3 lines before/after
	ctxStart := start - 3
	if ctxStart < 0 {
		ctxStart = 0
	}
	ctxEndOrig := endOrig + 3
	if ctxEndOrig >= len(origLines) {
		ctxEndOrig = len(origLines) - 1
	}
	ctxEndFixed := endFixed + 3
	if ctxEndFixed >= len(fixedLines) {
		ctxEndFixed = len(fixedLines) - 1
	}

	// Context before
	for i := ctxStart; i < start; i++ {
		sb.WriteString(fmt.Sprintf("  %s\n", origLines[i]))
	}
	// Removed lines
	for i := start; i <= endOrig && i < len(origLines); i++ {
		sb.WriteString(delStyle.Render(fmt.Sprintf("- %s", origLines[i])) + "\n")
	}
	// Added lines
	for i := start; i <= endFixed && i < len(fixedLines); i++ {
		sb.WriteString(addStyle.Render(fmt.Sprintf("+ %s", fixedLines[i])) + "\n")
	}
	// Context after
	afterStart := endOrig + 1
	if afterStart < 0 {
		afterStart = 0
	}
	for i := afterStart; i <= ctxEndOrig && i < len(origLines); i++ {
		sb.WriteString(fmt.Sprintf("  %s\n", origLines[i]))
	}

	if sb.Len() == 0 {
		return "(no changes)"
	}
	return sb.String()
}

func (m Model) renderDetail() string {
	if len(m.findings) == 0 {
		return "No findings to review."
	}

	var sb strings.Builder
	item := m.findings[m.cursor]
	f := item.Finding

	// Finding header
	sevStyle := m.severityStyle(f.Severity)
	sb.WriteString(sevStyle.Render(fmt.Sprintf(" %s ", strings.ToUpper(f.Severity.String()))))
	sb.WriteString(fmt.Sprintf("  %s  [%s]\n", f.Location(), m.sourceLabel(f)))

	// Action badge
	if item.Action != ActionPending {
		actionStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(m.actionColor(item.Action)).
			Padding(0, 1)
		sb.WriteString(actionStyle.Render(item.Action.String()))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Pending fix preview (browse mode)
	if m.browsePendingFix != nil && m.browsePendingFix.index == m.cursor {
		pf := m.browsePendingFix.proposal
		confirmStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)
		sb.WriteString(confirmStyle.Render("Proposed fix: " + pf.Explanation))
		sb.WriteString("\n\n")

		diffBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("214")).
			Padding(0, 1)
		sb.WriteString(diffBox.Render(m.renderFixDiff(pf.Original, pf.FixedContent)))
		sb.WriteString("\n\n")

		hintStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("214"))
		sb.WriteString(hintStyle.Render("Press y to apply, n to discard"))
		sb.WriteString("\n\n")
	}

	// Fix error
	if item.FixError != "" {
		errStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)
		sb.WriteString(errStyle.Render("Fix error: "+item.FixError) + "\n\n")
	}

	// Fix explanation (after successful fix)
	if item.Explanation != "" {
		explStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("78")).
			Bold(true)
		sb.WriteString(explStyle.Render("Fix applied: "+item.Explanation) + "\n\n")
	}

	// Message
	msgStyle := lipgloss.NewStyle().Bold(true)
	sb.WriteString(msgStyle.Render(f.Message))
	sb.WriteString("\n\n")

	// Suggestion
	if f.Suggestion != "" {
		sugStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("78")).
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)
		sb.WriteString(sugStyle.Render("Suggestion: " + f.Suggestion))
		sb.WriteString("\n\n")
	}

	// Category + confidence (agent findings)
	if f.Source == "agent" {
		sb.WriteString(fmt.Sprintf("Category: %s | Confidence: %s\n\n", f.Category, f.Confidence.String()))
	}

	// Inline diff context
	if m.detail {
		diffCtx := m.getDiffContext(f)
		if diffCtx != "" {
			borderStyle := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("240")).
				Padding(0, 1)
			sb.WriteString(borderStyle.Render(diffCtx))
			sb.WriteString("\n")
		}
	}

	// All findings list (compact)
	sb.WriteString("\n")
	sb.WriteString(m.renderFindingsList())

	return sb.String()
}

func (m Model) renderFindingsList() string {
	var sb strings.Builder
	listHeader := lipgloss.NewStyle().Bold(true).Underline(true)
	sb.WriteString(listHeader.Render("All Findings"))
	sb.WriteString("\n\n")

	for i, item := range m.findings {
		f := item.Finding
		cursor := "  "
		if i == m.cursor {
			cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Render("> ")
		}

		sev := m.severityBadge(f.Severity)
		action := ""
		if item.Action != ActionPending {
			action = lipgloss.NewStyle().
				Foreground(m.actionColor(item.Action)).
				Render(fmt.Sprintf(" [%s]", item.Action.String()))
		}

		line := fmt.Sprintf("%s%s %-20s %s%s",
			cursor, sev, truncateStr(f.Location(), 20), truncateStr(f.Message, 50), action)

		if i == m.cursor {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}

		sb.WriteString(line + "\n")
	}

	return sb.String()
}

func (m Model) renderFooter() string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("241")).
		Background(lipgloss.Color("236")).
		Padding(0, 1)

	var keys string
	if m.mode == modeChat {
		if m.chatPending != nil {
			keys = "Type 'accept' to apply fix, 'cancel' to discard  esc:browse mode  ctrl+c:quit"
		} else {
			keys = "enter:send  ctrl+p/n:prev/next finding  esc:browse mode  ctrl+c:quit"
		}
	} else if m.browsePendingFix != nil {
		keys = "y:apply fix  n:discard fix  q:quit"
	} else {
		keys = "j/k:navigate  a:accept  d:dismiss  f:AI fix  r:reset  c:chat  enter:diff  tab:next  q:quit"
	}
	return style.Render(keys)
}

func (m Model) getDiffContext(f model.Finding) string {
	if m.diff == nil {
		return ""
	}

	for _, file := range m.diff.Files {
		if file.Path() != f.FilePath {
			continue
		}

		var sb strings.Builder
		for _, hunk := range file.Hunks {
			hunkEnd := hunk.NewStart + hunk.NewCount
			if f.Line > 0 && (f.Line < hunk.NewStart || f.Line > hunkEnd) {
				continue
			}

			sb.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@ %s\n",
				hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount, hunk.Header))

			for _, line := range hunk.Lines {
				prefix := " "
				lineStyle := lipgloss.NewStyle()

				switch line.Type {
				case model.LineAdded:
					prefix = "+"
					lineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
					if line.NewNum == f.Line {
						lineStyle = lipgloss.NewStyle().
							Foreground(lipgloss.Color("78")).
							Background(lipgloss.Color("22")).
							Bold(true)
					}
				case model.LineRemoved:
					prefix = "-"
					lineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
				}

				sb.WriteString(lineStyle.Render(fmt.Sprintf("%s%s", prefix, line.Content)) + "\n")
			}
		}

		if sb.Len() > 0 {
			return sb.String()
		}
	}

	return ""
}

func (m Model) severityStyle(sev model.Severity) lipgloss.Style {
	switch sev {
	case model.SeverityError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("196")).Bold(true)
	case model.SeverityWarn:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("0")).Background(lipgloss.Color("214")).Bold(true)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Background(lipgloss.Color("33")).Bold(true)
	}
}

func (m Model) severityBadge(sev model.Severity) string {
	switch sev {
	case model.SeverityError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("E")
	case model.SeverityWarn:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("W")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Render("I")
	}
}

func (m Model) sourceLabel(f model.Finding) string {
	if f.RuleID != "" && f.RuleID != "agent" {
		return f.RuleID
	}
	return f.Source
}

func (m Model) actionColor(a Action) lipgloss.Color {
	switch a {
	case ActionAccepted:
		return lipgloss.Color("78")
	case ActionDismissed:
		return lipgloss.Color("241")
	case ActionFixed:
		return lipgloss.Color("33")
	case ActionFixing:
		return lipgloss.Color("214")
	default:
		return lipgloss.Color("245")
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}
