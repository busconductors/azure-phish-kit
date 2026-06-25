// Package main — Bubble Tea model, update loop, and rendering for the
// GLNT Campaign Manager TUI.
package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/strasser-lab/azure-phish-kit/campaign-manager/core"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type focusType string

const (
	focusList   focusType = "list"
	focusDetail focusType = "detail"
	focusForm   focusType = "form"
)

type stepType int

const (
	stepLink stepType = iota + 1
	stepVerify
	stepPreview
	stepDeploy
)

const stepCount = 4

var stepNames = map[stepType]string{
	stepLink:    "Generate Link",
	stepVerify:  "Verify Leads",
	stepPreview: "Preview Lure",
	stepDeploy:  "Deploy",
}

var stepDescriptions = map[stepType][]string{
	stepLink: {
		"Encrypt campaign metadata with the",
		"AES-256 key and generate a base64url",
		"phishing link fragment.",
		"",
		"The link is embedded into the lure HTML",
		"at the {LINK} placeholder.",
	},
	stepVerify: {
		"Verify email addresses against target",
		"mail servers (SMTP RCPT TO probe).",
		"",
		"Valid (delivered) leads are counted and",
		"the campaign status is updated.",
		"",
		"Enable --smtp-proxy for SOCKS5 routing.",
	},
	stepPreview: {
		"Read the lure HTML file, replace the",
		"{LINK} placeholder with the generated",
		"link, and display a preview snippet.",
		"",
		"Verify that assets, branding, and the",
		"redirect URL look correct.",
	},
	stepDeploy: {
		"Mark the campaign as deployed.",
		"",
		"The campaign is ready for SuperMailer",
		"or Amazon SES dispatch.",
		"",
		"Monitor deliveries and captures in the",
		"analytics dashboard.",
	},
}

// ---------------------------------------------------------------------------
// Styles
// ---------------------------------------------------------------------------

var (
	headerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("63")).
			Foreground(lipgloss.Color("255")).
			Bold(true).
			Padding(0, 1)

	panelBorderStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("240"))

	listSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("39")).
				Bold(true)

	listNormalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	statusActiveStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	statusDraftStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	statusVerifiedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	statusDeployedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)

	stepActiveStyle   = lipgloss.NewStyle().Background(lipgloss.Color("63")).Foreground(lipgloss.Color("255")).Padding(0, 1)
	stepInactiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	keyHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39"))

	formLabelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("39")).
			Bold(true)

	formTitleStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("63")).
			Foreground(lipgloss.Color("255")).
			Bold(true).
			Padding(0, 1)

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))
)

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

type model struct {
	store      *core.Store
	luresPath  string
	phishKey   string
	smtpProxy  string
	campaigns  []core.Campaign
	selected   int
	listOffset int
	focus      focusType
	step       stepType
	statusMsg  string
	statusErr  bool
	width      int
	height     int

	// Form state
	formInputs []textinput.Model
	formFocus  int
}

func newModel(storePath, luresPath, phishKey, smtpProxy string) model {
	store := core.NewStore(storePath)
	campaigns := store.List()

	inputs := make([]textinput.Model, 4)

	inputs[0] = textinput.New()
	inputs[0].Placeholder = "campaign name (e.g., q3-phishing)"
	inputs[0].CharLimit = 64
	inputs[0].Focus()

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "lure filename (e.g., fake-mfa.html)"
	inputs[1].CharLimit = 128

	inputs[2] = textinput.New()
	inputs[2].Placeholder = "phishlet (e.g., microsoft, okta)"
	inputs[2].CharLimit = 32

	inputs[3] = textinput.New()
	inputs[3].Placeholder = "lead CSV path (e.g., leads/targets.csv)"
	inputs[3].CharLimit = 256

	m := model{
		store:      store,
		luresPath:  luresPath,
		phishKey:   phishKey,
		smtpProxy:  smtpProxy,
		campaigns:  campaigns,
		focus:      focusList,
		step:       stepLink,
		formInputs: inputs,
	}
	return m
}

