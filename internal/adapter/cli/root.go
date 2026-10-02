package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/atsuya-m/pluto/internal/adapter/presenter/errdetail"
	jsonpresenter "github.com/atsuya-m/pluto/internal/adapter/presenter/json"
	textpresenter "github.com/atsuya-m/pluto/internal/adapter/presenter/text"
	"github.com/atsuya-m/pluto/internal/bootstrap"
)

const (
	exitOK       = 0
	exitError    = 1
	exitUsage    = 2
	exitNotFound = 3
	exitRPC      = 4
)

type options struct {
	cfg        bootstrap.Config
	output     string
	configPath string
	profile    string
	flags      *pflag.FlagSet
	getenv     func(string) (string, bool)
	cwd        func() (string, error)
}

type presentedError struct {
	err error
}

func (e *presentedError) Error() string { return e.err.Error() }
func (e *presentedError) Unwrap() error { return e.err }

func Execute(ctx context.Context) int {
	return run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return runWithEnv(ctx, args, stdin, stdout, stderr, os.LookupEnv, os.Getwd)
}

func runWithEnv(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) (string, bool), cwd func() (string, error)) int {
	opts := &options{getenv: getenv, cwd: cwd}
	root := newRootCommand(opts)
	root.SetArgs(args)
	root.SetIn(stdin)
	root.SetOut(stdout)
	root.SetErr(stderr)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return exitOK
	}

	var presented *presentedError
	if errors.As(err, &presented) {
		return exitCode(errdetail.From(presented.err))
	}
	_, _ = fmt.Fprintln(stderr, "error:", err)
	return exitUsage
}

func newRootCommand(opts *options) *cobra.Command {
	root := &cobra.Command{
		Use:           "pluto",
		Short:         "Explore and call Protobuf RPC APIs",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	flags := root.PersistentFlags()
	flags.StringSliceVarP(&opts.cfg.SchemaPaths, "schema", "s", []string{"."}, "proto files or directories")
	flags.StringSliceVarP(&opts.cfg.ImportPaths, "import-path", "I", nil, "additional proto import paths")
	flags.BoolVarP(&opts.cfg.Reflection, "reflection", "r", false, "load schema from the server via gRPC server reflection")
	flags.StringVarP(&opts.cfg.Target, "target", "t", "http://localhost:8080", "server base URL")
	flags.StringVar(&opts.cfg.Protocol, "protocol", "connect", "wire protocol: connect, grpc, grpcweb")
	flags.BoolVar(&opts.cfg.JSONCodec, "json-codec", false, "use JSON instead of binary protobuf on the wire")
	flags.DurationVar(&opts.cfg.Timeout, "timeout", 60*time.Second, "timeout for unary calls (0 for none; streaming calls have no timeout)")
	flags.StringArrayVarP(&opts.cfg.Headers, "header", "H", nil, "request header 'Key: Value' (repeatable)")
	flags.StringVarP(&opts.output, "output", "o", "text", "output format: text, json")
	flags.StringVar(&opts.configPath, "config", "", "config file (default: .pluto.yaml in the current or a parent directory, then user config dir/pluto/config.yaml; env PLUTO_CONFIG)")
	flags.StringVarP(&opts.profile, "profile", "p", "", "profile in the config file (env PLUTO_PROFILE)")
	opts.flags = flags

	root.AddCommand(newDescCommand(opts), newCallCommand(opts), newReplCommand(opts), newProfilesCommand(opts))
	return root
}

func (o *options) app() (*bootstrap.Application, error) {
	if o.output != "text" && o.output != "json" {
		return nil, fmt.Errorf("unknown output format %q", o.output)
	}
	cfg, err := o.resolveConfig()
	if err != nil {
		return nil, err
	}
	o.cfg = cfg
	return bootstrap.New(cfg)
}

func (o *options) configFile() (*bootstrap.ConfigFile, error) {
	explicit := o.configPath
	if explicit == "" {
		explicit, _ = o.getenv("PLUTO_CONFIG")
	}
	cwd, err := o.cwd()
	if err != nil {
		return nil, err
	}
	path, err := bootstrap.FindConfigFile(explicit, cwd)
	if err != nil || path == "" {
		return nil, err
	}
	return bootstrap.LoadConfigFile(path)
}

func (o *options) resolveConfig() (bootstrap.Config, error) {
	file, err := o.configFile()
	if err != nil {
		return o.cfg, err
	}
	name := o.profile
	if name == "" {
		name, _ = o.getenv("PLUTO_PROFILE")
	}
	if file == nil {
		if name != "" {
			return o.cfg, fmt.Errorf("profile %q requested but no config file was found (create %s or pass --config)", name, bootstrap.ProjectConfigName)
		}
		return o.cfg, nil
	}
	if name == "" {
		name = file.DefaultProfile
	}
	if name == "" {
		return o.cfg, nil
	}
	profile, err := file.Profile(name, o.getenv)
	if err != nil {
		return o.cfg, err
	}
	return o.cfg.WithProfile(profile, o.flags.Changed), nil
}

func (o *options) present(w io.Writer, text func(io.Writer) error, json func(io.Writer) error) error {
	if o.output == "json" {
		return json(w)
	}
	return text(w)
}

func (o *options) fail(cmd *cobra.Command, err error) error {
	d := errdetail.From(err)
	if o.output == "json" {
		_ = jsonpresenter.Error(cmd.OutOrStdout(), d)
	} else {
		_ = textpresenter.Error(cmd.ErrOrStderr(), d)
	}
	return &presentedError{err: err}
}

func exitCode(d errdetail.ErrorDetail) int {
	switch d.Kind {
	case errdetail.KindNotFound, errdetail.KindAmbiguous:
		return exitNotFound
	case errdetail.KindRPC:
		return exitRPC
	default:
		return exitError
	}
}
