default: build
all: build

AVDLC=node_modules/.bin/avdlc

go-build-stamp: avdl/*.avdl $(AVDLC) | deps
	@mkdir -p go/
	$(AVDLC) -b -l go -o go/ avdl/*.avdl
	(cd go && go fmt ./...)
	date > $@

clean:
	rm -rf *build-stamp
	$(AVDLC) -c -b -l go -o go avdl/*.avdl

deps:
	npm install

build: go-build-stamp

.PHONY: clean deps
