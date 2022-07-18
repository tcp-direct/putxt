package main

// meant to act as a simple example

import (
	"errors"
	"strings"
	"time"

	"git.tcp.direct/kayos/common/squish"

	termbin "git.tcp.direct/kayos/putxt"
)

type handler struct{}

func (h *handler) Ingest(data []byte) ([]byte, error) {
	var err error
	data, err = squish.Gunzip(data)
	if err != nil {
		return nil, err
	}
	if strings.ReplaceAll(string(data), "\n", "") == "ping" {
		println("got ping, sending pong...")
		return []byte("pong"), nil
	}
	println(string(data))
	return []byte("invalid request"), errors.New("invalid data")
}

func main() {
	td := termbin.NewTermDumpster(&handler{}).WithGzip().WithMaxSize(3 << 20).WithTimeout(5 * time.Second)
	err := td.Listen("127.0.0.1", "8888")
	if err != nil {
		println(err.Error())
		return
	}
}
