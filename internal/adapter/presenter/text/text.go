package text

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/atsuya-m/pluto/internal/adapter/presenter/errdetail"
	"github.com/atsuya-m/pluto/internal/application/usecase"
)

func Services(w io.Writer, out usecase.ListServicesOutput) error {
	for _, s := range out.Services {
		if _, err := fmt.Fprintln(w, s.FullName); err != nil {
			return err
		}
	}
	return nil
}

func RPCs(w io.Writer, out usecase.ListRPCsOutput) error {
	for _, r := range out.RPCs {
		if _, err := fmt.Fprintf(w, "%s\t%s\n", r.FullName, Signature(r)); err != nil {
			return err
		}
	}
	return nil
}

func Signature(r usecase.RPCSummary) string {
	req, res := r.RequestType, r.ResponseType
	if r.ClientStreaming {
		req = "stream " + req
	}
	if r.ServerStreaming {
		res = "stream " + res
	}
	return fmt.Sprintf("(%s) returns (%s)", req, res)
}

func RPC(w io.Writer, out usecase.DescribeRPCOutput) error {
	_, err := io.WriteString(w, RPCString(out.RPC))
	return err
}

func RPCString(r usecase.RPCDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "rpc %s %s\n\n", r.FullName, Signature(r.RPCSummary))
	b.WriteString(MessageString(r.Request))
	b.WriteString("\n")
	b.WriteString(MessageString(r.Response))
	return b.String()
}

func Message(w io.Writer, out usecase.DescribeMessageOutput) error {
	_, err := io.WriteString(w, MessageString(out.Message))
	return err
}

func MessageString(m usecase.MessageDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "message %s {\n", m.FullName)

	oneofs := map[string][]usecase.FieldDetail{}
	var order []string
	for _, f := range m.Fields {
		if f.Oneof == "" {
			fmt.Fprintf(&b, "  %s\n", fieldLine(f))
			continue
		}
		if _, ok := oneofs[f.Oneof]; !ok {
			order = append(order, f.Oneof)
		}
		oneofs[f.Oneof] = append(oneofs[f.Oneof], f)
	}
	for _, name := range order {
		fmt.Fprintf(&b, "  oneof %s {\n", name)
		for _, f := range oneofs[name] {
			fmt.Fprintf(&b, "    %s\n", fieldLine(f))
		}
		b.WriteString("  }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func fieldLine(f usecase.FieldDetail) string {
	line := fmt.Sprintf("%s %s = %d;", f.Type, f.Name, f.Number)
	if f.Label != "" {
		line = f.Label + " " + line
	}
	if len(f.EnumValues) > 0 {
		line += "  // " + strings.Join(f.EnumValues, " | ")
	}
	return line
}

func ProtoJSON(msg proto.Message) (string, error) {
	b, err := protojson.Marshal(msg)
	if err != nil {
		return "", err
	}
	if len(b) == 0 {
		return "{}", nil
	}
	var out bytes.Buffer
	if err := json.Indent(&out, b, "", "  "); err != nil {
		return "", err
	}
	return out.String(), nil
}

func Invoke(w io.Writer, out usecase.InvokeRPCOutput) error {
	s, err := ProtoJSON(out.Message)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, s)
	return err
}

func Headers(h map[string][]string) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s: %s\n", k, strings.Join(h[k], ", "))
	}
	return b.String()
}

func Error(w io.Writer, d errdetail.ErrorDetail) error {
	var b strings.Builder
	fmt.Fprintf(&b, "error: %s\n", d.Message)
	if d.Kind == errdetail.KindRPC {
		b.Reset()
		fmt.Fprintf(&b, "rpc error: code = %s, message = %s\n", d.Code, d.Message)
	}
	for _, c := range d.Candidates {
		fmt.Fprintf(&b, "  - %s\n", c)
	}
	for _, detail := range d.Details {
		fmt.Fprintf(&b, "  detail: %s\n", detail)
	}
	for _, k := range sortedKeys(d.Metadata) {
		fmt.Fprintf(&b, "  metadata: %s: %s\n", k, strings.Join(d.Metadata[k], ", "))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
