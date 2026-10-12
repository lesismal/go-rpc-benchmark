// Command client is the arpc benchmark client: one arpc.Client per
// connection.
package main

import (
	"net"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/codec"
	alog "github.com/lesismal/arpc/log"

	"go-rpc-benchmark/codec/pbcodec"
	"go-rpc-benchmark/pb/echo"
	"go-rpc-benchmark/runner"
)

type arpcClient struct {
	opts    runner.Options
	clients []*arpc.Client
}

func (c *arpcClient) Echo(conn int, action, msg string) (string, error) {
	rsp := &echo.Response{}
	err := c.clients[conn].Call("Echo", &echo.Request{Action: action, Msg: msg}, rsp, c.opts.Timeout)
	return rsp.Msg, err
}

func (c *arpcClient) Close() error {
	for _, cli := range c.clients {
		if cli != nil {
			cli.Stop()
		}
	}
	return nil
}

func main() {
	alog.SetLevel(alog.LevelNone)
	codec.DefaultCodec = pbcodec.Codec{}
	arpc.EnablePool(true)
	runner.Main("arpc", func(opts runner.Options) (runner.Client, error) {
		c := &arpcClient{opts: opts, clients: make([]*arpc.Client, opts.Conns)}
		err := runner.Parallel(opts.Conns, opts.DialParallel, func(i int) error {
			cli, err := arpc.NewClient(func() (net.Conn, error) {
				return net.DialTimeout("tcp", opts.Addr, opts.Timeout)
			})
			c.clients[i] = cli
			return err
		})
		if err != nil {
			c.Close()
			return nil, err
		}
		return c, nil
	})
}
