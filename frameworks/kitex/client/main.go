// Command client is the kitex benchmark client, kitex-benchmark's kitex: one
// client on kitex's default transport, long connections from its pool.
//
// A pooled connection carries one call at a time: a call takes an idle
// connection, or dials one, and gives it back when it is answered. So kitex
// picks the connection of a call itself, and holds as many connections as
// there are calls in flight - the runner's concurrency - rather than -conns:
// 100 at 10 connections, where the other frameworks multiplex 10 calls over
// each one. The report's Server Conns column says how many it held. The pool
// keeps up to that many idle, so that none is closed and dialed again between
// two calls (kitex-benchmark keeps 1000).
package main

import (
	"context"
	"time"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/connpool"
	"github.com/cloudwego/kitex/pkg/klog"

	"go-rpc-benchmark/kitex_gen/echo"
	echosvr "go-rpc-benchmark/kitex_gen/echo/echo"
	"go-rpc-benchmark/runner"
)

type kitexClient struct {
	cli echosvr.Client
}

func (c *kitexClient) Echo(_ int, action, msg string) (string, error) {
	rsp, err := c.cli.Echo(context.Background(), &echo.Request{Action: action, Msg: msg})
	if err != nil {
		return "", err
	}
	return rsp.Msg, nil
}

func (c *kitexClient) Close() error { return nil }

func main() {
	klog.SetLevel(klog.LevelWarn)
	runner.Main("kitex", func(opts runner.Options) (runner.Client, error) {
		idle := max(opts.Concurrency, opts.Conns)
		cli, err := echosvr.NewClient("echo.kitex",
			client.WithHostPorts(opts.Addr),
			client.WithLongConnection(connpool.IdleConfig{
				MaxIdlePerAddress: idle,
				MaxIdleGlobal:     idle,
				MaxIdleTimeout:    time.Minute,
			}),
			client.WithRPCTimeout(opts.Timeout),
			client.WithConnectTimeout(opts.Timeout),
		)
		if err != nil {
			return nil, err
		}
		return &kitexClient{cli: cli}, nil
	})
}
