package putxt

import (
	"time"

	"git.tcp.direct/kayos/common/pool"
	"github.com/yunginnanet/Rate5"
)

type TermDumpster struct {
	gzip    bool
	maxSize int64
	timeout time.Duration
	handler Handler
	log     Logger
	Pool    pool.BufferFactory
	*rate5.Limiter
}

func NewTermDumpster(handler Handler) *TermDumpster {
	td := &TermDumpster{
		maxSize: 3 << 20,
		timeout: 5 * time.Second,
		Limiter: rate5.NewStrictLimiter(60, 10),
		handler: handler,
		log:     dummyLogger{},
		Pool:    pool.NewBufferFactory(),
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
