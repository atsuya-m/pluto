package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type profileJSON struct {
	Name        string   `json:"name"`
	Default     bool     `json:"default"`
	Target      string   `json:"target,omitempty"`
	Protocol    string   `json:"protocol,omitempty"`
	Reflection  bool     `json:"reflection"`
	Schema      []string `json:"schema,omitempty"`
	ImportPaths []string `json:"import_paths,omitempty"`
	Headers     []string `json:"headers,omitempty"`
}

func newProfilesCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "profiles",
		Short: "List profiles in the config file (header values are not shown)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.output != "text" && opts.output != "json" {
				return fmt.Errorf("unknown output format %q", opts.output)
			}
			file, err := opts.configFile()
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if file == nil {
				if opts.output == "json" {
					return writeJSON(w, map[string]any{"config": nil, "profiles": []profileJSON{}})
				}
				_, err := fmt.Fprintln(w, "no config file found (create .pluto.yaml or pass --config)")
				return err
			}

			var profiles []profileJSON
			for _, name := range file.ProfileNames() {
				p, _ := file.Summary(name)
				headers := make([]string, 0, len(p.Headers))
				for k := range p.Headers {
					headers = append(headers, k)
				}
				sort.Strings(headers)
				profiles = append(profiles, profileJSON{
					Name:        name,
					Default:     name == file.DefaultProfile,
					Target:      p.Target,
					Protocol:    p.Protocol,
					Reflection:  p.Reflection != nil && *p.Reflection,
					Schema:      p.Schema,
					ImportPaths: p.ImportPaths,
					Headers:     headers,
				})
			}
			if opts.output == "json" {
				return writeJSON(w, map[string]any{"config": file.Path, "profiles": profiles})
			}
			return writeProfilesText(w, file.Path, profiles)
		},
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func writeProfilesText(w io.Writer, path string, profiles []profileJSON) error {
	var b strings.Builder
	fmt.Fprintf(&b, "config: %s\n", path)
	for _, p := range profiles {
		mark := " "
		if p.Default {
			mark = "*"
		}
		schema := strings.Join(p.Schema, ",")
		if p.Reflection {
			schema = "reflection"
		}
		fmt.Fprintf(&b, "%s %s\t%s\t%s\t%s", mark, p.Name, p.Target, p.Protocol, schema)
		if len(p.Headers) > 0 {
			fmt.Fprintf(&b, "\theaders: %s", strings.Join(p.Headers, ", "))
		}
		b.WriteString("\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}