// ---------------------------------------------------------------------------
// Bubble Tea interface
// ---------------------------------------------------------------------------

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		if m.focus == focusForm {
			cmd = m.updateForm(msg)
			return m, cmd
		}
		return m.updateMain(msg)

	case tickMsg:
		// Refresh campaign list from store.
		m.campaigns = m.store.List()
		if m.selected >= len(m.campaigns) && len(m.campaigns) > 0 {
			m.selected = len(m.campaigns) - 1
		}
		cmds = append(cmds, tickCmd())
	}

	return m, tea.Batch(cmds...)
}

// updateMain handles keyboard input when not in form mode.
func (m model) updateMain(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys (work regardless of focus).
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "n":
		// Open new-campaign form.
		m.focus = focusForm
		m.formFocus = 0
		m.resetForm()
		return m, textinput.Blink
	}

	// Focus-dependent keys.
	switch m.focus {
	case focusList:
		return m.updateListKeys(key)
	case focusDetail:
		return m.updateDetailKeys(key)
	}
	return m, nil
}

// updateListKeys handles keys when the campaign list panel is focused.
func (m model) updateListKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "j", "down":
		m.moveDown()
	case "k", "up":
		m.moveUp()
	case "enter":
		if len(m.campaigns) > 0 {
			m.focus = focusDetail
			m.step = stepLink
			m.statusMsg = ""
			m.statusErr = false
		}
	case "tab":
		if len(m.campaigns) > 0 {
			m.focus = focusDetail
		}
	}
	return m, nil
}

// updateDetailKeys handles keys when the workflow/detail panel is focused.
func (m model) updateDetailKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "tab":
		m.focus = focusList
	case "l", "L":
		return m.doGenerateLink()
	case "v", "V":
		return m.doVerifyLeads()
	case "p", "P":
		return m.doPreview()
	case "d", "D":
		return m.doDeploy()
	case "1", "2", "3", "4":
		m.step = stepType(key[0] - '0')
		m.statusMsg = ""
		m.statusErr = false
	}

	return m, nil
}

// ---------------------------------------------------------------------------
// Form handling
// ---------------------------------------------------------------------------

func (m *model) resetForm() {
	for i := range m.formInputs {
		m.formInputs[i].SetValue("")
		m.formInputs[i].Blur()
	}
	m.formInputs[0].Focus()
}

func (m model) updateForm(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()

	switch key {
	case "esc":
		m.focus = focusList
		m.statusMsg = ""
		m.statusErr = false
		return nil
	case "tab":
		m.formFocus = (m.formFocus + 1) % len(m.formInputs)
		for i := range m.formInputs {
			if i == m.formFocus {
				m.formInputs[i].Focus()
			} else {
				m.formInputs[i].Blur()
			}
		}
		return textinput.Blink
	case "shift+tab":
		m.formFocus = (m.formFocus - 1 + len(m.formInputs)) % len(m.formInputs)
		for i := range m.formInputs {
			if i == m.formFocus {
				m.formInputs[i].Focus()
			} else {
				m.formInputs[i].Blur()
			}
		}
		return textinput.Blink
	case "enter":
		if m.formFocus == len(m.formInputs)-1 {
			// Last field — submit.
			return m.doNewCampaign()
		}
		// Move to next field.
		m.formFocus = (m.formFocus + 1) % len(m.formInputs)
		for i := range m.formInputs {
			if i == m.formFocus {
				m.formInputs[i].Focus()
			} else {
				m.formInputs[i].Blur()
			}
		}
		return textinput.Blink
	}

	// Delegate to the focused text input.
	var cmd tea.Cmd
	for i := range m.formInputs {
		if i == m.formFocus {
			m.formInputs[i], cmd = m.formInputs[i].Update(msg)
		}
	}
	return cmd
}

// ---------------------------------------------------------------------------
// Navigation helpers
// ---------------------------------------------------------------------------

func (m *model) moveUp() {
	if m.selected > 0 {
		m.selected--
	}
	// If selected is above visible area, scroll up.
	if m.selected < m.listOffset {
		m.listOffset = m.selected
	}
}

func (m *model) moveDown() {
	if m.selected < len(m.campaigns)-1 {
		m.selected++
	}
	// If selected is below visible area, scroll down.
	visibleRows := m.listContentHeight()
	if visibleRows <= 0 {
		return
	}
	if m.selected >= m.listOffset+visibleRows {
		m.listOffset = m.selected - visibleRows + 1
	}
}

