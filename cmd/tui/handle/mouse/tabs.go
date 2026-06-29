// SPDX-FileContributor: FlameFlag <github@flameflag.dev>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package mouse

import (
	"github.com/asciimoo/hister/cmd/tui/model"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pkg/browser"
	"github.com/rs/zerolog/log"
)

func focusAddField(m *model.Model, idx int) {
	if m.AddFocusIdx < len(m.AddInputs) {
		m.AddInputs[m.AddFocusIdx].Blur()
	}
	m.AddFocusIdx = idx
	if idx < len(m.AddInputs) {
		m.AddInputs[idx].Focus()
	}
}

func focusRulesInput(m *model.Model, section, field int) {
	m.RulesSection = section
	m.RulesEditingIdx = -1
	m.RulesEditingSection = section
	m.BlurAllRulesInputs()
	m.RulesFormFocus = field
	if inp := m.FocusedRulesInput(); inp != nil {
		inp.Focus()
	}
}

var addClickTargets = [...]struct{ y1, y2, idx int }{
	{model.AddURLLabelY, model.AddURLInputY, 0},
	{model.AddTitleLabelY, model.AddTitleInputY, 1},
	{model.AddTextLabelY, model.AddTextInputY, 2},
}

// --- non-search tab handling ---

// History tab layout
const (
	historyItemHeight    = 3 // rows per history item (title + URL + separator)
	historyClickableRows = 2 // clickable rows per item (title + URL, not separator)
)

// Rules tab layout (relative to content area)
const (
	rulesFirstInputY = 4 // Y of first section input
	rulesItemsOffset = 1 // items list starts 1 row below input
	rulesSectionGap  = 2 // rows between last item and next section's input
)

func (h *Handler) nonSearchTab(m *model.Model, msg tea.MouseMsg) tea.Cmd {
	if isLeftClick(msg) {
		if msg.Y == model.RowTabBar {
			return h.tabBar(m, msg)
		}
		if msg.Y == model.RowHints(m.Height) {
			return h.hintRegions(m, msg)
		}
		switch m.ActiveTab {
		case model.TabHistory:
			return historyClick(m, msg)
		case model.TabRules:
			return rulesClick(m, msg)
		case model.TabAdd:
			return h.addClick(m, msg)
		}
		return nil
	}
	switch m.ActiveTab {
	case model.TabHistory:
		if len(m.HistoryItems) > 0 {
			handleScroll(msg, &m.HistoryIdx, 0, len(m.HistoryItems)-1, nil)
		}
	case model.TabRules:
		if !m.RulesLoading && m.RulesFormFocus == model.RulesFieldList {
			if n := m.RulesSectionLen(m.RulesSection); n > 0 {
				handleScroll(msg, &m.RulesIdx, 0, n-1, nil)
			}
		}
	}
	return nil
}

// --- tab click handlers ---

func historyClick(m *model.Model, msg tea.MouseMsg) tea.Cmd {
	if len(m.HistoryItems) == 0 || msg.Y < model.RowVPStart {
		return nil
	}
	idx := (msg.Y - model.RowVPStart) / historyItemHeight
	if idx >= 0 && idx < len(m.HistoryItems) && (msg.Y-model.RowVPStart)%historyItemHeight < historyClickableRows {
		if idx == m.HistoryIdx {
			if err := browser.OpenURL(m.HistoryItems[idx].URL); err != nil {
				log.Warn().Err(err).Msg("failed to open URL in browser")
			}
		} else {
			m.HistoryIdx = idx
		}
	}
	return nil
}

func rulesClick(m *model.Model, msg tea.MouseMsg) tea.Cmd {
	if m.RulesLoading {
		return nil
	}
	s := max(len(m.RulesData.Skip), 1)
	p := max(len(m.RulesData.Priority), 1)

	skipInputY := rulesFirstInputY
	skipItemsY := skipInputY + rulesItemsOffset
	prioInputY := skipItemsY + s + rulesSectionGap
	prioItemsY := prioInputY + rulesItemsOffset
	aliasInputY := prioItemsY + p + rulesSectionGap
	aliasItemsY := aliasInputY + rulesItemsOffset

	type section struct {
		inputY int
		items  Region
		sec    int
		field  int
	}
	sections := [...]section{
		{skipInputY, Region{Y: skipItemsY, H: len(m.RulesData.Skip)}, 0, model.RulesFieldSkip},
		{prioInputY, Region{Y: prioItemsY, H: len(m.RulesData.Priority)}, 1, model.RulesFieldPriority},
		{aliasInputY, Region{Y: aliasItemsY, H: len(m.RulesData.Aliases)}, 2, model.RulesFieldAliasKey},
	}

	y := msg.Y
	for _, sec := range sections {
		if y == sec.inputY {
			focusRulesInput(m, sec.sec, sec.field)
			return nil
		}
		if sec.items.ContainsY(y) {
			idx := y - sec.items.Y
			if sec.sec == 2 && idx >= len(m.SortedAliasKeys()) {
				return nil
			}
			m.BlurAllRulesInputs()
			m.RulesFormFocus = model.RulesFieldList
			m.RulesSection = sec.sec
			m.RulesIdx = idx
			return nil
		}
	}
	return nil
}

func (h *Handler) addClick(m *model.Model, msg tea.MouseMsg) tea.Cmd {
	for _, t := range addClickTargets {
		if msg.Y == t.y1 || msg.Y == t.y2 {
			focusAddField(m, t.idx)
			return nil
		}
	}
	if msg.Y == model.AddSubmitY {
		focusAddField(m, model.AddSubmitFieldIdx)
		return h.SubmitAdd(m)
	}
	return nil
}
