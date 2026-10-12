// Command server is the benchmark server of fib's grpc
// (github.com/lesismal/fib/grpc): gRPC over an HTTP/2 transport of fib's own,
// on fib's engine.
package main

import (
	"context"
	"log"

	fib "github.com/lesismal/fib"
	"github.com/lesismal/fib/grpc"

	"go-rpc-benchmark/codec/pbcodec"
	"go-rpc-benchmark/pb/echo"
	"go-rpc-benchmark/pb/fibgrpc"
	"go-rpc-benchmark/service"
)

type echoServer struct {
	fibgrpc.UnimplementedEchoServer
}

func (echoServer) Echo(_ context.Context, req *echo.Request) (*echo.Response, error) {
	action, msg := service.Echo(req.Action, req.Msg)
	return &echo.Response{Action: action, Msg: msg}, nil
}

func main() {
	addr := service.Flags(":20006")
	// fib's grpc depends on no protobuf package; the program gives it one.
	grpc.RegisterCodec(pbcodec.Codec{})

	svr := grpc.NewServer()
	fibgrpc.RegisterEchoServer(svr, echoServer{})

	config := fib.DefaultConfig()
	config.Addr = addr
	engine, err := fib.Bind(config, svr)
	if err != nil {
		log.Fatal(err)
	}
	defer engine.Close()
	go func() {
		service.WaitSignal()
		engine.Stop()
	}()
	if err := engine.Run(); err != nil {
		log.Println(err)
	}
}
