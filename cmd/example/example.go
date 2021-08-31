package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	gemini "github.com/kulak/gemini"
)

type ExampleHandler struct {
}

func (h ExampleHandler) ServeGemini(w gemini.ResponseWriter, req *gemini.Request) {
	log.Printf("request: %s, user: %v", req.URL.Path, strings.Join(userName(req), " "))
	switch req.URL.Path {
	case "/":
		err := w.WriteStatusMsg(gemini.StatusSuccess, "text/gemini")
		requireNoError(err)
		_, err = w.WriteBody([]byte("Hello, world!"))
		requireNoError(err)
	case "/user":
		if req.Certificate() == nil {
			w.WriteStatusMsg(gemini.StatusCertRequired, "Authentication Required")
			return
		}
		w.WriteStatusMsg(gemini.StatusSuccess, "text/gemini")
		w.WriteBody([]byte(req.Certificate().Subject.CommonName))
	case "/die":
		requireNoError(errors.New("must die"))
	case "/file":
		gemini.ServeFileName("cmd/example/hello.gmi", "text/gemini")(w, req)
	case "/post":
		if req.URL.Scheme != gemini.SchemaTitan {
			w.WriteStatusMsg(gemini.StatusSuccess, "text/gemini")
			w.WriteBody([]byte("Use titan scheme to upload data"))
			return
		}
		payload, err := req.ReadTitanPayload()
		requireNoError(err)
		w.WriteStatusMsg(gemini.StatusSuccess, "text/gemini")
		w.WriteBody([]byte("Titan Upload Parameters\r\n"))
		w.WriteBody([]byte(fmt.Sprintf("Upload MIME Type: %s\r\n", req.Titan.Mime)))
		w.WriteBody([]byte(fmt.Sprintf("Token: %s\r\n", req.Titan.Token)))
		w.WriteBody([]byte(fmt.Sprintf("Size: %v\r\n", req.Titan.Size)))
		w.WriteBody([]byte("Payload:\r\n"))
		w.WriteBody(payload)

	default:
		w.WriteStatusMsg(gemini.StatusNotFound, req.URL.Path)
	}

}

func requireNoError(err error) {
	if err != nil {
		panic(err)
	}
}

func dateToStr(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 36)
}

func userName(r *gemini.Request) []string {
	cert := r.Certificate()
	if cert == nil {
		return []string{""}
	}
	return []string{cert.Subject.CommonName, cert.SerialNumber.String(), dateToStr(cert.NotBefore), dateToStr(cert.NotAfter)}
}

func main() {
	var host, cert, key string
	flag.StringVar(&host, "host", ":1965", "listen on host and port.  Example: hostname:1965")
	flag.StringVar(&cert, "cert", "server.crt.pem", "certificate file")
	flag.StringVar(&key, "key", "server.key.pem", "private key associated with certificate file")
	flag.Parse()

	handler := ExampleHandler{}

	err := gemini.ListenAndServe(host, cert, key, gemini.TrapPanic(handler.ServeGemini))
	if err != nil {
		log.Fatal(err)
	}
}
