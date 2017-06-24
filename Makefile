all: build

build:
	go build cmd/adapter.go
.PHONY: build

docker-build: adapter
	docker build -t cm-adapter -f deploy/Dockerfile .	
.PHONY: docker-build

clean:
	rm adapter
.PHONY: clean
