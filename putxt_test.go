package putxt

// meant to act as a simple example

import (
	"bytes"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"git.tcp.direct/kayos/common/entropy"
	"git.tcp.direct/kayos/common/squish"
)

type handler struct {
	t      *testing.T
	needle []byte
}

func (h *handler) Ingest(data []byte) ([]byte, error) {
	var err error
	data, err = squish.Gunzip(data)
	if err != nil {
		return nil, err
	}
	if strings.ReplaceAll(string(data), "\n", "") == string(h.needle) {
		h.t.Log("got needle, echoing it back...")
		return h.needle, nil
	}
	return []byte("invalid request"), errors.New("data does not match generated test needle: " + string(data))
}

func TestPutxt(t *testing.T) {
	socketPath := t.TempDir() + "/putxt.sock"
	testHandler := &handler{t: t, needle: []byte(entropy.RandStr(4096))}
	td := NewTermDumpster(testHandler).WithGzip().WithMaxSize(3 << 20).WithTimeout(5 * time.Second)
	var errChan = make(chan error)
	go func() {
		err := td.ListenUnixSocket(socketPath)
		if err != nil {
			errChan <- err
		}
	}()
	select {
	case err := <-errChan:
		t.Fatalf("failed to listen on unix socket: %v", err.Error())
	default:
		time.Sleep(10 * time.Millisecond)
		c, err := net.Dial("unix", socketPath)
		if err != nil {
			t.Fatalf("failed to connect to unix socket: %v", err.Error())
		}
		defer c.Close()
		res := make(chan []byte)
		go func() {
			buf := make([]byte, 4096)
			n, err := c.Read(buf)
			if err != nil {
				errChan <- err
			}
			res <- buf[:n]
		}()
		_, err = c.Write(testHandler.needle)
		if err != nil {
			t.Fatalf("failed to write to unix socket: %v", err.Error())
		}

		buf := <-res
		if !bytes.Equal(buf, testHandler.needle) {
			t.Fatalf("expected %s, got %s", testHandler.needle, string(buf))
		}
	}
}
