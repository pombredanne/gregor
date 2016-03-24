
# Gregor

[![Build Status](https://travis-ci.org/keybase/gregor.svg?branch=master)](https://travis-ci.org/keybase/gregor)

Gregor is a simple system for consuming, persisting, and broadcasting
notifications to a collection of clients for a user.

## Documents

  * [Design](doc/design.md)
  * [Architecture](doc/arch.md)
  * [Code Organization](doc/code.md)

## Repository Layout

  * `.` — the top level interface to all major Gregor objects.  Right now, it just contains an interface.
  * [`storage/`](storage/) — storage engines for persisting Gregor objects. Right now, only SQL is implemented.
  * [`test/`](test/) — Test code that is used throughout
  * [`keybase/`](keybase/) — Keybase-specific code that uses Keybase protocols.  Eventually we might fork this out into a different repo, but it's easiest to leave it in now for interating.
    * [`keybase/protocol/go`](keybase/protocol/go/) — Keybase-flavored AVDL-generated data types that meet the high-level [Gregor interface](./interface.go).
    * [`keybase/rpc/`](keybase/rpc/) — Code for managing and routing RPCs.
