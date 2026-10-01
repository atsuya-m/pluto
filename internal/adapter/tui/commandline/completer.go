package commandline

import (
	"sort"
	"strings"

	"github.com/atsuya-m/pluto/internal/application/usecase"
)

type Suggestion struct {
	Text        string
	Description string
	aliases     []string
}

var commandSuggestions = []Suggestion{
	{Text: "call", Description: "open the request editor for an rpc"},
	{Text: "desc", Description: "describe an rpc or message"},
	{Text: "rpcs", Description: "select an rpc (or list rpcs of a service)"},
	{Text: "services", Description: "list services"},
	{Text: "edit", Description: "reopen the last request"},
	{Text: "header", Description: "show or change request headers"},
	{Text: "reload", Description: "reload the rpc list"},
	{Text: "clear", Description: "clear the screen"},
	{Text: "help", Description: "show help"},
	{Text: "exit", Description: "quit"},
}

type Completer struct {
	rpcs     []Suggestion
	services []Suggestion
	headers  []Suggestion
}

func (c Completer) WithHeaderKeys(keys []string) Completer {
	c.headers = nil
	for _, k := range keys {
		c.headers = append(c.headers, Suggestion{Text: k, Description: "request header"})
	}
	return c
}

func NewCompleter(rpcs []usecase.RPCSummary) Completer {
	counts := map[string]int{}
	for _, r := range rpcs {
		counts[r.Name]++
	}

	var c Completer
	services := map[string]int{}
	for _, r := range rpcs {
		short := lastSegment(r.Service)
		text := r.Name
		if counts[r.Name] > 1 {
			text = short + "." + r.Name
		}
		c.rpcs = append(c.rpcs, Suggestion{
			Text:        text,
			Description: short + " · " + lastSegment(r.RequestType) + " → " + lastSegment(r.ResponseType) + streamingLabel(r),
			aliases:     []string{r.Name, short + "." + r.Name, r.FullName},
		})
		services[r.Service]++
	}

	names := make([]string, 0, len(services))
	for s := range services {
		names = append(names, s)
	}
	sort.Strings(names)
	for _, s := range names {
		c.services = append(c.services, Suggestion{
			Text:        lastSegment(s),
			Description: s,
			aliases:     []string{lastSegment(s), s},
		})
	}
	return c
}

func streamingLabel(r usecase.RPCSummary) string {
	switch {
	case r.ClientStreaming && r.ServerStreaming:
		return "  [bidi stream]"
	case r.ClientStreaming:
		return "  [client stream]"
	case r.ServerStreaming:
		return "  [server stream]"
	default:
		return ""
	}
}

func (c Completer) Complete(input string) (string, []Suggestion) {
	if strings.TrimSpace(input) == "" {
		return "", nil
	}
	words := strings.Fields(input)
	current := ""
	if !strings.HasSuffix(input, " ") {
		current = words[len(words)-1]
		words = words[:len(words)-1]
	}

	var pool []Suggestion
	switch {
	case len(words) == 0:
		pool = append(append(pool, commandSuggestions...), c.rpcs...)
	case len(words) == 1 && isOneOf(words[0], "call"):
		pool = c.rpcs
	case len(words) == 1 && isOneOf(words[0], "desc", "describe"):
		pool = append([]Suggestion{
			{Text: "rpc", Description: "describe an rpc"},
			{Text: "message", Description: "describe a message"},
		}, c.rpcs...)
	case len(words) == 2 && isOneOf(words[0], "desc", "describe") && words[1] == "rpc":
		pool = c.rpcs
	case len(words) == 1 && isOneOf(words[0], "rpcs", "ls"):
		pool = c.services
	case len(words) == 1 && isOneOf(words[0], "header", "headers"):
		pool = []Suggestion{
			{Text: "set", Description: "set a header (replaces existing values)"},
			{Text: "add", Description: "add a header value"},
			{Text: "rm", Description: "remove a header"},
			{Text: "clear", Description: "remove all headers"},
		}
	case len(words) == 2 && isOneOf(words[0], "header", "headers") && isOneOf(words[1], "rm", "set", "add"):
		pool = c.headers
	}
	return current, filter(pool, current)
}

func filter(pool []Suggestion, word string) []Suggestion {
	if word == "" {
		return pool
	}
	w := strings.ToLower(word)
	var prefix, contains []Suggestion
	for _, s := range pool {
		switch match(s, w) {
		case 2:
			prefix = append(prefix, s)
		case 1:
			if len([]rune(w)) >= 2 {
				contains = append(contains, s)
			}
		}
	}
	return append(prefix, contains...)
}

func match(s Suggestion, w string) int {
	best := 0
	for _, candidate := range append([]string{s.Text}, s.aliases...) {
		c := strings.ToLower(candidate)
		switch {
		case strings.HasPrefix(c, w):
			return 2
		case strings.Contains(c, w):
			best = 1
		}
	}
	return best
}

func isOneOf(s string, options ...string) bool {
	s = strings.ToLower(s)
	for _, o := range options {
		if s == o {
			return true
		}
	}
	return false
}

func lastSegment(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}
