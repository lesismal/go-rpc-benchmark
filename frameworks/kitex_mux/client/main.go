// Command client is the kitex_mux benchmark client, kitex-benchmark's
// kitex-mux.
//
// Kitex's client picks the connection of a call itself, so the connection
// the runner asks for is not the one used: one client multiplexes every call
// over -conns connections (client.WithMuxConnection), round robin, which is
// how kitex holds a fixed number of connections to a server. Its pooled
// transport would instead open a connection for every call in flight.
//
// The mux pool dials lazily: a call that lands on an empty slot dials, and
// calls that land on empty slots at once share one dial. Two rounds of calls
// one at a time fill every slot, which the runner's report confirms in its
// Server Conns column.
package main

import (
	"context"

	"github.com/cloudwego/kitex/client"
	"github.com/cloudwego/kitex/pkg/klog"

	"go-rpc-benchmark/kitex_gen/echo"
	echosvr "go-rpc-benchmark/kitex_gen/echo/echo"
	"go-rpc-benchmark/runner"
	"go-rpc-benchmark/service"
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
	runner.Main("kitex_mux", func(opts runner.Options) (runner.Client, error) {
		cli, err := echosvr.NewClient("echo.kitex-mux",
			client.WithHostPorts(opts.Addr),
			client.WithMuxConnection(opts.Conns),
			client.WithRPCTimeout(opts.Timeout),
			client.WithConnectTimeout(opts.Timeout),
		)
		if err != nil {
			return nil, err
		}
		c := &kitexClient{cli: cli}
		for range 2 * opts.Conns {
			if _, err := c.Echo(0, service.ActionEcho, "ping"); err != nil {
				return nil, err
			}
		}
		return c, nil
	})
}
