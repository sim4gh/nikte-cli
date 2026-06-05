package cli

import (
	"fmt"
	"strings"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sim4gh/nikte-cli/internal/util"
)

// Styles for the interactive list.
var (
	tuiHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	tuiSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("13"))
	tuiDimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	tuiStatusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

// tuiModel is the Bubble Tea model backing `nk ls -i`.
type tuiModel struct {
	items         []Item
	cursor        int
	status        string
	pendingDelete bool
	quitting      bool
}

// deletedMsg is emitted after a delete attempt completes.
type deletedMsg struct {
	id  string
	err error
}

// refreshedMsg carries a freshly fetched item list.
type refreshedMsg struct {
	items []Item
}

func newTUIModel(items []Item) tuiModel {
	return tuiModel{items: items, status: "c copy ID · enter copy & quit · d delete · r refresh · q quit"}
}

func (m tuiModel) Init() tea.Cmd { return nil }

func (m tuiModel) selected() (Item, bool) {
	if m.cursor < 0 || m.cursor >= len(m.items) {
		return Item{}, false
	}
	return m.items[m.cursor], true
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// A pending delete intercepts the next keystroke for confirmation.
		if m.pendingDelete {
			switch msg.String() {
			case "y", "Y":
				if item, ok := m.selected(); ok {
					m.pendingDelete = false
					m.status = "Deleting " + item.ID + "..."
					return m, deleteItemCmd(item.ID)
				}
			default:
				m.pendingDelete = false
				m.status = "Delete cancelled"
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "esc", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "g", "home":
			m.cursor = 0
		case "G", "end":
			m.cursor = len(m.items) - 1
		case "c":
			if item, ok := m.selected(); ok {
				if err := clipboard.WriteAll(item.ID); err == nil {
					m.status = "Copied ID " + item.ID
				} else {
					m.status = "Failed to copy ID"
				}
			}
		case "enter":
			if item, ok := m.selected(); ok {
				_ = clipboard.WriteAll(item.ID)
				m.status = item.ID
				m.quitting = true
				return m, tea.Quit
			}
		case "d":
			if _, ok := m.selected(); ok {
				m.pendingDelete = true
				m.status = "Delete this item? press y to confirm, any other key to cancel"
			}
		case "r":
			m.status = "Refreshing..."
			return m, refreshItemsCmd()
		}

	case deletedMsg:
		if msg.err != nil {
			m.status = "Delete failed: " + msg.err.Error()
		} else {
			m.status = "Deleted " + msg.id
			m.removeByID(msg.id)
		}

	case refreshedMsg:
		m.items = msg.items
		if m.cursor >= len(m.items) {
			m.cursor = len(m.items) - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.status = fmt.Sprintf("Refreshed · %d items", len(m.items))
	}

	return m, nil
}

func (m *tuiModel) removeByID(id string) {
	for i, item := range m.items {
		if item.ID == id {
			m.items = append(m.items[:i], m.items[i+1:]...)
			break
		}
	}
	if m.cursor >= len(m.items) {
		m.cursor = len(m.items) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m tuiModel) View() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(tuiHeaderStyle.Render("nikte items") + "\n\n")

	if len(m.items) == 0 {
		b.WriteString(tuiDimStyle.Render("  No items.") + "\n")
	}

	for i, item := range m.items {
		line := formatTUIRow(item)
		if i == m.cursor {
			b.WriteString(tuiSelectedStyle.Render("> "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}

	b.WriteString("\n" + tuiStatusStyle.Render(m.status) + "\n")
	return b.String()
}

// formatTUIRow renders a single item as a fixed-width row.
func formatTUIRow(item Item) string {
	typeName := tuiTypeLabel(item.Type)

	content := item.Preview
	if content == "" {
		content = item.Filename
	}
	content = util.Truncate(util.ReplaceNewlines(content), 40)

	size := ""
	if item.Size > 0 {
		size = util.FormatBytes(item.Size)
	}

	expiry := "-"
	if item.ExpiresAt > 0 {
		expiry = util.FormatExpiry(item.ExpiresAt)
	}

	return fmt.Sprintf("%-5s %-10s %-42s %-9s %s", item.ID, typeName, content, size, expiry)
}

func tuiTypeLabel(t string) string {
	switch t {
	case "text":
		return "Text"
	case "file":
		return "File"
	case "screenshot":
		return "Screenshot"
	case "profile":
		return "Pro"
	default:
		return "?"
	}
}

// deleteItemCmd deletes an item by ID off the UI thread.
func deleteItemCmd(id string) tea.Cmd {
	return func() tea.Msg {
		result := tryDelete(id)
		if result.success {
			return deletedMsg{id: id}
		}
		return deletedMsg{id: id, err: fmt.Errorf("%s", result.error)}
	}
}

// refreshItemsCmd re-fetches all items off the UI thread.
func refreshItemsCmd() tea.Cmd {
	return func() tea.Msg {
		return refreshedMsg{items: fetchAllItems()}
	}
}

// runListTUI launches the interactive list with a pre-fetched item set.
func runListTUI(items []Item) error {
	p := tea.NewProgram(newTUIModel(items))
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	// If the user pressed enter, print the selected ID so it can be piped/copied.
	if fm, ok := finalModel.(tuiModel); ok {
		if item, ok := fm.selected(); ok && fm.status == item.ID {
			fmt.Println(item.ID)
		}
	}
	return nil
}
