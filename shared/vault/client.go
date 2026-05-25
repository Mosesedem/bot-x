package vault

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/vault-client-go"
)

type Client struct {
	client *vault.Client
}

// New creates a new Vault client with the given address and auth token.
func New(addr, token string) (*Client, error) {
	client, err := vault.New(
		vault.WithAddress(addr),
	)
	if err != nil {
		return nil, err
	}

	if err := client.SetToken(token); err != nil {
		return nil, err
	}

	return &Client{client: client}, nil
}

// GetSecret reads a KV-v2 secret from Vault under the 'secret/' mount path.
func (c *Client) GetSecret(ctx context.Context, path string) (map[string]interface{}, error) {
	resp, err := c.client.Secrets.KvV2Read(ctx, path, vault.WithMountPath("secret"))
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.Data.Data == nil {
		return nil, errors.New("secret not found or data is empty")
	}
	return resp.Data.Data, nil
}

// GetSecretString reads a single string key from a KV-v2 secret.
func (c *Client) GetSecretString(ctx context.Context, path, key string) (string, error) {
	data, err := c.GetSecret(ctx, path)
	if err != nil {
		return "", err
	}
	val, ok := data[key]
	if !ok {
		return "", fmt.Errorf("key %s not found in secret %s", key, path)
	}
	strVal, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("key %s in secret %s is not a string", key, path)
	}
	return strVal, nil
}
