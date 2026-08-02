// Package services: Docker client, instance lifecycle, dashboard session
// bootstrap and the unified gateway proxy.
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// DockerClient is a thin client for the subset of the Docker Engine API
// the portal needs. It talks directly to the daemon over a unix socket
// (or DOCKER_HOST TCP), so no heavy SDK dependency is required.
type DockerClient struct {
	baseURL    string // e.g. "http://docker" (unix) or "http://host:2375"
	transport  *http.Transport
	httpClient *http.Client
}

// NewDockerClient builds a client from a DOCKER_HOST-style URL.
func NewDockerClient(dockerHost string) (*DockerClient, error) {
	dc := &DockerClient{baseURL: "http://docker"}
	tr := &http.Transport{}
	if strings.HasPrefix(dockerHost, "unix://") {
		sockPath := strings.TrimPrefix(dockerHost, "unix://")
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", sockPath)
		}
	} else {
		host := strings.TrimPrefix(dockerHost, "tcp://")
		dc.baseURL = "http://" + host
		tr.DialContext = (&net.Dialer{Timeout: 5 * time.Second}).DialContext
	}
	dc.transport = tr
	dc.httpClient = &http.Client{
		Transport: tr,
		Timeout:   120 * time.Second,
	}
	return dc, nil
}

// Ping verifies the daemon is reachable.
func (d *DockerClient) Ping(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+"/_ping", nil)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("docker unreachable: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

// ── Networks ───────────────────────────────────────────────────────────

// EnsureNetwork creates a bridge network if it does not exist.
func (d *DockerClient) EnsureNetwork(ctx context.Context, name string) error {
	_, status, err := d.do(ctx, http.MethodGet, "/networks/"+name, nil, nil)
	if err == nil && status == http.StatusOK {
		return nil
	}
	body := map[string]any{"Name": name, "Driver": "bridge", "CheckDuplicate": true}
	_, status, err = d.do(ctx, http.MethodPost, "/networks/create", body, nil)
	if err != nil {
		return err
	}
	if status == http.StatusConflict {
		return nil // already exists (race)
	}
	if status >= 300 {
		return fmt.Errorf("network create: status %d", status)
	}
	return nil
}

// ── Volumes ────────────────────────────────────────────────────────────

// EnsureVolume creates a named volume if it does not exist.
func (d *DockerClient) EnsureVolume(ctx context.Context, name string) error {
	_, status, err := d.do(ctx, http.MethodGet, "/volumes/"+name, nil, nil)
	if err == nil && status == http.StatusOK {
		return nil
	}
	body := map[string]any{"Name": name}
	_, status, err = d.do(ctx, http.MethodPost, "/volumes/create", body, nil)
	if err != nil {
		return err
	}
	if status == http.StatusConflict {
		return nil
	}
	if status >= 300 {
		return fmt.Errorf("volume create: status %d", status)
	}
	return nil
}

// RemoveVolume deletes a named volume (force).
func (d *DockerClient) RemoveVolume(ctx context.Context, name string) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, d.baseURL+"/volumes/"+name+"?force=1", nil)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

// ── Containers ─────────────────────────────────────────────────────────

// ContainerSpec is the subset of create options the portal uses.
type ContainerSpec struct {
	Name          string
	Image         string
	Command       []string
	Env           []string
	Network       string
	VolumeMount   string // "name:/opt/data"
	RestartPolicy string
	Labels        map[string]string
	MemLimit      string // optional, e.g. "2g"
}

