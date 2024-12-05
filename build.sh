#!/bin/sh

rm go.mod go.sum
go mod init quester
go mod tidy
go build .
