// Command client is the benchmark client of fib's arpc: one arpc.Client per
// connection, all of them on one fib engine.
package main

import (
	fib "github.com/lesismal/fib"
	"github.com/lesismal/fib/arpc"

	"go-rpc-benchmark/codec/pbcodec"
	"go-rpc-benchmark/pb/echo"
	"go-rpc-benchmark/runner"
)

type fibArpcClient struct {
	opts    runner.Options
	engine  *fib.Engine
	stopped chan struct{}
	clients []*arpc.Client
}

func (c *fibArpcClient) Echo(conn int, action, msg string) (string, error) {
	rsp := &echo.Response{}
	err := c.clients[conn].Call("Echo", &echo.Request{Action: action, Msg: msg}, rsp, c.opts.Timeout)
	return rsp.Msg, err
}

func (c *fibArpcClient) Close() error {
	for _, cli := range c.clients {
		if cli != nil {
			cli.Stop()
		}
	}
	c.engine.Stop()
	<-c.stopped
	return c.engine.Close()
}

func main() {
	arpc.DefaultCodec = pbcodec.Codec{}
	runner.Main("fib_arpc", func(opts runner.Options) (runner.Client, error) {
		engine, err := fib.NewEngine(fib.DefaultConfig(), nil)
		if err != nil {
			return nil, err
		}
		c := &fibArpcClient{opts: opts, engine: engine, stopped: make(chan struct{}), clients: make([]*arpc.Client, opts.Conns)}
		go func() {
			defer close(c.stopped)
			engine.Run()
		}()
		err = runner.Parallel(opts.Conns, opts.DialParallel, func(i int) error {
			cli, err := arpc.Dial(engine, "tcp", opts.Addr, opts.Timeout, nil)
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
