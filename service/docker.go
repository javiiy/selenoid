package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/aandryashin/selenoid/config"
	"github.com/docker/engine-api/client"
	"github.com/docker/engine-api/types"
	"github.com/docker/engine-api/types/container"
	"github.com/docker/engine-api/types/network"
	"github.com/docker/go-connections/nat"
)

type Docker struct {
	Client  *client.Client
	Service *config.Browser
}

func (docker *Docker) StartWithCancel() (*url.URL, func(), error) {
	port, err := nat.NewPort("tcp", docker.Service.Port)
	if err != nil {
		return nil, nil, err
	}
	ctx := context.Background()
	log.Println("Creating Docker container", docker.Service.Image, "...")
	resp, err := docker.Client.ContainerCreate(ctx,
		&container.Config{
			Hostname:     "localhost",
			Image:        docker.Service.Image.(string),
			ExposedPorts: map[nat.Port]struct{}{port: struct{}{}},
		},
		&container.HostConfig{
			AutoRemove: true,
			PortBindings: nat.PortMap{
				port: []nat.PortBinding{nat.PortBinding{HostIP: "127.0.0.1"}},
			},
			ShmSize:    268435456,
			Privileged: true,
		},
		&network.NetworkingConfig{}, "")
	if err != nil {
		log.Println("error creating container:", err)
		return nil, nil, err
	}
	log.Println("Starting container...")
	err = docker.Client.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{})
	if err != nil {
		log.Println("error starting container:", err)
		return nil, nil, err
	}
	log.Printf("Container %s started\n", resp.ID)
	stat, err := docker.Client.ContainerInspect(ctx, resp.ID)
	if err != nil {
		log.Printf("unable to inspect container %s: %s\n", resp.ID, err)
		return nil, nil, err
	}
	_, ok := stat.NetworkSettings.Ports[port]
	if !ok {
		err := errors.New(fmt.Sprintf("no bingings available for %v...\n", port))
		log.Println(err)
		return nil, nil, err
	}
	if len(stat.NetworkSettings.Ports[port]) != 1 {
		err := errors.New(fmt.Sprintf("error: wrong number of port bindings"))
		log.Println(err)
		return nil, nil, err
	}
	addr := stat.NetworkSettings.Ports[port][0]
	host := fmt.Sprintf("http://%s:%s%s", addr.HostIP, addr.HostPort, docker.Service.Path)
	s := time.Now()
	err = wait(host, 10*time.Second)
	if err != nil {
		log.Println(err)
		return nil, nil, err
	}
	log.Println(time.Since(s))
	u, _ := url.Parse(host)
	log.Println("proxying requests to:", host)
	//	reader, _ := docker.Client.ContainerLogs(context.Background(), resp.ID, types.ContainerLogsOptions{
	//		ShowStdout: true,
	//		ShowStderr: true,
	//		Follow:     true,
	//	})
	return u, func() { stop(ctx, docker.Client, resp.ID) }, nil
}

func stop(ctx context.Context, cli *client.Client, id string) {
	fmt.Println("Stopping container", id)
	err := cli.ContainerStop(ctx, id, nil)
	if err != nil {
		log.Println("error: unable to stop container", id, err)
		return
	}
	cli.ContainerWait(ctx, id)
	fmt.Printf("Container %s stopped\n", id)
	err = cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{Force: true})
	if err != nil {
		fmt.Println("error: unable to remove container", id, err)
		return
	}
	fmt.Printf("Container %s removed\n", id)
}
