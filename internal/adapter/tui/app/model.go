package app

import (
	"context"
	"net/http"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/atsuya-m/pluto/internal/adapter/presenter/errdetail"
	"github.com/atsuya-m/pluto/internal/adapter/tui/clipboard"
	"github.com/atsuya-m/pluto/internal/adapter/tui/commandline"
	"github.com/atsuya-m/pluto/internal/adapter/tui/jsonview"
	"github.com/atsuya-m/pluto/internal/adapter/tui/requesteditor"
	"github.com/atsuya-m/pluto/internal/adapter/tui/rpcselector"
	"github.com/atsuya-m/pluto/internal/adapter/tui/style"
	"github.com/atsuya-m/pluto/internal/application/usecase"
)

type Mode int

const (
	ModeCommand Mode = iota
	ModeRPCSelector
	ModeRequestEditor
	ModePreview
	ModeInvoking
	ModeStreaming
	ModeResponseViewer
)

type KeyMap struct {
	Quit    key.Binding
	Send    key.Binding
	Edit    key.Binding
	Back    key.Binding
	Finish  key.Binding
	Explore key.Binding
	Print   key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit:    key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
		Send:    key.NewBinding(key.WithKeys("enter", "ctrl+s"), key.WithHelp("enter", "send")),
		Edit:    key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit request")),
		Back:    key.NewBinding(key.WithKeys("esc", "q", "ctrl+g"), key.WithHelp("q/esc", "back")),
		Finish:  key.NewBinding(key.WithKeys("ctrl+x"), key.WithHelp("C-x", "finish sending")),
		Explore: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "explore")),
		Print:   key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "print")),
	}
}

type result struct {
	ok       bool
	stream   bool
	clientSt bool
	stopped  bool
	count    int
	sent     int
	rpc      string
	duration string
	detail   errdetail.ErrorDetail
}

type Model struct {
	ctx  context.Context
	deps Dependencies
	keys KeyMap

	mode   Mode
	width  int
	height int

	command  commandline.Model
	selector rpcselector.Model
	editor   requesteditor.Model
	spinner  spinner.Model

	hasEditor bool
	rpcs      []usecase.RPCSummary
	loadErr   error
	result    result
	cancel    context.CancelFunc

	streamEvents <-chan tea.Msg
	streamCount  int
	stopping     bool

	session   *usecase.StreamSession
	sentCount int
	sendSeq   int

	headers http.Header

	body      string
	lastBody  string
	viewer    jsonview.Model
	exploring bool
	notice    string
}

func NewModel(ctx context.Context, deps Dependencies) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = style.Prompt
	command := commandline.New()
	command.SetHeaderKeys(headerKeys(deps.Headers))
	if deps.Clipboard == nil {
		deps.Clipboard = func(text string) (string, error) {
			method, err := clipboard.Write(text)
			return string(method), err
		}
	}
	return Model{
		ctx:      ctx,
		deps:     deps,
		keys:     DefaultKeyMap(),
		mode:     ModeCommand,
		width:    80,
		height:   24,
		command:  command,
		selector: rpcselector.New(),
		spinner:  sp,
		headers:  deps.Headers.Clone(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		tea.Println(banner(m.deps)),
		loadRPCs(m.ctx, m.deps.ListRPCs),
		m.command.Focus(),
	)
}

func (m Model) Mode() Mode {
	return m.mode
}
