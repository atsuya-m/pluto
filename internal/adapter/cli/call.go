package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"

	"github.com/atsuya-m/pluto/internal/adapter/presenter/errdetail"
	jsonpresenter "github.com/atsuya-m/pluto/internal/adapter/presenter/json"
	textpresenter "github.com/atsuya-m/pluto/internal/adapter/presenter/text"
	"github.com/atsuya-m/pluto/internal/application/usecase"
	"github.com/atsuya-m/pluto/internal/bootstrap"
)

func newCallCommand(opts *options) *cobra.Command {
	var data string

	cmd := &cobra.Command{
		Use:   "call <rpc>",
		Short: "Call an rpc with JSON request(s); streaming rpcs take newline separated JSON or a JSON array",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := opts.app()
			if err != nil {
				return err
			}
			body, err := readData(cmd.InOrStdin(), data)
			if err != nil {
				return err
			}
			prepared, err := app.PrepareRequest.Execute(cmd.Context(), usecase.PrepareRequestInput{
				RPCName: args[0],
				Data:    body,
			})
			if err != nil {
				return opts.fail(cmd, err)
			}
			if prepared.RPC.ClientStreaming {
				return callClientStream(cmd, opts, app, prepared)
			}
			if prepared.RPC.ServerStreaming {
				return callServerStream(cmd, opts, app, prepared)
			}
			out, err := app.InvokeRPC.Execute(cmd.Context(), usecase.InvokeRPCInput{
				RPCName: prepared.RPC.FullName,
				Message: prepared.Builder.Message(),
				Headers: app.Headers,
			})
			if err != nil {
				return opts.fail(cmd, err)
			}
			return opts.present(cmd.OutOrStdout(),
				func(w io.Writer) error { return textpresenter.Invoke(w, out) },
				func(w io.Writer) error { return jsonpresenter.Invoke(w, out) })
		},
	}
	cmd.Flags().StringVarP(&data, "data", "d", "", "request JSON, '@file' to read a file, '-' for stdin")
	return cmd
}

func callClientStream(cmd *cobra.Command, opts *options, app *bootstrap.Application, prepared usecase.PrepareRequestOutput) error {
	w := cmd.OutOrStdout()
	fail := func(err error) error {
		if opts.output == "json" {
			_ = jsonpresenter.ErrorLine(w, errdetail.From(err))
		} else {
			_ = textpresenter.Error(cmd.ErrOrStderr(), errdetail.From(err))
		}
		return &presentedError{err: err}
	}
	if len(prepared.Messages) == 0 {
		return fmt.Errorf("%s is a streaming rpc; pass one or more request messages with -d (newline separated JSON or a JSON array)", prepared.RPC.FullName)
	}

	opened, err := app.OpenStream.Execute(cmd.Context(), usecase.OpenStreamInput{RPCName: prepared.RPC.FullName, Headers: app.Headers})
	if err != nil {
		return fail(err)
	}
	session := opened.Session
	defer func() { _ = session.Close() }()

	received := make(chan error, 1)
	go func() {
		received <- session.ReceiveAll(func(msg proto.Message) error {
			return printStreamMessage(w, opts, msg)
		})
	}()

	for _, msg := range prepared.Messages {
		if err := session.Send(msg); err != nil {
			_ = session.CloseSend()
			<-received
			return fail(err)
		}
	}
	if err := session.CloseSend(); err != nil {
		return fail(err)
	}
	if err := <-received; err != nil {
		return fail(err)
	}
	if opts.output == "json" {
		sent, got := session.Counts()
		return jsonpresenter.ClientStreamSummary(w, sent, got, session.Headers(), session.Trailers())
	}
	return nil
}

func printStreamMessage(w io.Writer, opts *options, msg proto.Message) error {
	if opts.output == "json" {
		return jsonpresenter.StreamMessage(w, msg)
	}
	s, err := textpresenter.ProtoJSON(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, s)
	return err
}

func callServerStream(cmd *cobra.Command, opts *options, app *bootstrap.Application, prepared usecase.PrepareRequestOutput) error {
	w := cmd.OutOrStdout()
	out, err := app.InvokeStream.Execute(cmd.Context(), usecase.InvokeServerStreamInput{
		RPCName: prepared.RPC.FullName,
		Message: prepared.Builder.Message(),
		Headers: app.Headers,
		OnMessage: func(msg proto.Message) error {
			return printStreamMessage(w, opts, msg)
		},
	})
	if err != nil {
		if opts.output == "json" {
			_ = jsonpresenter.ErrorLine(w, errdetail.From(err))
		} else {
			_ = textpresenter.Error(cmd.ErrOrStderr(), errdetail.From(err))
		}
		return &presentedError{err: err}
	}
	if opts.output == "json" {
		return jsonpresenter.StreamSummary(w, out)
	}
	return nil
}

func readData(stdin io.Reader, data string) ([]byte, error) {
	switch {
	case data == "-":
		return io.ReadAll(stdin)
	case strings.HasPrefix(data, "@"):
		b, err := os.ReadFile(strings.TrimPrefix(data, "@"))
		if err != nil {
			return nil, fmt.Errorf("read request data: %w", err)
		}
		return b, nil
	default:
		return []byte(data), nil
	}
}