func (m model) listContentHeight() int {
	// Screen height minus header (1), status bar (1), and panel borders (2).
	h := m.height - 4
	if h < 3 {
		h = 3
	}
	return h
}

// ---------------------------------------------------------------------------
// Core action methods
// ---------------------------------------------------------------------------

// doGenerateLink calls core.GenerateLink and updates the campaign.
func (m model) doGenerateLink() (tea.Model, tea.Cmd) {
	if m.selected >= len(m.campaigns) {
		m.statusMsg = "No campaign selected."
		m.statusErr = true
		return m, nil
	}
	c := m.campaigns[m.selected]

	link, err := core.GenerateLink(
		m.phishKey,
		"https://www.office.com", // default redirect
		c.ID,
		c.Phishlet,
		"shared-doc",
		"", // BCC mode — no per-recipient email
	)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Link generation failed: %v", err)
		m.statusErr = true
		return m, nil
	}

	c.Link = link
	m.store.Put(c)
	m.statusMsg = fmt.Sprintf("Link generated for %s: %s", c.Name, link)
	m.statusErr = false
	m.tick()

	return m, tickCmd()
}

// doVerifyLeads calls core.VerifyLeads and updates the campaign.
func (m model) doVerifyLeads() (tea.Model, tea.Cmd) {
	if m.selected >= len(m.campaigns) {
		m.statusMsg = "No campaign selected."
		m.statusErr = true
		return m, nil
	}
	c := m.campaigns[m.selected]

	if c.LeadFile == "" {
		m.statusMsg = "Campaign has no lead file set."
		m.statusErr = true
		return m, nil
	}

	enableSMTP := true // always enable SMTP check

	m.statusMsg = fmt.Sprintf("Verifying leads in %s ...", c.LeadFile)
	m.statusErr = false

	total, valid, invalid, catchAll, err := core.VerifyLeads(c.LeadFile, enableSMTP, m.smtpProxy)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Verify failed: %v", err)
		m.statusErr = true
		return m, tickCmd()
	}

	c.LeadCount = valid
	c.Status = core.StatusVerified
	m.store.Put(c)

	m.statusMsg = fmt.Sprintf("Verified %d leads: %d valid, %d invalid, %d catch-all (of %d total)",
		valid+invalid+catchAll, valid, invalid, catchAll, total)
	m.statusErr = false
	m.tick()

	return m, tickCmd()
}

// doPreview calls core.PreviewLure and shows a snippet.
func (m model) doPreview() (tea.Model, tea.Cmd) {
	if m.selected >= len(m.campaigns) {
		m.statusMsg = "No campaign selected."
		m.statusErr = true
		return m, nil
	}
	c := m.campaigns[m.selected]

	if c.Lure == "" {
		m.statusMsg = "Campaign has no lure file set."
		m.statusErr = true
		return m, nil
	}

	lurePath := filepath.Join(m.luresPath, c.Lure)

	// Generate a link first if not already present.
	link := c.Link
	if link == "" {
		var err error
		link, err = core.GenerateLink(
			m.phishKey,
			"https://www.office.com",
			c.ID,
			c.Phishlet,
			"shared-doc",
			"",
		)
		if err != nil {
			m.statusMsg = fmt.Sprintf("Preview failed (link gen): %v", err)
			m.statusErr = true
			return m, nil
		}
		c.Link = link
		m.store.Put(c)
	}

	html, err := core.PreviewLure(lurePath, link)
	if err != nil {
		m.statusMsg = fmt.Sprintf("Preview failed: %v", err)
		m.statusErr = true
		return m, tickCmd()
	}

	// Truncate HTML for status display.
	preview := html
	if len(preview) > 120 {
		preview = preview[:117] + "..."
	}
	m.statusMsg = fmt.Sprintf("Preview (%s): %s", c.Lure, preview)
	m.statusErr = false
	m.tick()

	return m, tickCmd()
}

