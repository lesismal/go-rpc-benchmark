// Command server is the rpcx benchmark server, encoding protobuf.
package main

import (
	"context"
	"log"

	rlog "github.com/smallnest/rpcx/log"
	"github.com/smallnest/rpcx/server"

	"go-rpc-benchmark/pb/echo"
	"go-rpc-benchmark/service"
)

// Echo is the service, registered under its type name: Echo.Echo.
type Echo struct{}

// Echo answers a call.
func (Echo) Echo(_ context.Context, req *echo.Request, rsp *echo.Response) error {
	rsp.Action, rsp.Msg = service.Echo(req.Action, req.Msg)
	return nil
}

func main() {
	addr := service.Flags(":20003")
	rlog.SetDummyLogger()

	svr := server.NewServer()
	if err := svr.Register(new(Echo), ""); err != nil {
		log.Fatal(err)
	}
	go func() {
		service.WaitSignal()
		svr.Close()
	}()
	if err := svr.Serve("tcp", addr); err != nil && err != server.ErrServerClosed {
		log.Println(err)
	}
}
