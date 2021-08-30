package main

import (
	"errors"
	"flag"
	"log"
	"strings"

	gemini "github.com/kulak/gemini"
)

type ExampleHandler struct {
}

func (h ExampleHandler) ServeGemini(w gemini.ResponseWriter, req *gemini.Request) {
	log.Printf("request: %s, user: %v", req.URL.Path, strings.Join(req.UserName(), " "))
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
	default:
		w.WriteStatusMsg(gemini.StatusNotFound, req.URL.Path)
	}

}

func requireNoError(err error) {
	if err != nil {
		panic(err)
	}
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
