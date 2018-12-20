// Copyright (c) 2019 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"

	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	pb "github.com/kata-containers/runtime/protocols/cache"
	vc "github.com/kata-containers/runtime/virtcontainers"
	vf "github.com/kata-containers/runtime/virtcontainers/factory"
	"github.com/kata-containers/runtime/virtcontainers/pkg/oci"
	"github.com/urfave/cli"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
)

type cacheServer struct {
	rpc     *grpc.Server
	factory vc.Factory
}

func (s *cacheServer) GetBaseVM(ctx context.Context, empty *google_protobuf.Empty) (*pb.JsonVM, error) {
	fmt.Println("1")
	vm, err := s.factory.GetBaseVM(ctx, vc.VMConfig{})
	if err != nil {
		fmt.Println("err", err)
		return nil, err
	}
	fmt.Println("vm", vm)
	jVM, err := json.Marshal(&vm)

	return &pb.JsonVM{Data: jVM}, nil
}

func GetUnixListener(path string) (net.Listener, error) {
	var err error
	if err = unix.Unlink(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0660); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

var cacheCLICommand = cli.Command{
	Name:  "cache",
	Usage: "run a cache server",
	Action: func(context *cli.Context) error {
		ctx, err := cliContextToContext(context)
		if err != nil {
			return err
		}
		runtimeConfig, ok := context.App.Metadata["runtimeConfig"].(oci.RuntimeConfig)
		if !ok {
			return errors.New("invalid runtime config")
		}

		factoryConfig := vf.Config{
			Template: runtimeConfig.FactoryConfig.Template,
			Cache: 1,
			VMConfig: vc.VMConfig{
				HypervisorType:   runtimeConfig.HypervisorType,
				HypervisorConfig: runtimeConfig.HypervisorConfig,
				AgentType:        runtimeConfig.AgentType,
				AgentConfig:      runtimeConfig.AgentConfig,
				ProxyType:        runtimeConfig.ProxyType,
				ProxyConfig:      runtimeConfig.ProxyConfig,
			},
		}
		f, err := vf.NewFactory(ctx, factoryConfig, false)
		if err != nil {
			return err
		}

		s := &cacheServer{
			rpc: grpc.NewServer(),
			factory: f,
		}
		pb.RegisterCacheServiceServer(s.rpc, s)

		l, err := GetUnixListener("/tmp/1.sock")
		if err != nil {
			return err
		}

		s.rpc.Serve(l)

		return nil
	},
}
