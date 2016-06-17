default: build
all: build

AVDLC=node_modules/.bin/avdlc
AVDL2JSON=node_modules/.bin/avdl2json

build: gregor1/*.avdl | install
	$(AVDLC) -b -l go -o gregor1/ gregor1/*.avdl
	$(AVDL2JSON) -2 -b -o gregor1 gregor1/*.avdl
	go fmt ./...
	date > go-build-stamp

install:
	npm i

clean: gregor1/*.avdl gregor1/*.json | install
	$(AVDLC) -c -b -l go -o gregor1 gregor1/*.avdl
	$(RM) -r gregor1/*.json
	$(RM) -r *build-stamp

.PHONY: build clean install

