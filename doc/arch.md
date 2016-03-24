
# Code Architecture

The central conceit of Gregor is a ["replicated state machine"](https://en.wikipedia.org/wiki/State_machine_replication),
but with the simplification that messages can be replayed on any state machine in any
order, without keeping them in lockstep. (We've picked a [Conflict-Free Replicated Data Type (CRDT)](https://en.wikipedia.org/wiki/Conflict-free_replicated_data_type) to implement this). In terms of the [CAP theorem](https://en.wikipedia.org/wiki/CAP_theorem), we'll be sacrificing **C**onsistency in favor of **A**vailability and **P**artition-tolerence. In other words,
[eventually consistent](https://en.wikipedia.org/wiki/Eventual_consistency).

In the code, we will have several implementations of the general `StateMachine` interface. The name of the game
will be to provide the plumbing to keep them all (eventually) in sync.

## High-Level Interface

The high-level go interface for state machines are as follows:

```go
// MessageConsumer consumes state update messages. It's half of
// the state machine protocol
type MessageConsumer interface {
  // ConsumeMessage is called on a new incoming message to mutate the state
  // of the state machine. Of course messages can be "inband" which actually
  // perform state mutations, or might be "out-of-band" that just use the
  // Gregor broadcast mechanism to make sure that all clients get the
  // notification.
  ConsumeMessage(m Message) error
}

// StateMachine is the central interface of the Gregor system. Various parts of the
// server and client infrastructure will implement various parts of this interface,
// to ensure that the state machine can be replicated, and that it can be queried.
type StateMachine interface {

  // StateMachines consume messages to update their internal state.
  MessageConsumer

  // State returns the state for the user u on device d at time t.
  // d can be nil, in which case the global state (across all devices)
  // is returned. If t is nil, then use Now, otherwise, return the state
  // at the given time.
  State(u UID, d DeviceID, t TimeOrOffset) (State, error)

  // InBandMessagesSince returns all messages since the given time
  // for the user u on device d.  If d is nil, then we'll return
  // all messages across all devices.  If d is a device, then we'll
  // return global messages and per-device messages for that device.
  InBandMessagesSince(u UID, d DeviceID, t TimeOrOffset) ([]InBandMessage, error)
}
```

The parts of the system that consume updates and can replay them
are full `StateMachine`s. Those that just forward state update messages
to other parts of the system implement the first half of the protocol
only, and are `MessageConsumer`s.


## Operational Architecture

Eventually we want Gregor to be scalable, shardable, etc. At first, to keep
things simple, we can consider a singleton roll-out: one gregor server connected
to one database:

### Singleton


```
                                            ┌──────────────────────────────────────────────┐
                                            │                                              │
                                            ▼                                              │
              ┌──────────────────────────────────────────────────────────┐                 │
              │                             │                    Server  │                 │
              │                             │                            │                 │
              │       ┌─────────────────────┘                            │                 │
              │       │                       ┌───────────────────┐      │                 │
              │       ▼                       │                   │      │
              │  ┌─────────┐  Consume         │    SQL Backend    │      │   Inject new message
              │  │ Message │─────────────────▶│  (StateMachine)   │      │
              │  └─────────┘                  │                   │      │                 │
              │       │                       └───────────────────┘      │                 │
              │       │                                                  │                 │
              │       │(copy)                                            │                 │
              │       │                       ┌───────────────────┐      │                 │
              │       ▼                       │                   │      │                 │
              │  ┌─────────┐  Consume         │    Broadcaster    │      │                 │
              │  │ Message │─────────────────▶│ (MessageConsumer) │      │                 │
              │  └─────────┘                  │                   │      │                 │
              │                               └───────────────────┘      │                 │
              │                                         │                │                 │
              └─────────────────────────────────────────┼────────────────┘                 │
                                                        │                                  │
                                                        │                                  │
                                                        │                                  │
          ┌─────────────────────┬──────────────────────┬┴─────────────────────┐            │
          │                     │                      │                      │            │
          │                     │                      │                      │            │
          │Consume              │Consume               │Consume        Consume│            │
          │                     │                      │                      │            │
          ▼                     ▼                      ▼                      ▼            │
┌───────────────────┐ ┌───────────────────┐  ┌───────────────────┐  ┌───────────────────┐  │
│                   │ │                   │  │                   │  │                   │  │
│   Client Store    │ │   Client Store    │  │   Client Store    │  │   Client Store    │  │
│  (StateMachine)   │ │  (StateMachine)   │  │  (StateMachine)   │  │  (StateMachine)   │──┘
│                   │ │                   │  │                   │  │                   │
└─────────┬─────────┤ └─────────┬─────────┤  └─────────┬─────────┤  └─────────┬─────────┤
          │device 1 │           │device 2 │            │device 3 │            │device 4 │
          └─────────┘           └─────────┘            └─────────┘            └─────────┘
```

### Fleet

Pretty soon, we'll want multiple gregors at work in parallel, all sharing the
same SQL store. In this case, incoming users are mapped to arbitrary gregor
processes, so a user's clients might connect to several of them. Thus, we need
a broadcast system on the backend. The receiving gregor will write the message
to stable storage, will then broadcast to the other gregors who have open
connections to the user, and all relevant gregors will broadcast to user
devices. We'll be relying on a sensible pub-sub system to allow gregors to
subsubscribe and unsubsubscribe from various per-user feeds. Redis is the
first step, but maybe we'll scale up in the future. The exact system specifics
should be hidden behind a Go interface as much as possible.


```
      ┌────────────────────────────────┐                                   ┌───────────────────────────────────┐
      │                                │                                   │                                   │
      │                                ▼                                   │                                   ▼
      │      ┌─────────────────────────┬────────────────────────┐          │              ┌────────────────────┬───────────────────┐
      │      │                         │               Server A │          │              │                    │          Server B │
      │      │       ┌─────────────────┘                        │          │              │                    │                   │
      │      │       │                                          │          │              │                    │                   │
      │      │       │                       ┌────────────────┐ │          │              │                    │                   │
      │      │       ▼                       │                │ │          │              │                    │                   │
      │      │  ┌─────────┐  Consume         │  SQL Backend   │ │          │              │                    │                   │
      │      │  │ Message │─────────────────▶│ (StateMachine) │ │      Broadcast          │                    │                   │
      │      │  └─────────┘                  │                │ │        to all           │                    │                   │
      │      │       │                       └────────────────┘ │     subscribers         │                    │                   │
      │      │       │                                          │          │              │      ┌─────────────┘                   │
      │      │       │(copy)                                    │          │              │      │                                 │
      │      │       │                       ┌───────────────┐  │          │              │      │                                 │
   Inject    │       ▼                       │               │  │          │              │      │                                 │
new message  │  ┌─────────┐  Consume         │   Publisher   │  │          │              │      │                                 │
             │  │ Message │─────────────────▶│ (MsgConsumer) │──┼──────────┘              │      │                                 │
      │      │  └─────────┘                  │               │  │                         │      │                                 │
      │      │       │                       └───────────────┘  │                         │      │                                 │
      │      │       │                                          │                         │      │                                 │
      │      │       │(copy)                                    │                         │      │                                 │
      │      │       │                       ┌───────────────┐  │                         │      │                ┌──────────────┐ │
      │      │       ▼                       │               │  │                         │      ▼                │              │ │
      │      │  ┌─────────┐  Consume         │  Broadcaster  │  │                         │ ┌─────────┐  Consume  │ Broadcaster  │ │
      │      │  │ Message │─────────────────▶│ (MsgConsumer) │  │                         │ │ Message │──────────▶│(MsgConsumer) │ │
      │      │  └─────────┘                  │               │  │                         │ └─────────┘           │              │ │
      │      │                               └───────┬───────┘  │                         │                       └──────────────┘ │
      │      │                                       │          │                         │                               │        │
      │      └───────────────────────────────────────┼──────────┘                         └───────────────────────────────┼────────┘
      │                                              │                                                                    │
      │                                              │                                                                    │
      │                                              │                                                                    │
      │                                              │                                                                    │
      │            ┌─────────────────────────┬───────┘                                                                    │
      │            │                         │                                                 ┌─────────────────────────┬┘
      │         Consume                    Consume                                             │                         │
      │            │                         │                                              Consume                   Consume
      │            │                         │                                                 │                         │
      │            │                         │                                                 │                         │
      │            │                         │                                                 │                         │
      │            │                         │                                                 │                         │
      │            ▼                         ▼                                                 ▼                         ▼
      │  ┌───────────────────┐     ┌───────────────────┐                             ┌──────────────────┐      ┌──────────────────┐
      │  │                   │     │                   │                             │                  │      │                  │
      │  │   Client Store    │     │   Client Store    │                             │   Client Store   │      │   Client Store   │
      └──│  (StateMachine)   │     │  (StateMachine)   │                             │  (StateMachine)  │      │  (StateMachine)  │
         │                   │     │                   │                             │                  │      │                  │
         └─────────┬─────────┤     └─────────┬─────────┤                             └─────────┬────────┤      └─────────┬────────┤
                   │device 1 │               │device 2 │                                       │device 3│                │device 4│
                   └─────────┘               └─────────┘                                       └────────┘                └────────┘
````

### Fleet with sharded backend

Eventually we'll grow too big for one SQL backend. In that case, we'll shard on UIDs and have
each gregor connect to all N shards. There will be no need for cross-shard joins (at least we don't
see one ATM), so the code to support the multiple shards shouldn't be onerous. Operationally,
the setup becomes more complex, but hopefully RDS or Aurora will take some of the pain out of that.

