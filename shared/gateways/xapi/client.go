package xapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BaseURL        string
	BearerToken    string
	ConsumerKey    string
	ConsumerSecret string
	AccessToken    string
	AccessSecret   string
	HTTPClient     *http.Client
}

type Client struct {
	baseURL        string
	bearerToken    string
	consumerKey    string
	consumerSecret string
	accessToken    string
	accessSecret   string
	httpClient     *http.Client
}

func New(cfg Config) *Client {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.twitter.com"
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &Client{
		baseURL:        baseURL,
		bearerToken:    cfg.BearerToken,
		consumerKey:    cfg.ConsumerKey,
		consumerSecret: cfg.ConsumerSecret,
		accessToken:    cfg.AccessToken,
		accessSecret:   cfg.AccessSecret,
		httpClient:     httpClient,
	}
}

type DMResponse struct {
	Event struct {
		ID string `json:"id"`
	} `json:"event"`
}

type TweetResponse struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

type XUser struct {
	ID              string `json:"id"`
	Username        string `json:"username"`
	Name            string `json:"name"`
	Verified        bool   `json:"verified"`
	ProfileImageURL string `json:"profile_image_url"`
	CreatedAt       string `json:"created_at"`
	PublicMetrics   struct {
		FollowersCount int32 `json:"followers_count"`
		FollowingCount int32 `json:"following_count"`
		TweetCount     int32 `json:"tweet_count"`
	} `json:"public_metrics"`
}

type UserProfile struct {
	TwitterID       string
	Handle          string
	FollowerCount   int32
	FollowingCount  int32
	TweetCount      int32
	AccountAgeDays  int32
	IsVerified      bool
	HasProfileImage bool
}

type TweetUser struct {
	TwitterID string
	Handle    string
}

func (c *Client) ensureConfigured(requireBearer, requireOAuth bool) error {
	if requireBearer && c.bearerToken == "" {
		return fmt.Errorf("x bearer token not configured")
	}
	if requireOAuth && (c.consumerKey == "" || c.consumerSecret == "" || c.accessToken == "" || c.accessSecret == "") {
		return fmt.Errorf("x oauth1 credentials not configured")
	}
	return nil
}

func percentEncode(v string) string {
	encoded := url.QueryEscape(v)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func nonce() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func normalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, parsed.Path), nil
}

func buildOAuth1Header(method, rawURL string, query url.Values, consumerKey, consumerSecret, accessToken, accessSecret string) (string, error) {
	requestNonce, err := nonce()
	if err != nil {
		return "", err
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	params := url.Values{}
	params.Set("oauth_consumer_key", consumerKey)
	params.Set("oauth_nonce", requestNonce)
	params.Set("oauth_signature_method", "HMAC-SHA1")
	params.Set("oauth_timestamp", timestamp)
	params.Set("oauth_token", accessToken)
	params.Set("oauth_version", "1.0")

	for key, values := range query {
		for _, value := range values {
			params.Add(key, value)
		}
	}

	flat := make([]string, 0, len(params))
	for key, values := range params {
		for _, value := range values {
			flat = append(flat, percentEncode(key)+"="+percentEncode(value))
		}
	}
	sort.Strings(flat)
	normalizedParams := strings.Join(flat, "&")

	baseURL, err := normalizeBaseURL(rawURL)
	if err != nil {
		return "", err
	}

	baseString := strings.ToUpper(method) + "&" + percentEncode(baseURL) + "&" + percentEncode(normalizedParams)
	signingKey := percentEncode(consumerSecret) + "&" + percentEncode(accessSecret)
	mac := hmac.New(sha1.New, []byte(signingKey))
	if _, err := mac.Write([]byte(baseString)); err != nil {
		return "", err
	}
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	header := "OAuth " + strings.Join([]string{
		fmt.Sprintf(`oauth_consumer_key="%s"`, percentEncode(consumerKey)),
		fmt.Sprintf(`oauth_nonce="%s"`, percentEncode(requestNonce)),
		fmt.Sprintf(`oauth_signature="%s"`, percentEncode(signature)),
		`oauth_signature_method="HMAC-SHA1"`,
		fmt.Sprintf(`oauth_timestamp="%s"`, timestamp),
		fmt.Sprintf(`oauth_token="%s"`, percentEncode(accessToken)),
		`oauth_version="1.0"`,
	}, ", ")

	return header, nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, query url.Values, body any, authHeader string) (int, []byte, error) {
	urlString := c.baseURL + path
	if len(query) > 0 {
		urlString += "?" + query.Encode()
	}

	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlString, reader)
	if err != nil {
		return 0, nil, err
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, respBody, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body any, requireBearer bool, out any) (int, []byte, error) {
	if err := c.ensureConfigured(requireBearer, false); err != nil {
		return 0, nil, err
	}

	authHeader := ""
	if requireBearer {
		authHeader = "Bearer " + c.bearerToken
	}

	statusCode, respBody, err := c.doRequest(ctx, method, path, query, body, authHeader)
	if err != nil {
		return statusCode, respBody, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return statusCode, respBody, fmt.Errorf("x api request to %s failed with status %d: %s", path, statusCode, string(respBody))
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return statusCode, respBody, err
		}
	}
	return statusCode, respBody, nil
}

