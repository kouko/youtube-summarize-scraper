package tui



import (

	"os"

	"path/filepath"

	"reflect"



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

// It also adds mouse click support for selecting files and navigating directories.

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



	// Set up logging for mouse click debugging if enabled

	if logFile := os.Getenv("YTSS_FILEPICKER_LOG"); logFile != "" {

		if _, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {


		} else {


		}


	}



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



// CurrentDirectory returns the current directory of the file picker.

func (m *FilePickerModel) CurrentDirectory() string {

	return m.fp.CurrentDirectory

}



// ClearChosenPath resets the picker's selected path. The bubbles picker keeps

// Path set after a selection and never clears it on directory navigation, so a

// later "is a path chosen?" check would report a stale file even when the user

// just opened a directory. Callers clear it right before an Enter that may

// select, so a non-empty Path afterwards means this Enter selected a file.

func (m *FilePickerModel) ClearChosenPath() {

	m.fp.Path = ""

}



// HandleMouseClick maps a click on the popup's file list to the picker: the

// cursor moves to the clicked row and Enter is sent, so a click behaves like

// keyboard navigation (Enter on a directory opens it, on a file selects it).

// contentY is a 0-based row inside the popup's content area, where row 0 is

// the first visible file (files[minIdx]).

//

// Returns whether the click was handled and any command the picker produced

// (the directory reload cmd, which the caller must run). The selection side

// effect (loading the config, closing the popup) is applied by the caller,

// mirroring the keyboard path in handleKey.

func (m *FilePickerModel) HandleMouseClick(contentY int) (bool, tea.Cmd) {

	if debug := os.Getenv("YTSS_MOUSE_DEBUG"); debug != "" {


	}

	// The picker renders one file per line: rows 0..maxIdx-minIdx map to

	// files[minIdx+row]. Its window/cursor fields are unexported; read the

	// three ints via reflection (names match bubbles v2.2.1). Unexported

	// fields cannot be read with Interface(), but Int() works.

	fp := reflect.ValueOf(&m.fp).Elem()

	selF := fp.FieldByName("selected")

	minF := fp.FieldByName("minIdx")

	maxF := fp.FieldByName("maxIdx")

	if !selF.IsValid() || !minF.IsValid() || !maxF.IsValid() {

		if debug := os.Getenv("YTSS_MOUSE_DEBUG"); debug != "" {


		}

		return false, nil

	}

	selected, minIdx, maxIdx := int(selF.Int()), int(minF.Int()), int(maxF.Int())

	if debug := os.Getenv("YTSS_MOUSE_DEBUG"); debug != "" {


	}

	if contentY < 0 || contentY > maxIdx-minIdx {

		// Above the file list (title/hint/separator rows) or below the

		// visible files (padding rows).

		if debug := os.Getenv("YTSS_MOUSE_DEBUG"); debug != "" {


		}

		return false, nil

	}

	target := minIdx + contentY

	if debug := os.Getenv("YTSS_MOUSE_DEBUG"); debug != "" {


	}



	var cmds []tea.Cmd

	// Clear any previously chosen path first: the picker never clears Path on

	// navigation, so without this a click on a directory would leave a stale

	// file path that the caller reads as "a file was selected".

	m.fp.Path = ""

	// Move the cursor to the clicked row with the same keys the picker

	// handles for keyboard navigation.

	step := tea.KeyDown

	n := target - selected

	if n < 0 {

		step = tea.KeyUp

		n = -n

	}

	if debug := os.Getenv("YTSS_MOUSE_DEBUG"); debug != "" {


	}

	for i := 0; i < n; i++ {

		var c tea.Cmd

		m.fp, c = m.fp.Update(tea.KeyPressMsg{Code: step})

		if c != nil {

			cmds = append(cmds, c)

		}

	}

	// Enter: navigate into a directory or select the highlighted file.

	var c tea.Cmd

	m.fp, c = m.fp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	if c != nil {

		cmds = append(cmds, c)

	}

	if debug := os.Getenv("YTSS_MOUSE_DEBUG"); debug != "" {


	}

	return true, tea.Batch(cmds...)

}
