// Command server is the kitex_mux benchmark server, kitex-benchmark's
// kitex-mux: Kitex Protobuf over TTHeader, with the multiplexed transport its
// client's mux connections need.
package main

import (
	"context"
	"log"
	"net"
	"time"

	"github.com/cloudwego/kitex/pkg/klog"
	"github.com/cloudwego/kitex/server"

	"go-rpc-benchmark/kitex_gen/echo"
	echosvr "go-rpc-benchmark/kitex_gen/echo/echo"
	"go-rpc-benchmark/service"
)

type echoImpl struct{}

func (*echoImpl) Echo(_ context.Context, req *echo.Request) (*echo.Response, error) {
	action, msg := service.Echo(req.Action, req.Msg)
	return &echo.Response{Action: action, Msg: msg}, nil
}

func main() {
	addr := service.Flags(":20007")
	klog.SetLevel(klog.LevelWarn)
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	svr := echosvr.NewServer(new(echoImpl),
		server.WithServiceAddr(tcpAddr),
		server.WithMuxTransport(),
		server.WithExitWaitTime(time.Second),
	)
	// Run returns on SIGINT or SIGTERM, which it waits for itself.
	if err := svr.Run(); err != nil {
		log.Fatal(err)
	}
}
