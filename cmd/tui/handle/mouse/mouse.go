// SPDX-FileContributor: FlameFlag <github@flameflag.dev>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package mouse

import (
	"github.com/asciimoo/hister/cmd/tui/model"
	"github.com/asciimoo/hister/cmd/tui/render"
	"github.com/asciimoo/hister/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/pkg/browser"
	"github.com/rs/zerolog/log"
)

type Deps struct {
	ExecuteAction              func(*model.Model, config.Action) tea.Cmd
	SwitchTab                  func(*model.Model, config.Action) tea.Cmd
	StartSearch                func(*model.Model, ...tea.Cmd) tea.Cmd
	CloseOverlay               func(*model.Model) tea.Cmd
	SubmitAdd                  func(*model.Model) tea.Cmd
	CloseThemePickerWithRevert func(*model.Model) tea.Cmd
	PreviewTheme               func(*model.Model)
	ExecuteContextMenuAction   func(*model.Model) tea.Cmd
}

type Handler struct{ Deps }

func New(d Deps) *Handler { return &Handler{d} }

type Region struct{ X, Y, W, H int }

func (r Region) Contains(msg tea.MouseMsg) bool {
	return msg.X >= r.X && msg.X < r.X+r.W && msg.Y >= r.Y && msg.Y < r.Y+r.H
}

func (r Region) ContainsY(y int) bool {
	return y >= r.Y && y < r.Y+r.H
}

// --- helpers ---

func vpRegion(m *model.Model) Region {
	top := model.RowVPStart
	bottom := model.RowVPEnd(m.Height)
	return Region{X: 0, Y: top, W: m.Width, H: bottom - top + 1}
}

func scrollToPercent(m *model.Model, mouseY int) {
	vp := vpRegion(m)
	if vp.H <= 1 {
		return
	}
	maxScroll := m.TotalLines - m.Viewport.Height
	if maxScroll <= 0 {
		return
	}
	relY := max(0, min(mouseY-vp.Y, vp.H-1))
	pct := float64(relY) / float64(vp.H-1)
	m.Viewport.SetYOffset(int(pct * float64(maxScroll)))
	contentY := m.Viewport.YOffset + m.Viewport.Height/2
	if idx := m.FindResultAtY(contentY); idx >= 0 {
		m.SelectedIdx = idx
	}
	render.RefreshViewport(m)
}

func wheelDelta(msg tea.MouseMsg) int {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		return -1
	case tea.MouseButtonWheelDown:
		return 1
	default:
		return 0
	}
}

func isLeftClick(msg tea.MouseMsg) bool {
	return msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft
}

func isOverlayState(s model.ViewState) bool {
	switch s {
	case model.StateHelp, model.StateDialog, model.StateThemePicker,
		model.StateSettings, model.StateContextMenu, model.StatePrioritizeInput:
		return true
	default:
		return false
	}
}

// handleScroll applies a wheel event to idx (clamped to [lo, hi]) and calls
// after when the index changes. Returns (nil, true) if a wheel event was
// consumed, (nil, false) otherwise.
func handleScroll(msg tea.MouseMsg, idx *int, lo, hi int, after func()) (tea.Cmd, bool) {
	delta := wheelDelta(msg)
	if delta == 0 {
		return nil, false
	}
	if lo <= hi {
		if model.ScrollIdx(idx, delta, lo, hi) && after != nil {
			after()
		}
	}
	return nil, true
}

// --- Handler methods ---

func (h *Handler) closeOverlayForState(m *model.Model) tea.Cmd {
	if m.State == model.StateThemePicker {
		return h.CloseThemePickerWithRevert(m)
	}
	return h.CloseOverlay(m)
}

func (h *Handler) hintRegions(m *model.Model, msg tea.MouseMsg) tea.Cmd {
	regions := render.ComputeHintRegions(m)
	for _, r := range regions {
		if msg.X >= r.X0 && msg.X < r.X1 {
			return h.ExecuteAction(m, r.Action)
		}
	}
	return nil
}

