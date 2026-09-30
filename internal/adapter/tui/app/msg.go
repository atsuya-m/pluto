package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/protobuf/proto"

	"github.com/atsuya-m/pluto/internal/application/usecase"
)

type rpcsLoadedMsg struct {
	RPCs []usecase.RPCSummary
}

type rpcsLoadFailedMsg struct {
	Err error
}

type preparedMsg struct {
	Output usecase.PrepareRequestOutput
}

type invokedMsg struct {
	Output   usecase.InvokeRPCOutput
	Duration time.Duration
}

type invokeFailedMsg struct {
	Err      error
	Duration time.Duration
}

type printMsg struct {
	Text string
}

type errMsg struct {
	Err error
}

type streamStartedMsg struct {
	Events <-chan tea.Msg
}

type streamMessageMsg struct {
	Message proto.Message
}

type streamDoneMsg struct {
	Output   usecase.InvokeServerStreamOutput
	Err      error
	Duration time.Duration
}

type streamOpenedMsg struct {
	Session *usecase.StreamSession
	Pending []proto.Message
	Finish  bool
}

type streamSentMsg struct {
	Message proto.Message
}

type streamSendFailedMsg struct {
	Err error
}

type savedListedMsg struct {
	Saved []usecase.SavedRequestSummary
}

type requestSavedMsg struct {
	Name string
	RPC  string
}

type requestDeletedMsg struct {
	Name string
}
