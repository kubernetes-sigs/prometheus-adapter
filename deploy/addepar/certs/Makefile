.PHONY: build clean deploy

all: clean build deploy

build:
	$(shell cd test && ../gencerts.sh)

clean:
	rm -rf test
	mkdir test

deploy:
	kubectl -n custom-metrics create -f test/cm-adapter-serving-certs.yaml

delete:
	kubectl -n custom-metrics delete -f test/cm-adapter-serving-certs.yaml
