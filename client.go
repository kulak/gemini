package gemini

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
)

type Client struct {
	// InsecureSkipVerify controls whether a client verifies the server's
	// certificate chain and host name. If InsecureSkipVerify is true, crypto/tls
	// accepts any certificate presented by the server and any host name in that
	// certificate. In this mode, TLS is susceptible to machine-in-the-middle
	// attacks unless custom verification is used. This should be used only for
	// testing or in combination with VerifyConnection or VerifyPeerCertificate.
	InsecureSkipVerify bool
}

// Fetch a resource from a Gemini server with the given URL
func (c Client) Fetch(url string) (*Response, error) {
	req, err := NewRequestWithContext(context.Background(), url, nil)
	if err != nil {
		return nil, err
	}
	var r = &response{}
	err = c.connect(r, req)
	if err != nil {
		return nil, err
	}
	defer r.conn.Close()
	err = r.WriteRequest(req.URL)
	if err != nil {
		return nil, err
	}

	return getResponse(r, req)
}

func (c Client) connect(r *response, req *Request) error {
	conf := &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: c.InsecureSkipVerify,
	}
	var err error
	r.conn, err = tls.Dial("tcp", req.URL.Host, conf)
	return err
}

func getResponse(r *response, req *Request) (*Response, error) {
	headerBytes, err := readHeader(r.conn)
	if err != nil {
		return nil, err
	}
	var res = &Response{}
	return Response{header.status, header.meta, conn}, nil
}

// func getHeader(conn io.Reader) (header, error) {
// 	line, err := readHeader(conn)
// 	if err != nil {
// 		return header{}, fmt.Errorf("failed to read header: %v", err)
// 	}

// 	fields := strings.Fields(string(line))
// 	status, err := strconv.Atoi(fields[0])
// 	if err != nil {
// 		return header{}, fmt.Errorf("unexpected status value %v: %v", fields[0], err)
// 	}

// 	meta := strings.Join(fields[1:], " ")

// 	return header{status, meta}, nil
// }

func readHeader(conn io.Reader) ([]byte, error) {
	var line []byte
	delim := []byte("\r\n")
	// A small buffer is inefficient but the maximum length of the header is small so it's okay
	buf := make([]byte, 1)

	for {
		_, err := conn.Read(buf)
		if err != nil {
			return []byte{}, err
		}

		line = append(line, buf...)
		if bytes.HasSuffix(line, delim) {
			return line[:len(line)-len(delim)], nil
		}
	}
}
