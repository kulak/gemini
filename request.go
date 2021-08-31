package gemini

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/url"
	urlpkg "net/url"
	"strconv"
	"strings"
	"time"
)

// NoBody is an io.ReadCloser with no bytes. Read always returns EOF
// and Close always returns nil. It can be used in an outgoing client
// request to explicitly signal that a request has zero bytes.
// An alternative, however, is to simply set Request.Body to nil.
var NoBody = noBody{}

type noBody struct{}

func (noBody) Read([]byte) (int, error)         { return 0, io.EOF }
func (noBody) Close() error                     { return nil }
func (noBody) WriteTo(io.Writer) (int64, error) { return 0, nil }

var (
	// verify that an io.Copy from NoBody won't require a buffer:
	_ io.WriterTo   = NoBody
	_ io.ReadCloser = NoBody
)

// Request contains the data of the client request
type Request struct {
	URL *url.URL

	ctx   context.Context
	conn  *tls.Conn
	Titan TitanRequest
}

type TitanRequest struct {
	Token string
	Mime  string

	// Size records the length of the associated content.
	// The value -1 indicates that the length is unknown.
	// Values >= 0 indicate that the given number of bytes may
	// be read from Body.
	//
	// For client requests, a value of 0 with a non-nil Body is
	// also treated as unknown.
	Size int64

	// Body is the request's body.
	//
	// For client requests, a nil body means the request has no
	// body, such as a GET request. The HTTP Client's Transport
	// is responsible for calling the Close method.
	//
	// For server requests, the Request Body is always non-nil
	// but will return EOF immediately when no body is present.
	// The Server will close the request body. The ServeHTTP
	// Handler does not need to.
	//
	// Body must allow Read to be called concurrently with Close.
	// In particular, calling Close should unblock a Read waiting
	// for input.
	Body io.ReadCloser

	getBody func() (io.ReadCloser, error)
}

func (r *Request) Reset(conn *tls.Conn, rawurl string) error {
	r.conn = conn
	r.Titan.Mime = ""
	r.Titan.Size = 0
	r.Titan.Token = ""
	r.Titan.Body = conn
	var err error
	r.URL, err = url.ParseRequestURI(rawurl)
	if err != nil {
		return fmt.Errorf("failed to parse request: %v, error: %v", rawurl, err)
	}
	if r.URL.Scheme == "" {
		return fmt.Errorf("request is missing scheme: %v", rawurl)
	}
	if r.URL.Scheme == SchemaTitan {
		err = r.resetTitanURL()
		if err != nil {
			return err
		}
	} else {
		r.resetGeminiURL()
	}
	return err
}

func (r *Request) resetTitanURL() error {
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
			var err error
			r.Titan.Size, err = strconv.ParseInt(val, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse titan size parameter: %s", val)
			}
		}
	}
	return nil
}

func (r *Request) resetGeminiURL() {
	// Gemini specific handling
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}
}

// ReadTitanPayload reads titan payload from the stream into byte slice.
func (r *Request) ReadTitanPayload() ([]byte, error) {
	buf := make([]byte, r.Titan.Size)
	_, err := io.ReadFull(r.Titan.Body, buf)
	return buf, err
}

func (r *Request) Certificate() *x509.Certificate {
	if len(r.conn.ConnectionState().PeerCertificates) > 0 {
		return r.conn.ConnectionState().PeerCertificates[0]
	}
	return nil
}

func dateToStr(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 36)
}

func (r *Request) UserName() []string {
	cert := r.Certificate()
	if cert == nil {
		return []string{""}
	}
	return []string{cert.Subject.CommonName, cert.SerialNumber.String(), dateToStr(cert.NotBefore), dateToStr(cert.NotAfter)}
}

// Context returns the request's context. To change the context, use
// WithContext.
//
// The returned context is always non-nil; it defaults to the
// background context.
//
// For outgoing client requests, the context controls cancellation.
//
// For incoming server requests, the context is canceled when the
// client's connection closes, the request is canceled (with HTTP/2),
// or when the ServeHTTP method returns.
func (r *Request) Context() context.Context {
	if r.ctx != nil {
		return r.ctx
	}
	return context.Background()
}

// WithContext returns a shallow copy of r with its context changed
// to ctx. The provided ctx must be non-nil.
//
// For outgoing client request, the context controls the entire
// lifetime of a request and its response: obtaining a connection,
// sending the request, and reading the response headers and body.
//
// To create a new request with a context, use NewRequestWithContext.
// To change the context of a request, such as an incoming request you
// want to modify before sending back out, use Request.Clone. Between
// those two uses, it's rare to need WithContext.
func (r *Request) WithContext(ctx context.Context) *Request {
	if ctx == nil {
		panic("nil context")
	}
	r2 := new(Request)
	*r2 = *r
	r2.ctx = ctx
	return r2
}

// NewRequestWithContext returns a new Request given a method, URL, and
// optional body.
//
// If the provided body is also an io.Closer, the returned
// Request.Body is set to body and will be closed by the Client
// methods Do, Post, and PostForm, and Transport.RoundTrip.
//
// NewRequestWithContext returns a Request suitable for use with
// Client.Do or Transport.RoundTrip. To create a request for use with
// testing a Server Handler, either use the NewRequest function in the
// net/http/httptest package, use ReadRequest, or manually update the
// Request fields. For an outgoing client request, the context
// controls the entire lifetime of a request and its response:
// obtaining a connection, sending the request, and reading the
// response headers and body. See the Request type's documentation for
// the difference between inbound and outbound request fields.
//
// If body is of type *bytes.Buffer, *bytes.Reader, or
// *strings.Reader, the returned request's ContentLength is set to its
// exact value (instead of -1), GetBody is populated (so 307 and 308
// redirects can replay the body), and Body is set to NoBody if the
// ContentLength is 0.
func NewRequestWithContext(ctx context.Context, url string, body io.Reader) (*Request, error) {
	if ctx == nil {
		return nil, errors.New("gemini: nil Context")
	}
	u, err := urlpkg.Parse(url)
	if err != nil {
		return nil, err
	}
	req := &Request{
		ctx: ctx,
		URL: u,
	}
	if req.URL.Scheme == SchemaGemini {
		return req, nil
	}
	return req, req.Titan.clientRequest(body)
}

func (r *TitanRequest) clientRequest(body io.Reader) error {
	rc, ok := body.(io.ReadCloser)
	if !ok && body != nil {
		rc = io.NopCloser(body)
	}
	r.Body = rc
	if body != nil {
		switch v := body.(type) {
		case *bytes.Buffer:
			r.Size = int64(v.Len())
			buf := v.Bytes()
			r.getBody = func() (io.ReadCloser, error) {
				r := bytes.NewReader(buf)
				return io.NopCloser(r), nil
			}
		case *bytes.Reader:
			r.Size = int64(v.Len())
			snapshot := *v
			r.getBody = func() (io.ReadCloser, error) {
				r := snapshot
				return io.NopCloser(&r), nil
			}
		case *strings.Reader:
			r.Size = int64(v.Len())
			snapshot := *v
			r.getBody = func() (io.ReadCloser, error) {
				r := snapshot
				return io.NopCloser(&r), nil
			}
		default:
			// indicate unknown length
			// r.Size = -1
			return errors.New("can't handle unknown size titan payloads")
		}
		if r.Size == 0 {
			r.getBody = func() (io.ReadCloser, error) { return NoBody, nil }
		}
	}
	return nil
}

func (r *TitanRequest) closeBody() error {
	if r.Body == nil {
		return nil
	}
	return r.Body.Close()
}
