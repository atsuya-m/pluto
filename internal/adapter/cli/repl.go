package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/atsuya-m/pluto/internal/adapter/tui/app"
	"github.com/atsuya-m/pluto/internal/bootstrap"
)

func newReplCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "repl",
		Short: "Start the interactive shell",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			a, err := opts.app()
			if err != nil {
				return err
			}
			return app.Run(cmd.Context(), app.Dependencies{
				ListServices:    a.ListServices,
				ListRPCs:        a.ListRPCs,
				DescribeRPC:     a.DescribeRPC,
				DescribeMessage: a.DescribeMessage,
				PrepareRequest:  a.PrepareRequest,
				InvokeRPC:       a.InvokeRPC,
				InvokeStream:    a.InvokeStream,
				OpenStream:      a.OpenStream,
				Headers:         a.Headers,
				Target:          a.Config.Target,
				Protocol:        a.Config.Protocol,
				SchemaSource:    schemaSource(a.Config),
				Profile:         a.Config.Profile,
			})
		},
	}
}

func schemaSource(cfg bootstrap.Config) string {
	if cfg.Reflection {
		return "reflection"
	}
	return strings.Join(cfg.SchemaPaths, ",")
}
