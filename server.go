package gemini

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
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

var errorRequestTooLong = errors.New("request exceeds 1024 length")

func getRequest(conn *tls.Conn) (*Request, error) {
	var err error
	var reqStr bytes.Buffer
	var buf = make([]byte, 1)
	var endCounter = 0
	for endCounter < 2 {
		var read int
		read, err = conn.Read(buf)
		if err != nil {
			return nil, fmt.Errorf("failed to read request: %v", err)
		}
		if read == 1 {
			// read until CRLF or "\r\n" sequence
			if (buf[0] == '\r' && endCounter == 0) || (buf[0] == '\n' && endCounter == 1) {
				endCounter += 1
			} else {
				reqStr.Write(buf)
			}
		} else {
			return nil, errors.New("read zero bytes")
		}
		if reqStr.Len() > 1024 {
			return nil, errorRequestTooLong
		}
	}
	log.Printf("raw request: %s", reqStr.String())
	r := &Request{}
	return r, r.Reset(conn, reqStr.String())
}

func (r *Request) Reset(conn *tls.Conn, rawurl string) error {
	r.Body = conn
	r.conn = conn
	r.Titan.Mime = ""
	r.Titan.Size = 0
	r.Titan.Token = ""
	var err error
	r.URL, err = url.ParseRequestURI(rawurl)
	if err != nil {
		return fmt.Errorf("failed to parse request: %v, error: %v", rawurl, err)
	}
	if r.URL.Scheme == "" {
		return fmt.Errorf("request is missing scheme: %v", rawurl)
	}
	if r.URL.Scheme == SchemaTitan {
		parts := strings.Split(r.URL.Path, ";")
		if len(parts) < 2 {
			return errors.New("titan parameters expected")
		}
		r.URL.Path, parts = parts[0], parts[1:]
		for _, part := range parts {
			kv := strings.Split(part, "=")
			if len(kv) != 2 {
				continue
			}
			key := kv[0]
			val := kv[1]
			switch key {
			case "token":
				r.Titan.Token = val
			case "mime":
				r.Titan.Mime = val
			case "size":
				r.Titan.Size, err = strconv.Atoi(val)
				if err != nil {
					return fmt.Errorf("failed to parse titan size parameter: %s", val)
				}
			}
		}
	} else {
		// Gemini specific handling
		if r.URL.Path == "" {
			r.URL.Path = "/"
		}
	}
	return err
}

type response struct {
	headerWritten bool
	conn          net.Conn
	err           error
}

var _ ResponseWriter = (*response)(nil)

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
