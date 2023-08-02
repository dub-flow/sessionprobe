#!/usr/bin/env sh

# get the current version of the tool from `./VERSION`
VERSION=$(cat VERSION)

FLAGS="-X main.AppVersion=$VERSION -s -w"

rm -rf releases
mkdir -p releases

# build for Windows
GOOS=windows GOARCH=amd64 go build -ldflags="$FLAGS" -trimpath
mv sessionprobe.exe releases/sessionprobe-windows-amd64.exe

# build for M1 Macs (arm64)
GOOS=darwin GOARCH=arm64 go build -ldflags="$FLAGS" -trimpath
mv sessionprobe releases/sessionprobe-mac-arm64

# build for Intel Macs (amd64)
GOOS=darwin GOARCH=amd64 go build -ldflags="$FLAGS" -trimpath
mv sessionprobe releases/sessionprobe-mac-amd64

# build for x64 Linux (amd64)
GOOS=linux GOARCH=amd64 go build -ldflags="$FLAGS" -trimpath
mv sessionprobe releases/sessionprobe-linux-amd64