package app

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/atsuya-m/pluto/internal/adapter/presenter/errdetail"
	textpresenter "github.com/atsuya-m/pluto/internal/adapter/presenter/text"
	"github.com/atsuya-m/pluto/internal/adapter/tui/commandline"
	"github.com/atsuya-m/pluto/internal/adapter/tui/jsonview"
	"github.com/atsuya-m/pluto/internal/adapter/tui/requesteditor"
	"github.com/atsuya-m/pluto/internal/adapter/tui/rpcselector"
	"github.com/atsuya-m/pluto/internal/adapter/tui/style"
	"github.com/atsuya-m/pluto/internal/application/usecase"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width <= 0 || msg.Height <= 0 {
			return m, nil
		}
		m.width, m.height = msg.Width, msg.Height
		m.command.SetWidth(msg.Width)
		m.selector.SetSize(msg.Width, min(msg.Height-2, 24))
		if m.hasEditor {
			m.editor.SetSize(msg.Width, msg.Height)
		}
		m.viewer.SetSize(msg.Width, m.viewerHeight())
		return m, nil

	case tea.KeyMsg:
		if key.Matches(msg, m.keys.Quit) {
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		}
		if splitRunes(m.mode) && msg.Type == tea.KeyRunes && len(msg.Runes) > 1 && !msg.Paste {
			var cmds []tea.Cmd
			var next tea.Model = m
			for _, r := range msg.Runes {
				var cmd tea.Cmd
				next, cmd = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt})
				cmds = append(cmds, cmd)
			}
			return next, tea.Batch(cmds...)
		}

	case rpcsLoadedMsg:
		m.rpcs = msg.RPCs
		m.loadErr = nil
		m.selector.SetRPCs(msg.RPCs)
		m.command.SetRPCs(msg.RPCs)
		return m, nil

	case rpcsLoadFailedMsg:
		m.loadErr = msg.Err
		return m, tea.Println(renderError("failed to load schema", errdetail.From(msg.Err)))

	case printMsg:
		return m, tea.Println(msg.Text)

	case errMsg:
		if m.mode == ModeRPCSelector {
			m.mode = ModeCommand
		}
		return m, tea.Println(renderError("error", errdetail.From(msg.Err)))

	case preparedMsg:
		m.editor = requesteditor.New(msg.Output.RPC, msg.Output.Input, msg.Output.Builder)
		m.editor.SetSize(m.width, m.height)
		m.hasEditor = true
		m.mode = ModeRequestEditor
		return m, nil

	case invokedMsg:
		m.cancel = nil
		m.mode = ModeCommand
		m.body, _ = textpresenter.ProtoJSON(msg.Output.Message)
		m.bodyRPC = msg.Output.RPC.FullName
		m.resHeaders, m.resTrailers = msg.Output.Headers, msg.Output.Trailers
		out := renderResponse(m.editor.RPC(), msg.Output, msg.Duration.Round(1e6).String())
		if lines := strings.Count(m.body, "\n") + 1; lines > m.viewerHeight() {
			out += "\n" + style.Subtle.Render(fmt.Sprintf("(%d lines • `view` to explore)", lines))
		}
		return m, tea.Batch(tea.Println(out), m.command.Focus())

	case streamStartedMsg:
		m.streamEvents = msg.Events
		return m, waitForStream(msg.Events)

	case streamMessageMsg:
		m.streamCount++
		m.lastBody, _ = textpresenter.ProtoJSON(msg.Message)
		return m, tea.Sequence(
			tea.Println(renderStreamMessage(m.streamCount, msg.Message)),
			waitForStream(m.streamEvents),
		)

	case streamOpenedMsg:
		m.session = msg.Session
		first := m.sendSeq + 1
		m.sendSeq += len(msg.Pending)
		return m, tea.Batch(startReceive(msg.Session), sendMessages(msg.Session, msg.Pending, first, msg.Finish))

	case streamSentMsg:
		m.sentCount++
		return m, nil

	case jsonview.CopyMsg:
		return m, copyToClipboard(m.deps.Clipboard, msg.Text, msg.Label)

	case copiedMsg:
		if msg.Err != nil {
			m.notice = style.Error.Render("✘ copy failed: " + msg.Err.Error())
		} else {
			m.notice = style.Success.Render("✔ copied") + style.Subtle.Render(fmt.Sprintf(" %s (%d chars, %s)", msg.Label, msg.Length, msg.Method))
		}
		return m, nil

	case streamSendFailedMsg:
		return m, tea.Println(renderError("send failed", errdetail.From(msg.Err)))

	case streamDoneMsg:
		m.resHeaders, m.resTrailers = msg.Output.Headers, msg.Output.Trailers
		if m.session != nil {
			m.resHeaders, m.resTrailers = m.session.Headers(), m.session.Trailers()
			_ = m.session.Close()
		}
		clientStreaming := m.editor.RPC().ClientStreaming
		m.body, m.lastBody = m.lastBody, ""
		m.bodyRPC = m.editor.RPC().FullName
		m.session = nil
		m.cancel = nil
		m.streamEvents = nil
		m.mode = ModeCommand
		r := result{
			ok:       msg.Err == nil,
			clientSt: clientStreaming,
			count:    m.streamCount,
			sent:     m.sentCount,
			rpc:      m.editor.RPC().FullName,
			duration: msg.Duration.Round(1e6).String(),
		}
		if msg.Err != nil && m.stopping && (errors.Is(msg.Err, context.Canceled) || errdetail.From(msg.Err).Code == "canceled") {
			r.ok, r.stopped = true, true
		}
		m.stopping = false
		if !r.ok {
			return m, tea.Batch(tea.Println(renderError(r.rpc+" failed", errdetail.From(msg.Err))), m.command.Focus())
		}
		return m, tea.Batch(tea.Println(renderStreamSummary(r)), m.command.Focus())

	case invokeFailedMsg:
		m.cancel = nil
		m.mode = ModeCommand
		return m, tea.Batch(tea.Println(renderError(m.editor.RPC().FullName+" failed", errdetail.From(msg.Err))), m.command.Focus())
	}

	switch m.mode {
	case ModeRPCSelector:
		return m.updateSelector(msg)
	case ModeRequestEditor:
		return m.updateEditor(msg)
	case ModePreview:
		return m.updatePreview(msg)
	case ModeInvoking, ModeStreaming:
		return m.updateInvoking(msg)
	case ModeExplorer:
		return m.updateExplorer(msg)
	default:
		return m.updateCommand(msg)
	}
}

