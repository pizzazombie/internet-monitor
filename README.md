# Internet Monitor

Tiny local service that keeps score when your internet decides to become a performance artist.

It runs as a single lightweight Go process, stores history in flat files, exposes a web UI on `localhost:5555`, and helps answer a boring but important question:

`Was my internet actually down, or was it just being weird again?`

## What it does

- checks internet reachability on a schedule using TCP, HTTP, and DNS probes;
- stores history in compact `.ndjson` files, no external database required;
- shows uptime, outage count, downtime, latency, and timeline charts in a local UI;
- supports optional low-frequency speed samples;
- runs comfortably in Docker as one small background service.

## Why this exists

Home internet failures are annoying because they are vague.

Sometimes Wi-Fi drops.
Sometimes the router is fine but the provider is down.
Sometimes everything looks fine until a call freezes and reality disagrees.

This service gives you a local timeline so you can stop arguing with your memory and start pointing at data instead.

## How it works

The monitor performs lightweight checks every `15s` by default:

- TCP probe to `1.1.1.1:443`
- HTTP probe to `https://cp.cloudflare.com/generate_204`
- DNS lookup for `example.com`

Each result is appended to daily `.ndjson` files under `/data`, and the built-in web app reads that history to render charts for arbitrary periods.

Optional speed checks are disabled by default. If you turn them on, they run rarely and download only a small payload, so they should not meaningfully affect normal usage unless you choose an aggressive schedule.

## LAN vs ISP failures

From inside a container, you cannot perfectly diagnose whether the exact problem was:

- your laptop's Wi-Fi;
- your router/local network;
- your ISP uplink;
- or some upstream routing issue.

What the service can do well is determine whether internet connectivity was available.

If you want a better hint about local-vs-upstream failures, set `IM_LAN_PROBE_ADDRESS` to something on your LAN, usually your router:

```yaml
IM_LAN_PROBE_ADDRESS: "192.168.0.1:80"
```

With that enabled, the monitor can label events more usefully:

- `lan reachable, upstream internet likely down`
- `internet unreachable, local network may be down`

Not perfect forensic truth, but much better than `eh, it glitched`.

## Run with Docker

```bash
docker compose up --build -d
```

Then open:

- UI: [http://localhost:5555](http://localhost:5555)
- health: [http://localhost:5555/healthz](http://localhost:5555/healthz)

Stop it with:

```bash
docker compose down
```

## Configuration

Key environment variables:

- `IM_CHECK_INTERVAL` — probe frequency, default `15s`
- `IM_REQUEST_TIMEOUT` — timeout per probe, default `3s`
- `IM_RETENTION_DAYS` — history retention, default `180`
- `IM_ENABLE_SPEED_TEST` — enable speed samples, default `false`
- `IM_SPEED_TEST_INTERVAL` — speed sample interval, default `6h`
- `IM_SPEED_TEST_BYTES` — bytes downloaded per speed sample, default `262144`
- `IM_SPEED_TEST_URL` — download URL used for the speed sample
- `IM_TCP_PROBE_ADDRESS` — external TCP probe target
- `IM_HTTP_PROBE_URL` — external HTTP probe target
- `IM_DNS_PROBE_HOST` — hostname used for DNS checks
- `IM_LAN_PROBE_ADDRESS` — optional LAN target such as a router IP and port

## Development

```bash
env GOCACHE=$(pwd)/.gocache go test ./...
env GOCACHE=$(pwd)/.gocache go run ./cmd/internet-monitor
```

## Stack

- Go 1.24
- embedded static frontend
- flat-file NDJSON storage
- Docker / Docker Compose

## Privacy and secrets

The service does not require API keys, accounts, or cloud services.

In the current version of this repo, there are no obvious secrets or passwords checked into the project. I still recommend a final human skim before making it public, because computers are fast and overconfident.
