package internal

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
)

var (
	// ErrExistingWriter is returned from MsgWriter when there is already a
	// message io.WriterCloser that hasn't been properly closed yet.
	ErrExistingWriter = errors.New("netconf: existing message writer still open")

	// ErrInvalidIO is returned when a write or read operation is called on
	// message io.Reader or a message io.Writer when they are no longer valid.
	// (i.e a new reader or writer has been obtained)
	ErrInvalidIO = errors.New("netconf: read/write on invalid io")

	// ErrMalformedChunk represents a message that invalid as defined in the chunk
	// framing in RFC6242
	ErrMalformedChunk = errors.New("netconf: invalid chunk")
)

type frameReader interface {
	io.ReadCloser
	io.ByteReader
}

type frameWriter interface {
	io.WriteCloser
	isClosed() bool
}

// Framer is a wrapper used for transports that implement the framing defined in
// RFC6242.  This supports End-of-Message and Chucked framing methods and
// will move from End-of-Message to Chunked framing after the `Upgrade` method
// has been called.
type Framer struct {
	r io.Reader
	w io.Writer

	br *bufio.Reader
	bw *bufio.Writer

	curReader frameReader
	curWriter frameWriter

	upgraded bool
}

// NewFramer return a new Framer to be used against the given io.Reader and io.Writer.
func NewFramer(r io.Reader, w io.Writer) *Framer {
	return &Framer{
		r:  r,
		w:  w,
		br: bufio.NewReader(r),
		bw: bufio.NewWriter(w),
	}
}

// DebugCapture will copy all *framed* input/output to the given
// `io.Writers` for sent or recv data.  Either sent of recv can be nil to not
// capture any data.
// Useful for displaying to a screen or capturing to a file for debugging.
//
// This needs to be called before `MsgReader` or `MsgWriter`.
func (f *Framer) DebugCapture(in io.Writer, out io.Writer) {
	if f.curReader != nil ||
		f.curWriter != nil ||
		f.bw.Buffered() > 0 ||
		f.br.Buffered() > 0 {
		panic("debug capture added with active reader or writer")
	}

	if out != nil {
		f.w = io.MultiWriter(f.w, out)
		f.bw = bufio.NewWriter(f.w)
	}

	if in != nil {
		f.r = io.TeeReader(f.r, in)
		f.br = bufio.NewReader(f.r)
	}
}

// Upgrade will cause the Framer to switch from End-of-Message framing to
// Chunked framing.  This is usually called after netconf exchanged the hello
// messages.
func (f *Framer) Upgrade() {
	f.upgraded = true
}

// MsgReader returns a new io.Reader that is good for reading exactly one netconf
// message.
//
// Only one reader can be used at a time.  When this is called with an existing
// reader then the underlying reader is advanced to the start of the next message
// and invalidates the old reader before returning a new one.
func (f *Framer) MsgReader() (io.ReadCloser, error) {
	if f.upgraded {
		f.curReader = &chunkReader{r: f.br}
	} else {
		f.curReader = &eomReader{r: f.br}
	}
	return f.curReader, nil
}

// MsgWriter returns an io.WriterCloser that is good for writing exactly one
// netconf message.
//
// One writer can be used at one time and calling this function with an
// existing, unclosed,  writer will result in an error.
func (f *Framer) MsgWriter() (io.WriteCloser, error) {
	if f.curWriter != nil && !f.curWriter.isClosed() {
		return nil, ErrExistingWriter
	}

	if f.upgraded {
		f.curWriter = &chunkWriter{w: f.bw}
	} else {
		f.curWriter = &eomWriter{w: f.bw}
	}
	return f.curWriter, nil
}

var endOfChunks = []byte("\n##\n")

type chunkReader struct {
	r         *bufio.Reader
	chunkLeft int
}

