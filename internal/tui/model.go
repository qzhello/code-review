package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
)

func (a Action) String() string {
	switch a {
	case ActionAccepted:
		return "accepted"
	case ActionDismissed:
		return "dismissed"
	case ActionFixed:
		return "fixed"
	default:
		return "pending"
	}
}

// FindingItem wraps a finding with user action state.
type FindingItem struct {
	Finding model.Finding
	Action  Action
	Note    string
}

// Model is the bubbletea model for the interactive review TUI.
type Model struct {
	findings []FindingItem
	diff     *model.DiffResult
	result   *review.Result
	cursor   int
	viewport viewport.Model
	width    int
	height   int
	ready    bool
	quit     bool
	detail   bool // showing detail view
}

// NewModel creates a new TUI model.
func NewModel(result *review.Result, diff *model.DiffResult) Model {
	items := make([]FindingItem, len(result.Findings))
	for i, f := range result.Findings {
		items[i] = FindingItem{Finding: f}
	}
	return Model{
		findings: items,
		diff:     diff,
		result:   result,
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
		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-6)
			m.viewport.SetContent(m.renderDetail())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 6
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quit = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.viewport.SetContent(m.renderDetail())
				m.viewport.GotoTop()
			}

		case "down", "j":
			if m.cursor < len(m.findings)-1 {
				m.cursor++
				m.viewport.SetContent(m.renderDetail())
				m.viewport.GotoTop()
			}

		case "a":
			// Accept finding
			m.findings[m.cursor].Action = ActionAccepted

		case "d":
			// Dismiss finding
			m.findings[m.cursor].Action = ActionDismissed

		case "f":
			// Mark as fixed
			m.findings[m.cursor].Action = ActionFixed

		case "r":
			// Reset action
			m.findings[m.cursor].Action = ActionPending

		case "enter":
			// Toggle detail view
			m.detail = !m.detail
			m.viewport.SetContent(m.renderDetail())
			m.viewport.GotoTop()

		case "tab":
			// Next pending finding
			for i := 1; i <= len(m.findings); i++ {
				idx := (m.cursor + i) % len(m.findings)
				if m.findings[idx].Action == ActionPending {
					m.cursor = idx
					m.viewport.SetContent(m.renderDetail())
					m.viewport.GotoTop()
					break
				}
			}
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View implements tea.Model.
func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	// Header
	header := m.renderHeader()

	// Finding list (left sidebar style shown in header)
	// Detail view in viewport
	body := m.viewport.View()

	// Footer with keybindings
	footer := m.renderFooter()

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
	for _, f := range m.findings {
		if f.Action == ActionPending {
			pending++
		}
	}

	title := titleStyle.Render(" Code Review ")
	count := countStyle.Render(fmt.Sprintf(" %d findings | %d pending | %d/%d ",
		len(m.findings), pending, m.cursor+1, len(m.findings)))

	return title + count
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

	keys := "j/k:navigate  a:accept  d:dismiss  f:fixed  r:reset  enter:toggle diff  tab:next pending  q:quit"
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
			// Check if this hunk contains the finding's line
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
					// Highlight the finding line
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
