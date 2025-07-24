package graphql

import (
	"encoding/json"

	. "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
)

// GraphQLProvider represents a GraphQL endpoint.
type GraphQLProvider struct {
	BaseProvider
	URL           string            `json:"url"`
	OperationType string            `json:"operation_type"` // query, mutation, subscription
	OperationName *string           `json:"operation_name,omitempty"`
	Auth          *Auth             `json:"auth,omitempty"`
	Headers       map[string]string `json:"headers,omitempty"`
	HeaderFields  []string          `json:"header_fields,omitempty"`
}

func UnmarshalGraphQLProvider(data []byte) (*GraphQLProvider, error) {
	type Alias GraphQLProvider
	aux := struct {
		*Alias
		Auth json.RawMessage `json:"auth"`
	}{Alias: (*Alias)(&GraphQLProvider{})}
	if err := json.Unmarshal(data, &aux); err != nil {
		return nil, err
	}
	gp := (*GraphQLProvider)(aux.Alias)
	if len(aux.Auth) > 0 {
		auth, err := UnmarshalAuth(aux.Auth)
		if err != nil {
			return nil, err
		}
		gp.Auth = &auth
	}
	return gp, nil
}
