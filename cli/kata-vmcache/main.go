// Copyright (c) 2019 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package main

import (
	"context"
	"fmt"
	"github.com/opencontainers/runtime-spec/specs-go"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	rDebug "runtime/debug"

	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	"github.com/kata-containers/runtime/pkg/katautils"
	pb "github.com/kata-containers/runtime/protocols/cache"
	vc "github.com/kata-containers/runtime/virtcontainers"
	vf "github.com/kata-containers/runtime/virtcontainers/factory"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc"

)

const vmCacheName = "kata-vmcache"
var usage = fmt.Sprintf(`%s kata VM cache server

When enable_vm_cache enabled, %s start the VM cache server
that created some VMs (number is set by option number) as
VM cache.
Each kata-runtime will request VM from VM cache server
through vm_cache_endpoint.
It helps speeding up new container creation.`,  vmCacheName, vmCacheName)

var kataLog = logrus.New()

type cacheServer struct {
	rpc     *grpc.Server
	factory vc.Factory
}

var jsonVMConfig *pb.GrpcVMConfig

// Config requests base factory config and convert it to gRPC protocol.
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

// GetBaseVM requests a paused VM and convert it to gRPC protocol.
func (s *cacheServer) GetBaseVM(ctx context.Context, empty *google_protobuf.Empty) (*pb.GrpcVM, error) {
	config := s.factory.Config()

	vm, err := s.factory.GetBaseVM(ctx, config)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to GetBaseVM")
	}

	return vm.ToGrpc(config)
}

func getUnixListener(path string) (net.Listener, error) {
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
				kataLog.WithField("stack", rDebug.Stack()).Debug("dump stack")
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

func main() {
	app := cli.NewApp()
	app.Name = vmCacheName
	app.Usage = usage
	app.Writer = os.Stdout
	app.Version = katautils.MakeVersionString(version, commit, specs.Version)
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  configFilePathOption,
			Usage: project + " config file path",
		},
		cli.UintFlag{
			Name:  "number, n",
			Value: 1,
			Usage: "number of cache",
		},
	}
	app.Action = func(c *cli.Context) error {
		cacheNum := c.Uint("number")
		if cacheNum == 0 {
			return errors.New("cache number must be greater than zero")
		}
		ctx := context.Background()
		fmt.Println(c.GlobalString(configFilePathOption))
		_, runtimeConfig, err := katautils.LoadConfiguration(c.GlobalString(configFilePathOption), false, false)
		if err != nil {
			return errors.Wrap(err,"invalid runtime config")
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

		l, err := getUnixListener(runtimeConfig.FactoryConfig.VMCacheEndpoint)
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
	}

	err := app.Run(os.Args)
	if err != nil {
		kataLog.Fatal(err)
	}
}
