#!/bin/bash
# Regenerates the code of proto/echo.proto; the output is committed, so that
# neither a build nor the benchmark needs these generators:
#
#   pb/echo        google.golang.org/protobuf messages and the grpc-go service
#                  (protoc-gen-go, protoc-gen-go-grpc), which arpc, rpcx, grpc
#                  and fib's arpc and grpc use
#   pb/fibgrpc     the same service for github.com/lesismal/fib/grpc: the
#                  grpc-go one with its imports pointed at fib's packages and
#                  its messages at pb/echo
#   kitex_gen      kitex's own messages and service (the kitex tool), which
#                  wants its go_package under kitex_gen
#
# Needs protoc, and on PATH:
#   go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
#   go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
#   go install github.com/cloudwego/kitex/tool/cmd/kitex@latest
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
module=go-rpc-benchmark

protoc -I proto \
    --go_out=. --go_opt=module=$module \
    --go-grpc_out=. --go-grpc_opt=module=$module \
    proto/echo.proto

mkdir -p pb/fibgrpc
perl -pe '
    s#^package echo$#package fibgrpc#;
    s#"google.golang.org/grpc#"github.com/lesismal/fib/grpc#;
    s#^(\tcontext "context")$#$1\n\techo "'$module'/pb/echo"#;
    s#\*(Request|Response)\b#*echo.$1#g;
    s#new\((Request|Response)\)#new(echo.$1)#g;
' pb/echo/echo_grpc.pb.go > pb/fibgrpc/echo_grpc.pb.go
gofmt -w pb/fibgrpc

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
sed "s#$module/pb/echo#$module/kitex_gen/echo#" proto/echo.proto > "$tmp/echo.proto"
rm -rf kitex_gen
kitex -module $module -gen-path kitex_gen "$tmp/echo.proto"
