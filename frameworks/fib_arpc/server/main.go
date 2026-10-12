// Command server is the benchmark server of fib's arpc
// (github.com/lesismal/fib/arpc): arpc's protocol on fib's engine, encoding
// protobuf.
package main

import (
	"log"

	fib "github.com/lesismal/fib"
	"github.com/lesismal/fib/arpc"

	"go-rpc-benchmark/codec/pbcodec"
	"go-rpc-benchmark/pb/echo"
	"go-rpc-benchmark/service"
)

func onEcho(ctx *arpc.Context) {
	req := &echo.Request{}
	if err := ctx.Bind(req); err != nil {
		ctx.Error(err)
		return
	}
	action, msg := service.Echo(req.Action, req.Msg)
	ctx.Write(&echo.Response{Action: action, Msg: msg})
}

func main() {
	addr := service.Flags(":20005")
	arpc.DefaultCodec = pbcodec.Codec{}

	svr := arpc.NewServer()
	// Every request is handled asynchronously, off the engine worker that
	// read it: fib's arpc runs each one as a task on the engine's handler
	// pool. That is its default, set here for every route and for Echo
	// itself, so that it does not hang on one.
	svr.Handler.SetAsyncResponse(true)
	svr.Handler.Handle("Echo", onEcho, true)

	config := fib.DefaultConfig()
	config.Addr = addr
	go func() {
		service.WaitSignal()
		svr.Stop()
	}()
	if err := svr.Serve(config); err != nil {
		log.Println(err)
	}
}
