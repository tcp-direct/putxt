package termbin

import (
	"bytes"
	"net"
	"sync"
	"time"

	"github.com/yunginnanet/Rate5"
	"golang.org/x/tools/godoc/util"
	"inet.af/netaddr"
)

type TermDumpster struct {
	gzip    bool
	maxSize int64
	timeout time.Duration
	handler Handler
	*rate5.Limiter
	*sync.Pool
}

type Handler interface {
	Ingest(data []byte) ([]byte, error)
}

func NewTermDumpster(handler Handler) *TermDumpster {
	td := &TermDumpster{
		maxSize: 3 << 20,
		timeout: 5 * time.Second,
		Limiter: rate5.NewStrictLimiter(60, 5),
		handler: handler,
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

type client struct {
	addr string
	net.Conn
}

func (c *client) UniqueKey() string {
	return c.addr
}

func newClient(c net.Conn) *client {
	cipp, _ := netaddr.ParseIPPort(c.RemoteAddr().String())
	return &client{addr: cipp.IP().String(), Conn: c}
}

func (td *TermDumpster) accept(c net.Conn) {
	var (
		final  []byte
		length int64
	)

	client := newClient(c)
	if td.Check(client) {
		if _, err := client.Write([]byte("RATELIMIT_REACHED")); err != nil {
			println(err.Error())
		}
		client.Close()
		// termStatus(Message{Type: Error, Remote: client.RemoteAddr().String(), Content: "RATELIMITED"})
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
				// termStatus(td.msg(EOF)
				return
			case "read tcp " + client.LocalAddr().String() + "->" + client.RemoteAddr().String() + ": i/o timeout":
				// termStatus(Message{Type: Finish, Size: length, Content: "TIMEOUT"})
				break readLoop
			default:
				// termStatus(Message{Type: Error, Size: length, Content: err.Error()})
				return
			}
		}

		length += n
		if length > td.maxSize {
			// termStatus(Message{Type: Error, Remote: client.RemoteAddr().String(), Size: length, Content: "MAX_SIZE_EXCEEDED"})
			if _, err := client.Write([]byte("MAX_SIZE_EXCEEDED")); err != nil {
				println(err.Error())
			}
			return
		}
		// termStatus(Message{Type: IncomingData, Remote: client.RemoteAddr().String(), Size: length})
	}
	if !util.IsText(buf.Bytes()) {
		// termStatus(Message{Type: Error, Remote: client.RemoteAddr().String(), Content: "BINARY_DATA_REJECTED"})
		if _, err := client.Write([]byte("BINARY_DATA_REJECTED")); err != nil {
			println(err.Error())
		}
		return
	}

	if td.gzip {
		var gzerr error
		if final, gzerr = gzipCompress(buf.Bytes()); gzerr == nil {
			// diff := len(buf.Bytes()) - len(final)
			// termStatus(Message{Type: Debug, Remote: client.RemoteAddr().String(), Content: "GZIP_RESULT", Size: diff})
		} else {
			final = buf.Bytes()
			// termStatus(Message{Type: Error, Remote: client.RemoteAddr().String(), Content: "GZIP_ERROR: " + gzerr.Error()})
		}
	}

	// termStatus(Message{Type: Final, Remote: client.RemoteAddr().String(), Size: len(final), Data: final, Content: "SUCCESS"})
	resp, err := td.handler.Ingest(final)
	if err != nil {
		// termStatus(Message{Type: Error, Remote: client.RemoteAddr().String(), Content: err.Error()})
		if resp == nil {
			_, _ = client.Write([]byte("INTERNAL_ERROR"))
		}
		println(err.Error())
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
			// termStatus(Message{Type: Error, Content: err.Error()})
		}
		go td.accept(c)
	}
}
