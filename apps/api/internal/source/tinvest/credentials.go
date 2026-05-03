package tinvest

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Credentials is the JSON shape stored encrypted in account_credentials.ciphertext
// for accounts of source_type='tinvest'.
type Credentials struct {
	Token            string `json:"token"`
	TInvestAccountID string `json:"tinvestAccountId"`
}

func (c Credentials) Validate() error {
	if c.Token == "" {
		return errors.New("tinvest credentials: empty token")
	}
	if c.TInvestAccountID == "" {
		return errors.New("tinvest credentials: empty tinvest account id")
	}
	return nil
}

// MarshalCredentials returns JSON bytes for encryption.
func MarshalCredentials(c Credentials) ([]byte, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(c)
}

// UnmarshalCredentials parses decrypted bytes back to Credentials.
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
