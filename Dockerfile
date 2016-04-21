FROM alpine:3.3
MAINTAINER Keybase <admin@keybase.io>

RUN adduser -D keybase
USER keybase
ENV PATH=/home/keybase:$PATH
WORKDIR /home/keybase

COPY ./gregord/gregord /home/keybase/gregord
