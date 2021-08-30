package gemini

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"
	"time"
)

// StatusCode is Gemini status codes as defined in the Gemini spec.
type StatusCode int

// Lists Gemini related URI schemas.
const (
	SchemaGemini = "gemini"
	SchemaTitan  = "titan"
)

// Provides status codes.
const (
	StatusPlainInput        StatusCode = 10
	StatusSensitiveInput    StatusCode = 11
	StatusSuccess           StatusCode = 20
	StatusTemporaryRedirect StatusCode = 30
	StatusPermanentRedirect StatusCode = 31
	StatusUnspecified       StatusCode = 40
	StatusServerUnavalable  StatusCode = 41
	StatusCGIError          StatusCode = 42
	StatusProxyError        StatusCode = 43
	StatusSlowDown          StatusCode = 44
	StatusGeneralPermFail   StatusCode = 50
	StatusNotFound          StatusCode = 51
	StatusGone              StatusCode = 52
	StatusProxyRefused      StatusCode = 53
	StatusBadRequest        StatusCode = 59
	StatusCertRequired      StatusCode = 60
	StatusCertNotAuthorized StatusCode = 61
	StatusCertNotValid      StatusCode = 62
)

// Request contains the data of the client request
type Request struct {
	URL   *url.URL
	Body  io.ReadCloser
	ctx   context.Context
	conn  *tls.Conn
	Titan TitanRequestParams
}

type TitanRequestParams struct {
	Token string
	Mime  string
	Size  int
}

// ReadTitanPayload reads titan payload from the stream into byte slice.
func (r *Request) ReadTitanPayload() ([]byte, error) {
	buf := make([]byte, r.Titan.Size)
	_, err := io.ReadFull(r.Body, buf)
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

type ResponseWriter interface {
	WriteStatusMsg(status StatusCode, msg string) error
	WriteBody([]byte) (int, error)
}

// ServeGemini is the interface a struct need to implement to be able to handle Gemini requests
type Handler interface {
	ServeGemini(ResponseWriter, *Request)
}

type HandlerFunc func(ResponseWriter, *Request)

// ServeGemini calls f(w, r).
func (f HandlerFunc) ServeGemini(w ResponseWriter, r *Request) {
	f(w, r)
}

// SimplifyStatus simplify the response status by omiting the detailed second digit of the status code.
func SimplifyStatus(status int) int {
	return (status / 10) * 10
}

func NotFound(w ResponseWriter, req *Request) {
	w.WriteStatusMsg(StatusNotFound, "404 Resource Not Found")
}

func TrapPanic(next HandlerFunc) HandlerFunc {
	return func(w ResponseWriter, req *Request) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("Trapped: %v", r)
				debug.PrintStack()
				w.WriteStatusMsg(StatusUnspecified, "Internal Server Error")
			}
		}()
		next(w, req)
	}
}

func ServeFile(file *os.File, mimeType string) HandlerFunc {
	return func(w ResponseWriter, r *Request) {
		w.WriteStatusMsg(StatusSuccess, mimeType)
		rw := w.(*response)
		_, _ = io.Copy(rw, file)
	}
}

func ServeFileName(name string, mimeType string) HandlerFunc {
	return func(w ResponseWriter, r *Request) {
		f, err := os.Open(name)
		if err != nil {
			panic(err)
		}
		ServeFile(f, mimeType)(w, r)
	}
}
