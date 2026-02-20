package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type uiStyles struct {
	header    lipgloss.Style
	meta      lipgloss.Style
	bar       lipgloss.Style
	panel     lipgloss.Style
	sidebar   lipgloss.Style
	status    lipgloss.Style
	active    lipgloss.Style
	inactive  lipgloss.Style
	muted     lipgloss.Style
	accent    lipgloss.Style
	warn      lipgloss.Style
	good      lipgloss.Style
	bad       lipgloss.Style
	canvas    lipgloss.Style
	logo      lipgloss.Style
	chipLabel lipgloss.Style
	chipValue lipgloss.Style
	gap       lipgloss.Style
	bg        lipgloss.Color
}

const baseBGHex = "#050916"

func buildStyles(noColor bool) uiStyles {
	if noColor {
		border := lipgloss.Border{
			Top: "-", Bottom: "-", Left: "|", Right: "|",
			TopLeft: "+", TopRight: "+", BottomLeft: "+", BottomRight: "+",
		}
		return uiStyles{
			header:    lipgloss.NewStyle().Bold(true),
			meta:      lipgloss.NewStyle(),
			bar:       lipgloss.NewStyle().Padding(0, 1).Border(border),
			panel:     lipgloss.NewStyle().Padding(1, 1).Border(border),
			sidebar:   lipgloss.NewStyle().Padding(1, 1).Border(border),
			status:    lipgloss.NewStyle().Padding(0, 1).Border(border),
			active:    lipgloss.NewStyle().Bold(true),
			inactive:  lipgloss.NewStyle(),
			muted:     lipgloss.NewStyle(),
			accent:    lipgloss.NewStyle(),
			warn:      lipgloss.NewStyle(),
			good:      lipgloss.NewStyle(),
			bad:       lipgloss.NewStyle(),
			canvas:    lipgloss.NewStyle(),
			logo:      lipgloss.NewStyle(),
			chipLabel: lipgloss.NewStyle(),
			chipValue: lipgloss.NewStyle(),
			gap:       lipgloss.NewStyle(),
			bg:        lipgloss.Color(""),
		}
	}

	border := lipgloss.Border{
		Top: "─", Bottom: "─", Left: "│", Right: "│",
		TopLeft: "╭", TopRight: "╮", BottomLeft: "╰", BottomRight: "╯",
	}

	primary := lipgloss.Color("#C084FC")
	secondary := lipgloss.Color("#22D3EE")
	accentAlt := lipgloss.Color("#F472B6")
	muted := lipgloss.Color("#94A3B8")
	bg := lipgloss.Color(baseBGHex)
	panelBg := bg
	sidebarBg := bg
	statusBg := bg
	good := lipgloss.Color("#10B981")
	warn := lipgloss.Color("#FBBF24")
	bad := lipgloss.Color("#F87171")

	return uiStyles{
		header:    lipgloss.NewStyle().Bold(true).Foreground(secondary).Background(bg),
		meta:      lipgloss.NewStyle().Foreground(muted).Background(bg),
		bar:       lipgloss.NewStyle().Padding(0, 2).Border(border).BorderForeground(primary).Background(panelBg),
		panel:     lipgloss.NewStyle().Padding(1, 2).Border(border).BorderForeground(primary).Background(panelBg),
		sidebar:   lipgloss.NewStyle().Padding(1, 2).Border(border).BorderForeground(secondary).Background(sidebarBg),
		status:    lipgloss.NewStyle().Padding(0, 2).Border(border).BorderForeground(secondary).Background(statusBg),
		active:    lipgloss.NewStyle().Bold(true).Foreground(secondary).Background(bg).Padding(0, 1),
		inactive:  lipgloss.NewStyle().Foreground(muted).Background(bg),
		muted:     lipgloss.NewStyle().Foreground(muted).Background(bg),
		accent:    lipgloss.NewStyle().Foreground(accentAlt).Bold(true).Background(bg),
		warn:      lipgloss.NewStyle().Foreground(warn).Bold(true).Background(bg),
		good:      lipgloss.NewStyle().Foreground(good).Bold(true).Background(bg),
		bad:       lipgloss.NewStyle().Foreground(bad).Bold(true).Background(bg),
		canvas:    lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0")).Background(bg).Padding(1, 2),
		logo:      lipgloss.NewStyle().Padding(1, 2).Background(bg),
		chipLabel: lipgloss.NewStyle().Foreground(muted).Bold(true).Background(bg),
		chipValue: lipgloss.NewStyle().Foreground(secondary).Background(bg),
		gap:       lipgloss.NewStyle().Background(bg),
		bg:        bg,
	}
}

func normalizeWidth(width int) int {
	if width < 0 {
		return 0
	}
	return width
}

func renderLogo(width int, styles uiStyles) string {
	logo := []string{
		" ██████╗ ███████╗██╗      █████╗ ████████╗ ██████╗ ",
		"██╔════╝ ██╔════╝██║     ██╔══██╗╚══██╔══╝██╔═══██╗",
		"██║  ███╗█████╗  ██║     ███████║   ██║   ██║   ██║",
		"██║   ██║██╔══╝  ██║     ██╔══██║   ██║   ██║   ██║",
		"╚██████╔╝███████╗███████╗██║  ██║   ██║   ╚██████╔╝",
		" ╚═════╝ ╚══════╝╚══════╝╚═╝  ╚═╝   ╚═╝    ╚═════╝ ",
		"        multi-scoop log gelateria terminal         ",
	}
	colored := make([]string, 0, len(logo))
	for _, line := range logo {
		colored = append(colored, rainbow(line, styles.bg))
	}
	return styles.logo.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, colored...))
}

func renderHeaderMeta(styles uiStyles) string {
	left := rainbow("Gelato", styles.bg)
	right := styles.meta.Render("multi-listener log console · tcp ingestion")
	spacer := styles.gap.Render("  ")
	return lipgloss.JoinHorizontal(lipgloss.Left, left, spacer, right)
}

func rainbow(text string, bg lipgloss.Color) string {
	colors := []lipgloss.Color{
		lipgloss.Color("#F472B6"),
		lipgloss.Color("#C084FC"),
		lipgloss.Color("#60A5FA"),
		lipgloss.Color("#34D399"),
		lipgloss.Color("#FBBF24"),
		lipgloss.Color("#F97316"),
	}
	var out strings.Builder
	colorIndex := 0
	for _, r := range text {
		if r == ' ' {
			out.WriteString(lipgloss.NewStyle().Background(bg).Render(" "))
			continue
		}
		style := lipgloss.NewStyle().Foreground(colors[colorIndex%len(colors)]).Background(bg).Bold(true)
		out.WriteString(style.Render(string(r)))
		colorIndex++
	}
	return out.String()
}
