# otter

Real-time pubsub for websocket clients.

This is ALPHA software and is under active development, the API may change at
any time.

## Features

* Token-based authentication
* Publish/Subscribe to arbitrarily named channels
* Presence information (i.e. named clients)
* Per-channel list of clients subscribed
* Alerts to backend on client subscribing, unsubscribing, or disconnecting
* Redis or redis cluster used as backend, multiple otter instances can run
  independantly and be used interchangeably

## Model

Otter is modeled to be a very simple, if somewhat restrictive communication
layer between a backend application and a large set of clients. The backend
application is completely arbitrary and must be written by you. When the backend
publishes to a channel all clients on that channel will receive the publish.
When a client publishes to a channel, ONLY backend applications subscribed to
that channel will receive it.

The interaction on the client-side is fairly straightforward:

* (Possibly optional, depending on config) Authenticate connection with token
  previously retrieved from backend. Authentication also includes presence
  information.
* Subscribe to desired channels.
* Read publishes from these channels.
* Publish to any channel to communicate with backend. A channel does not need to
  be subscribed to to be published to.

The interaction for the backend is fairly similar:

* Subscribe to desired channels.
* Receive publishes from clients on these channels.
* Send publishes to clients on any channel. A channel does not need to be
  subscribed to to be published to.
* Provide authentication tokens out-of-band, if necessary.

## Interfaces

There are two interfaces (ports) over which interactions with otter may be
performed:

* Websocket - used by clients and backend applications to publish to channels
  and receive publishes from them.

* REST - used by backend applications (and possibly clients, if the port is made
  available to them) to retrieve non-push data, e.g. current list of subscribed
  clients for a channel.

## Websocket

### On connection

Immediately upon connecting otter will send the connection a JSON object
containing its ID:

```json
{"id":"asdfqwer1234uiop"}
```

A connection's ID is unique to it, and will change upon every new connection.

### Commands

Commands to the websocket interface do not return anything upon success. This
negates the problem during normal operation of having the client cull out
messages which are returns from a previous command (versus a publish from a
channel) and matching those command returns with the commands which spawned
them.

If a command generates an error, however, an object like this will be returned:

```
{
    "error":"some error message",
    "from":{
        "command":"original command",
        "...":"..."
    }
}
```

The `from` field will contain the original command object which generated this
error, and any extra fields which were sent with it.

The commands available over the websocket interface are not likely to generate
errors. Subscribe and publish should never fail, and auth will only fail if the
generated token is incorrect. If there was a database error then the command can
simply be retried with the given information.

#### auth

TODO describe new signatures, don't ever use the word "token"

When a client connects it may authenticate using an authentication token which
signs a presence string. The authentication token is a SHA256-HMAC of the
precense string and a secret key which the backend app and otter share. The
presence string is completely arbitrary, and may contain any information you
wish to use to identify a client.

```json
{
    "command":"auth",
    "presence":"arbitrary text",
    "signature":"<sha256hmac(presence_text, secret_key)>"
}
```

Backend applications do not need to call the auth command, they do, however,
need to send a `signature` field on all commands the do send. This signature
should be the SHA256-HMAC of their connection ID and the shared secret.

#### sub

Used to subscribe to a channel. Once this is received otter will begin sending
all backend application publishes to that channel. If the connection is already
subscribed then nothing happens.

```json
{
    "command":"sub",
    "channel":"channel name"
}
```

#### unsub

Used to unsubscribe from a channel. If the connection is not subscribed then
nothing happens.

```json
{
    "command":"unsub",
    "channel":"channel name"
}
```

#### pub

Used to publish to a channel. A client's publish to a channel will only go to
the backend applications subscribed to the channel, and vice-versa. A connection
does not need to be subscribed to a channel to publish to it.

```json
{
    "command":"pub",
    "channel":"channel name",
    "message":"message"
}
```

### Receiving publishes

Publishes will be pushed to a connection and will look like this:

```json
{
    "type":"pub",
    "channel":"channel name",
    "message":"message",
    "connection":{
        "id":"connection id",
        "presence":"some string"
    }
}
```

The `connection` field gives the connection ID and the `presence` information of
the publishing client (if they did the `auth` command, if not it will be
omitted). If the client is receiving a publish it can assume it came from a
backend application, and vice-versa.

Additionally, there are two special push messages a backend application can
receive:

```json
{
    "type":"sub",
    "channel":"channel name",
    "connection":{
        "id":"connection id",
        "presence":"some string"
    }
}
```

and

```json
{
    "type":"unsub",
    "channel":"channel name",
    "connection":{
        "id":"connection id",
        "presence":"some string"
    }
}
```

These indicate when a client has subscribed or unsubscribed from a channel the
backend application has subscribed to. If the client is disconnected that counts
as an unsub. Backend application connections *do not* generate sub and unsub
messages to other backend applications.
