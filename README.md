# otter

Real-time pubsub for websocket clients.

This is ALPHA software and is under active development, the API may change at
any time.

## Features

* Signature-based authentication
* Publish/Subscribe to arbitrarily named channels
* Presence information (i.e. named clients)
* Per-channel list of clients subscribed
* Alerts to backend on client subscribing, unsubscribing, or disconnecting
* Redis or redis cluster used as backend, multiple otter instances can run
  independently and be used interchangeably

## Model

Otter is modeled to be a very simple, if somewhat restrictive communication
layer between a backend application and a large set of clients. The backend
application is completely arbitrary and must be written by you. When the backend
publishes to a channel all clients on that channel will receive the publish.
When a client publishes to a channel, ONLY backend applications subscribed to
that channel will receive it.

The interaction on the client-side is fairly straightforward:

* (Possibly optional, depending on config) Authenticate connection with
  signature previously retrieved from backend. Authentication also includes
  presence information.
* Subscribe to desired channels.
* Read publishes from these channels.
* Publish to any channel to communicate with backend. A channel does not need to
  be subscribed to to be published to.

The interaction for the backend is fairly similar:

* Authenticate to prove that connection is backend.
* Subscribe to desired channels.
* Receive publishes from clients on these channels.
* Send publishes to clients on any channel. A channel does not need to be
  subscribed to to be published to.
* Provide authentication signatures to clients out-of-band, if necessary.

## Authentication

Connections to otter can optionally identify themselves using a "presence"
string, which is a string containing any arbitrary text that the connection
wants. This string, if given, *must* be accompanied by a signature. Signatures
are generated by the backend application and subsequently checked by otter when
it sees them, using the secret that both the backend and otter share.

*Presence strings are always optional*.

A signature of some presence string `arbitrary_string` is generated like so:

```
// hex encoded result of
SHA256HMAC(current_timestamp + "_" + arbitrary_string, secret)
```

where `current_timestamp` is the unix timestamp in string form.

For backend applications which are connecting to otter, the presence string must
be the string `"backend"`.

## Subscribing

Channels are subscribed to by making a websocket connection to an endpoint like
so:

```
GET ws://otterhost/subs/<channel1>,<channel2>?presence=arbitrary&sig=sig
```

Once subscribed otter will push publishes directed at any of the channels to the
connection. Remember that clients only receive publishes from backend
applications, and vice-versa. Publishes look like this:

```json
{
    "type":"pub",
    "channel":"channel name",
    "message":{"foo":"bar"},
    "connection":{
        "id":"connection id",
        "presence":"some string"
    }
}
```

The `connection` field gives the connection ID and the `presence` information of
the publishing client (if they did sent any, otherwise the field is omitted).
If the client is receiving a publish it can assume it came from a backend
application, and vice-versa.

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
backend application has subscribed to. A client is considered unsubscribed when
its connection is closed. Backend application connections *do not* generate sub
and unsub messages to other backend applications.

## Publishing

Publishes are accomplished by POSTing to a channel's (or multiple channels')
endpoint:

```
POST http://otterhost/subs/<channel1>,<channel2>?presence=arbitrary&sig=sig

{"foo":"bar"}
```

The POST body can be any arbitrary json, and will appear as the `message` field
in the publishes that subscribed clients receive.
