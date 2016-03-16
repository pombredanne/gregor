# Gregor
## A Messaging and Notification System for Front-End UX Synchronoization

### Goals

We want to solve two problems at once: (1) the ability to sync client UX state
cleanly across all devices; and (2) more generally the ability to notify
clients of server-side changes (e.g., to signify the need to flush caches).
We don’t think we can generalize all problems into one system, but we think
some problems can be generalized into a generic state-sync, and other problems
would benefit from push-style notifications to clients; and that a client-push
system is common to both goals.

#### Properties of the State-Sync

* Scalable/performant on the server side: we can have a huge cluster of machines all acting on behalf of the same user, and we won’t see race conditions or serialization delays
* Low bandwidth: don’t resend lots of data if you don’t have to
* Idempotent: if a message is replayed, you won’t get a notification twice
* Reorder-tolerant: messages can be arbitrarily reordered (potentially due to retransmits on a crappy network)
* Sookie-like ease-of-use, making it easy to add new notification types
* Supports “all device broadcast” with single-device dismissal
* Supports notification delay until a client comes back online
* Works with push notifications on Android and iOS
* Supports email notifications if no client is online for a given timeout
* Supports event aggregation after sends,

### Design

#### Main abstraction

There’s a JSON blob that is synced consistently across all devices, and the
server.  The server or the clients can change it, and all devices will see the
updates.  The JSON blob will summarize all badge-like UX state that’s not
captured in the user’s sigchain, like: someone followed the user; a followee’s
proof broke; a followee just added a new key; a friend just joined and proved
some proofs that were needed for a KBFS share; a followee upgraded their
proofs; etc.

#### Normal Operation

Keep a JSON blob for each user on the server’s DB with key-value pairs of the form:

```javascript
	randomID : {
		times : { created : ctime, expires : etime, remind_at : [t1, t2, ...] },
		for_device: id,
		category : category,
		body : {...}
	}
```

These blobs can then be transformed into a more-standard JSON object on the
client or server-side, but it’s crucial we have this strict formulation to
make it convenient to dismiss and asynchronously edit the blob on both the
server and the client.

Some state is device-specific, and others apply to all devices (and therefore
have empty forDevice fields).  If you wanted to send a notification that needs
to dismissed on all currently known devices, you’d make N different
notifications, one for each device, which vary just in different forDevice
fields. Then, they can be individually dismissed.

Note this approach does not work in the case in which (1) a notification is
sent to all devices, for individual dismissal; and them  (2) a new device is
added.  In this case the new device won’t have to dismiss the notification
before it fully disappears.  This might be OK, I’m not quite sure yet.  If
it’s not OK then we need a more involved system for multi-dismissals.

The general notification sent to the client is of the form:


```javascript

	{ system: "gregor", body : { create, dismiss } }

```

Create contains a dictionary of randomID/value pairs of new data fields to
create in the user state.  This can be 0, 1 or more.  If 0 creations are
specified, it is safe to omit the create field altogether.  A typical create
might look like:

```javascript
	create : {
		"aabbccee" : {
			times : {
				created : 140000,
				expires :150000,
				remind_at : [ 142000, 143000 ] },
			for_device : "33aabb",
			category : "user.joined",
			body : {
				social_media_name : "bob@twitter",
				keybase : "kbob"
			}
		}
	}
```

The dismiss field is also optional, and tells which notifications are to be dismissed along with this creation.  Dismissals can be by ID, or by time range crossed with category:

```javascript
	dismiss: {
		ids : [ … ],
		intervals : [{
			begin : ….,
			end : ….,
			category : …
		}]
	}
```

When new creations come in to the server, the creation is stored in a queue of
notifications, and is also applied to the user’s JSON object. Also, the
notification ID is added to the expire and remind_at queues, which are
sorted/indexed in time order.  When new dismissals come in, the dismissal is
stored in the notification queue, the user’s JSON object is updated (deleting
a key or keys), and the original notification in the notification queue is
flagged as “dismissed.”

Clients mirror this information from the server.  On cold boot they download
the whole JSON object, but then get incremental create and dismiss
notifications to keep their JSON object up-to-date.  They should follow the
Flux/Redux model of only sending the data in one direction.  Clients should
change local state by sending create/dismiss notifications to the server, and
then consuming and acting on the resulting notification they get back from the
server.

And finally, we can use this same channel for “external” notifications, that
will be outside this state sync system but can be delegated to other
subsystems, like “favorites management” or “tracker list management”:

```javascript

	{ system : “kbfs.favorites”, body : ... }

```

#### Full Resyncs

