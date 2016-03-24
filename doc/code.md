
# Code Organization Plan for Gregor

Gregor will both be used as a library (as in the Keybase client) and also as
a standalone server, for running in our data center. In this doc, we specify
some details as to how the code is laid out, and how different components
interface with each other.

## The Gregor Main Process


### Configuration

* Should use Go Flags to read in flags
* Should read config values either out of the environment or out of flags
* Maybe at first we shouldn't have configuration files, and we should add them
  only if we see a reason (given it's doing to be dockered from the beginning).

### The Main Loop

The main loop is of the form:

* Receive an incoming message
* Run the message through each of the known `MessageConsumer`s registered in
  the process, calling `ConsumeMessage` on each. If any one step errors, then
  we still have to deliver to all subsequent `MessageConsumer`s. We'll wind up
  in a broken state, but we're not doing message delivery steps in an atomic
  transaction, so there's no way around this. Our story is that the consumers
  will have to resync to recover, as driven by clients.
 	* We might want to fail further processing if we have a problem persisting the message
 	  to SQL, for the reason that the state won't be recoverable without a persistent
 	  record of the message.

### GoRoutine Organization

#### List of Goroutines

* Main goroutine (handles routing and new connections, and creation/dismissal of per-user goroutines)
* One goroutine per user
* One goroutine per user per client, probably interior to the RPC library
* One goroutine per SQL worker, with a potential pool of ~10 workers

#### Incoming Connections

1. New connection handled by the main thread
1. Forks off a goroutine to handle authentication
1. Once authentication is complete, pass the connection back to the main thread
1. Main thread makes a new per-user goroutine and/ors delegates to the eisting per-user goroutine
1. Per-user goroutine makes an new RPC server to handle incoming requests on the new connection
1. Subscribe for pubusb notifications for this user (if not already done)

#### Handling Incoming Messages

1. New incoming message comes in on a per-user-per-client goroutine to an RPC server
1. Message injected into the per-user goroutine
1. Message injected into the main goroutine
1. Run message through registered MessageConsumers
	1. SQL
    	1. Pick an available worker
    	1. Store to disk
	1. Publisher (redis)
		1. fire off a redis notification
	1. Broadcaster
    	1. Send the message to the per-user goroutine
    	1. For each client
    		1. Send a `BroadcastMessage` RPC

#### EOF on connection

1. When we hear an EOF on connection, close the connection (of course)
1. Per-user goroutine updates accounting
1. If this is the last connection for the user, the per-user goroutine sends a message
   to the main goroutine, who cleans out the per-user goroutine and state,
   and also unsubscribes from pubsub.  The main thread will serialize EOFs
   and new connections so that we don't race when a new connection happens
   around the same time the last connection for a user dies. **Note this
   is the trickiest case, we should make sure we have a solution for it
   first!**

