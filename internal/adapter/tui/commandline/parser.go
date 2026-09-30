package commandline

import (
	"errors"
	"strings"
)

var ErrUnterminatedQuote = errors.New("unterminated quote")

type Command struct {
	Name string
	Args []string
}

func ParseCommand(input string) (Command, error) {
	var (
		tokens  []string
		current strings.Builder
		quote   rune
		inToken bool
	)
	for _, r := range input {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
		case r == '"' || r == '\'':
			quote = r
			inToken = true
		case r == ' ' || r == '\t':
			if inToken {
				tokens = append(tokens, current.String())
				current.Reset()
				inToken = false
			}
		default:
			current.WriteRune(r)
			inToken = true
		}
	}
	if quote != 0 {
		return Command{}, ErrUnterminatedQuote
	}
	if inToken {
		tokens = append(tokens, current.String())
	}
	if len(tokens) == 0 {
		return Command{}, nil
	}
	return Command{Name: tokens[0], Args: tokens[1:]}, nil
}
