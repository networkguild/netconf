package netconf

import (
	"fmt"
	"io"
	"testing"
)

type testServer struct {
	t   *testing.T
	in  chan []byte
	out chan []byte
}

func newTestServer(t *testing.T) *testServer {
	return &testServer{
		t:   t,
		in:  make(chan []byte),
		out: make(chan []byte),
	}
}

func (s *testServer) handle(r io.ReadCloser, w io.WriteCloser) {
	in, err := io.ReadAll(r)
	if err != nil {
		panic(fmt.Sprintf("testerver: failed to read incomming message: %v", err))
	}
	s.t.Logf("testserver recv: %s", in)
	go func() { s.in <- in }()

	out, ok := <-s.out
	if !ok {
		panic("testserver: no message to send")
	}
	s.t.Logf("testserver send: %s", out)

	_, err = w.Write(out)
	if err != nil {
		panic(fmt.Sprintf("testserver: failed to write message: %v", err))
	}

	if err := w.Close(); err != nil {
		panic("testserver: failed to close outbound message")
	}
}

func (s *testServer) queueResp(p []byte)         { go func() { s.out <- p }() }
func (s *testServer) queueRespString(str string) { s.queueResp([]byte(str)) }
func (s *testServer) popReq() ([]byte, error) {
	msg, ok := <-s.in
	if !ok {
		return nil, fmt.Errorf("testserver: no message to read")
	}
	return msg, nil
}

func (s *testServer) transport() *testTransport { return newTestTransport(s.handle) }

type testTransport struct {
	handler func(r io.ReadCloser, w io.WriteCloser)
	out     chan io.ReadCloser
	// msgReceived, msgSent int
}

func newTestTransport(handler func(r io.ReadCloser, w io.WriteCloser)) *testTransport {
	return &testTransport{
		handler: handler,
		out:     make(chan io.ReadCloser),
	}
}

func (s *testTransport) MsgReader() (io.ReadCloser, error) {
	return <-s.out, nil
}

func (s *testTransport) MsgWriter() (io.WriteCloser, error) {
	inr, inw := io.Pipe()
	outr, outw := io.Pipe()

	go func() { s.out <- outr }()
	go s.handler(inr, outw)

	return inw, nil
}

func (s *testTransport) Close() error {
	if len(s.out) > 0 {
		return fmt.Errorf("testtransport: remaining outboard messages not sent at close")
	}
	return nil
}
