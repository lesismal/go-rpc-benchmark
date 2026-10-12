#!/bin/bash

# Builds every framework's server and client, and the report command, into
# output/bin.

if ! declare -p frameworks >/dev/null 2>&1; then
    . ./script/env.sh || { return 1 2>/dev/null || exit 1; }
fi

# -buildvcs=false: go build otherwise runs git to stamp the binaries, and
# fails wherever git refuses the checkout (e.g. run as root on a checkout
# another user owns). Nothing reads the stamp.
go_build() { go build -buildvcs=false "$@"; }

build_benchmark() {
    mkdir -p ./output/bin ./output/log ./output/report || return 1
    for f in "${frameworks[@]}"; do
        echo "build ${f} ..."
        go_build -o "./output/bin/${f}.server" "./frameworks/${f}/server" || return 1
        go_build -o "./output/bin/${f}.client" "./frameworks/${f}/client" || return 1
    done
    echo "build report ..."
    go_build -o ./output/bin/report ./report || return 1
    echo "build done"
}

build_benchmark || { return 1 2>/dev/null || exit 1; }
