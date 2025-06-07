#!/bin/sh -eux
             # In an ideal future: ↓
OSes="linux" #OSes="android darwin dragonfly freebsd linux nacl netbsd openbsd plan9 solaris"
ARCHs="amd64 arm64 riscv64"

for GOOS in $OSes; do
    export GOOS
    for GOARCH in $ARCHs; do
        export GOARCH                                       # In an ideal future: ↓
        go build -o "./dbin_$GOARCH"                        # go build -o "./dbin_$GOARCH_$GOOS"
        strip -sx --strip-all-gnu "./dbin_$GOARCH"          # strip -sx "./dbin_$GOARCH_$GOOS"
        cp "./dbin_$GOARCH" "./dbin_$GOARCH.upx"            # cp "./dbin_$GOARCH" "./dbin_$GOARCH_$GOOS.upx"
        upx "./dbin_$GOARCH.upx" || rm "./dbin_$GOARCH.upx" # upx "./dbin_$GOARCH_$GOOS.upx"
    done
done
