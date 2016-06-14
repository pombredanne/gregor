default: build
all: build

AVDLC=node_modules/.bin/avdlc

build: $(AVDLC) gregor1/*.avdl
	$(AVDLC) -b -l go -o gregor1/ gregor1/*.avdl
	go fmt ./...
	date > go-build-stamp

$(AVDLC):
	npm install

clean: $(AVDLC) gregor1/*.avdl
	$(AVDLC) -c -b -l go -o gregor1 gregor1/*.avdl
	$(RM) -r *build-stamp

.PHONY: build clean
