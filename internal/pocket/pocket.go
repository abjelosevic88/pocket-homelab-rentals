// Package pocket talks to sibling Pocket apps (Notes, Travel, Places,
// Bookshelf, Kitchen, Watch, Photos, Voice): the things a post can start
// from. Same bearer-token JSON API everywhere.
package pocket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a bearer-token JSON client for one Pocket app.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New creates a client.
func New(baseURL, token string, timeout time.Duration) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Token: token, HTTP: &http.Client{Timeout: timeout}}
}

// Configured reports whether URL and token are set.
func (c *Client) Configured() bool { return c != nil && c.BaseURL != "" && c.Token != "" }

// ErrNotFound is a 404 from the sibling app.
var ErrNotFound = errors.New("not found")

func (c *Client) do(ctx context.Context, method, path string, q url.Values, body any, v any) error {
	if !c.Configured() {
		return errors.New("not configured")
	}
	u := c.BaseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errors.New("token rejected (401)")
	}
	if res.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2000))
		_ = json.Unmarshal(b, &e)
		if e.Error != "" {
			return fmt.Errorf("HTTP %d: %s", res.StatusCode, e.Error)
		}
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if v == nil || res.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(res.Body).Decode(v)
}

// Health is the connection test for any Pocket app: /healthz plus a token check.
func (c *Client) Health(ctx context.Context) (string, error) {
	var out struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if !c.Configured() {
		return "", errors.New("not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/healthz", nil)
	if err != nil {
		return "", err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	_ = json.NewDecoder(res.Body).Decode(&out)
	var me map[string]any
	if err := c.do(ctx, http.MethodGet, "/api/v1/auth/me", nil, nil, &me); err != nil {
		return out.Version, err
	}
	return out.Version, nil
}

// Do is a raw JSON call (for a connection test).
func (c *Client) Do(ctx context.Context, method, path string, q url.Values, body, v any) error {
	return c.do(ctx, method, path, q, body, v)
}

// list decodes a list that may be bare or wrapped in {key:[…]} / {results:[…]}.
func list[T any](raw json.RawMessage, keys ...string) ([]T, error) {
	var out []T
	if err := json.Unmarshal(raw, &out); err == nil {
		return out, nil
	}
	var wrap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	for _, k := range append(keys, "results", "items") {
		if b, ok := wrap[k]; ok {
			if err := json.Unmarshal(b, &out); err != nil {
				return nil, err
			}
			return out, nil
		}
	}
	return []T{}, nil
}

// Bytes fetches a file (a photo) with the token, up to max bytes, and its type.
func (c *Client) Bytes(ctx context.Context, path string, max int64) ([]byte, string, error) {
	if !c.Configured() {
		return nil, "", errors.New("not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, "", ErrNotFound
	}
	if res.StatusCode >= 300 {
		return nil, "", fmt.Errorf("HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, max+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(b)) > max {
		return nil, "", errors.New("the file is too big")
	}
	return b, res.Header.Get("Content-Type"), nil
}

// ---------------- Pocket Money ----------------

// Account is a Money account the person can see.
type Account struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Currency string `json:"currency"`
	Everyday bool   `json:"everyday"`
	Archived bool   `json:"archived"`
	Role     string `json:"role"` // owner | editor | viewer
}

// Accounts lists the accounts the person can book on.
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/accounts", nil, nil, &raw); err != nil {
		return nil, err
	}
	all, err := list[Account](raw, "accounts")
	if err != nil {
		return nil, err
	}
	out := []Account{}
	for _, a := range all {
		if !a.Archived && a.Role != "viewer" {
			out = append(out, a)
		}
	}
	return out, nil
}

// Category is a Money category.
type Category struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Kind     string `json:"kind"` // expense | income | both
	Archived bool   `json:"archived"`
}

// Categories lists the categories usable on an account, of one kind ("" = all).
func (c *Client) Categories(ctx context.Context, accountID int64, kind string) ([]Category, error) {
	var raw json.RawMessage
	path := "/api/v1/categories"
	if accountID > 0 {
		path = fmt.Sprintf("/api/v1/accounts/%d/categories", accountID)
	}
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &raw); err != nil {
		return nil, err
	}
	all, err := list[Category](raw, "categories")
	if err != nil {
		return nil, err
	}
	out := []Category{}
	for _, ct := range all {
		if ct.Archived {
			continue
		}
		if kind != "" && ct.Kind != kind && ct.Kind != "both" && ct.Kind != "" {
			continue
		}
		out = append(out, ct)
	}
	return out, nil
}

// Tx is what Rentals books: rent in (income) or a cost (expense), in its
// own currency (Money converts to the account's).
type Tx struct {
	Kind        string // income | expense
	AccountID   int64
	Date        string
	Amount      int64 // minor units of Currency
	Currency    string
	Description string
	Payee       string
	CategoryID  int64
	Tags        []string
	Notes       string
	ExternalID  string
}

// CreateTx books a transaction and returns its id. The same ExternalID
// twice gives back the same transaction (Money keeps it idempotent).
func (c *Client) CreateTx(ctx context.Context, t Tx) (int64, error) {
	tags := t.Tags
	if tags == nil {
		tags = []string{}
	}
	body := map[string]any{"kind": t.Kind, "account_id": t.AccountID, "date": t.Date, "amount": t.Amount, "currency": t.Currency,
		"description": t.Description, "payee": t.Payee, "tags": tags, "notes": t.Notes, "source": "rentals", "external_id": t.ExternalID}
	if t.CategoryID > 0 {
		body["category_id"] = t.CategoryID
	}
	var out struct {
		ID int64 `json:"id"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v1/transactions", nil, body, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// UpdateTx changes what Rentals owns on the transaction: date, amount,
// description and payee. Account, category and tags stay as the person left them.
func (c *Client) UpdateTx(ctx context.Context, id int64, t Tx) error {
	body := map[string]any{"date": t.Date, "amount": t.Amount, "currency": t.Currency, "description": t.Description, "payee": t.Payee}
	return c.do(ctx, http.MethodPut, fmt.Sprintf("/api/v1/transactions/%d", id), nil, body, nil)
}

// DeleteTx removes a transaction; one already gone counts as deleted.
func (c *Client) DeleteTx(ctx context.Context, id int64) error {
	err := c.do(ctx, http.MethodDelete, fmt.Sprintf("/api/v1/transactions/%d", id), nil, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// MoneyTx is a transaction as Money lists it.
type MoneyTx struct {
	ID          int64    `json:"id"`
	Kind        string   `json:"kind"`
	Date        string   `json:"date"`
	AccountID   int64    `json:"account_id"`
	AccountName string   `json:"account_name"`
	Amount      int64    `json:"amount"`
	Currency    string   `json:"currency"`
	Description string   `json:"description"`
	Payee       string   `json:"payee"`
	Notes       string   `json:"notes"`
	Source      string   `json:"source"`
	Tags        []string `json:"tags"`
}

// Transactions lists transactions of a kind between two days (newest first).
func (c *Client) Transactions(ctx context.Context, kind, from, to string, limit int) ([]MoneyTx, error) {
	q := url.Values{"from": {from}, "to": {to}, "limit": {fmt.Sprint(limit)}}
	if kind != "" {
		q.Set("kind", kind)
	}
	var raw json.RawMessage
	if err := c.do(ctx, http.MethodGet, "/api/v1/transactions", q, nil, &raw); err != nil {
		return nil, err
	}
	out, err := list[MoneyTx](raw, "items", "transactions")
	if err != nil {
		return nil, err
	}
	// Money lists the account currency's amount; a foreign one is on the side
	return out, nil
}
