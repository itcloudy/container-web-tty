package backend

import (
	"context"
	"sync"
	"time"

	apiTypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
	"github.com/wrfly/container-web-tty/config"
	"github.com/wrfly/container-web-tty/types"
)

type DockerCli struct {
	cli             *client.Client
	containers      map[string]types.Container
	containersMutex *sync.RWMutex
}

func NewDockerCli(conf config.DockerConfig) (*DockerCli, []string, error) {
	host := conf.DockerHost
	if host[:1] == "/" {
		host = "unix://" + host
	} else {
		host = "tcp://" + host
	}
	version := "v1.24"
	UA := map[string]string{"User-Agent": "engine-api-cli-1.0"}
	cli, err := client.NewClient(host, version, nil, UA)
	if err != nil {
		logrus.Errorf("create new docker client error: %s", err)
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ping, err := cli.Ping(ctx)
	if err != nil {
		return nil, nil, err
	}
	logrus.Infof("New docker client: OS [%s], API [%s]", ping.OSType, ping.APIVersion)

	return &DockerCli{
		cli:             cli,
		containers:      map[string]types.Container{},
		containersMutex: &sync.RWMutex{},
	}, []string{conf.DockerPath, "exec", "-ti"}, nil
}

func getContainerIP(networkSettings *apiTypes.SummaryNetworkSettings) []string {
	ips := []string{}

	if networkSettings == nil {
		return ips
	}

	for net := range networkSettings.Networks {
		ips = append(ips, networkSettings.Networks[net].IPAddress)
	}

	return ips
}

func (docker DockerCli) GetInfo(ID string) types.Container {
	if len(docker.containers) == 0 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		docker.List(ctx)
		cancel()
	}
	docker.containersMutex.RLock()
	defer docker.containersMutex.RUnlock()
	return docker.containers[ID]
}

func (docker DockerCli) List(ctx context.Context) []types.Container {
	cs, err := docker.cli.ContainerList(ctx, apiTypes.ContainerListOptions{})
	if err != nil {
		logrus.Errorf("list containers eror: %s", err)
		return nil
	}
	containers := []types.Container{}
	for _, container := range cs {
		containers = append(containers, types.Container{
			ID:      container.ID,
			Name:    container.Names[0][1:],
			Image:   container.Image,
			Command: container.Command,
			IPs:     getContainerIP(container.NetworkSettings),
			Status:  container.Status,
			State:   container.State,
		})
	}

	docker.containersMutex.Lock()
	defer docker.containersMutex.Unlock()
	for _, c := range containers {
		// see list.html:31
		docker.containers[c.ID[:12]] = c
	}

	return containers
}

func (docker DockerCli) exist(ctx context.Context, cid, path string) bool {
	_, err := docker.cli.ContainerStatPath(ctx, cid, path)
	if err != nil {
		return false
	}
	return true
}

func (docker DockerCli) GetShell(ctx context.Context, cid string) string {
	for _, sh := range types.SHELL_LIST {
		if docker.exist(ctx, cid, sh) {
			return sh
		}
	}
	// generally it would'n come here
	return ""
}
