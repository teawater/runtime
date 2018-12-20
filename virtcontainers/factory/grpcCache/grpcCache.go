// Copyright (c) 2019 HyperHQ Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// grpcCache implements base vm factory that get base vm from grpc

package grpcCache

import (
	"context"
	"fmt"
	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	pb "github.com/kata-containers/runtime/protocols/cache"
	vc "github.com/kata-containers/runtime/virtcontainers"
	"github.com/kata-containers/runtime/virtcontainers/factory/base"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

type grpcCache struct {
	conn   *grpc.ClientConn
	config *vc.VMConfig
}

// New returns a new direct vm factory.
func New(ctx context.Context, endpoint string) (base.FactoryBase, error) {
	conn, err := grpc.Dial(fmt.Sprintf("unix://%s", endpoint), grpc.WithInsecure())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect %q", endpoint)
	}

	jConfig, err := pb.NewCacheServiceClient(conn).Config(ctx, &google_protobuf.Empty{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to Config")
	}

	config, err := vc.GrpcToVMConfig(jConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to convert JSON to VMConfig")
	}

	return &grpcCache{conn: conn, config: config}, nil
}

// Config returns the direct factory's configuration.
func (g *grpcCache) Config() vc.VMConfig {
	return *g.config
}

// GetBaseVM create a new VM directly.
func (g *grpcCache) GetBaseVM(ctx context.Context, config vc.VMConfig) (*vc.VM, error) {
	defer g.conn.Close()
	gVM, err := pb.NewCacheServiceClient(g.conn).GetBaseVM(ctx, &google_protobuf.Empty{})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to GetBaseVM")
	}
	return vc.NewVMFromGrpc(ctx, gVM, *g.config)
}

// CloseFactory closes the direct vm factory.
func (g *grpcCache) CloseFactory(ctx context.Context) {
}
