package clipboard

import (
	"github.com/atotto/clipboard"
	"github.com/muesli/termenv"
)

type Method string

const (
	MethodSystem Method = "clipboard"
	MethodOSC52  Method = "terminal (OSC 52)"
)

func Write(text string) (Method, error) {
	if err := clipboard.WriteAll(text); err == nil {
		return MethodSystem, nil
	}
	termenv.Copy(text)
	return MethodOSC52, nil
}
