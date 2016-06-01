#!/bin/bash

_term() {
    echo "Caught TERM signal"
    kill -TERM "$GREGOR"
    exit 0
}
trap _term SIGTERM

dbinit -y -mysql-dsn=$MYSQL_DSN
gregord -bind-address=$GREGOR_BIND_ADDRESS -auth-server=$GREGOR_AUTH_SERVER -mysql-dsn=$MYSQL_DSN -debug &
GREGOR=$!

wait "$GREGOR"
