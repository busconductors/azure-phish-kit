package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"strconv"

	"golang.org/x/term"
)

// App holds the full TUI application state.
type App struct {
	campaigns  []Campaign
	selected   int  // index into campaigns
	currentStep int  // 1-4 workflow step
	focusPanel int  // 0 = campaign list (left), 1 = workflow (right)
	proxyOnline bool
	lastCapture string
	newEvents   int
	width       int
	height      int
	quit        bool

	// Debug / status
	statusMsg string
}

// NewApp constructs the application and seeds it with sample campaigns.
func NewApp() *App {
	w, h, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		w, h = 80, 24
	}

	return &App{
		campaigns:   seedCampaigns(),
		selected:    0,
		currentStep: 1,
		focusPanel:  0,
		proxyOnline: true,
		lastCapture: time.Now().UTC().Format("15:04 MST"),
		newEvents:   3,
		width:       w,
		height:      h,
	}
}

// seedCampaigns returns plausible demo data so the UI shows something on first run.
func seedCampaigns() []Campaign {
	return []Campaign{
		{
			ID:        "c001",
			Name:      "q3-phishing",
			Lure:      "fake-mfa.html",
			Phishlet:  "microsoft",
			Status:    "active",
			LeadCount: 5210,
			Link:      "https://phish.example.com/?t=ZXhhbXBsZS5jb20",
			LeadFile:  "leads/q3-targets.csv",
			CreatedAt: "2026-06-15",
		},
		{
			ID:        "c002",
			Name:      "exec-payroll",
			Lure:      "payroll-update.html",
			Phishlet:  "microsoft",
			Status:    "ready",
			LeadCount: 340,
			Link:      "",
			LeadFile:  "leads/exec-payroll.csv",
			CreatedAt: "2026-06-18",
		},
		{
			ID:        "c003",
			Name:      "q2-it-audit",
			Lure:      "it-audit.html",
			Phishlet:  "okta",
			Status:    "draft",
			LeadCount: 0,
			Link:      "",
			LeadFile:  "",
			CreatedAt: "2026-06-20",
		},
	}
}

// ---------------------------------------------------------------------------
// Main loop
// ---------------------------------------------------------------------------

// Run enters the TUI event loop. It returns when the user presses Q.
func (a *App) Run() {
	// Clear screen once.
	fmt.Print("\033[2J")
	fmt.Print("\033[?25l") // hide cursor
	defer func() {
		fmt.Print("\033[?25h") // show cursor
		fmt.Print("\033[2J")   // clear
		fmt.Print("\033[H")    // home
	}()

	// Keyboard input channel.
	keyCh := make(chan []byte, 16)
	go a.readInputLoop(keyCh)

	// SIGWINCH → terminal resize.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	// Periodic refresh (updates last-capture clock, etc.).
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Initial draw.
	a.refreshSize()
	a.draw()

	for !a.quit {
		select {
		case key := <-keyCh:
			a.handleKey(key)
			a.draw()
		case <-sigCh:
			a.refreshSize()
			a.draw()
		case <-ticker.C:
			a.lastCapture = time.Now().UTC().Format("15:04 MST")
			a.draw()
		}
	}
}

// ---------------------------------------------------------------------------
// Keyboard input
// ---------------------------------------------------------------------------

// readInputLoop runs in a goroutine, pushing raw byte sequences onto ch.
// Single bytes (regular keys, Ctrl+key) are sent as-is. Escape sequences
// (arrow keys, etc.) are collected with a short deadline and sent as one slice.
func (a *App) readInputLoop(ch chan<- []byte) {
	one := make([]byte, 1)
	for {
		_, err := os.Stdin.Read(one)
		if err != nil {
			return
		}
		if one[0] != '\x1b' {
			ch <- []byte{one[0]}
			continue
		}
		// ESC received — try to slurp the rest of an escape sequence.
		os.Stdin.SetReadDeadline(time.Now().Add(40 * time.Millisecond))
		rest := make([]byte, 5)
		n, _ := os.Stdin.Read(rest)
		os.Stdin.SetReadDeadline(time.Time{})
		ch <- append([]byte{'\x1b'}, rest[:n]...)
	}
}

