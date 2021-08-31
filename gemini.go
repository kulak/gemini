package gemini

import (
	"bytes"
	"errors"
	"io"
	"log"
	"os"
	"runtime/debug"
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

var errorRequestTooLong = errors.New("request exceeds 1024 length")

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
		if len(line) > 1024 {
			return []byte{}, errorRequestTooLong
		}
	}
}
