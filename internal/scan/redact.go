package scan

import (
	"bytes"
	"io"
	"regexp"
	"sync"
)

// A line is the unit redaction rules match on, so an unterminated line has to
// be held. The observed process decides when to emit a newline, so the hold is
// bounded and the buffer is flushed as a fragment once it grows past this.
const maxRedactBuffer = 64 * 1024

// Streaming output arrives one line at a time, so the multi-line block rule
// RedactString applies can never see a whole key. These bound the block by
// state instead, and stay broader than the single-line AP-SECRET-003 rule so
// an unrecognised key type still has its body suppressed.
var (
	beginPrivateKey = regexp.MustCompile(`-----BEGIN [^-\n]*PRIVATE KEY( BLOCK)?-----`)
	endPrivateKey   = regexp.MustCompile(`-----END [^-\n]*PRIVATE KEY( BLOCK)?-----`)
)

// NewRedactingWriter applies the secret rules to output on its way to disk. A
// recorded command is a process AgentProof does not control, so its output is
// the one evidence file that would otherwise be copied verbatim.
func NewRedactingWriter(sink io.Writer) io.WriteCloser {
	return &redactingWriter{sink: sink}
}

type redactingWriter struct {
	// A recorded command's stdout and stderr are copied into one log by
	// separate goroutines, so the partial line and the key-block state below
	// are shared across streams.
	mu    sync.Mutex
	sink  io.Writer
	buf   []byte
	inKey bool
	err   error
}

func (w *redactingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return 0, w.err
	}
	w.buf = append(w.buf, p...)
	for {
		end := bytes.IndexByte(w.buf, '\n')
		if end < 0 {
			break
		}
		line := string(w.buf[:end])
		w.buf = w.buf[end+1:]
		if err := w.emit(line, "\n"); err != nil {
			return 0, err
		}
	}
	if len(w.buf) >= maxRedactBuffer {
		// A secret straddling this boundary survives. That is the price of
		// refusing to let the observed process choose AgentProof's memory use,
		// and it only applies to lines longer than the bound.
		fragment := string(w.buf)
		w.buf = w.buf[:0]
		if err := w.emit(fragment, ""); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Close flushes output that never ended in a newline. Without it a trailing
// line would be dropped, which is evidence loss rather than redaction.
func (w *redactingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return w.err
	}
	if len(w.buf) == 0 {
		return nil
	}
	line := string(w.buf)
	w.buf = nil
	return w.emit(line, "")
}

func (w *redactingWriter) emit(line, terminator string) error {
	if w.inKey {
		// Key material is suppressed entirely: the block already announced
		// itself with one marker, and emitting the body after that marker
		// would be a redaction that leaks anyway.
		if endPrivateKey.MatchString(line) {
			w.inKey = false
		}
		return nil
	}
	if beginPrivateKey.MatchString(line) && !endPrivateKey.MatchString(line) {
		w.inKey = true
		return w.write("[REDACTED:AP-SECRET-003]" + terminator)
	}
	return w.write(RedactString(line) + terminator)
}

func (w *redactingWriter) write(value string) error {
	if _, err := io.WriteString(w.sink, value); err != nil {
		w.err = err
		return err
	}
	return nil
}
