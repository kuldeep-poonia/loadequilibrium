package collector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type DockerClient struct {
	host       string
	http       *http.Client
	baseURL    string
	socketPath string
}

type dockerContainerSummary struct {
	ID              string            `json:"Id"`
	Names           []string          `json:"Names"`
	Labels          map[string]string `json:"Labels"`
	Ports           []dockerPort      `json:"Ports"`
	NetworkSettings struct {
		Networks map[string]dockerNetwork `json:"Networks"`
	} `json:"NetworkSettings"`
}

type dockerPort struct {
	IP          string `json:"IP"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort"`
	Type        string `json:"Type"`
}

type dockerNetwork struct {
	IPAddress string   `json:"IPAddress"`
	Aliases   []string `json:"Aliases"`
}

type dockerInspect struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Labels       map[string]string      `json:"Labels"`
		ExposedPorts map[string]interface{} `json:"ExposedPorts"`
	} `json:"Config"`
	HostConfig struct {
		Memory int64 `json:"Memory"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Networks map[string]dockerNetwork `json:"Networks"`
	} `json:"NetworkSettings"`
	State struct {
		Running bool `json:"Running"`
	} `json:"State"`
}

func NewDockerClient(host string) (*DockerClient, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}

	d := &DockerClient{host: host}
	transport := &http.Transport{
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     90 * time.Second,
	}

	switch {
	case strings.HasPrefix(host, "unix://"):
		d.socketPath = strings.TrimPrefix(host, "unix://")
		d.baseURL = "http://docker"
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", d.socketPath)
		}
	case strings.HasPrefix(host, "tcp://"):
		d.baseURL = "http://" + strings.TrimPrefix(host, "tcp://")
	case strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://"):
		d.baseURL = strings.TrimRight(host, "/")
	default:
		return nil, fmt.Errorf("unsupported DOCKER_HOST %q", host)
	}

	d.http = &http.Client{Transport: transport, Timeout: 10 * time.Second}
	return d, nil
}

func (d *DockerClient) Discover(ctx context.Context, cfg Config) ([]ServiceTarget, error) {
	if d == nil {
		return nil, errors.New("docker client is nil")
	}

	selfNetworks := d.selfNetworks(ctx)
	var summaries []dockerContainerSummary
	if err := d.getJSON(ctx, "/containers/json?all=0", &summaries); err != nil {
		return nil, err
	}

	targets := make([]ServiceTarget, 0, len(summaries))
	for _, summary := range summaries {
		labels := mergeLabels(summary.Labels, nil)
		if !labelTruthy(labels[cfg.DiscoveryLabel]) {
			continue
		}

		inspect, err := d.inspect(ctx, summary.ID)
		if err != nil {
			continue
		}
		if !inspect.State.Running {
			continue
		}

		labels = mergeLabels(labels, inspect.Config.Labels)
		target := ServiceTarget{
			ContainerID:      inspect.ID,
			ContainerName:    cleanContainerName(firstNonEmpty(inspect.Name, firstName(summary.Names), summary.ID)),
			ServiceID:        serviceIDFromLabels(labels, inspect.Name, summary.Names, summary.ID),
			Environment:      strings.TrimSpace(labels["le.environment"]),
			Team:             strings.TrimSpace(labels["le.team"]),
			Labels:           labels,
			MemoryLimitBytes: inspect.HostConfig.Memory,
			DiscoveredAt:     time.Now(),
		}

		target.MetricsURLs = buildMetricsURLs(target, labels, inspect, summary, selfNetworks, cfg)
		if len(target.MetricsURLs) == 0 {
			continue
		}
		targets = append(targets, target)
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].ServiceID < targets[j].ServiceID
	})
	return targets, nil
}

func (d *DockerClient) selfNetworks(ctx context.Context) map[string]struct{} {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return nil
	}
	inspect, err := d.inspect(ctx, host)
	if err != nil {
		return nil
	}
	out := make(map[string]struct{}, len(inspect.NetworkSettings.Networks))
	for name := range inspect.NetworkSettings.Networks {
		out[name] = struct{}{}
	}
	return out
}

func (d *DockerClient) inspect(ctx context.Context, id string) (*dockerInspect, error) {
	var inspect dockerInspect
	if err := d.getJSON(ctx, "/containers/"+url.PathEscape(id)+"/json", &inspect); err != nil {
		return nil, err
	}
	return &inspect, nil
}

func (d *DockerClient) getJSON(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.baseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("docker api %s status=%d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func buildMetricsURLs(
	target ServiceTarget,
	labels map[string]string,
	inspect *dockerInspect,
	summary dockerContainerSummary,
	selfNetworks map[string]struct{},
	cfg Config,
) []string {
	if raw := strings.TrimSpace(labels["le.metrics_url"]); raw != "" {
		return []string{raw}
	}

	path := cleanPath(firstNonEmpty(labels["le.metrics_path"], cfg.DefaultMetricsPath))
	scheme := firstNonEmpty(labels["le.metrics_scheme"], "http")
	explicitHost := strings.TrimSpace(labels["le.metrics_host"])
	explicitPort := intLabel(labels, "le.metrics_port")

	ports := candidatePorts(explicitPort, inspect, summary)
	hosts := candidateHosts(explicitHost, target, inspect, selfNetworks)
	if len(ports) == 0 || len(hosts) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	urls := make([]string, 0, len(ports)*len(hosts))
	for _, host := range hosts {
		for _, port := range ports {
			if host == "" || port <= 0 {
				continue
			}
			u := fmt.Sprintf("%s://%s:%d%s", scheme, host, port, path)
			if _, ok := seen[u]; ok {
				continue
			}
			seen[u] = struct{}{}
			urls = append(urls, u)
		}
	}
	return urls
}

func candidatePorts(explicit int, inspect *dockerInspect, summary dockerContainerSummary) []int {
	if explicit > 0 {
		return []int{explicit}
	}

	seen := make(map[int]struct{})
	var ports []int
	add := func(port int) {
		if port <= 0 {
			return
		}
		if _, exists := seen[port]; exists {
			return
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}

	for _, p := range summary.Ports {
		if strings.EqualFold(p.Type, "tcp") {
			add(p.PrivatePort)
		}
	}
	for raw := range inspect.Config.ExposedPorts {
		parts := strings.Split(raw, "/")
		if len(parts) > 0 {
			if port, err := strconv.Atoi(parts[0]); err == nil {
				add(port)
			}
		}
	}
	for _, common := range []int{9090, 8080, 8000, 3000, 5000, 9100} {
		add(common)
	}
	return ports
}

func candidateHosts(explicit string, target ServiceTarget, inspect *dockerInspect, selfNetworks map[string]struct{}) []string {
	if explicit != "" {
		return []string{explicit}
	}

	seen := make(map[string]struct{})
	var hosts []string
	add := func(host string) {
		host = strings.TrimSpace(host)
		if host == "" {
			return
		}
		if _, ok := seen[host]; ok {
			return
		}
		seen[host] = struct{}{}
		hosts = append(hosts, host)
	}

	for networkName, network := range inspect.NetworkSettings.Networks {
		if len(selfNetworks) > 0 {
			if _, shared := selfNetworks[networkName]; !shared {
				continue
			}
		}
		for _, alias := range network.Aliases {
			if alias == "" || strings.HasPrefix(target.ContainerID, alias) {
				continue
			}
			add(alias)
		}
		add(target.ContainerName)
		add(network.IPAddress)
	}
	if len(hosts) == 0 {
		for _, network := range inspect.NetworkSettings.Networks {
			add(network.IPAddress)
		}
	}
	return hosts
}

func serviceIDFromLabels(labels map[string]string, name string, names []string, id string) string {
	if service := strings.TrimSpace(labels["le.service"]); service != "" {
		return service
	}
	if service := strings.TrimSpace(labels["com.docker.compose.service"]); service != "" {
		return service
	}
	cleaned := cleanContainerName(firstNonEmpty(name, firstName(names), id))
	if cleaned != "" {
		return cleaned
	}
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func mergeLabels(primary, secondary map[string]string) map[string]string {
	out := make(map[string]string, len(primary)+len(secondary))
	for k, v := range primary {
		out[k] = v
	}
	for k, v := range secondary {
		out[k] = v
	}
	return out
}

func labelTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on", "enabled":
		return true
	default:
		return false
	}
}

func intLabel(labels map[string]string, key string) int {
	if labels == nil {
		return 0
	}
	if value := strings.TrimSpace(labels[key]); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return 0
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

func cleanContainerName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "/")
	return name
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
