package termbin

import (
	"net"
	"strconv"
	"time"

	"github.com/yunginnanet/Rate5"
	"golang.org/x/tools/godoc/util"
	ipa "inet.af/netaddr"
)

var (
	// Msg is a channel for status information during concurrent server operations
	Msg chan Message
	// Reply is a channel to receive messages to send our client upon completion
	Reply chan Message
	// Timeout is the read timeout for incoming data in seconds
	Timeout = 4
	// MaxSize for incoming data (default: 3 << 30 or 3 MiB)
	MaxSize = 3 << 20
	// Gzip is our toggle for enabling or disabling gzip compression
	Gzip = true
	// UseChannel enables or disables sending status messages through an exported channel, otherwise debug messages will be printed.
	UseChannel = true
	// Rater is a ratelimiter powered by Rate5.
	Rater *rate5.Limiter
)

type MessageType uint8

const (
	Error MessageType = iota
	IncomingData
	NewConn
	Finish
	Debug
	Final
	ReturnURL
	ReturnError
)

// Message is a struct that encapsulates status messages to send down the Msg channel
type Message struct {
	Type    MessageType
	RAddr   string
	Content string
	Bytes   []byte
	Size    int
}

func MsgTypeToStr(m MessageType) (s string) {
	switch m {
	case Error:
		s = "ERROR"
	case IncomingData:
		s = "INCOMING_DATA"
	case Finish:
		s = "FINISH"
	case Debug:
		s = "DEBUG"
	case Final:
		s = "FINAL"
	case NewConn:
		s = "NEW_CONN"
	default:
		s = "INVALID"
	}
	return
}

// Identity is used for rate5's Identity implementation.
type Identity struct {
	Actual net.IP
	Addr   net.Addr
}

func UseGzip(toggle bool) {
	Gzip = toggle
}

// UniqueKey implements rate5's Identity interface.
func (t *Identity) UniqueKey() string {
	if t.Addr != nil {
		t.Actual = net.ParseIP(ipa.MustParseIPPort(t.Addr.String()).String())
	}
	return t.Actual.String()
}

func init() {
	Msg = make(chan Message)
	Reply = make(chan Message)
	Rater = rate5.NewDefaultLimiter()
}

func termStatus(m Message) {
	if UseChannel {
		Msg <- m
	} else {
		var suffix string
		if m.Size != 0 {
			suffix = " (" + strconv.Itoa(m.Size) + ")"
		}
		println("[" + m.RAddr + "][" + MsgTypeToStr(m.Type) + "] " + m.Content + suffix)
	}
}

func serve(c net.Conn) {
	termStatus(Message{Type: NewConn, RAddr: c.RemoteAddr().String()})
	var (
		data   []byte
		final  []byte
		length int
		done   bool
	)

	if Rater.Check(&Identity{Addr: c.RemoteAddr()}) {
		if _, err := c.Write([]byte("RATELIMIT_REACHED")); err != nil {
			println(err.Error())
		}
		c.Close()
		termStatus(Message{Type: Error, RAddr: c.RemoteAddr().String(), Content: "RATELIMITED"})
		return
	}

	for {
		if err := c.SetReadDeadline(time.Now().Add(time.Duration(Timeout) * time.Second)); err != nil {
			println(err.Error())
		}
		buf := make([]byte, MaxSize)
		n, err := c.Read(buf)
		if err != nil {
			switch err.Error() {
			case "EOF":
				c.Close()
				termStatus(Message{Type: Error, RAddr: c.RemoteAddr().String(), Content: "EOF"})
				return
			case "read tcp " + c.LocalAddr().String() + "->" + c.RemoteAddr().String() + ": i/o timeout":
				termStatus(Message{Type: Finish, RAddr: c.RemoteAddr().String(), Size: length, Content: "TIMEOUT"})
				done = true
			default:
				c.Close()
				termStatus(Message{Type: Error, Size: length, Content: err.Error()})
				return
			}
		}
		if done {
			break
		}
		length += n
		if length > MaxSize {
			termStatus(Message{Type: Error, RAddr: c.RemoteAddr().String(), Size: length, Content: "MAX_SIZE_EXCEEDED"})
			if _, err := c.Write([]byte("MAX_SIZE_EXCEEDED")); err != nil {
				println(err.Error())
			}
			c.Close()
			return
		}
		data = append(data, buf[:n]...)
		termStatus(Message{Type: IncomingData, RAddr: c.RemoteAddr().String(), Size: length})
	}
	if !util.IsText(data) {
		termStatus(Message{Type: Error, RAddr: c.RemoteAddr().String(), Content: "BINARY_DATA_REJECTED"})
		if _, err := c.Write([]byte("BINARY_DATA_REJECTED")); err != nil {
			println(err.Error())
		}
		c.Close()
		return
	}

	final = data

	if Gzip {
		var gzerr error
		if final, gzerr = gzipCompress(data); gzerr == nil {
			diff := len(data) - len(final)
			termStatus(Message{Type: Debug, RAddr: c.RemoteAddr().String(), Content: "GZIP_RESULT", Size: diff})
		} else {
			termStatus(Message{Type: Error, RAddr: c.RemoteAddr().String(), Content: "GZIP_ERROR: " + gzerr.Error()})
		}
	}

	termStatus(Message{Type: Final, RAddr: c.RemoteAddr().String(), Size: len(final), Bytes: final, Content: "SUCCESS"})
	url := <-Reply
	switch url.Type {
	case ReturnURL:
		c.Write([]byte(url.Content))
	case ReturnError:
		c.Write([]byte("ERROR: " + url.Content))
	default:
		//
	}
	c.Close()
}

// Listen starts the TCP server
func Listen(addr string, port string) error {
	l, err := net.Listen("tcp", addr+":"+port)
	if err != nil {
		return err
	}
	defer l.Close()
	for {
		c, err := l.Accept()
		if err != nil {
			termStatus(Message{Type: Error, Content: err.Error()})
		}
		go serve(c)
	}
}