// CreateContainer creates (and starts) a container.
func (d *DockerClient) CreateContainer(ctx context.Context, spec ContainerSpec) (string, error) {
	// Remove a leftover with the same name first.
	_, status, _ := d.do(ctx, http.MethodDelete, "/containers/"+spec.Name+"?force=1", nil, nil)
	_ = status

	volumes := map[string]struct{}{}
	mounts := []map[string]any{}
	if spec.VolumeMount != "" {
		parts := strings.SplitN(spec.VolumeMount, ":", 2)
		volumes[parts[1]] = struct{}{}
		mounts = append(mounts, map[string]any{
			"Type":   "volume",
			"Source": parts[0],
			"Target": parts[1],
		})
	}

	hostConfig := map[string]any{
		"RestartPolicy": map[string]any{"Name": spec.RestartPolicy},
	}
	if spec.MemLimit != "" {
		hostConfig["Memory"] = parseMemLimit(spec.MemLimit)
	}

	body := map[string]any{
		"Image":      spec.Image,
		"Cmd":        spec.Command,
		"Env":        spec.Env,
		"Hostname":   spec.Name,
		"Labels":     spec.Labels,
		"Volumes":    volumes,
		"HostConfig": hostConfig,
	}
	if spec.Network != "" {
		body["NetworkingConfig"] = map[string]any{
			"EndpointsConfig": map[string]any{
				spec.Network: map[string]any{},
			},
		}
	}
	respBody, status, err := d.do(ctx, http.MethodPost, "/containers/create?name="+spec.Name, body, nil)
	if err != nil {
		return "", err
	}
	if status >= 300 {
		return "", fmt.Errorf("container create: status %d: %s", status, truncate(string(respBody), 400))
	}
	var created struct {
		ID       string   `json:"Id"`
		Warnings []string `json:"Warnings"`
	}
	_ = json.Unmarshal(respBody, &created)

	if _, status, err := d.do(ctx, http.MethodPost, "/containers/"+created.ID+"/start", nil, nil); err != nil {
		return "", err
	} else if status >= 300 {
		return "", fmt.Errorf("container start: status %d", status)
	}
	return created.ID, nil
}

// ContainerAction performs start / stop / restart.
func (d *DockerClient) ContainerAction(ctx context.Context, name, action string) error {
	path := "/containers/" + name + "/" + action
	if action == "stop" {
		path += "?t=30"
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, d.baseURL+path, nil)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return nil
}

// RemoveContainer force-removes a container (best effort).
func (d *DockerClient) RemoveContainer(ctx context.Context, name string) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodDelete, d.baseURL+"/containers/"+name+"?force=1", nil)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
}

// ContainerState returns "running" | "stopped" | "" for a container.
func (d *DockerClient) ContainerState(ctx context.Context, name string) string {
	body, status, err := d.do(ctx, http.MethodGet, "/containers/"+name+"/json", nil, nil)
	if err != nil || status != http.StatusOK {
		return ""
	}
	var info struct {
		State struct {
			Running    bool   `json:"Running"`
			Restarting bool   `json:"Restarting"`
			Status     string `json:"Status"`
		} `json:"State"`
	}
	_ = json.Unmarshal(body, &info)
	switch {
	case info.State.Running:
		return "running"
	case info.State.Restarting:
		return "restarting"
	default:
		return "stopped"
	}
}

// ContainerLogs returns the last N lines of container logs.
func (d *DockerClient) ContainerLogs(ctx context.Context, name string, tail int) string {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/containers/%s/logs?stdout=1&stderr=1&tail=%d&timestamps=0", d.baseURL, name, tail), nil)
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Sprintf("(unable to read logs: %v)", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return stripDockerLogFrames(raw)
}

// ── helpers ────────────────────────────────────────────────────────────

func (d *DockerClient) do(ctx context.Context, method, path string, body any, query map[string]string) ([]byte, int, error) {
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.baseURL+path, reader)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	q := req.URL.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

func parseMemLimit(s string) int64 {
	mult := int64(1)
	trimmed := strings.TrimSpace(strings.ToLower(s))
	switch {
	case strings.HasSuffix(trimmed, "g"):
		mult = 1 << 30
		trimmed = strings.TrimSuffix(trimmed, "g")
	case strings.HasSuffix(trimmed, "m"):
		mult = 1 << 20
		trimmed = strings.TrimSuffix(trimmed, "m")
	case strings.HasSuffix(trimmed, "k"):
		mult = 1 << 10
		trimmed = strings.TrimSuffix(trimmed, "k")
	}
	var v float64
	fmt.Sscanf(trimmed, "%f", &v)
	return int64(v * float64(mult))
}

// stripDockerLogFrames removes the 8-byte docker log stream framing
// (stream type + size) that the logs endpoint returns.
func stripDockerLogFrames(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if len(raw) >= 8 {
		// All bytes between 0x00-0x0f after the frame header would be
		// suspicious; docker framing starts with a 1-byte stream id + 3
		// reserved + 4-byte length. Heuristic: if it looks framed, strip.
		buf := make([]byte, 0, len(raw))
		off := 0
		for off+8 <= len(raw) {
			size := int(raw[off+4])<<24 | int(raw[off+5])<<16 | int(raw[off+6])<<8 | int(raw[off+7])
			if size <= 0 || off+8+size > len(raw) {
				// not framable — return raw as-is
				return string(raw)
			}
			buf = append(buf, raw[off+8:off+8+size]...)
			off += 8 + size
		}
		if off == len(raw) {
			return string(buf)
		}
	}
	return string(raw)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
