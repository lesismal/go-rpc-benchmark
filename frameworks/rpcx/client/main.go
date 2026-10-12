// Command client is the rpcx benchmark client: one rpcx client.Client per
// connection, which is the connection rpcx's XClient pools are made of.
package main

import (
	"context"

	"github.com/smallnest/rpcx/client"
	rlog "github.com/smallnest/rpcx/log"
	"github.com/smallnest/rpcx/protocol"

	"go-rpc-benchmark/pb/echo"
	"go-rpc-benchmark/runner"
)

type rpcxClient struct {
	opts    runner.Options
	clients []*client.Client
}

func (c *rpcxClient) Echo(conn int, action, msg string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.opts.Timeout)
	defer cancel()
	rsp := &echo.Response{}
	err := c.clients[conn].Call(ctx, "Echo", "Echo", &echo.Request{Action: action, Msg: msg}, rsp)
	return rsp.Msg, err
}

func (c *rpcxClient) Close() error {
	for _, cli := range c.clients {
		if cli != nil {
			cli.Close()
		}
	}
	return nil
}

func main() {
	rlog.SetDummyLogger()
	runner.Main("rpcx", func(opts runner.Options) (runner.Client, error) {
		option := client.DefaultOption
		option.SerializeType = protocol.ProtoBuffer
		option.ConnectTimeout = opts.Timeout
		c := &rpcxClient{opts: opts, clients: make([]*client.Client, opts.Conns)}
		err := runner.Parallel(opts.Conns, opts.DialParallel, func(i int) error {
			cli := client.NewClient(option)
			if err := cli.Connect("tcp", opts.Addr); err != nil {
				return err
			}
			c.clients[i] = cli
			return nil
		})
		if err != nil {
			c.Close()
			return nil, err
		}
		return c, nil
	})
}