func (r *chunkReader) readHeader() error {
	peeked, err := r.r.Peek(4)
	switch err {
	case nil:
		break
	case io.EOF:
		return io.ErrUnexpectedEOF
	default:
		return err
	}

	if _, err := r.r.Discard(2); err != nil {
		return err
	}

	if peeked[0] != '\n' || peeked[1] != '#' {
		return ErrMalformedChunk
	}

	if peeked[2] == '#' && peeked[3] == '\n' {
		if _, err := r.r.Discard(2); err != nil {
			return err
		}
		return io.EOF
	}

	var n int
	for {
		c, err := r.r.ReadByte()
		if err != nil {
			return err
		}

		if c == '\n' {
			break
		}
		if c < '0' || c > '9' {
			return ErrMalformedChunk
		}
		n = n*10 + int(c) - '0'
	}

	const maxChunk = 4294967295
	if n < 1 || n > maxChunk {
		return ErrMalformedChunk
	}

	r.chunkLeft = n
	return nil
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.r == nil {
		return 0, ErrInvalidIO
	}

	if r.chunkLeft <= 0 {
		if err := r.readHeader(); err != nil {
			return 0, err
		}
	}

	if len(p) > r.chunkLeft {
		p = p[:r.chunkLeft]
	}

	n, err := r.r.Read(p)
	r.chunkLeft -= n
	return n, err
}

func (r *chunkReader) ReadByte() (byte, error) {
	if r.r == nil {
		return 0, ErrInvalidIO
	}

	if r.chunkLeft <= 0 {
		if err := r.readHeader(); err != nil {
			return 0, err
		}
	}

	b, err := r.r.ReadByte()
	if err != nil {
		return 0, err
	}
	r.chunkLeft--
	return b, nil
}

// Close will read the rest of the frame and consume it including
// the end-of-frame markers if we haven't already done so.
func (r *chunkReader) Close() error {
	defer func() { r.r = nil }()
	return nil
}

type chunkWriter struct {
	w *bufio.Writer
}

func (w *chunkWriter) Write(p []byte) (int, error) {
	if w.w == nil {
		return 0, ErrInvalidIO
	}

	if _, err := fmt.Fprintf(w.w, "\n#%d\n", len(p)); err != nil {
		return 0, err
	}

	return w.w.Write(p)
}

func (w *chunkWriter) Close() error {
	defer func() { w.w = nil }()
	if _, err := w.w.Write(endOfChunks); err != nil {
		return err
	}
	return w.w.Flush()
}

func (w *chunkWriter) isClosed() bool { return w.w == nil }

var endOfMsg = []byte("]]>]]>")

type eomReader struct {
	r *bufio.Reader
}

func (r *eomReader) Read(p []byte) (int, error) {
	for i := 0; i < len(p); i++ {
		b, err := r.ReadByte()
		if err != nil {
			return i, err
		}
		p[i] = b
	}
	return len(p), nil
}

func (r *eomReader) ReadByte() (byte, error) {
	if r.r == nil {
		return 0, ErrInvalidIO
	}

	b, err := r.r.ReadByte()
	if err != nil {
		if err == io.EOF {
			return b, io.ErrUnexpectedEOF
		}
		return b, err
	}

	if b == endOfMsg[0] {
		peeked, err := r.r.Peek(len(endOfMsg) - 1)
		if err != nil {
			if err == io.EOF {
				return 0, io.ErrUnexpectedEOF
			}
			return 0, err
		}

		if bytes.Equal(peeked, endOfMsg[1:]) {
			if _, err := r.r.Discard(len(endOfMsg) - 1); err != nil {
				return 0, err
			}

			return 0, io.EOF
		}
	}

	return b, nil
}

// Close will read the rest of the frame and consume it including
// the end-of-frame marker.
func (r *eomReader) Close() error {
	defer func() { r.r = nil }()

	var err error
	for err == nil {
		_, err = r.ReadByte()
		if err == io.EOF {
			return nil
		}
	}
	return err
}

type eomWriter struct {
	w *bufio.Writer
}

func (w *eomWriter) Write(p []byte) (int, error) {
	if w.w == nil {
		return 0, ErrInvalidIO
	}
	return w.w.Write(p)
}

func (w *eomWriter) Close() error {
	defer func() { w.w = nil }()

	if err := w.w.WriteByte('\n'); err != nil {
		return err
	}

	if _, err := w.w.Write(endOfMsg); err != nil {
		return err
	}

	return w.w.Flush()
}

func (w *eomWriter) isClosed() bool { return w.w == nil }
