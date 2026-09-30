package cli

import (
	"io"

	"github.com/spf13/cobra"

	jsonpresenter "github.com/atsuya-m/pluto/internal/adapter/presenter/json"
	textpresenter "github.com/atsuya-m/pluto/internal/adapter/presenter/text"
	"github.com/atsuya-m/pluto/internal/application/usecase"
)

func newDescCommand(opts *options) *cobra.Command {
	desc := &cobra.Command{
		Use:     "desc",
		Aliases: []string{"describe"},
		Short:   "Describe services, rpcs and messages",
	}

	desc.AddCommand(
		&cobra.Command{
			Use:   "services",
			Short: "List services",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				app, err := opts.app()
				if err != nil {
					return err
				}
				out, err := app.ListServices.Execute(cmd.Context(), usecase.ListServicesInput{})
				if err != nil {
					return opts.fail(cmd, err)
				}
				return opts.present(cmd.OutOrStdout(),
					func(w io.Writer) error { return textpresenter.Services(w, out) },
					func(w io.Writer) error { return jsonpresenter.Services(w, out) })
			},
		},
		&cobra.Command{
			Use:   "rpcs [service]",
			Short: "List rpcs",
			Args:  cobra.MaximumNArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				app, err := opts.app()
				if err != nil {
					return err
				}
				in := usecase.ListRPCsInput{}
				if len(args) == 1 {
					in.Service = args[0]
				}
				out, err := app.ListRPCs.Execute(cmd.Context(), in)
				if err != nil {
					return opts.fail(cmd, err)
				}
				return opts.present(cmd.OutOrStdout(),
					func(w io.Writer) error { return textpresenter.RPCs(w, out) },
					func(w io.Writer) error { return jsonpresenter.RPCs(w, out) })
			},
		},
		&cobra.Command{
			Use:   "rpc <name>",
			Short: "Describe an rpc and its request/response messages",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				app, err := opts.app()
				if err != nil {
					return err
				}
				out, err := app.DescribeRPC.Execute(cmd.Context(), usecase.DescribeRPCInput{Name: args[0]})
				if err != nil {
					return opts.fail(cmd, err)
				}
				return opts.present(cmd.OutOrStdout(),
					func(w io.Writer) error { return textpresenter.RPC(w, out) },
					func(w io.Writer) error { return jsonpresenter.RPC(w, out) })
			},
		},
		&cobra.Command{
			Use:   "message <name>",
			Short: "Describe a message",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				app, err := opts.app()
				if err != nil {
					return err
				}
				out, err := app.DescribeMessage.Execute(cmd.Context(), usecase.DescribeMessageInput{Name: args[0]})
				if err != nil {
					return opts.fail(cmd, err)
				}
				return opts.present(cmd.OutOrStdout(),
					func(w io.Writer) error { return textpresenter.Message(w, out) },
					func(w io.Writer) error { return jsonpresenter.Message(w, out) })
			},
		},
	)
	return desc
}
