package json

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/atsuya-m/pluto/internal/adapter/presenter/errdetail"
	"github.com/atsuya-m/pluto/internal/application/usecase"
)

type serviceJSON struct {
	Name     string   `json:"name"`
	FullName string   `json:"full_name"`
	RPCs     []string `json:"rpcs"`
}

type rpcJSON struct {
	Name            string `json:"name"`
	FullName        string `json:"full_name"`
	Service         string `json:"service"`
	RequestType     string `json:"request_type"`
	ResponseType    string `json:"response_type"`
	ClientStreaming bool   `json:"client_streaming"`
	ServerStreaming bool   `json:"server_streaming"`
}

type fieldJSON struct {
	Name        string   `json:"name"`
	JSONName    string   `json:"json_name"`
	Number      int      `json:"number"`
	Type        string   `json:"type"`
	Label       string   `json:"label,omitempty"`
	Oneof       string   `json:"oneof,omitempty"`
	HasPresence bool     `json:"has_presence"`
	EnumValues  []string `json:"enum_values,omitempty"`
}

type messageJSON struct {
	Name     string      `json:"name"`
	FullName string      `json:"full_name"`
	Fields   []fieldJSON `json:"fields"`
}

type rpcDetailJSON struct {
	rpcJSON
	Request  messageJSON `json:"request"`
	Response messageJSON `json:"response"`
}

type invokeJSON struct {
	Message  json.RawMessage `json:"message"`
	Headers  http.Header     `json:"headers"`
	Trailers http.Header     `json:"trailers"`
}

type streamMessageJSON struct {
	Message json.RawMessage `json:"message"`
}

type streamSummaryJSON struct {
	Summary struct {
		Count    int         `json:"count"`
		Headers  http.Header `json:"headers"`
		Trailers http.Header `json:"trailers"`
	} `json:"summary"`
}

type errorJSON struct {
	Error errdetail.ErrorDetail `json:"error"`
}

func encode(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func Services(w io.Writer, out usecase.ListServicesOutput) error {
	items := make([]serviceJSON, 0, len(out.Services))
	for _, s := range out.Services {
		items = append(items, serviceJSON{Name: s.Name, FullName: s.FullName, RPCs: s.RPCs})
	}
	return encode(w, map[string]any{"services": items})
}

func RPCs(w io.Writer, out usecase.ListRPCsOutput) error {
	items := make([]rpcJSON, 0, len(out.RPCs))
	for _, r := range out.RPCs {
		items = append(items, toRPC(r))
	}
	return encode(w, map[string]any{"rpcs": items})
}

func RPC(w io.Writer, out usecase.DescribeRPCOutput) error {
	return encode(w, map[string]any{"rpc": rpcDetailJSON{
		rpcJSON:  toRPC(out.RPC.RPCSummary),
		Request:  toMessage(out.RPC.Request),
		Response: toMessage(out.RPC.Response),
	}})
}

func Message(w io.Writer, out usecase.DescribeMessageOutput) error {
	return encode(w, map[string]any{"message": toMessage(out.Message)})
}

func Invoke(w io.Writer, out usecase.InvokeRPCOutput) error {
	b, err := compactMessage(out.Message)
	if err != nil {
		return err
	}
	return encode(w, invokeJSON{
		Message:  b,
		Headers:  nonNil(out.Headers),
		Trailers: nonNil(out.Trailers),
	})
}

func StreamMessage(w io.Writer, msg proto.Message) error {
	b, err := compactMessage(msg)
	if err != nil {
		return err
	}
	return encodeLine(w, streamMessageJSON{Message: b})
}

func StreamSummary(w io.Writer, out usecase.InvokeServerStreamOutput) error {
	var v streamSummaryJSON
	v.Summary.Count = out.Count
	v.Summary.Headers = nonNil(out.Headers)
	v.Summary.Trailers = nonNil(out.Trailers)
	return encodeLine(w, v)
}

func ClientStreamSummary(w io.Writer, sent, received int, headers, trailers http.Header) error {
	return encodeLine(w, map[string]any{"summary": map[string]any{
		"sent":     sent,
		"received": received,
		"headers":  nonNil(headers),
		"trailers": nonNil(trailers),
	}})
}

func ErrorLine(w io.Writer, d errdetail.ErrorDetail) error {
	return encodeLine(w, errorJSON{Error: d})
}

func encodeLine(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func compactMessage(msg proto.Message) (json.RawMessage, error) {
	b, err := protojson.Marshal(msg)
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return json.RawMessage("{}"), nil
	}
	var out bytes.Buffer
	if err := json.Compact(&out, b); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func Error(w io.Writer, d errdetail.ErrorDetail) error {
	return encode(w, errorJSON{Error: d})
}

func nonNil(h http.Header) http.Header {
	if h == nil {
		return http.Header{}
	}
	return h
}

func toRPC(r usecase.RPCSummary) rpcJSON {
	return rpcJSON{
		Name:            r.Name,
		FullName:        r.FullName,
		Service:         r.Service,
		RequestType:     r.RequestType,
		ResponseType:    r.ResponseType,
		ClientStreaming: r.ClientStreaming,
		ServerStreaming: r.ServerStreaming,
	}
}

func toMessage(m usecase.MessageDetail) messageJSON {
	fields := make([]fieldJSON, 0, len(m.Fields))
	for _, f := range m.Fields {
		fields = append(fields, fieldJSON{
			Name:        f.Name,
			JSONName:    f.JSONName,
			Number:      f.Number,
			Type:        f.Type,
			Label:       f.Label,
			Oneof:       f.Oneof,
			HasPresence: f.HasPresence,
			EnumValues:  f.EnumValues,
		})
	}
	return messageJSON{Name: m.Name, FullName: m.FullName, Fields: fields}
}
