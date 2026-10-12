// Command server is the arpc benchmark server, encoding protobuf.
package main

import (
	"log"

	"github.com/lesismal/arpc"
	"github.com/lesismal/arpc/codec"
	alog "github.com/lesismal/arpc/log"

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
	addr := service.Flags(":20002")
	alog.SetLevel(alog.LevelNone)
	codec.DefaultCodec = pbcodec.Codec{}

	svr := arpc.NewServer()
	// As kitex-benchmark runs it: pooled message buffers, and every call
	// answered off the connection's read goroutine (arpc's default).
	svr.Handler.EnablePool(true)
	svr.Handler.SetAsyncResponse(true)
	svr.Handler.Handle("Echo", onEcho)

	go func() {
		service.WaitSignal()
		svr.Stop()
	}()
	if err := svr.Run(addr); err != nil {
		log.Println(err)
	}
}
