// Copyright (c) 2019 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// grpcCache implements base vm factory that get base vm from grpc

package grpcCache

import (
	"context"
	"encoding/json"
	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	pb "github.com/kata-containers/runtime/protocols/cache"
	vc "github.com/kata-containers/runtime/virtcontainers"
	"github.com/kata-containers/runtime/virtcontainers/factory/base"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"net"
	"time"
)

type grpcCache struct {
	config vc.VMConfig
}

// New returns a new direct vm factory.
func New(ctx context.Context, config vc.VMConfig) base.FactoryBase {
	return &grpcCache{config}
}

// Config returns the direct factory's configuration.
func (d *grpcCache) Config() vc.VMConfig {
	return d.config
}

// GetBaseVM create a new VM directly.
func (d *grpcCache) GetBaseVM(ctx context.Context, config vc.VMConfig) (*vc.VM, error) {
	/*gopts := []grpc.DialOption{
		grpc.WithInsecure(),
		grpc.WithBackoffMaxDelay(3 * time.Second),
		grpc.WithDialer(dialer.Dialer),

		// TODO(stevvooe): We may need to allow configuration of this on the client.
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(defaults.DefaultMaxRecvMsgSize)),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(defaults.DefaultMaxSendMsgSize)),
	}
	conn, err := grpc.Dial(fmt.Sprintf("unix://%s", "/tmp/1.sock"), gopts...)*/
	//fmt.Sprintf("unix://%s", "/tmp/1.sock"),
	conn, err := grpc.Dial(
		//fmt.Sprintf("unix://%s", "/tmp/1.sock"),
		"/tmp/1.sock",
		grpc.WithInsecure(),
		grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
			//return nil, fmt.Errorf("ttt addr %s", addr)
			return net.DialTimeout("unix", addr, timeout)
		}))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect %q", "/tmp/1.sock")
	}
	defer conn.Close()
	jVM, err := pb.NewCacheServiceClient(conn).GetBaseVM(ctx, &google_protobuf.Empty{})
	if err != nil {
		return nil, errors.Wrapf(err, "2 failed to connect %q", "/tmp/1.sock")
	}
	var vm vc.VM
	err = json.Unmarshal(jVM.Data, &vm)
	if err != nil {
		return nil, err
	}

	return &vm, nil
	/*_, err := net.Dial("unix", "/tmp/1.sock")
	if err != nil {
		return nil, errors.Wrapf(err, "2 failed to connect %q", "/tmp/1.sock")
	}
	return nil, fmt.Errorf("success")*/
}

// CloseFactory closes the direct vm factory.
func (d *grpcCache) CloseFactory(ctx context.Context) {
}