func (c *Client) doJSONWithAuth(ctx context.Context, method, path string, query url.Values, body any, authHeader string, out any) (int, []byte, error) {
	statusCode, respBody, err := c.doRequest(ctx, method, path, query, body, authHeader)
	if err != nil {
		return statusCode, respBody, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return statusCode, respBody, fmt.Errorf("x api request to %s failed with status %d: %s", path, statusCode, string(respBody))
	}
	if out != nil {
		if err := json.Unmarshal(respBody, out); err != nil {
			return statusCode, respBody, err
		}
	}
	return statusCode, respBody, nil
}

// SendDM sends a direct message via the v1.1 API using OAuth1 user context.
// DMs require OAuth1 — Bearer token (app-only) is rejected by the API.
func (c *Client) SendDM(ctx context.Context, userID, message string) (string, error) {
	if err := c.ensureConfigured(false, true); err != nil {
		return "", err
	}
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("user id and message are required")
	}

	path := "/1.1/direct_messages/events/new.json"
	payload := map[string]any{
		"event": map[string]any{
			"type": "message_create",
			"message_create": map[string]any{
				"target":       map[string]string{"recipient_id": userID},
				"message_data": map[string]string{"text": message},
			},
		},
	}
	authHeader, err := buildOAuth1Header("POST", c.baseURL+path, nil, c.consumerKey, c.consumerSecret, c.accessToken, c.accessSecret)
	if err != nil {
		return "", err
	}

	var resp DMResponse
	_, _, err = c.doJSONWithAuth(ctx, "POST", path, nil, payload, authHeader, &resp)
	if err != nil {
		return "", err
	}
	if resp.Event.ID == "" {
		return "", fmt.Errorf("x api returned an empty dm id")
	}
	return resp.Event.ID, nil
}

// ReplyToTweet posts a reply tweet using OAuth1 user context via the v2 API.
//
// IMPORTANT: POST /2/tweets requires OAuth 1.0a User Context (or OAuth 2.0 PKCE).
// Bearer token (app-only) is READ-ONLY and is rejected with HTTP 403 for writes.
// This was previously broken — fixed to use OAuth1 header signing.
func (c *Client) ReplyToTweet(ctx context.Context, tweetID, text string) (string, error) {
	if err := c.ensureConfigured(false, true); err != nil {
		return "", err
	}
	if strings.TrimSpace(tweetID) == "" || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("tweet id and text are required")
	}

	path := "/2/tweets"
	fullURL := c.baseURL + path
	authHeader, err := buildOAuth1Header("POST", fullURL, nil, c.consumerKey, c.consumerSecret, c.accessToken, c.accessSecret)
	if err != nil {
		return "", err
	}

	body := map[string]any{
		"text":  text,
		"reply": map[string]any{"in_reply_to_tweet_id": tweetID},
	}

	var resp TweetResponse
	_, _, err = c.doJSONWithAuth(ctx, "POST", path, nil, body, authHeader, &resp)
	if err != nil {
		return "", err
	}
	if resp.Data.ID == "" {
		return "", fmt.Errorf("x api returned an empty reply tweet id")
	}
	return resp.Data.ID, nil
}