// doDeploy marks the campaign as deployed.
func (m model) doDeploy() (tea.Model, tea.Cmd) {
	if m.selected >= len(m.campaigns) {
		m.statusMsg = "No campaign selected."
		m.statusErr = true
		return m, nil
	}
	c := m.campaigns[m.selected]

	c.Status = core.StatusDeployed
	m.store.Put(c)

	m.statusMsg = fmt.Sprintf("Campaign %s deployed — ready for SuperMailer dispatch.", c.Name)
	m.statusErr = false
	m.tick()

	return m, tickCmd()
}

// doNewCampaign creates a campaign from form inputs and saves it.
func (m model) doNewCampaign() tea.Cmd {
	name := strings.TrimSpace(m.formInputs[0].Value())
	if name == "" {
		m.statusMsg = "Campaign name is required."
		m.statusErr = true
		return nil
	}

	c := core.Campaign{
		ID:        fmt.Sprintf("c%d", time.Now().Unix()),
		Name:      name,
		Lure:      strings.TrimSpace(m.formInputs[1].Value()),
		Phishlet:  strings.TrimSpace(m.formInputs[2].Value()),
		LeadFile:  strings.TrimSpace(m.formInputs[3].Value()),
		Status:    core.StatusDraft,
		CreatedAt: time.Now().UTC().Format("2006-01-02"),
	}

	m.store.Put(c)
	m.focus = focusList
	m.statusMsg = fmt.Sprintf("Created campaign: %s", c.Name)
	m.statusErr = false
	m.tick()

	return tickCmd()
}

// tick reloads campaigns from the store.
func (m *model) tick() {
	m.campaigns = m.store.List()
	if m.selected >= len(m.campaigns) {
		m.selected = 0
	}
	if len(m.campaigns) == 0 {
		m.selected = 0
		m.listOffset = 0
	}
}

// ---------------------------------------------------------------------------
// Tick (async refresh)
// ---------------------------------------------------------------------------

type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg{}
	})
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (m model) View() string {
	if m.focus == focusForm {
		return m.renderForm()
	}

	// Calculate layout.
	listW, detailW := m.panelWidths()
	contentH := m.height - 2 // header + status bar
	if contentH < 6 {
		contentH = 6
	}

	// Render sections.
	header := m.renderHeader()
	list := m.renderList(listW, contentH)
	detail := m.renderDetail(detailW, contentH)

	// Join panels side-by-side.
	panels := lipgloss.JoinHorizontal(lipgloss.Top, list, detail)
	status := m.renderStatusBar()

	return header + "\n" + panels + "\n" + status
}

// panelWidths returns the left and right panel widths.
func (m model) panelWidths() (int, int) {
	leftW := m.width * 30 / 100
	if leftW < 20 {
		leftW = 20
	}
	if leftW > 38 {
		leftW = 38
	}
	rightW := m.width - leftW - 1
	if rightW < 30 {
		rightW = 30
	}
	return leftW, rightW
}

// ---------------------------------------------------------------------------
// Render: header
// ---------------------------------------------------------------------------

func (m model) renderHeader() string {
	title := " GLNT Campaign Manager "
	if m.width > 40 {
		title = fmt.Sprintf(" GLNT Campaign Manager  |  %d campaigns ",
			len(m.campaigns))
	}
	return headerStyle.Copy().Width(m.width).Render(padRight(title, m.width))
}

// ---------------------------------------------------------------------------
// Render: campaign list (left panel)
// ---------------------------------------------------------------------------

func (m model) renderList(width, height int) string {
	// Inner content height (excluding borders).
	innerW := width - 2
	innerH := height - 2
	if innerH < 1 {
		innerH = 1
	}

	// Clamp list offset.
	maxOff := len(m.campaigns) - innerH
	if maxOff < 0 {
		maxOff = 0
	}
	if m.listOffset > maxOff {
		m.listOffset = maxOff
	}
	if m.listOffset < 0 {
		m.listOffset = 0
	}

	var sb strings.Builder
	sb.WriteString(panelBorderStyle.Copy().BorderTop(false).BorderBottom(false).BorderRight(false).
		Width(innerW).Render("CAMPAIGNS") + "\n")

	for i := 0; i < innerH; i++ {
		idx := m.listOffset + i
		row := ""
		if idx < len(m.campaigns) {
			row = m.renderCampaignRow(idx, innerW)
		}
		sb.WriteString(padRight(row, innerW) + "\n")
	}

	return panelBorderStyle.Copy().Width(width).Height(height).Render(
		strings.TrimRight(sb.String(), "\n"),
	)
}

