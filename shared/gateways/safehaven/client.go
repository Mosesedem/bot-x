package safehaven

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Config struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	PrivateKey   *rsa.PrivateKey // RS256 for JWT assertion
}

type tokenCache struct {
	mu        sync.RWMutex
	token     string
	expiresAt time.Time
}

type Client struct {
	config     Config
	httpClient *http.Client
	tokenCache tokenCache
}

func New(cfg Config) *Client {
	return &Client{
		config:     cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

func (c *Client) getToken(ctx context.Context) (string, error) {
	c.tokenCache.mu.RLock()
	if c.tokenCache.token != "" && c.tokenCache.expiresAt.After(time.Now().Add(30*time.Second)) {
		token := c.tokenCache.token
		c.tokenCache.mu.RUnlock()
		return token, nil
	}
	c.tokenCache.mu.RUnlock()

	c.tokenCache.mu.Lock()
	defer c.tokenCache.mu.Unlock()

	// Recheck after acquiring lock
	if c.tokenCache.token != "" && c.tokenCache.expiresAt.After(time.Now().Add(30*time.Second)) {
		return c.tokenCache.token, nil
	}

	token, expiresAt, err := c.authenticate(ctx)
	if err != nil {
		return "", err
	}

	c.tokenCache.token = token
	c.tokenCache.expiresAt = expiresAt
	return token, nil
}

func (c *Client) authenticate(ctx context.Context) (string, time.Time, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": c.config.ClientID,
		"sub": c.config.ClientID,
		"aud": fmt.Sprintf("%s/oauth2/token", c.config.BaseURL),
		"exp": now.Add(5 * time.Minute).Unix(),
		"iat": now.Unix(),
	})

	assertion, err := token.SignedString(c.config.PrivateKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign JWT assertion: %w", err)
	}

	form := fmt.Sprintf("grant_type=client_credentials&client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer&client_assertion=%s", assertion)
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/oauth2/token", c.config.BaseURL), strings.NewReader(form))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("oauth token request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var tr TokenResponse
	if err := json.Unmarshal(bodyBytes, &tr); err != nil {
		return "", time.Time{}, err
	}

	expiresAt := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return tr.AccessToken, expiresAt, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}, out interface{}) error {
	token, err := c.getToken(ctx)
	if err != nil {
		return err
	}

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(jsonData)
	}

	url := fmt.Sprintf("%s%s", c.config.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request to %s failed with status %d: %s", path, resp.StatusCode, string(respBody))
	}

	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return err
		}
	}
	return nil
}

type CreateVirtualAccountRequest struct {
	AccountName string `json:"accountName"`
	BankCode    string `json:"bankCode"`
	ExternalRef string `json:"externalRef"`
}

type VirtualAccount struct {
	AccountNumber string `json:"accountNumber"`
	BankName      string `json:"bankName"`
	AccountName   string `json:"accountName"`
	Reference     string `json:"reference"`
}

func (c *Client) CreateVirtualAccount(ctx context.Context, req CreateVirtualAccountRequest) (*VirtualAccount, error) {
	var va VirtualAccount
	err := c.doRequest(ctx, "POST", "/virtual-accounts", req, &va)
	if err != nil {
		return nil, err
	}
	return &va, nil
}

type NameEnquiryRequest struct {
	AccountNumber string `json:"accountNumber"`
	BankCode      string `json:"bankCode"`
}

type NameEnquiryResponse struct {
	AccountName   string `json:"accountName"`
	AccountNumber string `json:"accountNumber"`
	BankCode      string `json:"bankCode"`
}

func (c *Client) NameEnquiry(ctx context.Context, req NameEnquiryRequest) (*NameEnquiryResponse, error) {
	var ne NameEnquiryResponse
	err := c.doRequest(ctx, "POST", "/transfers/name-enquiry", req, &ne)
	if err != nil {
		return nil, err
	}
	return &ne, nil
}

type TransferRequest struct {
	NameEnquiryReference string `json:"nameEnquiryReference"`
	DebitAccountNumber   string `json:"debitAccountNumber"`
	BeneficiaryBank      string `json:"beneficiaryBank"`
	BeneficiaryAccount   string `json:"beneficiaryAccount"`
	BeneficiaryName      string `json:"beneficiaryName"`
	NarrationID          string `json:"narrationId"`
	Amount               int64  `json:"amount"` // cents/kobo
	IdempotencyKey       string `json:"idempotencyKey"`
}

type TransferResponse struct {
	Reference string `json:"reference"`
	Status    string `json: "status"`
	Amount    int64  `json:"amount"`
	SessionID string `json:"sessionId"`
}

func (c *Client) Transfer(ctx context.Context, req TransferRequest) (*TransferResponse, error) {
	var tr TransferResponse
	err := c.doRequest(ctx, "POST", "/transfers", req, &tr)
	if err != nil {
		return nil, err
	}
	return &tr, nil
}

type TransferStatusRequest struct {
	SessionID string `json:"sessionId"`
}

type TransferStatusResponse struct {
	Status    string `json:"status"`
	Reference string `json:"reference"`
}

func (c *Client) TransferStatus(ctx context.Context, req TransferStatusRequest) (*TransferStatusResponse, error) {
	var ts TransferStatusResponse
	err := c.doRequest(ctx, "POST", "/transfers/status", req, &ts)
	if err != nil {
		return nil, err
	}
	return &ts, nil
}

type InitiateKYCRequest struct {
	Type        string `json:"type"`
	Identifier  string `json:"identifier"`
	PhoneNumber string `json:"phoneNumber"`
	ExternalRef string `json:"externalRef"`
}

type InitiateKYCResponse struct {
	Reference string `json:"reference"`
	Status    string `json:"status"`
}

func (c *Client) InitiateKYC(ctx context.Context, req InitiateKYCRequest) (*InitiateKYCResponse, error) {
	var kyc InitiateKYCResponse
	err := c.doRequest(ctx, "POST", "/identity/initiate", req, &kyc)
	if err != nil {
		return nil, err
	}
	return &kyc, nil
}

type ValidateKYCRequest struct {
	Reference string `json:"reference"`
	OTP       string `json:"otp"`
}

type ValidateKYCResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func (c *Client) ValidateKYC(ctx context.Context, req ValidateKYCRequest) (*ValidateKYCResponse, error) {
	var val ValidateKYCResponse
	err := c.doRequest(ctx, "POST", "/identity/validate", req, &val)
	if err != nil {
		return nil, err
	}
	return &val, nil
}

func VerifyWebhookSignature(payload []byte, signature string, secret string) bool {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	expectedSignature := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}
