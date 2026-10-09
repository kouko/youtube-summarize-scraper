package tui

import (
	"os"
	"path/filepath"

	// bubbles v2.2.1 filepicker: Enter on a directory always navigates into
	// it (Open branch); DirAllowed only gates selecting a directory as Path,
	// so DirAllowed=false + FileAllowed=true means "browse freely, select
	// only files"; Path is set on Select (Enter on a file), which we diff
	// across Update calls to emit ConfigSelectedMsg.
	"charm.land/bubbles/v2/filepicker"
	tea "charm.land/bubbletea/v2"
)

// ConfigSelectedMsg is sent when the user selects a config file in the picker.
type ConfigSelectedMsg struct {
	Path string
}

// FilePickerModel wraps bubbletea's filepicker with ytss-specific behavior:
// it filters .yaml/.yml and emits ConfigSelectedMsg when a file is picked.
type FilePickerModel struct {
	fp     filepicker.Model
	chosen string // last selected path, to detect new selections
}

// NewFilePickerModel creates a file picker for ytss config files (.yaml/.yml).
func NewFilePickerModel() *FilePickerModel {
	fp := filepicker.New()
	fp.AllowedTypes = []string{".yaml", ".yml"}
	fp.ShowHidden = false
	fp.DirAllowed = false
	fp.FileAllowed = true
	// AutoHeight sizes the picker to the full terminal height on every
	// WindowSizeMsg, pushing the bottom panels off screen. The main model
	// calls SetHeight with the panel's inner height instead (see View).
	fp.AutoHeight = false

	// Set initial directory: prefer YTSS_CONFIG_DIR, then common ytss config dir, then home
	startDir := os.Getenv("YTSS_CONFIG_DIR")
	if startDir == "" {
		home, _ := os.UserHomeDir()
		candidates := []string{
			filepath.Join(home, "kouko-obsidian-vault", "_config"),
			filepath.Join(home, ".config", "ytss"),
			home,
		}
		for _, d := range candidates {
			if _, err := os.Stat(d); err == nil {
				startDir = d
				break
			}
		}
	}
	if startDir != "" {
		fp.CurrentDirectory = startDir
	}

	return &FilePickerModel{fp: fp}
}

// Init implements tea.Model.
func (m *FilePickerModel) Init() tea.Cmd {
	return m.fp.Init()
}

// Update forwards messages to the underlying filepicker and emits
// ConfigSelectedMsg when the user selects a file (Enter on a file).
func (m *FilePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.fp, cmd = m.fp.Update(msg)

	// filepicker sets Path on Select (Enter on a file); emit a selection
	// message once per new path.
	if m.fp.Path != "" && m.fp.Path != m.chosen {
		m.chosen = m.fp.Path
		path := m.fp.Path
		return m, tea.Batch(cmd, func() tea.Msg {
			return ConfigSelectedMsg{Path: path}
		})
	}
	return m, cmd
}

// View implements tea.Model.
func (m *FilePickerModel) View() tea.View {
	return tea.NewView(m.fp.View())
}

// ChosenPath returns the last selected file path.
func (m *FilePickerModel) ChosenPath() string {
	return m.fp.Path
}

// SetHeight fixes the number of visible rows in the picker (wrapper around
// the bubbles API; the main model sizes the picker to its panel).
func (m *FilePickerModel) SetHeight(h int) {
	m.fp.SetHeight(h)
}
