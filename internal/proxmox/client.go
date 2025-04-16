package proxmox

import (
	"context"
	"fmt"

	"github.com/luthermonson/go-proxmox"
	"github.com/yusing/go-proxy/internal/utils/pool"
)

type Client struct {
	*proxmox.Client
	proxmox.Cluster
	Version *proxmox.Version
}

var Clients = pool.New[*Client]("proxmox_clients")

func NewClient(baseUrl string, opts ...proxmox.Option) *Client {
	return &Client{Client: proxmox.NewClient(baseUrl, opts...)}
}

func (c *Client) UpdateClusterInfo(ctx context.Context) (err error) {
	c.Version, err = c.Client.Version(ctx)
	if err != nil {
		return err
	}
	// requires (/, Sys.Audit)
	if err := c.Get(ctx, "/cluster/status", &c.Cluster); err != nil {
		return err
	}
	for _, node := range c.Cluster.Nodes {
		Nodes.Add(&Node{name: node.Name, id: node.ID, client: c.Client})
	}
	return nil
}

// Key implements pool.Object
func (c *Client) Key() string {
	return c.Cluster.ID
}

// Name implements pool.Object
func (c *Client) Name() string {
	return c.Cluster.Name
}

// MarshalMap implements pool.Object
func (c *Client) MarshalMap() map[string]any {
	return map[string]any{
		"version": c.Version,
		"cluster": map[string]any{
			"name":    c.Cluster.Name,
			"id":      c.Cluster.ID,
			"version": c.Cluster.Version,
			"nodes":   c.Cluster.Nodes,
			"quorate": c.Cluster.Quorate,
		},
	}
}

func (c *Client) NumNodes() int {
	return len(c.Cluster.Nodes)
}

func (c *Client) String() string {
	return fmt.Sprintf("%s (%s)", c.Cluster.Name, c.Cluster.ID)
}
