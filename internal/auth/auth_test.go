package auth

import "testing"

func TestApiKeyAuth_Validate(t *testing.T) {
	tests := []struct {
		name    string
		auth    *ApiKeyAuth
		wantErr bool
	}{
		{
			name:    "valid default location",
			auth:    NewApiKeyAuth("secret"),
			wantErr: false,
		},
		{
			name:    "missing api key",
			auth:    NewApiKeyAuth(""),
			wantErr: true,
		},
		{
			name: "invalid location",
			auth: &ApiKeyAuth{
				AuthType: APIKeyType,
				APIKey:   "secret",
				VarName:  "X-Api-Key",
				Location: "body",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestApiKeyAuth_Type(t *testing.T) {
	a := NewApiKeyAuth("secret")
	if got := a.Type(); got != APIKeyType {
		t.Errorf("Type() = %s, want %s", got, APIKeyType)
	}
}

func TestBasicAuth_Validate(t *testing.T) {
	tests := []struct {
		name    string
		auth    *BasicAuth
		wantErr bool
	}{
		{
			name:    "valid",
			auth:    NewBasicAuth("user", "pass"),
			wantErr: false,
		},
		{
			name:    "missing username",
			auth:    &BasicAuth{AuthType: BasicType, Username: "", Password: "pass"},
			wantErr: true,
		},
		{
			name:    "missing password",
			auth:    &BasicAuth{AuthType: BasicType, Username: "user", Password: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBasicAuth_Type(t *testing.T) {
	b := NewBasicAuth("user", "pass")
	if got := b.Type(); got != BasicType {
		t.Errorf("Type() = %s, want %s", got, BasicType)
	}
}

func TestOAuth2Auth_Validate(t *testing.T) {
	scope := "read:all"
	tests := []struct {
		name    string
		auth    *OAuth2Auth
		wantErr bool
	}{
		{
			name:    "valid",
			auth:    NewOAuth2Auth("https://auth.example.com/token", "client", "secret", &scope),
			wantErr: false,
		},
		{
			name:    "missing token url",
			auth:    &OAuth2Auth{AuthType: OAuth2Type, TokenURL: "", ClientID: "client", ClientSecret: "secret"},
			wantErr: true,
		},
		{
			name:    "missing client id",
			auth:    &OAuth2Auth{AuthType: OAuth2Type, TokenURL: "https://auth.example.com/token", ClientID: "", ClientSecret: "secret"},
			wantErr: true,
		},
		{
			name:    "missing client secret",
			auth:    &OAuth2Auth{AuthType: OAuth2Type, TokenURL: "https://auth.example.com/token", ClientID: "client", ClientSecret: ""},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.auth.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOAuth2Auth_Type(t *testing.T) {
	o := NewOAuth2Auth("https://auth.example.com/token", "client", "secret", nil)
	if got := o.Type(); got != OAuth2Type {
		t.Errorf("Type() = %s, want %s", got, OAuth2Type)
	}
}
