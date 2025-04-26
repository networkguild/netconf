package transport

import (
	"io"
)

// Transport is used for a netconf.ISession to talk to the device.  It is message
// oriented to allow for framing and other details to happen on a per-message
// basis.
type Transport interface {
	// MsgReader returns a new io.Reader to read a single netconf message. There
	// can only be a single reader for transport at a time.  Obtaining a new
	// reader should advance the stream to the start of the next message.`
	MsgReader() (io.ReadCloser, error)

	// MsgWriter returns a new io.WriteCloser to write a single netconf message.
	// After writing a message the writer must be closed. Implementers should
	// make sure only a single writer can be obtained and return an error if
	// multiple writers are attempted.
	MsgWriter() (io.WriteCloser, error)

	// Close will close the underlying transport.
	Close() error
}
