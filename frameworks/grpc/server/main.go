// Command server is the grpc-go benchmark server.
package main

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"

	"go-rpc-benchmark/pb/echo"
	"go-rpc-benchmark/service"
)

type echoServer struct {
	echo.UnimplementedEchoServer
}

func (echoServer) Echo(_ context.Context, req *echo.Request) (*echo.Response, error) {
	action, msg := service.Echo(req.Action, req.Msg)
	return &echo.Response{Action: action, Msg: msg}, nil
}

func main() {
	addr := service.Flags(":20004")
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	svr := grpc.NewServer()
	echo.RegisterEchoServer(svr, echoServer{})
	go func() {
		service.WaitSignal()
		svr.Stop()
	}()
	if err := svr.Serve(ln); err != nil {
		log.Println(err)
	}
}
