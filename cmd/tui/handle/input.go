// SPDX-FileContributor: FlameFlag <github@flameflag.dev>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package handle

import (
	"github.com/asciimoo/hister/cmd/tui/model"
	"github.com/asciimoo/hister/cmd/tui/render"
	"github.com/asciimoo/hister/config"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/pkg/browser"
	"github.com/rs/zerolog/log"
)

func InputKeys(m *model.Model, msg tea.KeyMsg) tea.Cmd {
	action := config.Action(m.Cfg.Hotkeys.TUI[msg.String()])
	if msg.Type == tea.KeyRunes && !msg.Alt {
		action = ""
	}

	// Try common actions first
	if cmd, handled := DispatchCommonAction(m, action); handled {
		return cmd
	}

	switch action {
	case config.ActionToggleFocus:
		if m.GetTotalResults() > 0 {
			m.State = model.StateResults
			m.TextInput.Blur()
			if m.SelectedIdx < 0 {
				m.SelectedIdx = 0
			}
			render.RefreshAndScroll(m)
		}
		return m.FlashHint(config.ActionToggleFocus)
	case config.ActionOpenResult:
		if m.SelectedIdx >= 0 {
			if m.SelectedIdx == m.Limit {
				m.Limit += model.ResultsPageSize
				render.RefreshAndScroll(m)
				return startSearch(m, m.FlashHint(config.ActionOpenResult))
			} else if u := m.GetSelectedURL(); u != "" {
				if err := browser.OpenURL(u); err != nil {
					log.Warn().Err(err).Msg("failed to open URL in browser")
				}
				return tea.Batch(m.FlashHint(config.ActionOpenResult), m.PostHistoryCmd(u))
			}
		}
		return m.FlashHint(config.ActionOpenResult)
	}

	var cmd tea.Cmd
	oldVal := m.TextInput.Value()
	m.TextInput, cmd = m.TextInput.Update(msg)
	if m.TextInput.Value() != oldVal {
		m.Limit = model.ResultsPageSize
		m.SelectedIdx = -1
		return startSearch(m, cmd)
	}
	return cmd
}

func ResultsKeys(m *model.Model, msg tea.KeyMsg) tea.Cmd {
	action := config.Action(m.Cfg.Hotkeys.TUI[msg.String()])

	if cmd, handled := DispatchCommonAction(m, action); handled {
		return cmd
	}

	switch action {
	case config.ActionToggleFocus:
		m.State = model.StateInput
		m.TextInput.Focus()
		render.RefreshViewport(m)
		return tea.Batch(textinput.Blink, m.FlashHint(config.ActionToggleFocus))
	case config.ActionOpenResult:
		if m.SelectedIdx == m.Limit {
			m.Limit += model.ResultsPageSize
			render.RefreshAndScroll(m)
			return startSearch(m, m.FlashHint(config.ActionOpenResult))
		} else if u := m.GetSelectedURL(); u != "" {
			if err := browser.OpenURL(u); err != nil {
				log.Warn().Err(err).Msg("failed to open URL in browser")
			}
			return tea.Batch(m.FlashHint(config.ActionOpenResult), m.PostHistoryCmd(u))
		}
		return m.FlashHint(config.ActionOpenResult)
	}

	var cmd tea.Cmd
	m.Viewport, cmd = m.Viewport.Update(msg)
	return cmd
}
