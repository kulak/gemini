.PHONY: all build clean

build:
	go build ./...

test:
	go test .

run: host=localhost:1965
run: cert=local/cert.pem
run: key=local/key.pem
run:
	go run cmd/example/example.go "-host=$(host)" "-cert=$(cert)" "-key=$(key)"

# make tag name=v0.0.0
tag: name=empty
tag:
	git tag $(name)
	git push origin $(name)
