package app

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/atsuya-m/pluto/internal/adapter/presenter/errdetail"
	textpresenter "github.com/atsuya-m/pluto/internal/adapter/presenter/text"
	"github.com/atsuya-m/pluto/internal/adapter/tui/style"
	"github.com/atsuya-m/pluto/internal/application/usecase"
)

const helpText = `commands:
  <rpc>                   same as ` + "`call <rpc>`" + `
  rpcs [service]          select an rpc interactively (or list rpcs of a service)
  call [rpc]              open the request editor for an rpc
  header                  show request headers (sensitive values are masked)
  header set|add <K> <V>  set or add a request header
  header rm <K> | clear   remove one header or all of them
  desc [rpc|message] <n>  describe an rpc or message
  services                list services
  edit                    reopen the last request
  reload                  reload rpc list for completion
  clear                   clear the screen
  view                    explore the last response (fold, search, y/Y copy)
  view headers            show the headers and trailers of the last response
  exit                    quit (or ctrl+c)

completion: suggestions pop up while typing • tab/shift+tab select • esc dismiss
history:    ↑/↓ or ctrl+p/ctrl+n (while no suggestion is selected)
editing:    emacs keys (ctrl+a/e/b/f/k/u/w/d, alt+b/f/d)`

func banner(d Dependencies) string {
	profile := ""
	if d.Profile != "" {
		profile = "profile " + d.Profile + " • "
	}
	return style.Title.Render("pluto") + style.Subtle.Render(fmt.Sprintf("  %starget %s (%s) • schema %s • type `help` for commands", profile, d.Target, d.Protocol, d.SchemaSource))
}

func (m Model) View() string {
	switch m.mode {
	case ModeRPCSelector:
		return m.selector.View()
	case ModeRequestEditor:
		return m.streamBanner() + m.editor.View()
	case ModePreview:
		return m.previewView()
	case ModeInvoking:
		return fmt.Sprintf("%s Calling %s...  %s\n", m.spinner.View(), m.editor.RPC().FullName, style.Help.Render("esc cancel"))
	case ModeStreaming:
		if m.editor.RPC().ClientStreaming {
			return fmt.Sprintf("%s Streaming %s... %d sent · %d received  %s\n", m.spinner.View(), m.editor.RPC().FullName, m.sentCount, m.streamCount, style.Help.Render("esc stop"))
		}
		return fmt.Sprintf("%s Streaming %s... %d messages  %s\n", m.spinner.View(), m.editor.RPC().FullName, m.streamCount, style.Help.Render("esc stop"))
	case ModeExplorer:
		return m.explorerView()
	default:
		return m.commandView()
	}
}

func (m Model) commandView() string {
	status := style.Subtle.Render(fmt.Sprintf("%d rpcs loaded", len(m.rpcs)))
	if m.loadErr != nil {
		status = style.Error.Render("schema load failed")
	}
	return m.command.View() + "\n" + style.Help.Render("tab complete • ↑/↓ history • help • ctrl+c quit • ") + status
}

func (m Model) previewView() string {
	var b strings.Builder
	rpc := m.editor.RPC()
	b.WriteString(style.Title.Render(rpc.Name) + style.Subtle.Render("  "+rpc.FullName) + "\n\n")
	b.WriteString(style.Selected.Render("Request") + "\n\n")
	body, err := textpresenter.ProtoJSON(m.editor.Builder().Message())
	if err != nil {
		body = style.Error.Render(err.Error())
	}
	lines := strings.Split(body, "\n")
	if limit := max(m.height-8, 5); len(lines) > limit {
		lines = append(lines[:limit], style.Subtle.Render(fmt.Sprintf("… %d more lines", len(lines)-limit)))
	}
	b.WriteString(strings.Join(lines, "\n") + "\n\n")
	b.WriteString(style.Help.Render("enter send • e edit • esc back"))
	return b.String()
}

func (m Model) explorerView() string {
	head := style.Title.Render("response") + style.Subtle.Render("  "+m.bodyRPC)
	notice := ""
	if m.notice != "" {
		notice = "\n" + m.notice
	}
	return head + "\n" + m.viewer.View() + notice + "\n" + style.Help.Render(m.viewer.Help()+" • q back")
}

func renderResponse(rpc usecase.RPCSummary, out usecase.InvokeRPCOutput, duration string) string {
	body, err := textpresenter.ProtoJSON(out.Message)
	if err != nil {
		body = err.Error()
	}
	return fmt.Sprintf("%s %s %s\n%s", style.Success.Render("✔"), rpc.FullName, style.Subtle.Render("("+duration+")"), body)
}

func (m Model) streamBanner() string {
	rpc := m.editor.RPC()
	if !rpc.ClientStreaming {
		return ""
	}
	if m.session == nil {
		return style.Subtle.Render("⇄ streaming rpc • C-s opens the stream and sends this message") + "\n\n"
	}
	return style.Success.Render(fmt.Sprintf("⇄ stream open · %d sent · %d received", m.sentCount, m.streamCount)) +
		style.Help.Render("  C-s send • C-x finish sending • esc (at top) cancel") + "\n\n"
}

func streamCounts(r result) string {
	if r.clientSt {
		return fmt.Sprintf("%d sent · %d received", r.sent, r.count)
	}
	return fmt.Sprintf("%d messages", r.count)
}

func renderStreamMessage(n int, msg proto.Message) string {
	body, err := textpresenter.ProtoJSON(msg)
	if err != nil {
		body = err.Error()
	}
	return style.Subtle.Render(fmt.Sprintf("← #%d", n)) + "\n" + body
}

func renderSentMessage(n int, msg proto.Message) string {
	body, err := textpresenter.ProtoJSON(msg)
	if err != nil {
		body = err.Error()
	}
	return style.Subtle.Render(fmt.Sprintf("→ #%d", n)) + "\n" + style.Subtle.Render(body)
}

func renderStreamSummary(r result) string {
	label := style.Success.Render("✔ stream closed")
	if r.stopped {
		label = style.Success.Render("■ stopped")
	}
	return fmt.Sprintf("%s %s %s", label, r.rpc, style.Subtle.Render(fmt.Sprintf("(%s, %s)", streamCounts(r), r.duration)))
}

func renderError(title string, d errdetail.ErrorDetail) string {
	var b strings.Builder
	b.WriteString(style.Failure.Render("✘ " + title))
	if d.Kind == errdetail.KindRPC {
		fmt.Fprintf(&b, "\n  code:    %s\n  message: %s", d.Code, d.Message)
	} else {
		b.WriteString("\n  " + d.Message)
	}
	for _, c := range d.Candidates {
		b.WriteString("\n    - " + c)
	}
	for _, detail := range d.Details {
		b.WriteString("\n  detail:  " + detail)
	}
	for _, k := range headerKeys(d.Metadata) {
		b.WriteString("\n  " + k + ": " + strings.Join(d.Metadata[k], ", "))
	}
	return style.Error.Render(b.String())
}