// handleKey dispatches a raw key sequence to the appropriate action.
// Returns true if the app should quit.
func (a *App) handleKey(key []byte) {
	switch {
	// --- Single-byte keys ---
	case len(key) == 1:
		switch key[0] {
		case 'q', 'Q', 0x03: // Ctrl+C
			a.quit = true
		case 'l', 'L':
			a.stepGenerateLink()
		case 'v', 'V':
			a.stepVerifyLeads()
		case 'p', 'P':
			a.stepPreview()
		case 'd', 'D':
			a.stepDeploy()
		case '\t':
			a.focusPanel = (a.focusPanel + 1) % 2
		case '\r': // Enter — context-dependent
			a.handleEnter()
		case 'j':
			if a.focusPanel == 0 {
				a.cursorDown()
			}
		case 'k':
			if a.focusPanel == 0 {
				a.cursorUp()
			}
		}

	// --- Escape sequences (arrow keys) ---
	case len(key) >= 3 && key[0] == '\x1b' && key[1] == '[':
		switch key[2] {
		case 'A': // Up
			a.cursorUp()
		case 'B': // Down
			a.cursorDown()
		case 'C': // Right
			a.focusPanel = 1
		case 'D': // Left
			a.focusPanel = 0
		}
	}
}

// ---------------------------------------------------------------------------
// Navigation helpers
// ---------------------------------------------------------------------------

func (a *App) cursorUp() {
	if a.selected > 0 {
		a.selected--
	}
}

func (a *App) cursorDown() {
	if a.selected < len(a.campaigns)-1 {
		a.selected++
	}
}

func (a *App) handleEnter() {
	if a.focusPanel == 0 && a.selected < len(a.campaigns) {
		// Activating a campaign moves focus to the workflow panel.
		a.focusPanel = 1
	}
}

// ---------------------------------------------------------------------------
// Workflow step handlers (stubs — wired to core logic in ../core/)
// ---------------------------------------------------------------------------

func (a *App) stepGenerateLink() {
	if a.selected >= len(a.campaigns) {
		a.statusMsg = "No campaign selected."
		return
	}
	a.currentStep = 1
	a.statusMsg = "Generating link... (core not yet wired)"
	// TODO: call core.GenerateLink(campaign)
}

func (a *App) stepVerifyLeads() {
	if a.selected >= len(a.campaigns) {
		a.statusMsg = "No campaign selected."
		return
	}
	a.currentStep = 2
	a.statusMsg = "Verifying leads... (core not yet wired)"
	// TODO: call core.VerifyLeads(campaign)
}

func (a *App) stepPreview() {
	if a.selected >= len(a.campaigns) {
		a.statusMsg = "No campaign selected."
		return
	}
	a.currentStep = 3
	a.statusMsg = "Opening preview... (core not yet wired)"
	// TODO: call core.Preview(campaign)
}

func (a *App) stepDeploy() {
	if a.selected >= len(a.campaigns) {
		a.statusMsg = "No campaign selected."
		return
	}
	a.currentStep = 4
	a.statusMsg = "Deploying campaign... (core not yet wired)"
	// TODO: call core.Deploy(campaign)
}

// ---------------------------------------------------------------------------
// Resize
// ---------------------------------------------------------------------------

func (a *App) refreshSize() {
	w, h, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		return
	}
	if w != a.width || h != a.height {
		a.width = w
		a.height = h
		// Re-clear so old artefacts are scrubbed.
		fmt.Print("\033[2J")
	}
}

// ---------------------------------------------------------------------------
// Layout calculator
// ---------------------------------------------------------------------------

