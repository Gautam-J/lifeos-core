# Claude Code Telemetry Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Each Claude Code session sends its OpenTelemetry metrics, events and traces to the Pi. Grafana shows them. The sessions are the interactive sessions on the MacBook and on the Pi, `claude -p`, Agent SDK runs, the Code tab of the desktop app, Remote Control, and (optional) sessions on claude.ai/code.

**Dependency:** `docs/superpowers/plans/2026-10-15-prometheus-grafana.md` is complete first. Prometheus and Grafana run, and Grafana opens at `dash.<domain>`.

**Scope limit:** This plan adds no feature to the `lifeos` binary. It adds no Go code, no migration, no table and no MCP tool. The changes are Docker services, their config files, one public hostname, the Claude Code settings of each machine, one dashboard and documentation. Issue #45 (the `agent_events` table in Postgres, for `query` and dreaming) stays open for later.

**Architecture:** One OpenTelemetry Collector runs in Docker on the Pi, on `127.0.0.1:4318`. It is the one receiver of all telemetry. It refuses each request without the bearer token, replaces secrets in the text, and sends each signal to its store: metrics to Prometheus, events to Loki, traces to Tempo. Grafana reads the three stores. Clients on the Pi post to the loopback address. Clients outside the Pi post to the public hostname `otel.<domain>`. Each machine has the settings in the `env` block of `~/.claude/settings.json`.

```
Claude Code (MacBook: CLI, claude -p, desktop app, VS Code, Agent SDK)
  -> https://otel.<domain>/v1/{metrics,logs,traces}    Authorization: Bearer <token>
  -> cloudflared -> 127.0.0.1:4318
Claude Code on the Pi (CLI, claude -p, Remote Control)
  -> http://127.0.0.1:4318/v1/{metrics,logs,traces}    Authorization: Bearer <token>

OTel Collector: bearertokenauth -> memory_limiter -> transform/secrets -> batch
  metrics -> Prometheus  http://prometheus:9090/api/v1/otlp
  logs    -> Loki        http://loki:3100/otlp
  traces  -> Tempo       http://tempo:4318
Grafana (dash.<domain>) reads Prometheus, Loki and Tempo
```

**Tech Stack:** Docker Compose, `otel/opentelemetry-collector-contrib`, `grafana/loki` 3.x, `grafana/tempo`, Prometheus 3.x and Grafana of the dependency plan (pinned releases, linux/arm64), cloudflared, Claude Code 2.1.300, bash, `jq`, restic.

**Spec:** Issue #45, `docs/design.md` (sections "Deployment and security" and "Monitoring") and `AGENTS.md` (section "Deploy"). The Claude Code reference is https://code.claude.com/docs/en/monitoring-usage. The section "Decisions in this plan" lists where this plan differs from #45 and from `AGENTS.md`.

## Decisions in this plan

