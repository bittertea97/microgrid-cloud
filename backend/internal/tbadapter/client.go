package tbadapter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Client is a minimal ThingsBoard REST client.
type Client struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewClient constructs a TB client.
func NewClient(baseURL, token string) (*Client, error) {
	if baseURL == "" {
		return nil, errors.New("tbadapter: empty base url")
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// Tenant represents a TB tenant.
type Tenant struct {
	ID   string
	Name string
}

// Asset represents a TB asset.
type Asset struct {
	ID   string
	Name string
}

// Device represents a TB device.
type Device struct {
	ID          string
	Name        string
	Credentials string
}

// RPCResponse represents a minimal RPC response.
type RPCResponse struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

// EnsureTenant finds or creates a tenant by name.
func (c *Client) EnsureTenant(ctx context.Context, tenantName string) (Tenant, error) {
	if tenantName == "" {
		return Tenant{}, errors.New("tbadapter: empty tenant name")
	}
	// If authenticated as tenant admin, reuse the current tenant from /api/auth/user.
	if c.token != "" {
		if user, err := c.currentUser(ctx); err == nil {
			if strings.ToUpper(user.Authority) != "SYS_ADMIN" && user.TenantID.ID != "" {
				return Tenant{ID: user.TenantID.ID, Name: tenantName}, nil
			}
		}
	}
	existing, ok, err := c.findTenant(ctx, tenantName)
	if err != nil {
		return Tenant{}, err
	}
	if ok {
		return existing, nil
	}
	body := map[string]any{"title": tenantName}
	var resp tenantResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/tenant", body, &resp); err != nil {
		return Tenant{}, err
	}
	return Tenant{ID: resp.ID.ID, Name: resp.Title}, nil
}

type authUser struct {
	Authority string  `json:"authority"`
	TenantID  entityID `json:"tenantId"`
}

func (c *Client) currentUser(ctx context.Context) (authUser, error) {
	var resp authUser
	if err := c.doJSON(ctx, http.MethodGet, "/api/auth/user", nil, &resp); err != nil {
		return authUser{}, err
	}
	return resp, nil
}

// EnsureAsset finds or creates an asset by external station id.
func (c *Client) EnsureAsset(ctx context.Context, tenantID, stationID, name, assetType string) (Asset, error) {
	if stationID == "" {
		return Asset{}, errors.New("tbadapter: empty station id")
	}
	if assetType == "" {
		assetType = "station"
	}
	foundID, ok, err := c.findEntityByAttribute(ctx, "ASSET", tenantID, "station_id", stationID)
	if err != nil {
		return Asset{}, err
	}
	if ok {
		return Asset{ID: foundID, Name: name}, nil
	}

	body := map[string]any{
		"name": name,
		"type": assetType,
		"tenantId": map[string]any{
			"entityType": "TENANT",
			"id":         tenantID,
		},
	}
	var resp assetDeviceResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/asset", body, &resp); err != nil {
		return Asset{}, err
	}
	return Asset{ID: resp.ID.ID, Name: resp.Name}, nil
}

// EnsureDevice finds or creates a device by external device id.
func (c *Client) EnsureDevice(ctx context.Context, tenantID, deviceID, name, deviceType, profile, credentials string) (Device, error) {
	if deviceID == "" {
		return Device{}, errors.New("tbadapter: empty device id")
	}
	if deviceType == "" {
		deviceType = "device"
	}
	foundID, ok, err := c.findEntityByAttribute(ctx, "DEVICE", tenantID, "device_id", deviceID)
	if err != nil {
		return Device{}, err
	}
	if ok {
		return Device{ID: foundID, Name: name, Credentials: credentials}, nil
	}

	body := map[string]any{
		"name": name,
		"type": deviceType,
		"tenantId": map[string]any{
			"entityType": "TENANT",
			"id":         tenantID,
		},
	}
	// NOTE: TB 4.x expects deviceProfileId; skip profile string to keep compatibility.

	var resp assetDeviceResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/device", body, &resp); err != nil {
		return Device{}, err
	}
	return Device{ID: resp.ID.ID, Name: resp.Name, Credentials: credentials}, nil
}