// layout returns (leftWidth, rightWidth, contentHeight) in terminal columns/rows.
func (a *App) layout() (int, int, int) {
	leftW := a.width * 28 / 100
	if leftW < 22 {
		leftW = 22
	}
	if leftW > 35 {
		leftW = 35
	}
	rightW := a.width - leftW - 1 // one column gap between panels
	if rightW < 30 {
		rightW = 30
	}
	contentH := a.height - 1 // bottom row reserved for status bar
	if contentH < 8 {
		contentH = 8
	}
	return leftW, rightW, contentH
}

// visibleLen counts printable characters, skipping ANSI escapes.
func visibleLen(s string) int {
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

// padRight returns s padded with spaces so that visibleLen(s) == total.
func padRight(s string, total int) string {
	vl := visibleLen(s)
	if vl >= total {
		return s
	}
	return s + strings.Repeat(" ", total-vl)
}

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

// draw renders the full TUI frame to stdout.
func (a *App) draw() {
	leftW, rightW, contentH := a.layout()
	panelH := contentH - 1 // subtract header row
	if panelH < 6 {
		panelH = 6
	}

	var sb strings.Builder
	sb.Grow(a.width * a.height * 12)

	// Home cursor
	sb.WriteString("\033[H")

	// ---- Header bar ----
	sb.WriteString("\033[1;37;44m") // bold white on blue
	sb.WriteString(padRight(" GLNT Campaign Manager v1.3 ", a.width))
	sb.WriteString("\033[0m\r\n")

	// ---- Panel rows ----
	for row := 0; row < panelH; row++ {
		lc := a.leftCell(leftW, panelH, row)
		rc := a.rightCell(rightW, panelH, row)
		sb.WriteString(padRight(lc, leftW))
		sb.WriteByte(' ')
		sb.WriteString(padRight(rc, rightW))
		sb.WriteString("\033[K\r\n")
	}

	// ---- Status bar ----
	proxyDot := "\033[32m●\033[90m"
	if !a.proxyOnline {
		proxyDot = "\033[31m●\033[90m"
	}
	var status string
	if a.statusMsg != "" {
		status = fmt.Sprintf(" %s %s ", proxyDot, a.statusMsg)
		a.statusMsg = ""
	} else {
		status = fmt.Sprintf(" %s online  │  Last capture: %s  │  Events: %d ",
			proxyDot, a.lastCapture, a.newEvents)
	}
	sb.WriteString("\033[90m") // dim
	sb.WriteString(padRight(status, a.width))
	sb.WriteString("\033[0m")

	fmt.Print(sb.String())
}

// leftCell returns the content for one row of the left (campaign list) panel.
func (a *App) leftCell(width, height, row int) string {
	switch {
	case row == 0:
		return "\033[90m╔" + strings.Repeat("═", width-2) + "╗\033[0m"
	case row == height-1:
		return "\033[90m╚" + strings.Repeat("═", width-2) + "╝\033[0m"
	default:
		idx := row - 1
		if idx >= len(a.campaigns) {
			return "\033[90m║\033[0m" + strings.Repeat(" ", width-2) + "\033[90m║\033[0m"
		}
		c := a.campaigns[idx]

		marker := "  "
		if a.focusPanel == 0 && idx == a.selected {
			marker = "\033[34m▶ \033[0m"
		}
		status := statusIcon(c.Status)
		nameMax := width - 10
		if nameMax < 1 {
			nameMax = 1
		}
		line := fmt.Sprintf(" %s%s %s", marker, truncate(c.Name, nameMax), status)
		return "\033[90m║\033[0m" + padRight(line, width-2) + "\033[90m║\033[0m"
	}
}

// rightCell returns the content for one row of the right (workflow) panel.
func (a *App) rightCell(width, height, row int) string {
	switch {
	case row == 0:
		return "\033[90m╔" + strings.Repeat("═", width-2) + "╗\033[0m"
	case row == height-1:
		return "\033[90m╚" + strings.Repeat("═", width-2) + "╝\033[0m"
	case row == 1:
		title := fmt.Sprintf(" Step %d: %s  ", a.currentStep, stepName(a.currentStep))
		// Show step progress indicators (1 2 3 4) on the right side.
		steps := ""
		for i := 1; i <= 4; i++ {
			if i == a.currentStep {
				steps += fmt.Sprintf("\033[1;37;44m %d \033[0m ", i)
			} else {
				steps += fmt.Sprintf("\033[90m %d \033[0m ", i)
			}
		}
		// Fit title + steps into the available width, dropping steps if too narrow.
		avail := width - 2
		titleVal := visibleLen(title)
		stepsVal := visibleLen(steps)
		if titleVal+stepsVal > avail {
			line := padRight(title, avail)
			return "\033[90m║\033[0m\033[1;37m" + line + "\033[0m\033[90m║\033[0m"
		}
		spacer := strings.Repeat(" ", avail-titleVal-stepsVal)
		line := title + spacer + steps
		return "\033[90m║\033[0m\033[1;37m" + padRight(line, avail) + "\033[0m\033[90m║\033[0m"
	case row == 2:
		return "\033[90m╠" + strings.Repeat("═", width-2) + "╣\033[0m"
	case row == height-2:
		keys := " Keys: [L]ink  [V]erify  [P]review  [D]eploy  [Q]uit  [Tab] focus "
		return "\033[90m║\033[0m\033[36m" + padRight(keys, width-2) + "\033[0m\033[90m║\033[0m"
	default:
		lines := stepContentLines(a.currentStep)
		idx := row - 3
		if idx >= 0 && idx < len(lines) {
			return "\033[90m║\033[0m " + padRight(lines[idx], width-3) + "\033[90m║\033[0m"
		}
		return "\033[90m║\033[0m" + strings.Repeat(" ", width-2) + "\033[90m║\033[0m"
	}
}

// ---------------------------------------------------------------------------
// Drawing helpers
// ---------------------------------------------------------------------------

// statusIcon returns a 3-letter colored status abbreviation.
func statusIcon(status string) string {
	switch status {
	case "active":
		return "\033[32mACT\033[0m"
	case "ready":
		return "\033[34mRDY\033[0m"
	case "draft":
		return "\033[90mDFT\033[0m"
	case "archived":
		return "\033[31mARC\033[0m"
	default:
		return "---"
	}
}

// truncate shortens s to max runes, appending an ellipsis if needed.
func truncate(s string, max int) string {
	if max < 1 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// stepName returns the human-readable name for a workflow step.
func stepName(step int) string {
	switch step {
	case 1:
		return "Generate Link"
	case 2:
		return "Verify Leads"
	case 3:
		return "Preview"
	case 4:
		return "Deploy"
	default:
		return "Unknown"
	}
}

// stepContentLines returns descriptive lines for the current workflow step.
func stepContentLines(step int) []string {
	switch step {
	case 1:
		return []string{
			"Generate a phishing link with the",
			"base64-encoded target address.",
			"",
			"The link is embedded into the lure page.",
			"Verified leads click through to Evilginx.",
			"",
			"Press [L] to generate the link.",
		}
	case 2:
		return []string{
			"Verify email addresses against the",
			"target mail server (SMTP RCPT TO).",
			"",
			"Valid leads are marked as verified and",
			"ready for campaign deployment.",
			"",
			"Press [V] to start verification.",
		}
	case 3:
		return []string{
			"Preview the phishing page with the",
			"generated link embedded in the lure.",
			"",
			"Check that all assets load correctly",
			"and the redirect works as expected.",
			"",
			"Press [P] to open the preview.",
		}
	case 4:
		return []string{
			"Deploy the campaign via SuperMailer",
			"or Amazon SES to all verified leads.",
			"",
			"Monitor delivery rates and capture",
			"events in real time.",
			"",
			"Press [D] to start deployment.",
		}
	default:
		return []string{"No content available for this step."}
	}
}

// Ensure strconv is referenced (used by future workflow wiring).
var _ = strconv.Itoa
