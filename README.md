# mmdump

`mmdump` is a simple tool for non-admins to export data from Mattermost.

**Pre-alpha software**. Expect crashes, data loss, silent data corruption etc.

# Rationale

It doesn't seem possible to export data from Mattermost without being an admin.

# Installation

```bash
$ go install github.com/tomyl/mmdump@latest
```

# Usage

1. Log in to the web version of Mattermost.
2. Copy the session cookie e.g. using Inspect -> Network in your brower.
3. Run
```bash
$ mmdump -endpoint https://mattermost.example.com/api/v4/ -dir mydumpdir -cookie <COOKIE>
```
By default posts in all channels are dumped. Add `-channel <CHANNELID>` to dump a single channel.

To lists channels in a dump:
```bash
$ mmdump -dir mydumpdir -channels
```

To lists posts for a channel:
```bash
$ mmdump -dir mydumpdir -posts <CHANNELID>
```

# TODO
* Currently not supporting pagination when fetching channels.
* Nicer CLI interface.
* Nicer posts output e.g. show reactions.