// CreateRelation creates an asset->device relation.
func (c *Client) CreateRelation(ctx context.Context, assetID, deviceID string) error {
	if assetID == "" || deviceID == "" {
		return errors.New("tbadapter: empty relation id")
	}
	body := map[string]any{
		"from":      map[string]any{"id": assetID, "entityType": "ASSET"},
		"to":        map[string]any{"id": deviceID, "entityType": "DEVICE"},
		"type":      "Contains",
		"typeGroup": "COMMON",
	}
	return c.doJSON(ctx, http.MethodPost, "/api/relation", body, nil)
}

// SetAttributes sets server-scope attributes.
func (c *Client) SetAttributes(ctx context.Context, entityType, entityID string, attrs map[string]any) error {
	if entityType == "" || entityID == "" {
		return errors.New("tbadapter: empty entity")
	}
	path := fmt.Sprintf("/api/plugins/telemetry/%s/%s/attributes/SERVER_SCOPE", strings.ToUpper(entityType), entityID)
	return c.doJSON(ctx, http.MethodPost, path, attrs, nil)
}

// SendRPC sends an RPC command to a device.
func (c *Client) SendRPC(ctx context.Context, deviceID, commandType string, payload json.RawMessage) (RPCResponse, error) {
	if deviceID == "" || commandType == "" {
		return RPCResponse{}, errors.New("tbadapter: invalid rpc args")
	}
	body := map[string]any{
		"method": commandType,
		"params": json.RawMessage(payload),
	}
	var resp RPCResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/rpc/"+deviceID, body, &resp); err != nil {
		return RPCResponse{}, err
	}
	return resp, nil
}

func (c *Client) findTenant(ctx context.Context, tenantName string) (Tenant, bool, error) {
	if tenantName == "" {
		return Tenant{}, false, nil
	}
	// Some TB deployments (e.g. 4.0) do not support GET /api/tenant?tenantTitle=...
	// Fall back to listing tenants and matching by title.
	for page := 0; page < 50; page++ {
		path := fmt.Sprintf("/api/tenants?page=%d&pageSize=100", page)
		var resp tenantsPage
		if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
			if errors.Is(err, errNotFound) {
				return Tenant{}, false, nil
			}
			return Tenant{}, false, err
		}
		for _, item := range resp.Data {
			if item.Title == tenantName {
				return Tenant{ID: item.ID.ID, Name: item.Title}, true, nil
			}
		}
		if !resp.HasNext {
			break
		}
	}
	return Tenant{}, false, nil
}

func (c *Client) findEntityByAttribute(ctx context.Context, entityType, tenantID, key, value string) (string, bool, error) {
	if entityType == "" || key == "" || value == "" {
		return "", false, errors.New("tbadapter: invalid attribute query")
	}
	// TB 4.x expects entity queries with entityFilter/keyFilters.
	body := map[string]any{
		"entityFilter": map[string]any{
			"type":            "entityType",
			"resolveMultiple": true,
			"entityType":      entityType,
		},
		"keyFilters": []any{
			map[string]any{
				"key": map[string]any{
					"type": "ATTRIBUTE",
					"key":  key,
				},
				"valueType": "STRING",
				"predicate": map[string]any{
					"type":      "STRING",
					"operation": "EQUAL",
					"value": map[string]any{
						"defaultValue": value,
					},
				},
			},
		},
		"pageLink": map[string]any{
			"pageSize": 1,
			"page":     0,
		},
	}
	var resp entityQueryResponse
	err := c.doJSON(ctx, http.MethodPost, "/api/entitiesQuery/find", body, &resp)
	if err != nil {
		if errors.Is(err, errNotFound) {
			return "", false, nil
		}
		return "", false, err
	}
	if len(resp.Data) == 0 {
		return "", false, nil
	}
	return resp.Data[0].EntityID.ID, true, nil
}

type tenantResponse struct {
	ID    entityID `json:"id"`
	Title string   `json:"title"`
}

type tenantsPage struct {
	Data    []tenantResponse `json:"data"`
	HasNext bool             `json:"hasNext"`
}

type assetDeviceResponse struct {
	ID   entityID `json:"id"`
	Name string   `json:"name"`
}

type entityID struct {
	ID string `json:"id"`
}

type entityQueryResponse struct {
	Data []entityQueryItem `json:"data"`
}

type entityQueryItem struct {
	EntityID entityIdentifier `json:"entityId"`
}

type entityIdentifier struct {
	ID string `json:"id"`
}

var errNotFound = errors.New("tbadapter: not found")

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var reqBody *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reqBody = bytes.NewReader(payload)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("X-Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("tbadapter: http %d", resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
