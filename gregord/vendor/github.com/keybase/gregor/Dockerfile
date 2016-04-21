FROM ubuntu:14.04
MAINTAINER Keybase <admin@keybase.io>

RUN useradd -m keybase
USER keybase
ENV PATH=/home/keybase:$PATH
WORKDIR /home/keybase

COPY ./gregord/gregord /home/keybase/gregord
