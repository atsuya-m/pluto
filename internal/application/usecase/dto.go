package usecase

import "github.com/atsuya-m/pluto/internal/domain/schema"

type ServiceSummary struct {
	Name     string
	FullName string
	RPCs     []string
}

type RPCSummary struct {
	Name            string
	FullName        string
	Service         string
	RequestType     string
	ResponseType    string
	ClientStreaming bool
	ServerStreaming bool
}

type FieldDetail struct {
	Name        string
	JSONName    string
	Number      int
	Type        string
	Label       string
	Oneof       string
	HasPresence bool
	EnumValues  []string
}

type MessageDetail struct {
	Name     string
	FullName string
	Fields   []FieldDetail
}

type RPCDetail struct {
	RPCSummary
	Request  MessageDetail
	Response MessageDetail
}

type EnumDetail struct {
	Name     string
	FullName string
	Values   []schema.EnumValue
}

func toServiceSummary(s schema.Service) ServiceSummary {
	rpcs := s.RPCs()
	names := make([]string, 0, len(rpcs))
	for _, r := range rpcs {
		names = append(names, r.Name())
	}
	return ServiceSummary{Name: s.Name(), FullName: s.FullName(), RPCs: names}
}

func toRPCSummary(r schema.RPC) RPCSummary {
	return RPCSummary{
		Name:            r.Name(),
		FullName:        r.FullName(),
		Service:         r.ServiceName(),
		RequestType:     r.Input().FullName(),
		ResponseType:    r.Output().FullName(),
		ClientStreaming: r.ClientStreaming(),
		ServerStreaming: r.ServerStreaming(),
	}
}

func toMessageDetail(m schema.Message) MessageDetail {
	fields := m.Fields()
	details := make([]FieldDetail, 0, len(fields))
	for _, f := range fields {
		d := FieldDetail{
			Name:        f.Name(),
			JSONName:    f.JSONName(),
			Number:      f.Number(),
			Type:        f.TypeName(),
			Label:       f.Label(),
			Oneof:       f.OneofName(),
			HasPresence: f.HasPresence(),
		}
		if e, ok := f.Enum(); ok && !f.IsMap() {
			for _, v := range e.Values() {
				d.EnumValues = append(d.EnumValues, v.Name)
			}
		}
		details = append(details, d)
	}
	return MessageDetail{Name: m.Name(), FullName: m.FullName(), Fields: details}
}
