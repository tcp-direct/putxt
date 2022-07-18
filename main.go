package termbin

import (
	"bytes"
	"fmt"
	"net"
	"sync"
	"time"

	"git.tcp.direct/kayos/common/squish"
	"github.com/yunginnanet/Rate5"
	"golang.org/x/tools/godoc/util"
	"inet.af/netaddr"
)

const (
	MessageRatelimited = "RATELIMIT_REACHED"
	MessageSizeLimited = "MAX_SIZE_EXCEEDED"
	MessageBinaryData  = "BINARY_DATA_REJECTED"
)

type TermDumpster struct {
	gzip    bool
	maxSize int64
	timeout time.Duration
	handler Handler
	log     Logger
	*rate5.Limiter
	*sync.Pool
}

type Logger interface {
	Printf(format string, v ...interface{})
}

type Handler interface {
	Ingest(data []byte) ([]byte, error)
}

type dummyLogger struct{}

func (dummyLogger) Printf(format string, v ...interface{}) {
	_, _ = fmt.Printf(format, v...)
}

func NewTermDumpster(handler Handler) *TermDumpster {
	td := &TermDumpster{
		maxSize: 3 << 20,
		timeout: 5 * time.Second,
		Limiter: rate5.NewStrictLimiter(60, 5),
		handler: handler,
		log:     dummyLogger{},
	}
	td.Pool = &sync.Pool{
		New: func() any { return new(bytes.Buffer) },
	}
	return td
}

func (td *TermDumpster) WithGzip() *TermDumpster {
	td.gzip = true
	return td
}

func (td *TermDumpster) WithMaxSize(size int64) *TermDumpster {
	td.maxSize = size
	return td
}

func (td *TermDumpster) WithTimeout(timeout time.Duration) *TermDumpster {
	td.timeout = timeout
	return td
}

func (td *TermDumpster) WithLogger(logger Logger) *TermDumpster {
	td.log = logger
	return td
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
	cipp, _ := netaddr.ParseIPPort(c.RemoteAddr().String())
	return &termbinClient{parent: td, addr: cipp.IP().String(), Conn: c}
}

func (c *termbinClient) write(data []byte) {
	if _, err := c.Write(data); err != nil {
		c.parent.log.Printf("termbinClient: %s error: %w", c.RemoteAddr().String(), err)
	}
}

func (c *termbinClient) writeString(data string) {
	c.write([]byte(data))
}

func (td *TermDumpster) accept(c net.Conn) {
	var (
		final  []byte
		length int64
	)
	client := td.newClient(c)
	if td.Check(client) {
		client.writeString(MessageRatelimited)
		client.Close()
		td.log.Printf("termbinClient: %s error: %s", client.RemoteAddr().String(), MessageRatelimited)
		return
	}
	buf := td.Pool.Get().(*bytes.Buffer)
	defer func() {
		_ = client.Close()
		buf.Reset()
		td.Put(buf)
	}()
readLoop:
	for {
		if err := client.SetReadDeadline(time.Now().Add(td.timeout)); err != nil {
			println(err.Error())
		}
		n, err := buf.ReadFrom(client)
		if err != nil {
			switch err.Error() {
			case "EOF":
				return
			case "read tcp " + client.LocalAddr().String() + "->" + client.RemoteAddr().String() + ": i/o timeout":
				break readLoop
			default:
				td.log.Printf("termbinClient: %s error: %w", client.RemoteAddr().String(), err)
				return
			}
		}
		length += n
		if length > td.maxSize {
			client.writeString(MessageRatelimited)
			return
		}
	}
	if !util.IsText(buf.Bytes()) {
		client.writeString(MessageBinaryData)
		return
	}
	if td.gzip {
		if final = squish.Gzip(buf.Bytes()); final == nil {
			final = buf.Bytes()
		}
	}
	resp, err := td.handler.Ingest(final)
	if err != nil {
		if resp == nil {
			client.writeString("INTERNAL_ERROR")
		}
		td.log.Printf("termbinClient: %s error: %w", client.RemoteAddr().String(), err)
	}
	client.Write(resp)
}

// Listen starts the TCP server
func (td *TermDumpster) Listen(addr string, port string) error {
	l, err := net.Listen("tcp", addr+":"+port)
	if err != nil {
		return err
	}
	defer l.Close()
	for {
		c, err := l.Accept()
		if err != nil {
			td.log.Printf("Error accepting connection: %s", err.Error())
			continue
		}
		go td.accept(c)
	}
}