func splitRunes(mode Mode) bool {
	switch mode {
	case ModePreview, ModeExplorer, ModeInvoking, ModeStreaming:
		return true
	default:
		return false
	}
}

func (m Model) updateCommand(msg tea.Msg) (tea.Model, tea.Cmd) {
	if sub, ok := msg.(commandline.SubmitMsg); ok {
		return m.execute(sub.Input)
	}
	var cmd tea.Cmd
	m.command, cmd = m.command.Update(msg)
	return m, cmd
}

func (m Model) execute(input string) (tea.Model, tea.Cmd) {
	echo := tea.Println(style.Prompt.Render("pluto> ") + maskCommandEcho(input))
	c, err := commandline.ParseCommand(input)
	if err != nil {
		return m, tea.Sequence(echo, tea.Println(style.Error.Render("✘ "+err.Error())))
	}

	switch strings.ToLower(c.Name) {
	case "":
		return m, nil
	case "help", "?":
		return m, tea.Sequence(echo, tea.Println(helpText))
	case "exit":
		return m, tea.Sequence(echo, tea.Quit)
	case "clear":
		return m, tea.ClearScreen
	case "services":
		return m, tea.Sequence(echo, listServices(m.ctx, m.deps.ListServices))
	case "rpcs", "ls":
		if len(c.Args) > 0 {
			return m, tea.Sequence(echo, listRPCsText(m.ctx, m.deps.ListRPCs, c.Args[0]))
		}
		return m.openSelector(echo)
	case "reload":
		return m, tea.Sequence(echo, loadRPCs(m.ctx, m.deps.ListRPCs))
	case "header", "headers":
		if len(c.Args) == 0 {
			return m, tea.Sequence(echo, tea.Println(formatHeaders(m.headers)))
		}
		next, note, err := applyHeaderCommand(m.headers, c.Args)
		if err != nil {
			return m, tea.Sequence(echo, tea.Println(style.Error.Render("✘ "+err.Error())))
		}
		m.headers = next
		m.command.SetHeaderKeys(headerKeys(next))
		return m, tea.Sequence(echo, tea.Println(style.Subtle.Render(note)))
	case "view":
		if m.body == "" {
			return m, tea.Sequence(echo, tea.Println(style.Error.Render("✘ no response to view yet")))
		}
		if len(c.Args) == 1 && c.Args[0] == "headers" {
			return m, tea.Sequence(echo, tea.Println(formatResponseMetadata(m.resHeaders, m.resTrailers)))
		}
		if len(c.Args) > 0 {
			return m, tea.Sequence(echo, tea.Println(style.Error.Render("✘ usage: view [headers]")))
		}
		m = m.openViewer()
		return m, echo
	case "desc", "describe":
		switch {
		case len(c.Args) == 1:
			return m, tea.Sequence(echo, describe(m.ctx, m.deps.DescribeRPC, m.deps.DescribeMessage, "", c.Args[0]))
		case len(c.Args) == 2 && (c.Args[0] == "rpc" || c.Args[0] == "message"):
			return m, tea.Sequence(echo, describe(m.ctx, m.deps.DescribeRPC, m.deps.DescribeMessage, c.Args[0], c.Args[1]))
		default:
			return m, tea.Sequence(echo, tea.Println(style.Error.Render("usage: desc [rpc|message] <name>")))
		}
	case "call":
		if len(c.Args) == 0 {
			return m.openSelector(echo)
		}
		return m, tea.Sequence(echo, prepare(m.ctx, m.deps.PrepareRequest, c.Args[0]))
	case "edit":
		if !m.hasEditor {
			return m, tea.Sequence(echo, tea.Println(style.Error.Render("✘ no request to edit yet")))
		}
		m.mode = ModeRequestEditor
		return m, echo
	default:
		return m, tea.Sequence(echo, prepareImplicit(m.ctx, m.deps.PrepareRequest, c.Name))
	}
}

