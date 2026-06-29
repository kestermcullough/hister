// SPDX-FileContributor: FlameFlag <github@flameflag.dev>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"time"

	"github.com/gorilla/websocket"

	"github.com/asciimoo/hister/client"
	"github.com/asciimoo/hister/config"
	"github.com/asciimoo/hister/server/indexer"

	tea "github.com/charmbracelet/bubbletea"
)

// represents the current UI state
type ViewState int

const (
	StateInput ViewState = iota
	StateResults
	StateDialog
	StateHelp
	StateThemePicker
	StateContextMenu
	StateSettings
	StatePrioritizeInput
)

func (s ViewState) String() string {
	return []string{"INPUT", "RESULTS", "DIALOG", "HELP", "THEME_PICKER", "CONTEXT_MENU", "SETTINGS", "PRIORITIZE_INPUT"}[s]
}

// sent over WebSocket to the search server
type SearchQuery struct {
	Text      string `json:"text"`
	Highlight string `json:"highlight"`
	Limit     int    `json:"limit"`
	Sort      string `json:"sort,omitempty"`
}

// Message types for bubbletea
type (
	ResultsMsg          struct{ Results *indexer.Results }
	ErrMsg              struct{ Err error }
	WsConnectedMsg      struct{ Conn *websocket.Conn }
	WsDisconnectedMsg   struct{ Err error }
	ReconnectMsg        struct{}
	HintClearMsg        struct{}
	SettingsErrClearMsg struct{}
	HistoryFetchedMsg   struct{ Items []HistoryItem }
	RulesFetchedMsg     struct{ Data RulesResponse }
	AddResultMsg        struct{ Err error }
	RulesSavedMsg       struct{ Err error }
)

type HistoryItem = client.HistoryItem

type RulesResponse = client.RulesResponse

type HintRegion struct {
	X0, X1 int
	Action config.Action
}

// holds one key → action row for the settings panel
type SettingsItem struct {
	Key    string
	Action config.Action
}

const (
	TabSearch  = 0
	TabHistory = 1
	TabRules   = 2
	TabAdd     = 3
)

const (
	RulesFieldSkip     = 0
	RulesFieldPriority = 1
	RulesFieldAliasKey = 2
	RulesFieldAliasVal = 3
	RulesFieldList     = 4
)

var ActionToTab = map[config.Action]int{
	config.ActionTabSearch:  TabSearch,
	config.ActionTabHistory: TabHistory,
	config.ActionTabRules:   TabRules,
	config.ActionTabAdd:     TabAdd,
}

var TabNames = []string{"Search", "History", "Rules", "Add"}

// Layout constants shared across packages (mouse handlers, render, model init).
const (
	ResultsPageSize   = 10 // results per page
	ScrollbarWidth    = 2  // columns reserved for scrollbar
	TabBarLeftPad     = 1  // leading space before first tab
	TabLabelPad       = 2  // brackets/spaces around tab name
	TabGap            = 1  // space between tab labels
	AddSubmitFieldIdx = 3  // focus index for submit button
	InputLeadingPad   = 2  // spaces before prompt ("  ")
	InputTrailingPad  = 1  // space after prompt (" ")
)

const (
	RowTabBar  = 0 // tab bar header
	RowInput   = 2 // search input line
	RowVPStart = 4 // first viewport row
)

// returns the hints row Y position for the given terminal height.
func RowHints(height int) int { return height - 1 }

// returns the last viewport row Y position for the given terminal height.
func RowVPEnd(height int) int { return height - 3 }

// Fixed layout overhead (header + dividers + input + hints)
const FixedLayoutRows = 6

const (
	MenuOpen        = 0
	MenuDelete      = 1
	MenuPrioritize  = 2
	MenuOptionCount = 3
)

var MenuOptionLabels = []string{"Open", "Delete", "Prioritize"}

// Dialog/overlay layout: border(1) + padding(1) + content rows
const (
	OverlayBorderRows  = 1 // top border row
	OverlayPaddingRows = 1 // padding inside border
)

// DialogBtnRowY returns the relative Y of the button row inside a dialog overlay.
// Layout: border(1) + padding(1) + title(1) + blank(1) + label(1) + blank(1) + buttons(1)
func DialogBtnRowY() int { return 7 }

// PrioritizeBtnRowY returns the relative Y of the button row inside prioritize dialog.
// Layout: border(1) + padding(1) + title(1) + blank(1) + label(1) + input(1) + blank(1) + buttons(1)
func PrioritizeBtnRowY() int { return 7 }

// Add tab row positions (relative to content area, after tab bar + dividers)
const (
	AddURLLabelY   = 5
	AddURLInputY   = 6
	AddTitleLabelY = 8
	AddTitleInputY = 9
	AddTextLabelY  = 11
	AddTextInputY  = 12
	AddSubmitY     = 14
)

var SearchTips = []string{
	"Type to search...",
	"Tip: Use quotes for exact phrases",
	"Tip: Press Tab to focus results",
	"Tip: Press Ctrl+D to delete a result",
	"Tip: Sort by domain with Ctrl+O",
	"Tip: Press Ctrl+T to change theme",
	"Tip: Press Ctrl+S to edit keybindings",
	"Tip: Press F1 for help",
	"Tip: Right-click for context menu",
	"Tip: Press Alt+2/3/4 for History/Rules/Add",
}

func (m *Model) FlashHint(action config.Action) tea.Cmd {
	m.HintFlash = action
	return ClearHintAfter()
}

func ClearHintAfter() tea.Cmd {
	return tea.Tick(350*time.Millisecond, func(_ time.Time) tea.Msg {
		return HintClearMsg{}
	})
}
