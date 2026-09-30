package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/protobuf/proto"

	textpresenter "github.com/atsuya-m/pluto/internal/adapter/presenter/text"
	"github.com/atsuya-m/pluto/internal/application/usecase"
	"github.com/atsuya-m/pluto/internal/domain/schema"
)

func loadRPCs(ctx context.Context, uc ListRPCsUseCase) tea.Cmd {
	return func() tea.Msg {
		out, err := uc.Execute(ctx, usecase.ListRPCsInput{})
		if err != nil {
			return rpcsLoadFailedMsg{Err: err}
		}
		return rpcsLoadedMsg{RPCs: out.RPCs}
	}
}

func prepare(ctx context.Context, uc PrepareRequestUseCase, name string) tea.Cmd {
	return prepareWith(ctx, uc, usecase.PrepareRequestInput{RPCName: name, RestoreLast: true})
}

func prepareWith(ctx context.Context, uc PrepareRequestUseCase, input usecase.PrepareRequestInput) tea.Cmd {
	return func() tea.Msg {
		out, err := uc.Execute(ctx, input)
		if err != nil {
			return errMsg{Err: err}
		}
		return preparedMsg{Output: out}
	}
}

func prepareImplicit(ctx context.Context, uc PrepareRequestUseCase, name string) tea.Cmd {
	return func() tea.Msg {
		out, err := uc.Execute(ctx, usecase.PrepareRequestInput{RPCName: name, RestoreLast: true})
		if errors.Is(err, schema.ErrSymbolNotFound) {
			return errMsg{Err: fmt.Errorf("unknown command or rpc %q (try `help`)", name)}
		}
		if err != nil {
			return errMsg{Err: err}
		}
		return preparedMsg{Output: out}
	}
}

func invoke(ctx context.Context, uc InvokeRPCUseCase, input usecase.InvokeRPCInput) tea.Cmd {
	return func() tea.Msg {
		start := time.Now()
		out, err := uc.Execute(ctx, input)
		if err != nil {
			return invokeFailedMsg{Err: err, Duration: time.Since(start)}
		}
		return invokedMsg{Output: out, Duration: time.Since(start)}
	}
}

func startStream(ctx context.Context, uc InvokeServerStreamUseCase, input usecase.InvokeServerStreamInput) tea.Cmd {
	return func() tea.Msg {
		events := make(chan tea.Msg, 16)
		input.OnMessage = func(msg proto.Message) error {
			select {
			case events <- streamMessageMsg{Message: msg}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		go func() {
			defer close(events)
			start := time.Now()
			out, err := uc.Execute(ctx, input)
			events <- streamDoneMsg{Output: out, Err: err, Duration: time.Since(start)}
		}()
		return streamStartedMsg{Events: events}
	}
}

func openStream(ctx context.Context, uc OpenStreamUseCase, input usecase.OpenStreamInput, pending []proto.Message, finish bool) tea.Cmd {
	return func() tea.Msg {
		out, err := uc.Execute(ctx, input)
		if err != nil {
			return streamDoneMsg{Err: err}
		}
		return streamOpenedMsg{Session: out.Session, Pending: pending, Finish: finish}
	}
}

func startReceive(session *usecase.StreamSession) tea.Cmd {
	return func() tea.Msg {
		events := make(chan tea.Msg, 16)
		go func() {
			defer close(events)
			start := time.Now()
			err := session.ReceiveAll(func(msg proto.Message) error {
				events <- streamMessageMsg{Message: msg}
				return nil
			})
			events <- streamDoneMsg{Err: err, Duration: time.Since(start)}
		}()
		return streamStartedMsg{Events: events}
	}
}

func sendMessages(session *usecase.StreamSession, msgs []proto.Message, firstSeq int, finish bool) tea.Cmd {
	var cmds []tea.Cmd
	for i, msg := range msgs {
		cmds = append(cmds, tea.Println(renderSentMessage(firstSeq+i, msg)), func() tea.Msg {
			if err := session.Send(msg); err != nil {
				return streamSendFailedMsg{Err: err}
			}
			return streamSentMsg{Message: msg}
		})
	}
	if finish {
		cmds = append(cmds, closeSend(session))
	}
	return tea.Sequence(cmds...)
}

func closeSend(session *usecase.StreamSession) tea.Cmd {
	return func() tea.Msg {
		if err := session.CloseSend(); err != nil {
			return streamSendFailedMsg{Err: err}
		}
		return nil
	}
}

func listSaved(ctx context.Context, uc ListSavedRequestsUseCase) tea.Cmd {
	if uc == nil {
		return nil
	}
	return func() tea.Msg {
		saved, err := uc.Execute(ctx)
		if err != nil {
			return errMsg{Err: err}
		}
		return savedListedMsg{Saved: saved}
	}
}

func saveRequest(ctx context.Context, uc SaveRequestUseCase, input usecase.SaveRequestInput) tea.Cmd {
	if uc == nil {
		return nil
	}
	return func() tea.Msg {
		if err := uc.Execute(ctx, input); err != nil {
			return errMsg{Err: err}
		}
		if input.Name == "" {
			return nil
		}
		return requestSavedMsg{Name: input.Name, RPC: input.RPCName}
	}
}

func deleteSaved(ctx context.Context, uc DeleteSavedRequestUseCase, name string) tea.Cmd {
	return func() tea.Msg {
		if err := uc.Execute(ctx, name); err != nil {
			return errMsg{Err: err}
		}
		return requestDeletedMsg{Name: name}
	}
}

func waitForStream(events <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			return nil
		}
		return msg
	}
}

func listServices(ctx context.Context, uc ListServicesUseCase) tea.Cmd {
	return func() tea.Msg {
		out, err := uc.Execute(ctx, usecase.ListServicesInput{})
		if err != nil {
			return errMsg{Err: err}
		}
		var b strings.Builder
		_ = textpresenter.Services(&b, out)
		return printMsg{Text: strings.TrimRight(b.String(), "\n")}
	}
}

func listRPCsText(ctx context.Context, uc ListRPCsUseCase, service string) tea.Cmd {
	return func() tea.Msg {
		out, err := uc.Execute(ctx, usecase.ListRPCsInput{Service: service})
		if err != nil {
			return errMsg{Err: err}
		}
		var b strings.Builder
		for _, r := range out.RPCs {
			b.WriteString(r.FullName + "  " + textpresenter.Signature(r) + "\n")
		}
		return printMsg{Text: strings.TrimRight(b.String(), "\n")}
	}
}

func describe(ctx context.Context, rpc DescribeRPCUseCase, msg DescribeMessageUseCase, kind, name string) tea.Cmd {
	return func() tea.Msg {
		if kind == "" || kind == "rpc" {
			out, err := rpc.Execute(ctx, usecase.DescribeRPCInput{Name: name})
			if err == nil {
				return printMsg{Text: strings.TrimRight(textpresenter.RPCString(out.RPC), "\n")}
			}
			if kind == "rpc" || !errors.Is(err, schema.ErrSymbolNotFound) {
				return errMsg{Err: err}
			}
		}
		out, err := msg.Execute(ctx, usecase.DescribeMessageInput{Name: name})
		if err != nil {
			return errMsg{Err: err}
		}
		return printMsg{Text: strings.TrimRight(textpresenter.MessageString(out.Message), "\n")}
	}
}
