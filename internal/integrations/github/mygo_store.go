package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/egoist/mygo"
)

const (
	CredentialService = "com.aetherdots.github"
	CredentialKey     = "github-user-credentials"
)

type MyGoCredentialStore struct{ secrets mygo.SecretsModule }

func NewMyGoCredentialStore() *MyGoCredentialStore {
	return &MyGoCredentialStore{secrets: mygo.Secrets.Service(CredentialService)}
}

func (s *MyGoCredentialStore) Load(ctx context.Context) (Credentials, error) {
	if err := ctx.Err(); err != nil {
		return Credentials{}, err
	}
	data, err := s.secrets.Get(CredentialKey)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return Credentials{}, ErrNotConnected
	}
	if err != nil {
		return Credentials{}, err
	}
	defer clear(data)
	var credentials Credentials
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&credentials); err != nil || !validOAuthToken(credentials.AccessToken) {
		return Credentials{}, ErrCredentialsCorrupt
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return Credentials{}, ErrCredentialsCorrupt
	}
	return credentials, nil
}

func (s *MyGoCredentialStore) Save(ctx context.Context, credentials Credentials) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validOAuthToken(credentials.AccessToken) || credentials.RefreshToken != "" && !validOAuthToken(credentials.RefreshToken) {
		return ErrCredentialsCorrupt
	}
	data, err := json.Marshal(credentials)
	if err != nil {
		return errors.New("could not encode GitHub credentials")
	}
	defer clear(data)
	if err := s.secrets.Set(CredentialKey, data); err != nil {
		return err
	}
	return nil
}

func (s *MyGoCredentialStore) Delete(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.secrets.Delete(CredentialKey)
}
