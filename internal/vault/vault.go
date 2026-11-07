package vault

import (
	"errors"
	"fmt"

	"github.com/egoist/mygo"
)

func Set(profileID int64, kind, value string) error {
	if value == "" {
		return nil
	}
	return mygo.Secrets.Set(secretKey(profileID, kind), []byte(value))
}

func Get(profileID int64, kind string) (string, error) {
	b, err := mygo.Secrets.Get(secretKey(profileID, kind))
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func Delete(profileID int64, kind string) error {
	return mygo.Secrets.Delete(secretKey(profileID, kind))
}

func secretKey(profileID int64, kind string) string {
	return fmt.Sprintf("profile:%d:%s", profileID, kind)
}