func (m model) renderCampaignRow(idx, width int) string {
	c := m.campaigns[idx]

	// Marker.
	marker := "  "
	style := listNormalStyle
	if m.focus == focusList && idx == m.selected {
		marker = "> "
		style = listSelectedStyle
	}

	// Status badge.
	statusStr := statusLabel(c.Status)

	// Build line: marker + name (trunc) + status.
	nameMax := width - 9 // marker(2) + space(1) + status(6)
	if nameMax < 3 {
		nameMax = 3
	}
	line := fmt.Sprintf("%s%s %s",
		marker,
		truncate(c.Name, nameMax),
		statusStr,
	)
	return style.Render(padRight(line, width))
}

// ---------------------------------------------------------------------------
// Render: detail/workflow (right panel)
// ---------------------------------------------------------------------------

func (m model) renderDetail(width, height int) string {
	innerW := width - 4  // inner padding
	if innerW < 20 {
		innerW = 20
	}

	var sb strings.Builder

	// --- Campaign info ---
	if m.selected < len(m.campaigns) {
		c := m.campaigns[m.selected]

		infoLines := m.renderCampaignInfo(c, innerW)
		for _, line := range infoLines {
			sb.WriteString(line + "\n")
		}
	} else {
		sb.WriteString(formLabelStyle.Render("No campaigns. Press [N] to create one.") + "\n")
	}

	sb.WriteString(strings.Repeat("─", innerW) + "\n")

	// --- Step indicators ---
	sb.WriteString(m.renderStepIndicators(innerW) + "\n")
	sb.WriteString(strings.Repeat("─", innerW) + "\n")

	// --- Step description ---
	for _, line := range stepDescriptions[m.step] {
		sb.WriteString(line + "\n")
	}

	// --- Key hints ---
	sb.WriteString("\n")
	hints := "Keys: [L]ink  [V]erify  [P]review  [D]eploy  [N]ew  [Q]uit"
	sb.WriteString(keyHintStyle.Render(padRight(hints, innerW)))
	sb.WriteString("\n")
	hints2 := "      [1-4] steps  [Tab] switch  [j/k] navigate  [Enter] select"
	sb.WriteString(keyHintStyle.Render(padRight(hints2, innerW)))

	return panelBorderStyle.Copy().Width(width).Height(height).Render(
		" " + formLabelStyle.Render("WORKFLOW") + "\n\n" +
			strings.TrimRight(sb.String(), "\n"),
	)
}