func (m Model) openSelector(echo tea.Cmd) (tea.Model, tea.Cmd) {
	if m.loadErr != nil {
		return m, tea.Sequence(echo, tea.Println(renderError("schema is not loaded", errdetail.From(m.loadErr))))
	}
	m.selector.ResetFilter()
	m.mode = ModeRPCSelector
	return m, echo
}

func (m Model) updateSelector(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case rpcselector.SelectedMsg:
		return m, prepare(m.ctx, m.deps.PrepareRequest, msg.RPC.FullName)
	case rpcselector.BackMsg:
		m.mode = ModeCommand
		return m, nil
	}
	var cmd tea.Cmd
	m.selector, cmd = m.selector.Update(msg)
	return m, cmd
}

func (m Model) updateEditor(msg tea.Msg) (tea.Model, tea.Cmd) {
	clientStreaming := m.editor.RPC().ClientStreaming
	if k, ok := msg.(tea.KeyMsg); ok && clientStreaming && key.Matches(k, m.keys.Finish) {
		if m.session == nil {
			return m, nil
		}
		m.mode = ModeStreaming
		return m, closeSend(m.session)
	}
	switch msg.(type) {
	case requesteditor.PreviewMsg:
		if clientStreaming {
			return m.sendStreamMessage()
		}
		m.mode = ModePreview
		return m, nil
	case requesteditor.CancelMsg:
		if m.session != nil && m.cancel != nil {
			m.stopping = true
			m.cancel()
			m.mode = ModeStreaming
			return m, nil
		}
		m.mode = ModeCommand
		return m, nil
	}
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return m, cmd
}

func (m Model) updatePreview(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(k, m.keys.Send):
		return m.startInvoke()
	case key.Matches(k, m.keys.Edit), key.Matches(k, m.keys.Back):
		m.mode = ModeRequestEditor
	}
	return m, nil
}

func (m Model) sendStreamMessage() (tea.Model, tea.Cmd) {
	msg := m.editor.Builder().Message()
	if m.session != nil {
		m.sendSeq++
		return m, sendMessages(m.session, []proto.Message{msg}, m.sendSeq, false)
	}
	return m.beginStream([]proto.Message{msg}, false)
}

func (m Model) beginStream(pending []proto.Message, finish bool) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	m.streamCount = 0
	m.sentCount = 0
	m.sendSeq = 0
	m.stopping = false
	rpc := m.editor.RPC()
	if finish {
		m.mode = ModeStreaming
	}
	return m, tea.Batch(
		tea.Println(style.Subtle.Render("⇄ "+rpc.FullName+" stream opened")),
		m.spinner.Tick,
		openStream(ctx, m.deps.OpenStream, usecase.OpenStreamInput{RPCName: rpc.FullName, Headers: m.headers}, pending, finish),
	)
}

func (m Model) startInvoke() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(m.ctx)
	m.cancel = cancel
	rpc := m.editor.RPC()
	reqJSON, _ := textpresenter.ProtoJSON(m.editor.Builder().Message())
	echo := tea.Println(style.Subtle.Render("→ "+rpc.FullName) + "\n" + style.Subtle.Render(reqJSON))

	if rpc.ServerStreaming {
		m.mode = ModeStreaming
		m.streamCount = 0
		m.stopping = false
		return m, tea.Batch(
			echo,
			m.spinner.Tick,
			startStream(ctx, m.deps.InvokeStream, usecase.InvokeServerStreamInput{
				RPCName: rpc.FullName,
				Message: m.editor.Builder().Message(),
				Headers: m.headers,
			}),
		)
	}

	m.mode = ModeInvoking
	return m, tea.Batch(
		echo,
		m.spinner.Tick,
		invoke(ctx, m.deps.InvokeRPC, usecase.InvokeRPCInput{
			RPCName: rpc.FullName,
			Message: m.editor.Builder().Message(),
			Headers: m.headers,
		}),
	)
}

func (m Model) updateInvoking(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && (k.String() == "esc" || k.String() == "ctrl+g") && m.cancel != nil {
		m.stopping = true
		m.cancel()
		return m, nil
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m Model) viewerHeight() int {
	return max(m.height-4, 5)
}

func (m Model) openViewer() Model {
	viewer, err := jsonview.New([]byte(m.body), m.width, m.viewerHeight())
	if err != nil {
		return m
	}
	m.viewer = viewer
	m.notice = ""
	m.mode = ModeExplorer
	return m
}

func (m Model) updateExplorer(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		m.viewer, cmd = m.viewer.Update(msg)
		return m, cmd
	}
	m.notice = ""
	if !m.viewer.Searching() && key.Matches(k, m.keys.Back) {
		m.mode = ModeCommand
		return m, m.command.Focus()
	}
	m.viewer, cmd = m.viewer.Update(k)
	return m, cmd
}
