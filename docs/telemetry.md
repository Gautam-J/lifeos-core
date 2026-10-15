# Claude Code telemetry

Each Claude Code session sends its OpenTelemetry data to the Pi. Grafana shows it in the dashboard "Claude usage" (folder LifeOS, at `dash.<domain>`).

```
Claude Code on the Pi          -> http://127.0.0.1:4318
Claude Code on another machine -> https://otel.<domain>   (the tunnel sends only /v1/metrics, /v1/logs, /v1/traces)
   Authorization: Bearer <token>

OTel collector (127.0.0.1:4318): token check, memory limit, secret removal
  metrics -> Prometheus   events -> Loki   traces -> Tempo
```

The config files are in `monitoring/`: `otel-collector.yaml`, `loki.yaml`, `tempo.yaml` and `prometheus.yml`. The services are in `deploy/docker-compose.yml`.

## What is collected

| Signal | Store | What |
| --- | --- | --- |
| Metrics | Prometheus | Sessions, tokens, cost at API prices, active time, lines of code, commits, pull requests, edit decisions. The labels include `host_name`, `app_entrypoint`, `session_id`, `model` and `vcs_repository_name`. |
| Events | Loki | One line for each prompt (with its text), reply (with its text), API request, API error, tool decision, tool result, hook and MCP server connection. `host_name` and `service_name` are labels; each other field is structured metadata. |
| Traces | Tempo | One trace for each prompt: a `claude_code.interaction` span with a `claude_code.llm_request` span for each model request, and the tool calls. The spans hold the prompt text and the model output too. |

The sessions are the terminal (`cli`), `claude -p` (`sdk-cli`), the Agent SDK, VS Code, the Code tab of the desktop app and Remote Control. Each one reads the `env` block of `~/.claude/settings.json` of its machine.

Not collected:

- Chats in the Claude app and on claude.ai. They are not Claude Code.
- Sessions on claude.ai/code.
- Cowork. It has its own OpenTelemetry settings in the admin console of an organization.
- Tool input (`OTEL_LOG_TOOL_DETAILS`), tool output (`OTEL_LOG_TOOL_CONTENT`) and raw API bodies (`OTEL_LOG_RAW_API_BODIES`). They are off.

## No data is deleted

- Prometheus: `--storage.tsdb.retention.time=100y`, no size limit.
- Loki: `retention_enabled: false` and no `retention_period`.
- Tempo: `block_retention: 876000h` (100 years) in the backend scheduler, the backend worker and the overrides. Tempo has no value for "keep for good".

The nightly backup (`deploy/backup.sh`) has the data of the three, in `/var/lib/lifeos-monitoring/`.

A delete in LifeOS does not reach this data. The text of a prompt stays in Loki and in Tempo, also after `Purge` removes the journal entry that the prompt held.

## Secrets

The collector replaces secrets in the attributes of each event and each span with `[removed: <kind>]`. The patterns are a copy of `internal/service/secrets.go` (rule 7 of `AGENTS.md`). A change there needs the same change in `monitoring/otel-collector.yaml`. The copy has no Luhn check, no Verhoeff check and no search for the word Aadhaar, so it removes some other long numbers too: a run of 13 to 19 digits, and a 12-digit number that starts with 2 to 9.

## The settings of a machine

Merge this into the `env` block of `~/.claude/settings.json`. Keep the other keys of the file. Then run `chmod 600 ~/.claude/settings.json`, because the file holds the token.

```json
{
  "env": {
    "CLAUDE_CODE_ENABLE_TELEMETRY": "1",
    "CLAUDE_CODE_ENHANCED_TELEMETRY_BETA": "1",
    "OTEL_METRICS_EXPORTER": "otlp",
    "OTEL_LOGS_EXPORTER": "otlp",
    "OTEL_TRACES_EXPORTER": "otlp",
    "OTEL_EXPORTER_OTLP_PROTOCOL": "http/protobuf",
    "OTEL_EXPORTER_OTLP_ENDPOINT": "<endpoint>",
    "OTEL_EXPORTER_OTLP_HEADERS": "Authorization=Bearer <token>",
    "OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE": "cumulative",
    "OTEL_LOG_USER_PROMPTS": "1",
    "OTEL_LOG_ASSISTANT_RESPONSES": "1",
    "OTEL_METRICS_INCLUDE_ENTRYPOINT": "true",
    "OTEL_METRICS_INCLUDE_VERSION": "true",
    "OTEL_METRICS_INCLUDE_REPOSITORY": "true",
    "OTEL_RESOURCE_ATTRIBUTES": "host.name=<machine>"
  }
}
```

- `<endpoint>`: `http://127.0.0.1:4318` on the Pi, `https://otel.<domain>` on each other machine.
- `<token>`: `OTEL_BEARER_TOKEN` of `deploy/.env`.
- `<machine>`: one fixed word for each machine, such as `pi` or `macbook`. Claude Code does not expand `$(hostname)`.

Claude Code ignores the `OTEL_*` exporter keys in the `.claude/settings.json` of a repository. Only the user file works.

A session reads the settings at its start. A session that ran before the change sends nothing: start a new one.

### Agent SDK

An Agent SDK program reads `~/.claude/settings.json` only with `settingSources: ['user']` (TypeScript) or `setting_sources=["user"]` (Python). Without it, give the program the variables in its environment.

## Add a machine

1. Put the block above into its `~/.claude/settings.json`, with its own `<machine>`.
2. Run `claude -p "Say ok."` on it.
3. In Grafana, Explore, Loki, run `{service_name="claude-code", host_name="<machine>"}`. The `user_prompt` line must show in about a minute.

If nothing arrives, run `claude -p "Say ok." --debug-file /tmp/otel.log` and read the lines with `[3P telemetry]`. A 401 means a wrong token.

## Change the token

1. Put a new value into `deploy/.env`: `openssl rand -hex 32`. Keep a copy in the password manager.
2. Run `sudo docker compose -f deploy/docker-compose.yml up -d otel-collector`.
3. Put the new token into `~/.claude/settings.json` of each machine.
4. Run `deploy/check-access.sh`. The `otel.` lines must show 401, 401, 200 and 404.

## Find a conversation

1. Open the dashboard "Claude usage". The panel Prompts shows each prompt with its repository.
2. Open a line. Its fields show `session_id`, `prompt_id` and `trace_id`.
3. In Explore, Loki, run `{service_name="claude-code"} | session_id="<id>"` for each event of that session in order.
4. In Explore, Tempo, open the `trace_id`. The tree shows each model request of the prompt, with its time and its tokens.

## Notes on the metrics

- Each session is its own counter series (the label `session_id`), and a short session sends one sample only. Prometheus writes a 0 at the start time of each counter (`--enable-feature=created-timestamp-zero-ingestion`), and the dashboard uses `increase(x[...] anchored)` (`--enable-feature=promql-extended-range-selectors`), which gives the exact change with no extrapolation.
- The cost is the price of the tokens at API prices. With a Pro or Max plan, it is not what you pay.
- A session outside a git repository with a remote has no `vcs_repository_name`. The dashboard shows it as `(no repository)`.
