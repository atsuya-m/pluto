package bootstrap

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const ProjectConfigName = ".pluto.yaml"

type Profile struct {
	Name        string
	Schema      []string
	ImportPaths []string
	Reflection  *bool
	Target      string
	Protocol    string
	JSONCodec   *bool
	Headers     map[string]string
}

type ConfigFile struct {
	Path           string
	DefaultProfile string
	profiles       map[string]profileYAML
}

type stringList []string

func (l *stringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*l = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		var items []string
		if err := node.Decode(&items); err != nil {
			return err
		}
		*l = items
		return nil
	default:
		return fmt.Errorf("line %d: expected a string or a list of strings", node.Line)
	}
}

type profileYAML struct {
	Schema      stringList        `yaml:"schema"`
	ImportPaths stringList        `yaml:"import_paths"`
	Reflection  *bool             `yaml:"reflection"`
	Target      string            `yaml:"target"`
	Protocol    string            `yaml:"protocol"`
	JSONCodec   *bool             `yaml:"json_codec"`
	Headers     map[string]string `yaml:"headers"`
}

type fileYAML struct {
	DefaultProfile string                 `yaml:"default_profile"`
	Profiles       map[string]profileYAML `yaml:"profiles"`
}

func FindConfigFile(explicit, cwd string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err != nil {
			return "", fmt.Errorf("config file: %w", err)
		}
		return explicit, nil
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, ProjectConfigName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if userDir, err := os.UserConfigDir(); err == nil {
		candidate := filepath.Join(userDir, "pluto", "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", nil
}

func LoadConfigFile(path string) (*ConfigFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var doc fileYAML
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if doc.DefaultProfile != "" {
		if _, ok := doc.Profiles[doc.DefaultProfile]; !ok {
			return nil, fmt.Errorf("config %s: default_profile %q is not defined", path, doc.DefaultProfile)
		}
	}
	return &ConfigFile{Path: path, DefaultProfile: doc.DefaultProfile, profiles: doc.Profiles}, nil
}

func (c *ConfigFile) ProfileNames() []string {
	names := make([]string, 0, len(c.profiles))
	for name := range c.profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *ConfigFile) Summary(name string) (Profile, bool) {
	raw, ok := c.profiles[name]
	if !ok {
		return Profile{}, false
	}
	return Profile{
		Name:        name,
		Schema:      raw.Schema,
		ImportPaths: raw.ImportPaths,
		Reflection:  raw.Reflection,
		Target:      raw.Target,
		Protocol:    raw.Protocol,
		JSONCodec:   raw.JSONCodec,
		Headers:     raw.Headers,
	}, true
}

func (c *ConfigFile) Profile(name string, lookup func(string) (string, bool)) (Profile, error) {
	raw, ok := c.profiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("profile %q is not defined in %s (available: %s)", name, c.Path, strings.Join(c.ProfileNames(), ", "))
	}
	e := expander{lookup: lookup, base: filepath.Dir(c.Path)}
	p := Profile{
		Name:       name,
		Reflection: raw.Reflection,
		JSONCodec:  raw.JSONCodec,
		Target:     e.value(raw.Target),
		Protocol:   e.value(raw.Protocol),
	}
	for _, s := range raw.Schema {
		p.Schema = append(p.Schema, e.path(s))
	}
	for _, s := range raw.ImportPaths {
		p.ImportPaths = append(p.ImportPaths, e.path(s))
	}
	if len(raw.Headers) > 0 {
		p.Headers = map[string]string{}
		for k, v := range raw.Headers {
			p.Headers[k] = e.value(v)
		}
	}
	if len(e.missing) > 0 {
		return Profile{}, fmt.Errorf("profile %q uses environment variables that are not set: %s", name, strings.Join(e.missingNames(), ", "))
	}
	return p, nil
}

type expander struct {
	lookup  func(string) (string, bool)
	base    string
	missing map[string]bool
}

func (e *expander) value(s string) string {
	return os.Expand(s, func(name string) string {
		v, ok := e.lookup(name)
		if !ok || v == "" {
			if e.missing == nil {
				e.missing = map[string]bool{}
			}
			e.missing[name] = true
		}
		return v
	})
}

func (e *expander) path(s string) string {
	if s == "" {
		return ""
	}
	s = e.value(s)
	if s == "~" || strings.HasPrefix(s, "~/") {
		if home, ok := e.lookup("HOME"); ok {
			s = filepath.Join(home, strings.TrimPrefix(s, "~"))
		}
	}
	if !filepath.IsAbs(s) {
		s = filepath.Join(e.base, s)
	}
	return s
}

func (e *expander) missingNames() []string {
	names := make([]string, 0, len(e.missing))
	for n := range e.missing {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (c Config) WithProfile(p Profile, changed func(flag string) bool) Config {
	c.Profile = p.Name
	if !changed("schema") && len(p.Schema) > 0 {
		c.SchemaPaths = p.Schema
	}
	if !changed("import-path") && len(p.ImportPaths) > 0 {
		c.ImportPaths = p.ImportPaths
	}
	if !changed("reflection") && p.Reflection != nil {
		c.Reflection = *p.Reflection
	}
	if !changed("target") && p.Target != "" {
		c.Target = p.Target
	}
	if !changed("protocol") && p.Protocol != "" {
		c.Protocol = p.Protocol
	}
	if !changed("json-codec") && p.JSONCodec != nil {
		c.JSONCodec = *p.JSONCodec
	}
	c.ProfileHeaders = p.Headers
	return c
}
