// Command client is the grpc-go benchmark client: one grpc.ClientConn per
// connection, each of which holds one HTTP/2 connection to the server.
package main

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"go-rpc-benchmark/pb/echo"
	"go-rpc-benchmark/runner"
)

type grpcClient struct {
	opts    runner.Options
	conns   []*grpc.ClientConn
	clients []echo.EchoClient
}

func (c *grpcClient) Echo(conn int, action, msg string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.opts.Timeout)
	defer cancel()
	rsp, err := c.clients[conn].Echo(ctx, &echo.Request{Action: action, Msg: msg})
	if err != nil {
		return "", err
	}
	return rsp.Msg, nil
}

func (c *grpcClient) Close() error {
	for _, cc := range c.conns {
		if cc != nil {
			cc.Close()
		}
	}
	return nil
}

func main() {
	runner.Main("grpc", func(opts runner.Options) (runner.Client, error) {
		c := &grpcClient{opts: opts, conns: make([]*grpc.ClientConn, opts.Conns), clients: make([]echo.EchoClient, opts.Conns)}
		// A ClientConn dials on its first call, which the runner makes over
		// every connection before it measures anything.
		for i := range c.conns {
			cc, err := grpc.NewClient(opts.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				c.Close()
				return nil, err
			}
			c.conns[i], c.clients[i] = cc, echo.NewEchoClient(cc)
		}
		return c, nil
	})
}
