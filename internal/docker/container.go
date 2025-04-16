package docker

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/yusing/go-proxy/agent/pkg/agent"
	"github.com/yusing/go-proxy/internal/gperr"
	idlewatcher "github.com/yusing/go-proxy/internal/idlewatcher/types"
	"github.com/yusing/go-proxy/internal/logging"
	"github.com/yusing/go-proxy/internal/utils"
	"github.com/yusing/go-proxy/internal/utils/strutils"
)

type (
	PortMapping = map[int]*container.Port
	Container   struct {
		_ utils.NoCopy

		DockerHost    string          `json:"docker_host"`
		Image         *ContainerImage `json:"image"`
		ContainerName string          `json:"container_name"`
		ContainerID   string          `json:"container_id"`

		Agent *agent.AgentConfig `json:"agent"`

		RouteConfig       map[string]string   `json:"route_config"`
		IdlewatcherConfig *idlewatcher.Config `json:"idlewatcher_config"`

		Mounts []string `json:"mounts"`

		PublicPortMapping  PortMapping `json:"public_ports"`  // non-zero publicPort:types.Port
		PrivatePortMapping PortMapping `json:"private_ports"` // privatePort:types.Port
		PublicHostname     string      `json:"public_hostname"`
		PrivateHostname    string      `json:"private_hostname"`

		Aliases    []string `json:"aliases"`
		IsExcluded bool     `json:"is_excluded"`
		IsExplicit bool     `json:"is_explicit"`
		Running    bool     `json:"running"`
	}
	ContainerImage struct {
		Author string `json:"author,omitempty"`
		Name   string `json:"name"`
		Tag    string `json:"tag,omitempty"`
	}
)

var DummyContainer = new(Container)

func FromDocker(c *container.Summary, dockerHost string) (res *Container) {
	isExplicit := false
	helper := containerHelper{c}
	res = &Container{
		DockerHost:    dockerHost,
		Image:         helper.parseImage(),
		ContainerName: helper.getName(),
		ContainerID:   c.ID,

		Mounts: helper.getMounts(),

		PublicPortMapping:  helper.getPublicPortMapping(),
		PrivatePortMapping: helper.getPrivatePortMapping(),

		Aliases:    helper.getAliases(),
		IsExcluded: strutils.ParseBool(helper.getDeleteLabel(LabelExclude)),
		IsExplicit: isExplicit,
		Running:    c.Status == "running" || c.State == "running",
	}

	if agent.IsDockerHostAgent(dockerHost) {
		var ok bool
		res.Agent, ok = agent.Agents.Get(dockerHost)
		if !ok {
			logging.Error().Msgf("agent %q not found", dockerHost)
		}
	}

	res.setPrivateHostname(helper)
	res.setPublicHostname()
	res.loadDeleteIdlewatcherLabels(helper)

	for lbl := range c.Labels {
		if strings.HasPrefix(lbl, NSProxy+".") {
			isExplicit = true
		} else {
			delete(c.Labels, lbl)
		}
	}
	res.RouteConfig = utils.FitMap(c.Labels)
	return
}

func FromInspectResponse(json container.InspectResponse, dockerHost string) *Container {
	ports := make([]container.Port, 0, len(json.NetworkSettings.Ports))
	for k, bindings := range json.NetworkSettings.Ports {
		proto, privPortStr := nat.SplitProtoPort(string(k))
		privPort, _ := strconv.ParseUint(privPortStr, 10, 16)
		ports = append(ports, container.Port{
			PrivatePort: uint16(privPort),
			Type:        proto,
		})
		for _, v := range bindings {
			pubPort, _ := strconv.ParseUint(v.HostPort, 10, 16)
			ports = append(ports, container.Port{
				IP:          v.HostIP,
				PublicPort:  uint16(pubPort),
				PrivatePort: uint16(privPort),
				Type:        proto,
			})
		}
	}
	cont := FromDocker(&container.Summary{
		ID:     json.ID,
		Names:  []string{strings.TrimPrefix(json.Name, "/")},
		Image:  json.Image,
		Ports:  ports,
		Labels: json.Config.Labels,
		State:  json.State.Status,
		Status: json.State.Status,
		Mounts: json.Mounts,
		NetworkSettings: &container.NetworkSettingsSummary{
			Networks: json.NetworkSettings.Networks,
		},
	}, dockerHost)
	return cont
}

func (c *Container) IsBlacklisted() bool {
	return c.Image.IsBlacklisted() || c.isDatabase()
}

var databaseMPs = map[string]struct{}{
	"/var/lib/postgresql/data": {},
	"/var/lib/mysql":           {},
	"/var/lib/mongodb":         {},
	"/var/lib/mariadb":         {},
	"/var/lib/memcached":       {},
	"/var/lib/rabbitmq":        {},
}

func (c *Container) isDatabase() bool {
	for _, m := range c.Mounts {
		if _, ok := databaseMPs[m]; ok {
			return true
		}
	}

	for _, v := range c.PrivatePortMapping {
		switch v.PrivatePort {
		// postgres, mysql or mariadb, redis, memcached, mongodb
		case 5432, 3306, 6379, 11211, 27017:
			return true
		}
	}
	return false
}

func (c *Container) setPublicHostname() {
	if !c.Running {
		return
	}
	if strings.HasPrefix(c.DockerHost, "unix://") {
		c.PublicHostname = "127.0.0.1"
		return
	}
	url, err := url.Parse(c.DockerHost)
	if err != nil {
		logging.Err(err).Msgf("invalid docker host %q, falling back to 127.0.0.1", c.DockerHost)
		c.PublicHostname = "127.0.0.1"
		return
	}
	c.PublicHostname = url.Hostname()
}

func (c *Container) setPrivateHostname(helper containerHelper) {
	if !strings.HasPrefix(c.DockerHost, "unix://") && c.Agent == nil {
		return
	}
	if helper.NetworkSettings == nil {
		return
	}
	for _, v := range helper.NetworkSettings.Networks {
		if v.IPAddress == "" {
			continue
		}
		c.PrivateHostname = v.IPAddress
		return
	}
}

func (c *Container) loadDeleteIdlewatcherLabels(helper containerHelper) {
	cfg := map[string]any{
		"idle_timeout":   helper.getDeleteLabel(LabelIdleTimeout),
		"wake_timeout":   helper.getDeleteLabel(LabelWakeTimeout),
		"stop_method":    helper.getDeleteLabel(LabelStopMethod),
		"stop_timeout":   helper.getDeleteLabel(LabelStopTimeout),
		"stop_signal":    helper.getDeleteLabel(LabelStopSignal),
		"start_endpoint": helper.getDeleteLabel(LabelStartEndpoint),
	}
	// set only if idlewatcher is enabled
	idleTimeout := cfg["idle_timeout"]
	if idleTimeout != "" {
		idwCfg := &idlewatcher.Config{
			Docker: &idlewatcher.DockerConfig{
				DockerHost:    c.DockerHost,
				ContainerID:   c.ContainerID,
				ContainerName: c.ContainerName,
			},
		}
		err := utils.MapUnmarshalValidate(cfg, idwCfg)
		if err != nil {
			gperr.LogWarn("invalid idlewatcher config", gperr.PrependSubject(c.ContainerName, err))
		} else {
			c.IdlewatcherConfig = idwCfg
		}
	}
}
