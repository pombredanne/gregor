FROM alpine:3.3
MAINTAINER Keybase <admin@keybase.io>

RUN adduser -D keybase
USER keybase
ENV PATH=/home/keybase:$PATH
WORKDIR /home/keybase

ENTRYPOINT ["gregord", "-bind-address", "0.0.0.0:9911", "-session-server", "fmprpc://kbweb.local:13009", "-mysql-dsn", "NOT_IMPLEMENTED", "-debug"]

COPY ./gregord/gregord /home/keybase/gregord