Clients can always fetch the full state from the server.  Or they can
conditionally load the state from the server at time T: the client sends the
hash of what it thinks the state is, and if it differs from what the server
thinks, then the server will send a new blob. This protocol is subject to race
conditions if two states were committed at exactly the same microsecond, but
worse comes to worse, the client does a full refresh.  We’re hoping these JSON
objects won’t be too big all told.

There’s also a server-pushed command to flush state, if there was a bug or a
version upgrade of some sort.  A notification of the form:

```javascript

	{ system: "gregor", body : { flush: true } }

```

prompts the client to refetch state from scratch.

#### Aggregation

Eventually the server can implement an aggregation system that can pull
published updates out of the update stream and aggregate them into a single
update. This can be done in the current schema by supplying create and dismiss
information in the same notification.  The server has to show some
intelligence and payload-specific understanding to do this effectively, so we
leave it as an open question...

### Worked Examples

Here are some examples that prompted the design of the system, and some
thoughts on how they’d behave with this new system:

#### Your Friend Joins KB After You’ve Social-Media-Shared with them

1. Alice shared a file in `/keybase/private/alice,bob@twitter`; and later your
friend `bob@twitter` signs up as kbob at keybase, completing the necessary
proofs.
2. A new notification is injected into the notification system:

```javascript

	{
		system : "gregor",
		body : {
			create : {
				"5d44d2ad9aabab30" : {
					times : {
						created : 1457056447,
						expires : 1488592477,
						remind_at : [ 1457402098 ] },
					for_device : null,
					category : "user.joined",
					body : {
						social_media_name : "bob@twitter",
						keybase : "kbob",
						uid : "f8da58e5a6d0d0800f361bc139015a19"
					}
				}
			}, dismiss : {}
		}
	}
```

3. Now Alice’s server-side JSON blob is:

```javascript
	{
		"5d44d2ad9aabab30" : {
			times : {
				created : 1457056447,
				expires : 1488592477,
				remind_at : [ 1457402098 ] },
			for_device : null,
			category : "user.joined",
			body : {
				social_media_name : "bob@twitter",
				keybase : "kbob",
				uid : "f8da58e5a6d0d0800f361bc139015a19"
			}
		}
	}
```

4. All online devices get sent the update in Step 2.
5. When offline devices come back online, they get the JSON blob in Step 3.
6. All of these devices show a tracker popup for kbob, saying he’s just joined keybase. On one of her
devices, Alice dismisses the notification.  This sends:

```javascript

	{ system : "gregor", body : { dismiss : { ids : [ "5d44d2ad9aabab30" ] }  }

```

to the server.

7. On the server, Alice’s JSON blob is rewritten to be `{}`. The notification is marked “dismissed”.
All of Alice’s online devices get sent:

```javascript

	{ system : "gregor", body : { dismiss : { ids : [ "5d44d2ad9aabab30" ] }  }
```

	8. On receipt, they apply this diff to their local JSON blobs and dismiss the
notifications showing as a result of “5d44d2ad9aabab30” You Have a New

#### Follower On Keybase

This example is almost as above.  The notification to the user is sent via the
GUI with a badge on the people tab, and a highlighted row showing who the new
follower is. Dismissals for all of the new users are sent when the user views
the tab.  All online devices get sent the update in Step 2.

When offline devices come back online, they get the JSON blob in Step 3. All
of these devices show a tracker popup for kbob, saying he’s just joined
keybase. On one of her devices, Alice dismisses the notification.  This sends:

```javascript

	{ system : "gregor", body : { dismiss : { ids : [ "5d44d2ad9aabab30" ] }  }

```

to the server.

On the server, Alice’s JSON blob is rewritten to be `{}`. The notification is
marked “dismissed”. All of Alice’s online devices get sent:

```javascript

	{ system : "gregor", body : { dismiss : { ids : [ "5d44d2ad9aabab30" ] }  }

```

On receipt, they apply this diff to their local JSON blobs and dismiss the
notifications showing as a result of “5d44d2ad9aabab30”.

#### Hey Alice, You Just Shared with bob@twitter

This example is almost as above. The notification contains the invite code
that you need to send to Bob somehow.  It also might result in an automatic
email notification too.

### Alice Just Deleted /keybase/private/alice,max from her favorites

1. Let’s say Alice deletes this favorite on device A.
1. Regular removal path is followed. The service sends a POST to `kbfs/favorite/delete.json`.
1. The row in the `tlf_favorites` table gets flipped from favorited to ignored.
1. The API server generates an “external system” notification:

```javascript

{ system: “kbfs.favorites”, body: { action: "delete", tlf: "/private/alice,max" } }

```

1. All devices who get this notification reload their favorites list from the server, and bust their caches on KBFS accordingly.

