package gemini

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
)

// ListenAndServe create a TCP server on the specified address and pass
// new connections to the given handler.
// Each request is handled in a separate goroutine.
func ListenAndServe(addr, certFile, keyFile string, handler Handler) error {
	if addr == "" {
		addr = "127.0.0.1:1965"
	}

	listener, err := listen(addr, certFile, keyFile)
	if err != nil {
		return err
	}

	err = serve(listener, handler)
	if err != nil {
		return err
	}

	err = listener.Close()
	if err != nil {
		return fmt.Errorf("failed to close the listener: %v", err)
	}

	return nil
}

func listen(addr, certFile, keyFile string) (net.Listener, error) {
	cer, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificates: %v", err)
	}

	config := &tls.Config{
		Certificates:       []tls.Certificate{cer},
		InsecureSkipVerify: true,
		ClientAuth:         tls.RequestClientCert,
	}
	ln, err := tls.Listen("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("failed to listen: %v", err)
	}

	return ln, nil
}

func serve(listener net.Listener, handler Handler) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		tlsConn := conn.(*tls.Conn)
		go handleConnection(tlsConn, handler)
	}
}

func handleConnection(conn *tls.Conn, handler Handler) {
	defer conn.Close()
	request, err := getRequest(conn)
	if err != nil {
		return
	}
	r := &response{conn: conn}

	handler.ServeGemini(r, request)
}

func getRequest(conn *tls.Conn) (*Request, error) {
	headerBytes, err := readHeader(conn)
	if err != nil {
		return nil, err
	}
	header := string(headerBytes)
	decodedHeader, err := url.QueryUnescape(header)
	if err != nil {
		return nil, fmt.Errorf("failed to decode URL: %s, error: %v", header, err)
	}
	log.Printf("raw request: %s, decoded: %s", header, decodedHeader)
	r := &Request{}
	return r, r.Reset(conn, decodedHeader)
}

type response struct {
	headerWritten bool
	conn          net.Conn
	err           error
}

var _ ResponseWriter = (*response)(nil)

func (w *response) WriteRequest(req *url.URL) error {
	if w.headerWritten {
		return errors.New("header has been sent already")
	}
	_, w.err = fmt.Fprintf(w.conn, "%s\r\n", req)
	if w.err != nil {
		w.err = fmt.Errorf("failed to write request header: %v", w.err)
		return w.err
	}
	w.headerWritten = true
	return nil
}

func (w *response) WriteStatusMsg(status StatusCode, msg string) error {
	if w.headerWritten {
		return errors.New("status has been sent already")
	}
	_, w.err = fmt.Fprintf(w.conn, "%d %s\r\n", status, msg)
	if w.err != nil {
		w.err = fmt.Errorf("failed to write response status message: %v", w.err)
		return w.err
	}
	w.headerWritten = true
	return nil
}

func (w *response) WriteBody(body []byte) (int, error) {
	if !w.headerWritten {
		return 0, errors.New("status message is not written")
	}
	if w.err != nil {
		return 0, w.err
	}
	var written int
	written, w.err = w.conn.Write(body)
	if w.err != nil {
		w.err = fmt.Errorf("failed to write response body: %v", w.err)
	}
	return written, w.err
}

// Write provides raw write and is for internal use only.
// It provides io.Copy compatible interface.
func (w *response) Write(body []byte) (int, error) {
	return w.WriteBody(body)
}
