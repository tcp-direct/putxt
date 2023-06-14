package putxt

import (
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"syscall"
	"time"

	"git.tcp.direct/kayos/common/squish"
	"golang.org/x/tools/godoc/util"
)

type Handler interface {
	Ingest(data []byte) ([]byte, error)
}

type termbinClient struct {
	parent *TermDumpster
	addr   string
	net.Conn
}

func (c *termbinClient) UniqueKey() string {
	return c.addr
}

func (td *TermDumpster) newClient(c net.Conn) *termbinClient {
	cipp, _ := netip.ParseAddrPort(c.RemoteAddr().String())
	return &termbinClient{parent: td, addr: cipp.Addr().String(), Conn: c}
}

func (c *termbinClient) write(data []byte) {
	if _, err := c.Write(data); err != nil {
		c.parent.log.Printf("termbinClient: %s error: %s", c.RemoteAddr().String(), err.Error())
	}
}

func (c *termbinClient) writeString(data string) {
	c.write([]byte(data))
}

func (td *TermDumpster) accept(c net.Conn) {
	var final []byte
	client := td.newClient(c)
	if td.Check(client) {
		client.writeString(MessageRatelimited)
		_ = client.Close()
		td.log.Printf("termbinClient: %s error: %s", client.RemoteAddr().String(), MessageRatelimited)
		return
	}
	buf := td.Pool.Get()
	defer func() {
		_ = client.Close()
		td.Pool.MustPut(buf)
	}()
	if err := client.SetReadDeadline(time.Now().Add(td.timeout)); err != nil {
		td.log.Printf("failed to set read deadline: %s error: %s", client.RemoteAddr().String(), err.Error())
		return
	}
readLoop:
	for {
		_, err := buf.ReadFrom(client)
		if err != nil {
			switch {
			case errors.Is(err, io.EOF):
				break readLoop
			case os.IsTimeout(err):
				break readLoop
			default:
				td.log.Printf("termbinClient: %s error: %s", client.RemoteAddr().String(), err.Error())
				return
			}
		}
		if int64(buf.Len()) > td.maxSize {
			client.writeString(MessageSizeLimited)
			return
		}
	}
	if !util.IsText(buf.Bytes()) {
		client.writeString(MessageBinaryData)
		return
	}
	if td.gzip {
		if final = squish.Gzip(buf.Bytes()); final == nil {
			client.writeString(MessageInternalError)
			td.log.Printf(
				"termbinClient: %s error: gzipping data provided empty result",
				client.RemoteAddr().String())
			return
		}
	}
	resp, err := td.handler.Ingest(final)
	if err != nil {
		if resp == nil {
			client.writeString(MessageInternalError)
		}
		td.log.Printf("termbinClient: %s error: %s", client.RemoteAddr().String(), err.Error())
		return
	}
	_, err = client.Write(resp)
	if err != nil {
		td.log.Printf("termbinClient: %s failed to deliver result: %s", client.RemoteAddr().String(), err.Error())
	}
}

func (td *TermDumpster) handle(l net.Listener) {
	defer l.Close()
	for {
		c, acceptErr := l.Accept()
		if acceptErr != nil {
			td.log.Printf("Error accepting connection: %s", acceptErr.Error())
			continue
		}
		go td.accept(c)
	}
}

// Listen starts the TCP server
func (td *TermDumpster) Listen(addr string, port string) error {
	l, err := net.Listen("tcp", addr+":"+port)
	if err != nil {
		return err
	}
	td.handle(l)
	return nil
}

// ListenUnixSocket starts the unix socket listener
func (td *TermDumpster) ListenUnixSocket(path string) error {
	unixAddr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return err
	}
	_ = syscall.Unlink(path)
	mask := syscall.Umask(0o077)
	unixListener, err := net.ListenUnix("unix", unixAddr)
	syscall.Umask(mask)
	if err != nil {
		return err
	}
	td.handle(unixListener)
	return nil
}
