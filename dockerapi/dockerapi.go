package dockerapi

import (
	"bytes"
	"context"
	"fmt"

	_ "github.com/lib/pq"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
)

const (
	Host     = "0.0.0.0"
	Port     = 5432
	User     = "postgres"
	Password = "password"
	Dbname   = "orchestrate"
)

//ListRunningContainers blah
func ListRunningContainers() []types.Container {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	containers, err := cli.ContainerList(context.Background(), types.ContainerListOptions{All: true})
	if err != nil {
		panic(err)
	}
	return containers
}

//DockerStats blah
func DockerStats(id string) types.ContainerStats {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	containers, err := cli.ContainerStats(context.Background(), id, false)
	if err != nil {
		panic(err)
	}
	buf := new(bytes.Buffer)
	buf.ReadFrom(containers.Body)
	newStr := buf.String()
	fmt.Println(newStr)
	return containers
}

//StopContainer blah
func StopContainer(containerID string) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	if err := cli.ContainerStop(context.Background(), containerID, nil); err != nil {
		panic(err)
	}
	fmt.Printf("Successfully stopped container %s\n", containerID)
}

//StartContainer h
func StartContainer(containerID string) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	if err := cli.ContainerStart(context.Background(), containerID, types.ContainerStartOptions{}); err != nil {
		panic(err)
	}
	fmt.Printf("Successfully started container %s\n", containerID)
}

//DeleteContainer h
func DeleteContainer(containerID string) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	if err := cli.ContainerRemove(context.Background(), containerID, types.ContainerRemoveOptions{}); err != nil {
		panic(err)
	}
	fmt.Printf("Successfully deleted container %s\n", containerID)

}

// need kubectl -n kube-system port-forward svc/tiller-deploy 44134:44134, or use https://github.com/helm/helm/issues/3663#issuecomment-451501588
// problem is that the jfrog helm charts only go back til 6.2.x for artifactory...
const (
	tillerHost = "127.0.0.1:44134"
)
