package logging

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"github.com/go-kratos/kratos/v2/log"
)

type stdLogger struct {
	w         io.Writer
	isDiscard bool
	mu        sync.Mutex
	pool      *sync.Pool
}

// NewStdLogger new a logger with writer.
func NewStdLogger(w io.Writer) log.Logger {
	l := &stdLogger{
		w:         w,
		isDiscard: w == io.Discard,
		pool: &sync.Pool{
			New: func() any {
				return new(bytes.Buffer)
			},
		},
	}
	return l
}

func (l *stdLogger) Log(level log.Level, keyvals ...any) error {
	if l.isDiscard || len(keyvals) == 0 {
		return nil
	}
	if (len(keyvals) & 1) == 1 {
		keyvals = append(keyvals, "KEYVALS UNPAIRED")
	}
	buf := l.pool.Get().(*bytes.Buffer)
	defer l.pool.Put(buf)

	buf.WriteString("[" + level.String() + "]\t")
	for i := 0; i < len(keyvals); i += 2 {
		_, _ = fmt.Fprintf(buf, " %s=%v", keyvals[i], keyvals[i+1])
	}
	buf.WriteByte('\n')
	defer buf.Reset()

	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := l.w.Write(buf.Bytes())
	return err
}

func (l *stdLogger) Close() error {
	return nil
}
