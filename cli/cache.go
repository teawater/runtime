// Copyright (c) 2019 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	runtimeDebug "runtime/debug"

	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	pb "github.com/kata-containers/runtime/protocols/cache"
	vc "github.com/kata-containers/runtime/virtcontainers"
	vf "github.com/kata-containers/runtime/virtcontainers/factory"
	"github.com/kata-containers/runtime/virtcontainers/pkg/oci"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
)

type cacheServer struct {
	rpc     *grpc.Server
	factory vc.Factory
}

var jsonVMConfig *pb.GrpcVMConfig

func (s *cacheServer) Config(ctx context.Context, empty *google_protobuf.Empty) (*pb.GrpcVMConfig, error) {
	if jsonVMConfig == nil {
		config := s.factory.Config()

		var err error
		jsonVMConfig, err = config.ToGrpc()
		if err != nil {
			return nil, err
		}
	}

	return jsonVMConfig, nil
}

func (s *cacheServer) GetBaseVM(ctx context.Context, empty *google_protobuf.Empty) (*pb.GrpcVM, error) {
	config := s.factory.Config()

	vm, err := s.factory.GetBaseVM(ctx, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to GetBaseVM")
	}

	grpcvm, err := vm.ToGrpc(config)

	return grpcvm, nil
}

func GetUnixListener(path string) (net.Listener, error) {
	err := os.MkdirAll(filepath.Dir(path), 0755)
	if err != nil {
		return nil, err
	}
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

var handledSignals = []os.Signal{
	unix.SIGTERM,
	unix.SIGINT,
	unix.SIGUSR1,
	unix.SIGPIPE,
}

func handleSignals(s *cacheServer, signals chan os.Signal) chan struct{} {
	done := make(chan struct{}, 1)
	go func() {
		for {
			sig := <-signals
			kataLog.WithField("signal", sig).Debug("received signal")
			switch sig {
			case unix.SIGUSR1:
				kataLog.WithField("stack", runtimeDebug.Stack()).Debug("dump stack")
			case unix.SIGPIPE:
				continue
			default:
				s.rpc.GracefulStop()
				close(done)
				return
			}
		}
	}()
	return done
}

var cacheCLICommand = cli.Command{
	Name:  "cache",
	Usage: "run a vm cache server",
	Flags: []cli.Flag{
		cli.UintFlag{
			Name:  "number, n",
			Value: 1,
			Usage: `number of cache`,
		},
	},
	Action: func(context *cli.Context) error {
		cacheNum := context.Uint("number")
		if cacheNum == 0 {
			return errors.New("number of cache must big than 0")
		}
		ctx, err := cliContextToContext(context)
		if err != nil {
			return err
		}
		runtimeConfig, ok := context.App.Metadata["runtimeConfig"].(oci.RuntimeConfig)
		if !ok {
			return errors.New("invalid runtime config")
		}
		if !runtimeConfig.FactoryConfig.VMCache {
			return errors.New("vm cache not enabled")
		}

		factoryConfig := vf.Config{
			Template: runtimeConfig.FactoryConfig.Template,
			Cache:    cacheNum,
			VMCache:  true,
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
		defer f.CloseFactory(ctx)

		s := &cacheServer{
			rpc:     grpc.NewServer(),
			factory: f,
		}
		pb.RegisterCacheServiceServer(s.rpc, s)

		l, err := GetUnixListener(runtimeConfig.FactoryConfig.VMCacheEndpoint)
		if err != nil {
			return err
		}
		defer l.Close()

		signals := make(chan os.Signal, 2048)
		done := handleSignals(s, signals)
		signal.Notify(signals, handledSignals...)

		kataLog.WithField("endpoint", runtimeConfig.FactoryConfig.VMCacheEndpoint).Info("VM cache server start")
		s.rpc.Serve(l)

		<-done

		kataLog.WithField("endpoint", runtimeConfig.FactoryConfig.VMCacheEndpoint).Info("VM cache server stop")

		return nil
	},
}
