// Command client is the benchmark client of fib's grpc: one grpc.ClientConn
// per connection, all of them on one fib engine.
package main

import (
	"context"

	fib "github.com/lesismal/fib"
	"github.com/lesismal/fib/grpc"

	"go-rpc-benchmark/codec/pbcodec"
	"go-rpc-benchmark/pb/echo"
	"go-rpc-benchmark/pb/fibgrpc"
	"go-rpc-benchmark/runner"
)

type fibGrpcClient struct {
	opts    runner.Options
	engine  *fib.Engine
	stopped chan struct{}
	conns   []*grpc.ClientConn
	clients []fibgrpc.EchoClient
}

func (c *fibGrpcClient) Echo(conn int, action, msg string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.opts.Timeout)
	defer cancel()
	rsp, err := c.clients[conn].Echo(ctx, &echo.Request{Action: action, Msg: msg})
	if err != nil {
		return "", err
	}
	return rsp.Msg, nil
}

func (c *fibGrpcClient) Close() error {
	// The connections close before their engine does.
	for _, cc := range c.conns {
		if cc != nil {
			cc.Close()
		}
	}
	c.engine.Stop()
	<-c.stopped
	return c.engine.Close()
}

func main() {
	grpc.RegisterCodec(pbcodec.Codec{})
	runner.Main("fib_grpc", func(opts runner.Options) (runner.Client, error) {
		engine, err := fib.NewEngine(fib.DefaultConfig(), nil)
		if err != nil {
			return nil, err
		}
		c := &fibGrpcClient{
			opts:    opts,
			engine:  engine,
			stopped: make(chan struct{}),
			conns:   make([]*grpc.ClientConn, opts.Conns),
			clients: make([]fibgrpc.EchoClient, opts.Conns),
		}
		go func() {
			defer close(c.stopped)
			engine.Run()
		}()
		// A ClientConn dials on its first call, which the runner makes over
		// every connection before it measures anything.
		for i := range c.conns {
			cc, err := grpc.NewClient(opts.Addr, grpc.WithEngine(engine), grpc.WithConnectTimeout(opts.Timeout))
			if err != nil {
				c.Close()
				return nil, err
			}
			c.conns[i], c.clients[i] = cc, fibgrpc.NewEchoClient(cc)
		}
		return c, nil
	})
}
