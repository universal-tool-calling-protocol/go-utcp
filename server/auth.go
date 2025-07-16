package core

import (
	"errors"
)

// AuthType represents the kind of authentication.
type AuthType string

const (
	// APIKeyType indicates API key–based authentication.
	APIKeyType AuthType = "api_key"

	// BasicType indicates basic username/password authentication.
	BasicType AuthType = "basic"

	// OAuth2Type indicates OAuth2 authentication.
	OAuth2Type AuthType = "oauth2"
)

// Auth is the interface all auth methods implement.
type Auth interface {
	// Type returns the authentication type.
	Type() AuthType

	// Validate checks that all required fields are set.
	Validate() error
}

// ApiKeyAuth holds config for API key–based authentication.
//
// The key can be provided directly or sourced from an environment variable.
type ApiKeyAuth struct {
	AuthType AuthType `json:"auth_type"`
	APIKey   string   `json:"api_key"`  // If it starts with '$', treated as injected variable.
	VarName  string   `json:"var_name"` // Header/query param/cookie name (default: "X-Api-Key").
	Location string   `json:"location"` // Where to include the key: header, query, or cookie.
}

// NewApiKeyAuth constructs an ApiKeyAuth with defaults.
func NewApiKeyAuth(apiKey string) *ApiKeyAuth {
	return &ApiKeyAuth{
		AuthType: APIKeyType,
		APIKey:   apiKey,
		VarName:  "X-Api-Key",
		Location: "header",
	}
}

// Type returns the auth type.
func (a *ApiKeyAuth) Type() AuthType {
	return a.AuthType
}

// Validate ensures required fields are present.
func (a *ApiKeyAuth) Validate() error {
	if a.APIKey == "" {
		return errors.New("api_key must be provided")
	}
	switch a.Location {
	case "header", "query", "cookie":
	default:
		return errors.New("location must be 'header', 'query', or 'cookie'")
	}
	return nil
}

// BasicAuth holds config for HTTP Basic authentication.
type BasicAuth struct {
	AuthType AuthType `json:"auth_type"`
	Username string   `json:"username"`
	Password string   `json:"password"`
}

// NewBasicAuth constructs a BasicAuth.
func NewBasicAuth(username, password string) *BasicAuth {
	return &BasicAuth{
		AuthType: BasicType,
		Username: username,
		Password: password,
	}
}

// Type returns the auth type.
func (b *BasicAuth) Type() AuthType {
	return b.AuthType
}

// Validate ensures required fields are present.
func (b *BasicAuth) Validate() error {
	if b.Username == "" {
		return errors.New("username must be provided")
	}
	if b.Password == "" {
		return errors.New("password must be provided")
	}
	return nil
}

// OAuth2Auth holds config for OAuth2 authentication.
type OAuth2Auth struct {
	AuthType     AuthType `json:"auth_type"`
	TokenURL     string   `json:"token_url"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scope        *string  `json:"scope,omitempty"` // Optional OAuth2 scope.
}

// NewOAuth2Auth constructs an OAuth2Auth.
func NewOAuth2Auth(tokenURL, clientID, clientSecret string, scope *string) *OAuth2Auth {
	return &OAuth2Auth{
		AuthType:     OAuth2Type,
		TokenURL:     tokenURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scope:        scope,
	}
}

// Type returns the auth type.
func (o *OAuth2Auth) Type() AuthType {
	return o.AuthType
}

// Validate ensures required fields are present.
func (o *OAuth2Auth) Validate() error {
	if o.TokenURL == "" {
		return errors.New("token_url must be provided")
	}
	if o.ClientID == "" {
		return errors.New("client_id must be provided")
	}
	if o.ClientSecret == "" {
		return errors.New("client_secret must be provided")
	}
	return nil
}