// Handle is the main entry point for mouse events.
func (h *Handler) Handle(m *model.Model, msg tea.MouseMsg) tea.Cmd {
	if m.ScrollbarDragging {
		if msg.Action == tea.MouseActionMotion {
			scrollToPercent(m, msg.Y)
			return nil
		}
		if msg.Action == tea.MouseActionRelease {
			m.ScrollbarDragging = false
			return nil
		}
	}

	if isOverlayState(m.State) {
		return h.overlay(m, msg)
	}

	if m.ActiveTab != model.TabSearch {
		return h.nonSearchTab(m, msg)
	}

	if cmd, ok := handleScroll(msg, &m.SelectedIdx, 0, m.GetTotalResults()-1, func() {
		render.RefreshAndScroll(m)
	}); ok {
		return cmd
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonRight {
		return rightClick(m, msg)
	}

	if !isLeftClick(msg) {
		return nil
	}

	if msg.Y == model.RowTabBar {
		return h.tabBar(m, msg)
	}
	if msg.Y == model.RowInput {
		return inputRow(m, msg)
	}
	if msg.Y == model.RowHints(m.Height) {
		return h.hintRegions(m, msg)
	}
	if m.TotalLines > m.Viewport.Height && m.Viewport.Height > 0 && msg.X >= m.Width-model.ScrollbarWidth {
		return scrollbarClick(m, msg)
	}
	return h.viewportClick(m, msg)
}

// --- search-tab handlers ---

func rightClick(m *model.Model, msg tea.MouseMsg) tea.Cmd {
	vp := vpRegion(m)
	if !vp.ContainsY(msg.Y) || len(m.LineOffsets) == 0 {
		return nil
	}
	contentY := (msg.Y - vp.Y) + m.Viewport.YOffset
	idx := m.FindResultAtY(contentY)
	if idx < 0 || idx >= m.GetTotalResults() || idx == m.Limit {
		return nil
	}
	m.SelectedIdx = idx
	render.RefreshViewport(m)
	offX, offY := render.MenuOverlayOffset(m)
	m.OpenContextMenu(idx, msg.X, msg.Y, offX, offY)
	return nil
}

func inputRow(m *model.Model, msg tea.MouseMsg) tea.Cmd {
	m.State = model.StateInput
	prefixW := model.InputLeadingPad + lipgloss.Width("❯") + model.InputTrailingPad
	pos := min(max(msg.X-prefixW, 0), len([]rune(m.TextInput.Value())))
	m.TextInput.SetCursor(pos)
	return m.TextInput.Focus()
}

func scrollbarClick(m *model.Model, msg tea.MouseMsg) tea.Cmd {
	vp := vpRegion(m)
	if vp.ContainsY(msg.Y) {
		m.ScrollbarDragging = true
		scrollToPercent(m, msg.Y)
	}
	return nil
}

func (h *Handler) viewportClick(m *model.Model, msg tea.MouseMsg) tea.Cmd {
	vp := vpRegion(m)
	if !vp.ContainsY(msg.Y) || len(m.LineOffsets) == 0 {
		return nil
	}
	contentY := (msg.Y - vp.Y) + m.Viewport.YOffset
	if m.SuggestionHeight > 0 && contentY < m.SuggestionHeight && m.Results != nil && m.Results.QuerySuggestion != "" {
		m.TextInput.SetValue(m.Results.QuerySuggestion)
		m.TextInput.SetCursor(len([]rune(m.Results.QuerySuggestion)))
		m.SelectedIdx = -1
		m.Limit = model.ResultsPageSize
		return h.StartSearch(m)
	}
	idx := m.FindResultAtY(contentY)
	if idx < 0 || idx >= m.GetTotalResults() {
		return nil
	}
	if m.State == model.StateInput {
		m.State = model.StateResults
		m.TextInput.Blur()
	}
	if idx == m.SelectedIdx {
		if m.SelectedIdx == m.Limit {
			m.Limit += model.ResultsPageSize
			render.RefreshAndScroll(m)
			return h.StartSearch(m)
		} else if u := m.GetSelectedURL(); u != "" {
			if err := browser.OpenURL(u); err != nil {
				log.Warn().Err(err).Msg("failed to open URL in browser")
			}
			return m.PostHistoryCmd(u)
		}
	} else {
		m.SelectedIdx = idx
		render.RefreshAndScroll(m)
	}
	return nil
}

// --- shared ---

func (h *Handler) tabBar(m *model.Model, msg tea.MouseMsg) tea.Cmd {
	x := model.TabBarLeftPad
	tabActions := []config.Action{config.ActionTabSearch, config.ActionTabHistory, config.ActionTabRules, config.ActionTabAdd}
	for i, name := range model.TabNames {
		labelW := len(name) + model.TabLabelPad
		if msg.X >= x && msg.X < x+labelW {
			return h.SwitchTab(m, tabActions[i])
		}
		x += labelW + model.TabGap
	}
	return nil
}