func (c *Client) GetTweetReplies(ctx context.Context, tweetID, cursor string, maxResults int) ([]TweetUser, string, error) {
	if err := c.ensureConfigured(true, false); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(tweetID) == "" {
		return nil, "", fmt.Errorf("tweet id is required")
	}
	if maxResults <= 0 {
		maxResults = 20
	}
	if maxResults > 100 {
		maxResults = 100
	}

	query := url.Values{}
	query.Set("query", fmt.Sprintf("conversation_id:%s is:reply", tweetID))
	query.Set("max_results", strconv.Itoa(maxResults))
	query.Set("expansions", "author_id")
	query.Set("user.fields", "username")
	if strings.TrimSpace(cursor) != "" {
		query.Set("next_token", cursor)
	}

	var resp struct {
		Data []struct {
			AuthorID string `json:"author_id"`
		} `json:"data"`
		Includes struct {
			Users []struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"users"`
		} `json:"includes"`
		Meta struct {
			NextToken string `json:"next_token"`
		} `json:"meta"`
	}
	_, _, err := c.doJSON(ctx, "GET", "/2/tweets/search/recent", query, nil, true, &resp)
	if err != nil {
		return nil, "", err
	}

	usersByID := make(map[string]string, len(resp.Includes.Users))
	for _, user := range resp.Includes.Users {
		usersByID[user.ID] = user.Username
	}

	users := make([]TweetUser, 0, len(resp.Data))
	for _, tweet := range resp.Data {
		users = append(users, TweetUser{TwitterID: tweet.AuthorID, Handle: usersByID[tweet.AuthorID]})
	}
	return users, resp.Meta.NextToken, nil
}

func (c *Client) GetRetweeters(ctx context.Context, tweetID, cursor string, maxResults int) ([]TweetUser, string, error) {
	if err := c.ensureConfigured(true, false); err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(tweetID) == "" {
		return nil, "", fmt.Errorf("tweet id is required")
	}
	if maxResults <= 0 {
		maxResults = 100
	}
	if maxResults > 100 {
		maxResults = 100
	}

	query := url.Values{}
	query.Set("user.fields", "username")
	query.Set("max_results", strconv.Itoa(maxResults))
	if strings.TrimSpace(cursor) != "" {
		query.Set("pagination_token", cursor)
	}

	var resp struct {
		Data []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"data"`
		Meta struct {
			NextToken string `json:"next_token"`
		} `json:"meta"`
	}
	_, _, err := c.doJSON(ctx, "GET", "/2/tweets/"+url.PathEscape(tweetID)+"/retweeted_by", query, nil, true, &resp)
	if err != nil {
		return nil, "", err
	}

	users := make([]TweetUser, 0, len(resp.Data))
	for _, user := range resp.Data {
		users = append(users, TweetUser{TwitterID: user.ID, Handle: user.Username})
	}
	return users, resp.Meta.NextToken, nil
}

func (c *Client) CheckFollows(ctx context.Context, followerID, followeeID string) (bool, error) {
	if err := c.ensureConfigured(true, false); err != nil {
		return false, err
	}
	if strings.TrimSpace(followerID) == "" || strings.TrimSpace(followeeID) == "" {
		return false, fmt.Errorf("follower id and followee id are required")
	}

	query := url.Values{}
	query.Set("max_results", "1000")
	query.Set("user.fields", "id")
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Meta struct {
			NextToken string `json:"next_token"`
		} `json:"meta"`
	}
	for {
		if token := query.Get("pagination_token"); token != "" {
			query.Set("pagination_token", token)
		}
		statusCode, body, err := c.doRequest(ctx, "GET", "/2/users/"+url.PathEscape(followerID)+"/following", query, nil, "Bearer "+c.bearerToken)
		if err != nil {
			return false, err
		}
		if statusCode == http.StatusNotFound {
			return false, nil
		}
		if statusCode < 200 || statusCode >= 300 {
			return false, fmt.Errorf("x api request to follow list failed with status %d: %s", statusCode, string(body))
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return false, err
		}
		for _, user := range resp.Data {
			if user.ID == followeeID {
				return true, nil
			}
		}
		if resp.Meta.NextToken == "" {
			return false, nil
		}
		query.Set("pagination_token", resp.Meta.NextToken)
	}
}

// GetUserProfile fetches a user profile by Twitter ID.
// Uses Bearer token for read-only GET — this is correct.
func (c *Client) GetUserProfile(ctx context.Context, twitterID string) (*UserProfile, error) {
	if err := c.ensureConfigured(true, false); err != nil {
		return nil, err
	}
	if strings.TrimSpace(twitterID) == "" {
		return nil, fmt.Errorf("twitter id is required")
	}

	query := url.Values{}
	// public_metrics returns follower/following/tweet counts
	query.Set("user.fields", "created_at,profile_image_url,public_metrics,verified,username")

	var resp struct {
		Data XUser `json:"data"`
	}
	_, _, err := c.doJSON(ctx, "GET", "/2/users/"+url.PathEscape(twitterID), query, nil, true, &resp)
	if err != nil {
		return nil, err
	}

	profile := &UserProfile{
		TwitterID:       resp.Data.ID,
		Handle:          resp.Data.Username,
		FollowerCount:   resp.Data.PublicMetrics.FollowersCount,
		FollowingCount:  resp.Data.PublicMetrics.FollowingCount,
		TweetCount:      resp.Data.PublicMetrics.TweetCount,
		IsVerified:      resp.Data.Verified,
		HasProfileImage: resp.Data.ProfileImageURL != "",
	}
	if resp.Data.CreatedAt != "" {
		if createdAt, parseErr := time.Parse(time.RFC3339, resp.Data.CreatedAt); parseErr == nil {
			profile.AccountAgeDays = int32(time.Since(createdAt).Hours() / 24)
		}
	}
	return profile, nil
}
