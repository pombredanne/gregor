FROM alpine:3.3
MAINTAINER Keybase <admin@keybase.io>

RUN apk add --update wget ca-certificates bash && \
    cd /tmp && \
    wget "https://github.com/andyshinn/alpine-pkg-glibc/releases/download/2.23-r1/glibc-2.23-r1.apk" \
        "https://github.com/andyshinn/alpine-pkg-glibc/releases/download/2.23-r1/glibc-bin-2.23-r1.apk" && \
    apk add --allow-untrusted glibc-2.23-r1.apk glibc-bin-2.23-r1.apk && \
    echo 'hosts: files mdns4_minimal [NOTFOUND=return] dns mdns4' >> /etc/nsswitch.conf && \
    adduser -D keybase && \
    rm -rf /tmp/* /var/cache/apk/*

USER keybase
ENV PATH=/home/keybase:$PATH
WORKDIR /home/keybase

COPY gregor.sh /home/keybase/gregor.sh
COPY gregord/gregord /home/keybase/gregord
COPY dbinit/dbinit /home/keybase/dbinit

ENTRYPOINT ["gregor.sh"]
