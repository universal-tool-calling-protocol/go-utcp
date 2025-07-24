package grpc

import (
	"encoding/json"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// GRPCProvider represents a gRPC service.
type GRPCProvider struct {
	BaseProvider
	Host        string `json:"host"`
	Port        int    `json:"port"`
	ServiceName string `json:"service_name"`
	MethodName  string `json:"method_name"`
	UseSSL      bool   `json:"use_ssl"`
	Auth        *Auth  `json:"auth,omitempty"`
}

func UnmarshalGRPCProvider(data []byte) (*GRPCProvider, error) {
	type Alias GRPCProvider
	aux := struct {
		*Alias
		Auth json.RawMessage `json:"auth"`
	}{Alias: (*Alias)(&GRPCProvider{})}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, err
	}
	gp := (*GRPCProvider)(aux.Alias)
	if len(aux.Auth) > 0 {
		auth, err := UnmarshalAuth(aux.Auth)
		if err != nil {
			return nil, err
		}
		gp.Auth = &auth
	}
	return gp, nil
}