| # | Decision | Reason |
| --- | --- | --- |
| 1 | Metrics go to Prometheus, events to Loki, traces to Tempo. Grafana reads all three. | Prometheus stores numbers only. The prompts and the replies are text, so they need a log store; the traces need a trace store. Loki and Tempo are the stores that Grafana reads with no plugin. |
| 2 | The collector is the one front door. | Prometheus, Loki and Tempo have no bearer token check on their OTLP input. The collector checks the token one time, removes secrets one time, and gives the clients one base URL. A later store (such as #45) is one more exporter in the collector, and the clients do not change. |
| 3 | One bearer token for all clients. The collector checks it (`bearertokenauth`). The clients send it in `OTEL_EXPORTER_OTLP_HEADERS` as `Authorization=Bearer <token>`. | Gautam's choice: simple auth. |
| 4 | `otel.<domain>` has **no** Cloudflare Access application. This is an exception to the rule of `AGENTS.md` "Never add a hostname to the tunnel before its Access application exists", as `ntfy.` is. | An Access application needs its own headers (a service token), and decision 3 has one header only. The risk: the collector is open to the internet. The limits: the tunnel sends only the three paths `/v1/metrics`, `/v1/logs` and `/v1/traces`, the token has 256 bits, the body limit is 4 MB, and `memory_limiter` stops a flood. A holder of the token can write telemetry and read nothing. |
| 5 | Each machine has all settings in the `env` block of `~/.claude/settings.json`. | Gautam's choice. Two limits follow. An Agent SDK run sends telemetry only when it loads the user settings (`settingSources: ['user']`) or gets the variables in its environment. A session on claude.ai/code has no such file; Task 9 uses the variables of the cloud environment. |
| 6 | Traces are on (`CLAUDE_CODE_ENHANCED_TELEMETRY_BETA=1`, `OTEL_TRACES_EXPORTER=otlp`), although they are beta. | Gautam's choice. The traces show each prompt as a tree of model requests and tool calls. A later `lifeos job` can start a trace and give `TRACEPARENT` to `claude -p`. |
| 7 | Text on: prompts and replies (`OTEL_LOG_USER_PROMPTS=1`, `OTEL_LOG_ASSISTANT_RESPONSES=1`). Off for the first week: `OTEL_LOG_TOOL_DETAILS`. Always off: `OTEL_LOG_RAW_API_BODIES`, `OTEL_LOG_TOOL_CONTENT`. | Tool details include the full input of each tool, such as the content of each file that Claude writes. Task 10 measures a week and then decides. Raw API bodies send the whole conversation with each request. |
| 8 | The collector replaces secrets in logs and traces with `[removed: <kind of secret>]`. It uses the patterns of `internal/service/secrets.go`. | Rule 7 of `AGENTS.md`. The regex copy has no Luhn and no Verhoeff check, so it removes some other long numbers too. That is the safe side. |
| 9 | The clients send cumulative metrics (`OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE=cumulative`). Claude Code sends delta by default. | The OTLP input of Prometheus wants cumulative values. |
| 10 | The repository comes from `OTEL_METRICS_INCLUDE_REPOSITORY=true`. The machine comes from `OTEL_RESOURCE_ATTRIBUTES=host.name=<name>`. Prometheus makes `host.name` a label. | Issue #45 sets `project=<repo>` in each repository. The new attribute does that with no setting per repository. Claude Code does not expand `$(hostname)`, so the name is a fixed word per machine. |
| 11 | No store deletes data. Prometheus: `100y` and no size limit (dependency plan). Loki: the compactor deletes nothing (`retention_enabled: false`, no `retention_period`). Tempo: `block_retention: 876000h` (100 years). | Gautam's choice: no old data is deleted, also data older than a year. Tempo deletes after 14 days by default and has no "keep for good" value, so it gets a time that does not end. The disk can fill, so Task 10 measures the growth. |
| 12 | Loki accepts structured metadata up to 256 KB per line. The default is 64 KB. | The text of a reply goes into structured metadata. Claude Code cuts the text at 61,440 characters, which is more than 64 KB in UTF-8 for text that is not ASCII. With the default, Loki refuses those lines. |
| 13 | The Loki data and the Tempo data go into the nightly restic backup. The dependency plan backs up Prometheus. | No store deletes data, so a lost SD card must not lose it. |

## What this plan does not cover

- Chats in the Claude app and on claude.ai. They are not Claude Code and send no telemetry. The MCP call log (comment on #26) covers their LifeOS tool calls.
- Cowork. It has its own OpenTelemetry form in the admin console of a Team or Enterprise organization.
- The events in Postgres, for `query` and dreaming (#45).
- Alerts. They come with the rest of the section "Monitoring" of the design doc.

## Prerequisites

Step 1 of Task 1 checks these.

- The dependency plan is complete: `prometheus` and `grafana` run, and `deploy/check-access.sh` passes for `dash.<domain>`.
- `claude --version` on the Pi and on the MacBook prints 2.1.300 or later.
- Nothing listens on `127.0.0.1:4318`.
- The Pi has at least 1 GB of memory available. The collector, Loki and Tempo use about 600 MB together.
- `deploy/.env` exists. Never print a value of it.

## Global Constraints

- Each service listens on `127.0.0.1` only. Loki and Tempo publish no port at all: only the collector and Grafana reach them, on the Compose network.
- Run `deploy/check-access.sh` after each change of the tunnel.
- No domain, team name, audience tag or token goes into git, into an issue or into this plan.
- The commits are `chore` and `docs`, so they make no release.

---

## Task 1: Run Loki and Tempo

About 60 minutes.

**Files:**
- Create: `monitoring/loki.yaml`, `monitoring/tempo.yaml`
- Modify: `deploy/docker-compose.yml`, `monitoring/grafana/provisioning/datasources/datasources.yaml`

- [ ] **Step 1: Check the prerequisites**

```sh
sudo docker compose -f deploy/docker-compose.yml ps
claude --version
ss -ltn | grep 4318 || echo "4318 is free"
free -h
```

Expected: `prometheus` and `grafana` run, 2.1.300 or later, `4318 is free`, 1 GB or more available.

- [ ] **Step 2: Pick the releases**

For `grafana/loki`, `grafana/tempo` and `otel/opentelemetry-collector-contrib`, pick the newest release that is at least two weeks old and has a `linux/arm64` image. Read the release notes. The config files of this plan are for Loki 3.x and Tempo 2.x in monolithic mode. A different major release can need a different config.

- [ ] **Step 3: Make the data folders**

```sh
install -d -m 700 /var/lib/lifeos-monitoring/loki /var/lib/lifeos-monitoring/tempo
```

- [ ] **Step 4: Write `monitoring/loki.yaml`**

```yaml
# Loki stores the events of Claude Code: one line for each prompt, reply,
# tool result and API request. It deletes nothing: the compactor has no
# retention, and limits_config has no retention_period.
auth_enabled: false

server:
  http_listen_port: 3100

common:
  path_prefix: /loki
  replication_factor: 1
  ring:
    kvstore:
      store: inmemory
  storage:
    filesystem:
      chunks_directory: /loki/chunks
      rules_directory: /loki/rules

compactor:
  working_directory: /loki/compactor
  # Delete no data (decision 11).
  retention_enabled: false

schema_config:
  configs:
    - from: 2026-10-01
      store: tsdb
      object_store: filesystem
      schema: v13
      index:
        prefix: index_
        period: 24h

limits_config:
  allow_structured_metadata: true
  # A reply can be 61,440 characters; the default of 64 KB refuses it.
  max_structured_metadata_size: 256KB
  volume_enabled: true

analytics:
  reporting_enabled: false
```

- [ ] **Step 5: Write `monitoring/tempo.yaml`**

```yaml
# Tempo stores the traces of Claude Code. It deletes nothing in practice:
# the default retention is 14 days, so block_retention is 100 years.
server:
  http_listen_port: 3200

distributor:
  receivers:
    otlp:
      protocols:
        http:
          endpoint: 0.0.0.0:4318

storage:
  trace:
    backend: local
    wal:
      path: /var/tempo/wal
    local:
      path: /var/tempo/blocks

compactor:
  compaction:
    block_retention: 876000h

usage_report:
  reporting_enabled: false
```

- [ ] **Step 6: Add the two services to `deploy/docker-compose.yml`**

```yaml
  # The events of Claude Code. Only the OTel collector writes here and only
  # Grafana reads, both on the Compose network, so no port is published.
  loki:
    image: grafana/loki:<tag of Step 2>
    restart: unless-stopped
    user: "1000:1000"
    command: ["-config.file=/etc/loki/config.yaml"]
    volumes:
      - ../monitoring/loki.yaml:/etc/loki/config.yaml:ro
      - /var/lib/lifeos-monitoring/loki:/loki
    mem_limit: 384m

  # The traces of Claude Code. As Loki: no published port.
  tempo:
    image: grafana/tempo:<tag of Step 2>
    restart: unless-stopped
    user: "1000:1000"
    command: ["-config.file=/etc/tempo/config.yaml"]
    volumes:
      - ../monitoring/tempo.yaml:/etc/tempo/config.yaml:ro
      - /var/lib/lifeos-monitoring/tempo:/var/tempo
    mem_limit: 384m
```

- [ ] **Step 7: Add the data sources**

Add to `monitoring/grafana/provisioning/datasources/datasources.yaml`:

```yaml
  - name: Loki
    uid: loki
    type: loki
    access: proxy
    url: http://loki:3100
  - name: Tempo
    uid: tempo
    type: tempo
    access: proxy
    url: http://tempo:3200
```

- [ ] **Step 8: Start and check**

```sh
sudo docker compose -f deploy/docker-compose.yml up -d loki tempo
sudo docker compose -f deploy/docker-compose.yml restart grafana
sudo docker compose -f deploy/docker-compose.yml logs --tail 20 loki tempo
sudo docker compose -f deploy/docker-compose.yml exec grafana wget -qO- http://loki:3100/ready
sudo docker compose -f deploy/docker-compose.yml exec grafana wget -qO- http://tempo:3200/ready
```

Expected: `ready` twice, and no error in the logs. In Grafana, Connections shows a successful test for Loki and for Tempo. `ss -ltn` shows no port 3100 and no port 3200. Then check that no store deletes data:

```sh
sudo docker compose -f deploy/docker-compose.yml exec grafana wget -qO- http://loki:3100/config | grep -E 'retention_enabled|retention_period'
sudo docker compose -f deploy/docker-compose.yml exec grafana wget -qO- http://tempo:3200/status/config | grep block_retention
```

Expected: `retention_enabled: false`, `retention_period: 0s`, and `block_retention: 876000h0m0s` (or the same value in a different form). If Tempo refuses the value, use the largest value that it takes, and write it into decision 11.

- [ ] **Step 9: Commit**

```sh
git add monitoring/loki.yaml monitoring/tempo.yaml monitoring/grafana deploy/docker-compose.yml
git commit -m "chore(deploy): run Loki and Tempo as Grafana data sources"
```

## Task 2: Let Prometheus take OTLP metrics

About 20 minutes.

**Files:**
- Modify: `deploy/docker-compose.yml`, `monitoring/prometheus.yml`

- [ ] **Step 1: Write the failing check**

```sh
curl -sS -o /dev/null -w '%{http_code}\n' -X POST -H 'Content-Type: application/json' -d '{}' \
  http://127.0.0.1:9090/api/v1/otlp/v1/metrics
```

Expected now: not 200. The OTLP input is off.

- [ ] **Step 2: Turn on the OTLP input**

Add `--web.enable-otlp-receiver` to the `command` of `prometheus` in `deploy/docker-compose.yml`. Check the flag in the release notes of the pinned release. Add this to `monitoring/prometheus.yml`:

```yaml
# The OTel collector sends the metrics of Claude Code here (OTLP). The two
# resource attributes become labels, so a query can group by machine and
# by repository.
otlp:
  promote_resource_attributes:
    - host.name
    - vcs.repository.name

storage:
  tsdb:
    # Clients send at different times; take samples up to 30 minutes late.
    out_of_order_time_window: 30m
```

- [ ] **Step 3: Restart and check**

```sh
sudo docker compose -f deploy/docker-compose.yml up -d prometheus
curl -sS -o /dev/null -w '%{http_code}\n' -X POST -H 'Content-Type: application/json' -d '{}' \
  http://127.0.0.1:9090/api/v1/otlp/v1/metrics
```

Expected: 200.

- [ ] **Step 4: Commit**

```sh
git add deploy/docker-compose.yml monitoring/prometheus.yml
git commit -m "chore(deploy): let Prometheus take OTLP metrics"
```

## Task 3: Run the collector with the token check

About 60 minutes.

**Files:**
- Create: `monitoring/otel-collector.yaml`
- Modify: `deploy/docker-compose.yml`, `deploy/.env.example`, `deploy/systemd/lifeos.service`, `internal/service/secrets.go` (a comment only)

- [ ] **Step 1: Make the token**

Add to `deploy/.env.example`:

```sh
# OTel collector (docker-compose.yml): the bearer token of the Claude Code
# telemetry. Each client sends it as OTEL_EXPORTER_OTLP_HEADERS
# "Authorization=Bearer <token>" in ~/.claude/settings.json.
OTEL_BEARER_TOKEN=
```

Add `OTEL_BEARER_TOKEN` to `UnsetEnvironment` in `deploy/systemd/lifeos.service`. Run `deploy/test.sh`. Put a value into `deploy/.env` with `openssl rand -hex 32`, and keep a copy in the password manager.

- [ ] **Step 2: Write `monitoring/otel-collector.yaml`**

```yaml
# OpenTelemetry Collector for the Claude Code telemetry. It refuses each
# request without the bearer token, removes secrets from the text and sends
# each signal to its store. A later store (#45) is one more exporter here.
extensions:
  bearertokenauth:
    token: ${env:OTEL_BEARER_TOKEN}

receivers:
  otlp:
    protocols:
      http:
        # 0.0.0.0 inside the container; Docker publishes it on 127.0.0.1 only.
        endpoint: 0.0.0.0:4318
        max_request_body_size: 4194304
        auth:
          authenticator: bearertokenauth

processors:
  memory_limiter:
    check_interval: 1s
    limit_mib: 200
    spike_limit_mib: 50
  # A copy of the patterns of internal/service/secrets.go (rule 7 of
  # AGENTS.md). No Luhn and no Verhoeff check, so some other long numbers
  # are removed too.
  transform/secrets:
    error_mode: ignore
    log_statements:
      - context: log
        statements: &secrets
          - replace_all_patterns(attributes, "value", "(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?(?:-----END [A-Z ]*PRIVATE KEY-----|\\z)", "[removed: private key]")
          - replace_all_patterns(attributes, "value", "\\b(?:AKIA|ASIA)[0-9A-Z]{16}\\b", "[removed: access token]")
          - replace_all_patterns(attributes, "value", "\\bgh[pousr]_[A-Za-z0-9]{36,}", "[removed: access token]")
          - replace_all_patterns(attributes, "value", "\\bgithub_pat_[A-Za-z0-9_]{22,}", "[removed: access token]")
          - replace_all_patterns(attributes, "value", "\\bxox[abprs]-[A-Za-z0-9-]{10,}", "[removed: access token]")
          - replace_all_patterns(attributes, "value", "\\bsk-[A-Za-z0-9_-]{20,}", "[removed: access token]")
          - replace_all_patterns(attributes, "value", "\\b[A-Z]{3}[ABCFGHJLPT][A-Z][0-9]{4}[A-Z]\\b", "[removed: PAN]")
          - replace_all_patterns(attributes, "value", "\\b(?:[0-9][ -]?){12,18}[0-9]\\b", "[removed: card number]")
          - replace_all_patterns(attributes, "value", "\\b[2-9][0-9]{3}[ -]?[0-9]{4}[ -]?[0-9]{4}\\b", "[removed: Aadhaar number]")
    trace_statements:
      - context: span
        statements: *secrets
  batch: {}

exporters:
  otlphttp/prometheus:
    endpoint: http://prometheus:9090/api/v1/otlp
  otlphttp/loki:
    endpoint: http://loki:3100/otlp
  otlphttp/tempo:
    endpoint: http://tempo:4318

service:
  extensions: [bearertokenauth]
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [memory_limiter, batch]
      exporters: [otlphttp/prometheus]
    logs:
      receivers: [otlp]
      processors: [memory_limiter, transform/secrets, batch]
      exporters: [otlphttp/loki]
    traces:
      receivers: [otlp]
      processors: [memory_limiter, transform/secrets, batch]
      exporters: [otlphttp/tempo]
```

The card pattern runs before the Aadhaar pattern, so a run of 13 to 19 digits is a card number and a run of 12 digits is an Aadhaar number. Check the names of the kinds against the error texts of `secrets.go`, and use the same words. If the collector refuses the YAML anchor (`&secrets`), write the nine statements two times. Add a comment to `internal/service/secrets.go`: a change of a pattern there needs the same change in `monitoring/otel-collector.yaml`.

- [ ] **Step 3: Add the collector to `deploy/docker-compose.yml`**

```yaml
  # Receives the OpenTelemetry data of each Claude Code session and sends it
  # to Prometheus, Loki and Tempo. Clients on the Pi post to this address;
  # other clients post to the public OTel hostname. The image has no shell,
  # so no healthcheck.
  otel-collector:
    image: otel/opentelemetry-collector-contrib:<tag of Task 1 Step 2>
    restart: unless-stopped
    command: ["--config=/etc/otelcol/config.yaml"]
    environment:
      OTEL_BEARER_TOKEN: ${OTEL_BEARER_TOKEN:?set OTEL_BEARER_TOKEN in deploy/.env}
    ports:
      - "127.0.0.1:4318:4318"
    volumes:
      - ../monitoring/otel-collector.yaml:/etc/otelcol/config.yaml:ro
    depends_on:
      - prometheus
      - loki
      - tempo
    mem_limit: 256m
```

- [ ] **Step 4: Check the token**

```sh
sudo docker compose -f deploy/docker-compose.yml up -d otel-collector
sudo docker compose -f deploy/docker-compose.yml logs --tail 20 otel-collector
set -a; . deploy/.env; set +a
for auth in "" "Authorization: Bearer wrong" "Authorization: Bearer $OTEL_BEARER_TOKEN"; do
  curl -sS -o /dev/null -w '%{http_code}\n' -X POST -H 'Content-Type: application/json' ${auth:+-H "$auth"} \
    -d '{}' http://127.0.0.1:4318/v1/logs
done
ss -ltn | grep 4318
```

Expected: `Everything is ready` in the log. Then `401`, `401`, `200`. `ss` shows `127.0.0.1:4318` only.

- [ ] **Step 5: Check the three stores and the scrub**

Make three OTLP JSON bodies in the scratchpad:

1. `metric.json`: one cumulative sum `lifeos_test_total` with the value 1 and the resource attribute `host.name=test`.
2. `log.json`: one log record with the body `test` and the attribute `prompt` = `my card is 4111 1111 1111 1111 and key AKIAABCDEFGHIJKLMNOP`, resource `service.name=lifeos-test`.
3. `trace.json`: one span `test` with the attribute `prompt` = the same text, resource `service.name=lifeos-test`.

Post each with the token to `/v1/metrics`, `/v1/logs` and `/v1/traces`. Then:

```sh
sleep 10
curl -sS 'http://127.0.0.1:9090/api/v1/query?query=lifeos_test_total' | jq '.data.result[0].metric'
sudo docker compose -f deploy/docker-compose.yml exec grafana wget -qO- \
  'http://loki:3100/loki/api/v1/query_range?query=%7Bservice_name%3D%22lifeos-test%22%7D&limit=1' | jq -c '.data.result[0]'
```

Expected: Prometheus has the series with `host_name="test"`. The Loki line has `[removed: card number]` and `[removed: access token]`, and no `4111`. In Grafana Explore, Tempo finds the trace of `service.name=lifeos-test`, and its `prompt` attribute is scrubbed too.

- [ ] **Step 6: Commit**

```sh
git add monitoring/otel-collector.yaml deploy/docker-compose.yml deploy/.env.example deploy/systemd/lifeos.service internal/service/secrets.go
git commit -m "chore(deploy): add an OTel collector that checks a bearer token and removes secrets"
```

## Task 4: Send the telemetry of the Pi sessions

About 30 minutes.

**Files (not in git):**
- Modify: `~/.claude/settings.json` on the Pi

- [ ] **Step 1: Add the `env` block**

Merge these keys into the `env` block of `~/.claude/settings.json`. Keep the other keys of the file. Put the token of `deploy/.env` in place of `<token>`. Make the file mode 600 (`chmod 600 ~/.claude/settings.json`), because it holds the token.

```json
{
  "env": {
    "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
    "CLAUDE_CODE_ENHANCED_TELEMETRY_BETA": "1",
    "OTEL_METRICS_EXPORTER": "otlp",
    "OTEL_LOGS_EXPORTER": "otlp",
    "OTEL_TRACES_EXPORTER": "otlp",
    "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
    "OTEL_EXPORTER_OTLP_ENDPOINT": "http://127.0.0.1:4318",
    "OTEL_EXPORTER_OTLP_HEADERS": "Authorization=Bearer <token>",
    "OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE": "cumulative",
    "OTEL_LOG_USER_PROMPTS": "1",
    "OTEL_LOG_ASSISTANT_RESPONSES": "1",
    "OTEL_METRICS_INCLUDE_ENTRYPOINT": "true",
    "OTEL_METRICS_INCLUDE_VERSION": "true",
    "OTEL_METRICS_INCLUDE_REPOSITORY": "true",
    "OTEL_RESOURCE_ATTRIBUTES": "host.name=pi"
  }
}
```

- [ ] **Step 2: Run one `claude -p` in this repository**

```sh
cd ~/lifeos-core
claude -p "Say the word ok." --debug-file /tmp/claude-otel-debug.log
sleep 70
grep '3P telemetry' /tmp/claude-otel-debug.log | tail -5
```

Expected: no export error. The 70-second wait covers the 60-second metric interval. The docs say: `OTEL_EXPORTER_OTLP_*` keys are ignored in `.claude/settings.json` of a project. If the debug log shows that the user file is ignored too, stop and tell Gautam.

- [ ] **Step 3: Check the three stores**

```sh
curl -sS 'http://127.0.0.1:9090/api/v1/label/__name__/values' | jq -r '.data[]' | grep claude_code
curl -sS 'http://127.0.0.1:9090/api/v1/query?query=claude_code_session_count_total' | jq '.data.result[].metric'
```

Expected: the metric names of Claude Code with the suffixes of Prometheus (for example `claude_code_token_usage_tokens_total`). Write the real names down for Task 7. The session series has `host_name="pi"` and `vcs_repository_name="lifeos-core"`. If the repository label is absent, the attribute is a data point attribute, not a resource attribute; write that down for Task 7.

In Grafana Explore:

1. Loki, query `{service_name="claude-code"}`: the lines `user_prompt`, `api_request` and `assistant_response` of this run. The structured metadata `prompt` is `Say the word ok.`, not `<REDACTED>`.
2. Tempo, search `service.name=claude-code`: one trace with a `claude_code.interaction` span and its `claude_code.llm_request` child.

Write down the label and metadata names that Loki shows (Loki changes the dots to underscores). Task 7 uses them.

- [ ] **Step 4: Check Remote Control**

If `claude remote-control` runs on the Pi, give it one prompt from the phone. Check that Loki has the prompt with `host_name="pi"`.

## Task 5: Open `otel.<domain>`

About 30 minutes.

**Files:**
- Modify: `/etc/cloudflared/config.yml` (not in git), `deploy/check-access.sh`, `deploy/test.sh`, `deploy/.env.example`, `deploy/systemd/lifeos.service`

- [ ] **Step 1: Add the check first**

1. Add `LIFEOS_OTEL_HOST=` to `deploy/.env.example`, next to the other hostnames, with the comment: "the OTel hostname has no Access application; the collector checks the bearer token". Add it to `UnsetEnvironment` in `deploy/systemd/lifeos.service`.
2. In `deploy/check-access.sh`, add a function or a block for the OTel hostname. It sends `POST /v1/logs` with the body `{}` and `Content-Type: application/json` three times:
   - with no token: want 401,
   - with `Authorization: Bearer wrong`: want 401,
   - with `Authorization: Bearer $OTEL_BEARER_TOKEN`: want 200.
   It also sends `GET /` with the token: want 404 from the tunnel. The 401 here comes from the collector, not from Access, so the block does not use `guarded`. Say that in a comment.
3. Add the cases to `deploy/test.sh`. Run it and see the new cases fail before the change of `check-access.sh`, then pass after it.

- [ ] **Step 2: Add the tunnel rule**

Back up the file first: `sudo cp /etc/cloudflared/config.yml /etc/cloudflared/config.yml.bak-$(date +%Y%m%d-%H%M%S)`. Add this rule before the last `http_status:404` rule. It has no `access` part (decision 4):

```yaml
  - hostname: otel.<domain>
    path: ^/v1/(metrics|logs|traces)$
    service: http://127.0.0.1:4318
```

Add the DNS route with `cloudflared tunnel route dns <tunnel> otel.<domain>`. Restart `cloudflared`. Put the hostname into `deploy/.env`.

- [ ] **Step 3: Check**

```sh
deploy/test.sh
deploy/check-access.sh
```

Expected: both pass. The OTel lines show 401, 401, 200 and 404.

- [ ] **Step 4: Commit**

```sh
git add deploy/check-access.sh deploy/test.sh deploy/.env.example deploy/systemd/lifeos.service
git commit -m "chore(deploy): check that the OTel hostname answers only with the bearer token"
```

## Task 6: Send the telemetry of the MacBook

About 30 minutes. All steps run on the MacBook.

- [ ] **Step 1: Add the `env` block**

Merge the block of Task 4 Step 1 into `~/.claude/settings.json`, with two changes:

- `"OTEL_EXPORTER_OTLP_ENDPOINT": "https://otel.<domain>"`
- `"OTEL_RESOURCE_ATTRIBUTES": "host.name=macbook"`

Run `chmod 600 ~/.claude/settings.json`.

- [ ] **Step 2: Check each entrypoint**

Run each one with one short prompt. Then check in Grafana Explore (Loki) that a `user_prompt` with `host_name="macbook"` and the expected entrypoint arrived:

| Entrypoint | How to start it | Expected `app.entrypoint` |
| --- | --- | --- |
| Terminal | `claude`, one prompt, `/exit` | `cli` |
| Print mode | `claude -p "Say ok."` | write down what it is |
| Desktop app | Code tab, one prompt in a local folder | write down what it is |
| VS Code | the Claude Code extension, one prompt | `claude-vscode` |
| Agent SDK | a short `query()` script with `settingSources: ['user']` | `sdk-ts` or `sdk-py` |

A row that is missing is a finding: write it into Task 10 and do not change the plan on your own.

## Task 7: Make the dashboard "Claude usage"

About 60 minutes.

**Files:**
- Create: `dashboards/claude-usage.json`, `monitoring/grafana/provisioning/dashboards/lifeos.yaml`
- Modify: `deploy/docker-compose.yml`

- [ ] **Step 1: Add the dashboard provider**

```yaml
# Grafana loads the dashboards of dashboards/. A change in Grafana is not
# saved; change the JSON file in git.
apiVersion: 1
providers:
  - name: lifeos
    folder: LifeOS
    type: file
    allowUiUpdates: false
    options:
      path: /var/lib/grafana-dashboards
```

Add `- ../dashboards:/var/lib/grafana-dashboards:ro` to the volumes of `grafana`.

- [ ] **Step 2: Build the dashboard in Grafana, then export it**

Use the metric names and labels of Task 4 Step 3. The days are in IST (dashboard time zone `Asia/Kolkata`). Panels:

| Panel | Source | Query (with the real names) |
| --- | --- | --- |
| Tokens per day, by type | Prometheus | `sum by (type) (increase(claude_code_token_usage_tokens_total[1d]))` |
| Estimated cost per day (API prices, not the Pro plan) | Prometheus | `sum(increase(claude_code_cost_usage_USD_total[1d]))` |
| Active time per day | Prometheus | `sum by (type) (increase(claude_code_active_time_seconds_total[1d]))` |
| Sessions per day, by machine and repository | Prometheus | `sum by (host_name, vcs_repository_name) (increase(claude_code_session_count_total[1d]))` |
| Lines changed, commits, pull requests per day | Prometheus | the three counters, `increase(...[1d])` |
| Prompts | Loki | `{service_name="claude-code"} \| event_name="user_prompt"`, the columns time, session, repository, prompt |
| Tool errors | Loki | `{service_name="claude-code"} \| event_name="tool_result" \| success="false"` |

Export the dashboard as JSON (Share, Export, with "Export for sharing externally" off). Save it as `dashboards/claude-usage.json`. Remove the `id` field and keep the `uid`.

- [ ] **Step 3: Check the provisioned dashboard**

```sh
sudo docker compose -f deploy/docker-compose.yml up -d grafana
```

Expected: the folder LifeOS in Grafana has "Claude usage", and each panel shows the data of Task 4 and Task 6.

- [ ] **Step 4: Commit**

```sh
git add dashboards/claude-usage.json monitoring/grafana/provisioning/dashboards deploy/docker-compose.yml
git commit -m "chore(dashboards): add the Claude usage dashboard"
```

## Task 8: Back up Loki and Tempo, and document

About 45 minutes.

**Files:**
- Modify: `deploy/backup.sh`, `docs/design.md`, `AGENTS.md`, `README.md` (if it lists the services), `assets/architecture.svg`
- Create: `docs/telemetry.md`

- [ ] **Step 1: Add the Loki and Tempo data to the backup**

In `deploy/backup.sh`, add `/var/lib/lifeos-monitoring/loki` and `/var/lib/lifeos-monitoring/tempo` to the two `restic backup` commands, with a comment. As with Prometheus, the written blocks do not change, and only the newest data in the write-ahead log can be cut. `backup.sh` has no `restic forget`, so restic keeps each snapshot. Run `deploy/test.sh`. Run `deploy/backup.sh` one time and check with `restic ls latest | grep -E 'lifeos-monitoring/(loki|tempo)'`.

- [ ] **Step 2: Write `docs/telemetry.md`**

The page has these parts, with no token and no domain:

1. What is collected, and what is not (the list "What this plan does not cover").
2. The `env` block of a machine (Task 4 Step 1, with `<token>` and `<domain>`).
3. How to add a machine, and how to change the token: a new value in `deploy/.env`, restart `otel-collector`, change each `~/.claude/settings.json`.
4. How to find a conversation: the dashboard, then Loki in Explore by `session_id`, then the trace in Tempo.
5. An Agent SDK run needs `settingSources: ['user']`.

- [ ] **Step 3: Update the design doc and `AGENTS.md`**

- `docs/design.md`, "Deployment and security": add the collector (`127.0.0.1:4318`), Loki and Tempo (no published port) to the component table. Add the `otel.` row to the hostname table: no Access application, the collector checks a bearer token. Add the memory of the three to the memory table.
- `docs/design.md`, "Monitoring": one paragraph on the Claude Code telemetry, with a link to `docs/telemetry.md`.
- `AGENTS.md`, "Deploy": the OTel hostname is the second hostname without Access, after ntfy; its lock is the bearer token of the collector, and `check-access.sh` checks it.
- `assets/architecture.svg`: add the collector, Loki, Tempo and the OTel hostname.

- [ ] **Step 4: Commit**

```sh
git add deploy/backup.sh
git commit -m "chore(deploy): back up the Loki and Tempo data"
git add docs/telemetry.md docs/design.md AGENTS.md README.md assets/architecture.svg
git commit -m "docs: describe the Claude Code telemetry setup"
```

## Task 9 (optional): Sessions on claude.ai/code

About 30 minutes, then one test.

- [ ] **Step 1: Set the cloud environment**

In each cloud environment that Gautam uses:

1. Set the network access to a custom list that allows `otel.<domain>`, in addition to what it allows now.
2. Add the variables of Task 6 Step 1, with `host.name=cloud`.

The docs advise against a collector credential in the variables of a cloud environment, because each session of that environment can read it. With decision 4, the token lets a holder write telemetry and read nothing.

- [ ] **Step 2: Test**

Start one cloud session and give one prompt. Check in Loki for a `user_prompt` with `host_name="cloud"` and a `ccr_session_id`. If nothing arrives, read the debug log of the session for `[3P telemetry]` lines. Write the result into the open question of #45.

## Task 10: One week later, measure and decide

About 30 minutes, on 2026-10-22 or later.

- [ ] **Step 1: Measure**

```sh
du -sh /var/lib/lifeos-monitoring/*
sudo docker stats --no-stream
free -h
```

- [ ] **Step 2: Check the coverage**

In Loki, count the `user_prompt` lines of the week by `host_name` and `app_entrypoint`. Each machine and each entrypoint that Gautam used has lines. A missing one is a finding.

- [ ] **Step 3: Decide with Gautam**

1. Turn on `OTEL_LOG_TOOL_DETAILS`? Base: the size of a week and the use of MCP tool names.
2. The disk: at the growth of this week, in how many months is the disk 85% full? No store deletes data (decision 11), so the answer tells when the disk-filling alert of the design doc is needed. Deletion is not an option.
3. Is #45 (events in Postgres, for `query` and dreaming) still needed, now that Loki has the events?

- [ ] **Step 4: Comment on #45**

Add a comment to #45 (no private content): metrics are in Prometheus, events in Loki and traces in Tempo through the collector; part 4 of #45 (Access with a service token, a split of the paths in the tunnel) is replaced by decisions 2 to 4 of this plan.

---

## Time

| Task | Time |
| --- | --- |
| Dependency: the Prometheus and Grafana plan | 2.5 hours |
| 1. Loki and Tempo | 60 min |
| 2. OTLP input of Prometheus | 20 min |
| 3. Collector and token | 60 min |
| 4. Pi sessions | 30 min |
| 5. `otel.<domain>` | 30 min |
| 6. MacBook | 30 min |
| 7. Dashboard | 60 min |
| 8. Backup and docs | 45 min |
| 9. claude.ai/code (optional) | 30 min |
| 10. Review after one week | 30 min |
| Total | about 9 hours with the dependency, 9.5 with Task 9 |

After Task 4, every session on the Pi shows in Grafana. After Task 6, every session on the MacBook shows too.
