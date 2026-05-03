package bybit

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Credentials is the JSON shape stored encrypted in account_credentials.ciphertext
// for accounts of source_type='bybit'.
type Credentials struct {
	APIKey    string `json:"apiKey"`
	APISecret string `json:"apiSecret"`
}

func (c Credentials) Validate() error {
	if c.APIKey == "" {
		return errors.New("bybit credentials: empty api key")
	}
	if c.APISecret == "" {
		return errors.New("bybit credentials: empty api secret")
	}
	return nil
}

func MarshalCredentials(c Credentials) ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(c)
}

func UnmarshalCredentials(b []byte) (Credentials, error) {
	var c Credentials
	if err := json.Unmarshal(b, &c); err != nil {
		return Credentials{}, fmt.Errorf("unmarshal credentials: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Credentials{}, err
	}
	return c, nil
}
