// Package client is a thin HTTP client for the Orkestra control plane API,
// used by the CLI so that commands act on the running server's state.
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/orkestra/internal/api"
	"github.com/orkestra/internal/propagation"
	"github.com/orkestra/internal/registry"
)

// requestTimeout stays above the server's 15s write timeout so the server,
// not the client, decides when a slow request has failed.
const requestTimeout = 20 * time.Second

// Client talks to an Orkestra control plane over its REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a Client for the control plane at baseURL,
// e.g. "http://localhost:8080".
func New(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

// RegisterCluster registers a cluster with the control plane. The kubeconfig
// path is read by the server, so it must be valid on the server's filesystem.
func (c *Client) RegisterCluster(name, kubeconfigPath string) (*registry.ClusterInfo, error) {
	var cluster registry.ClusterInfo
	body := api.RegisterClusterRequest{Name: name, KubeconfigPath: kubeconfigPath}
	if err := c.do(http.MethodPost, "/api/v1/clusters", body, &cluster); err != nil {
		return nil, err
	}
	return &cluster, nil
}

// ListClusters returns all registered clusters.
func (c *Client) ListClusters() ([]registry.ClusterInfo, error) {
	var clusters []registry.ClusterInfo
	if err := c.do(http.MethodGet, "/api/v1/clusters", nil, &clusters); err != nil {
		return nil, err
	}
	return clusters, nil
}

// Propagate sends a deployment manifest to be applied to the named clusters.
func (c *Client) Propagate(manifest string, clusters []string) (*api.PropagateResponse, error) {
	var resp api.PropagateResponse
	body := api.PropagateRequest{Manifest: manifest, Clusters: clusters}
	if err := c.do(http.MethodPost, "/api/v1/deployments", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ListDeployments returns every recorded propagation.
func (c *Client) ListDeployments() ([]propagation.Record, error) {
	var records []propagation.Record
	if err := c.do(http.MethodGet, "/api/v1/deployments", nil, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// DeploymentStatus returns a deployment's live rollout state per cluster.
func (c *Client) DeploymentStatus(namespace, name string) (*propagation.DeploymentStatus, error) {
	var status propagation.DeploymentStatus
	path := "/api/v1/deployments/" + url.PathEscape(namespace) + "/" + url.PathEscape(name)
	if err := c.do(http.MethodGet, path, nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// do sends a JSON request and decodes a successful JSON response into out.
// Non-2xx responses are returned as errors carrying the server's message.
func (c *Client) do(method, path string, in, out interface{}) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("failed to build request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach Orkestra server at %s (is `orkestra serve` running?): %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&apiErr) == nil && apiErr.Error != "" {
			return fmt.Errorf("%s", apiErr.Error)
		}
		return fmt.Errorf("server returned %s", resp.Status)
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}
