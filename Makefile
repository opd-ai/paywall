fmt:
	find . -name '*.go' -exec gofumpt -w -s -extra {} \;

build:
	go build -o ex ./example

run: build
	./ex