func (m model) renderCampaignInfo(c core.Campaign, width int) []string {
	var lines []string

	nameLine := fmt.Sprintf("%s %s",
		formLabelStyle.Render("Name:"),
		c.Name,
	)
	lines = append(lines, padRight(nameLine, width))

	idLine := fmt.Sprintf("%s %s",
		lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("ID:"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(c.ID),
	)
	lines = append(lines, padRight(idLine, width))

	lureLine := fmt.Sprintf("%s %s",
		formLabelStyle.Render("Lure:"),
		c.Lure,
	)
	lines = append(lines, padRight(lureLine, width))

	phishLine := fmt.Sprintf("%s %s",
		formLabelStyle.Render("Phishlet:"),
		c.Phishlet,
	)
	lines = append(lines, padRight(phishLine, width))

	statusLine := fmt.Sprintf("%s %s",
		formLabelStyle.Render("Status:"),
		statusLabel(c.Status),
	)
	lines = append(lines, padRight(statusLine, width))

	leadsLine := fmt.Sprintf("%s %d",
		formLabelStyle.Render("Leads:"),
		c.LeadCount,
	)
	lines = append(lines, padRight(leadsLine, width))

	if c.LeadFile != "" {
		fileLine := fmt.Sprintf("%s %s",
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("CSV:"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(c.LeadFile),
		)
		lines = append(lines, padRight(fileLine, width))
	}

	if c.Link != "" {
		linkDisplay := c.Link
		if len(linkDisplay) > width-8 {
			linkDisplay = linkDisplay[:width-11] + "..."
		}
		linkLine := fmt.Sprintf("%s %s",
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Link:"),
			lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(linkDisplay),
		)
		lines = append(lines, padRight(linkLine, width))
	}

	dateLine := fmt.Sprintf("%s %s",
		lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Created:"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(c.CreatedAt),
	)
	lines = append(lines, padRight(dateLine, width))

	return lines
}

func (m model) renderStepIndicators(width int) string {
	var parts []string
	for i := stepType(1); i <= stepCount; i++ {
		name := stepNames[i][:1] // First letter.
		if i == stepLink {
			name = "L"
		} else if i == stepVerify {
			name = "V"
		} else if i == stepPreview {
			name = "P"
		} else {
			name = "D"
		}
		if i == m.step {
			parts = append(parts, stepActiveStyle.Render(fmt.Sprintf(" %s ", name)))
		} else {
			parts = append(parts, stepInactiveStyle.Render(fmt.Sprintf(" %s ", name)))
		}
	}
	joined := strings.Join(parts, " ")
	return padRight(joined, width)
}

// ---------------------------------------------------------------------------
// Render: status bar
// ---------------------------------------------------------------------------

func (m model) renderStatusBar() string {
	var text string
	if m.statusMsg != "" {
		prefix := ""
		if m.statusErr {
			prefix = errorStyle.Render("! ") + statusBarStyle.Render("")
		}
		text = fmt.Sprintf(" %s%s ", prefix, m.statusMsg)
		// Clear status after one render.
		// m.statusMsg stays until next action or navigation.
	} else {
		focusLabel := "LIST"
		if m.focus == focusDetail {
			focusLabel = "DETAIL"
		}
		text = fmt.Sprintf(" Focus: %s  |  %d campaigns loaded ",
			focusLabel, len(m.campaigns))
	}
	return statusBarStyle.Copy().Width(m.width).Render(padRight(text, m.width))
}

// ---------------------------------------------------------------------------
// Render: new campaign form
// ---------------------------------------------------------------------------

func (m model) renderForm() string {
	formW := 60
	if m.width < formW+4 {
		formW = m.width - 4
	}

	var sb strings.Builder

	sb.WriteString(formTitleStyle.Copy().Width(m.width).Render(
		padRight(" NEW CAMPAIGN ", m.width),
	))
	sb.WriteString("\n\n")

	labels := []string{"Name", "Lure File", "Phishlet", "Lead CSV"}
	for i, input := range m.formInputs {
		label := formLabelStyle.Render(fmt.Sprintf("  %-12s", labels[i]+":"))
		view := input.View()
		sb.WriteString(label)
		sb.WriteString(formLabelStyle.Render("│ "))
		sb.WriteString(view)
		sb.WriteString("\n\n")
	}

	sb.WriteString("\n")
	hint := keyHintStyle.Render("  [Tab/Shift+Tab] navigate  [Enter] submit/next  [Esc] cancel")
	sb.WriteString(padRight(hint, formW))

	// Center the form.
	formContent := sb.String()
	centered := lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(formContent)

	return centered + "\n" + m.renderStatusBar()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func statusLabel(status string) string {
	switch status {
	case core.StatusActive:
		return statusActiveStyle.Render("ACT")
	case core.StatusDraft:
		return statusDraftStyle.Render("DFT")
	case core.StatusVerified:
		return statusVerifiedStyle.Render("VER")
	case core.StatusDeployed:
		return statusDeployedStyle.Render("DEP")
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(status)
	}
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max < 2 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

func padRight(s string, total int) string {
	// Calculate visible width (strip ANSI).
	vis := visibleWidth(s)
	if vis >= total {
		return s
	}
	return s + strings.Repeat(" ", total-vis)
}

func visibleWidth(s string) int {
	n := 0
	esc := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			esc = true
			continue
		}
		if esc {
			if s[i] == 'm' {
				esc = false
			}
			continue
		}
		n++
	}
	return n
}
