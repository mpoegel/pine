package arborist

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	api "github.com/mpoegel/pine/pkg/api"
)

type Client interface {
	StartTree(ctx context.Context, name string) error
	StopTree(ctx context.Context, name string) error
	RestartTree(ctx context.Context, name string) error
	GetTreeStatus(ctx context.Context, name string) (*api.TreeStatusResponse, error)
}

type ClientImpl struct {
	httpClient *http.Client
}

func NewClient(endpoint string) Client {
	return &ClientImpl{
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", endpoint)
				},
			},
		},
	}
}

func (c *ClientImpl) StartTree(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost/tree/start/"+name, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pine returned %s", resp.Status)
	}

	return nil
}

func (c *ClientImpl) StopTree(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost/tree/stop/"+name, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pine returned %s", resp.Status)
	}

	return nil
}

func (c *ClientImpl) RestartTree(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost/tree/restart/"+name, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pine returned %s", resp.Status)
	}

	return nil
}

func (c *ClientImpl) GetTreeStatus(ctx context.Context, name string) (*api.TreeStatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/tree/"+name, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pine returned %s", resp.Status)
	}

	res := &api.TreeStatusResponse{}
	decoder := json.NewDecoder(resp.Body)
	defer resp.Body.Close()
	if err := decoder.Decode(res); err != nil {
		return nil, err
	}

	return res, nil
}
