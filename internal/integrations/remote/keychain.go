package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"

	"github.com/egoist/mygo"
)

const (
	credentialService = "com.aetherdots.remote"
	credentialKey     = "cloudflare-device"
)

var (
	deviceIDPattern    = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)
	deviceTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{40,128}$`)
)

type KeychainStore struct{ secrets mygo.SecretsModule }

func NewKeychainStore() *KeychainStore {
	return &KeychainStore{secrets: mygo.Secrets.Service(credentialService)}
}

func (s *KeychainStore) Load(ctx context.Context) (DeviceCredential, error) {
	if err := ctx.Err(); err != nil {
		return DeviceCredential{}, err
	}
	data, err := s.secrets.Get(credentialKey)
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return DeviceCredential{}, errors.New("not connected")
	}
	if err != nil {
		return DeviceCredential{}, err
	}
	defer clear(data)
	var credential DeviceCredential
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&credential); err != nil || !validCredential(credential) {
		return DeviceCredential{}, errors.New("the saved mobile connection is invalid")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return DeviceCredential{}, errors.New("the saved mobile connection is invalid")
	}
	return credential, nil
}

func (s *KeychainStore) Save(ctx context.Context, credential DeviceCredential) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validCredential(credential) {
		return errors.New("the relay returned an invalid desktop credential")
	}
	data, err := json.Marshal(credential)
	if err != nil {
		return errors.New("could not encode the desktop credential")
	}
	defer clear(data)
	return s.secrets.Set(credentialKey, data)
}

func (s *KeychainStore) Delete(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.secrets.Delete(credentialKey)
}

func validCredential(credential DeviceCredential) bool {
	return deviceIDPattern.MatchString(credential.ID) && deviceTokenPattern.MatchString(credential.Token)
}
