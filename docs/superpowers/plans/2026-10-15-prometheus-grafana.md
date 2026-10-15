# Prometheus and Grafana Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prometheus and Grafana run on the Pi. Grafana opens at `dash.<domain>` with one login: the login of Cloudflare Access. Grafana has no login of its own. Grafana reads Prometheus. Prometheus deletes no data. The telemetry plan (`2026-10-15-claude-code-telemetry.md`) needs this plan first.

**Scope limit:** This plan adds no feature to the `lifeos` binary. It adds two Docker services, their config files, one public hostname and documentation. The rest of the section "Monitoring" of the design doc (node_exporter, postgres_exporter, blackbox_exporter, the alerts, Healthchecks.io, `/metrics` on `lifeos`, the Postgres data source) is not in this plan. Each of those gets its own issue.

**Architecture:** Prometheus and Grafana run in the Compose project of `deploy/docker-compose.yml`, as `postgres` and `ntfy` do. Prometheus listens on `127.0.0.1:9090`, Grafana on `127.0.0.1:3000`. Grafana reaches Prometheus on the Compose network (`http://prometheus:9090`). The data of each is in a folder below `/var/lib/lifeos-monitoring`, owned by `gautam`. The config files are in `monitoring/`, as the design doc says. Only Grafana has a public hostname.

**Tech Stack:** Docker Compose, `prom/prometheus` 3.x, `grafana/grafana` (pinned releases, linux/arm64), cloudflared, Cloudflare Access, bash.

**Spec:** `docs/design.md` (sections "Dashboard", "Deployment and security" and "Monitoring") and `AGENTS.md` (section "Deploy"). The plan after this one is `docs/superpowers/plans/2026-10-15-claude-code-telemetry.md`.

## Decisions in this plan

| # | Decision | Reason |
| --- | --- | --- |
| 1 | Prometheus deletes no data. The retention time is `100y`, and there is no size limit. The design doc says 30 days. | Gautam's choice: no old data is deleted. Prometheus has no "keep for good" value: with no retention flag it keeps 15 days, so the flag gets a time that does not end. A size limit deletes the oldest data when the limit is reached, so there is none. The disk can fill, so Task 10 of the telemetry plan measures the growth. |
| 2 | The containers run as `gautam` (`user: "1000:1000"`), with bind mounts below `/var/lib/lifeos-monitoring`. | Each image has its own default user (Prometheus `nobody`, Grafana 472). One owner makes the folders simple to back up and to read. |
| 3 | `dash.<domain>` has one lock: the Access application (email code). Grafana has no login: anonymous access with the role `Admin`, no login form, no basic auth. | Gautam's choice. The design doc has two locks: Access, then the Grafana login. Grafana is then like the admin pages of `lifeos`: Access guards the hostname, and a program on the Pi itself reaches `127.0.0.1:3000` with no login. No other device of the home network reaches it, because it listens on `127.0.0.1` only. |
| 4 | Grafana sends no usage report and does not look for updates. | A service on the Pi calls no other site that it does not need. |
| 5 | The config files are in `monitoring/`. The dashboards are in `dashboards/`, from the first dashboard on. | The design doc names `monitoring/` for the provisioning files. `AGENTS.md` names `dashboards/` for the dashboards. This plan has no dashboard, so it makes no `dashboards/` folder. |

## Prerequisites

Step 1 of Task 1 checks these.

- Docker runs, and the `lifeos` Compose project runs (`postgres`, `postgres-test`, `ntfy`).
- Nothing listens on `127.0.0.1:9090` and `127.0.0.1:3000`. On 2026-10-15, both were free.
- The Pi has at least 1 GB of memory available. On 2026-10-15, it had 4.6 GB. Prometheus and Grafana use about 450 MB together.
- `deploy/.env` exists. Never print a value of it.

## Global Constraints

- Each service listens on `127.0.0.1` only.
- No hostname goes into the tunnel before its Access application exists. Run `deploy/check-access.sh` after each change of the tunnel or of an Access application.
- No domain, team name, audience tag or password goes into git.
- Each new key of `deploy/.env` goes into `deploy/.env.example` and into `UnsetEnvironment` of `deploy/systemd/lifeos.service`. `deploy/test.sh` checks the list.
- The commits are `chore` and `docs`, so they make no release.

---

## Task 1: Run Prometheus

About 30 minutes.

**Files:**
- Create: `monitoring/prometheus.yml`
- Modify: `deploy/docker-compose.yml`

- [ ] **Step 1: Check the prerequisites**

```sh
sudo docker compose -f deploy/docker-compose.yml ps
ss -ltn | grep -E ':(9090|3000) ' || echo "9090 and 3000 are free"
free -h
```

- [ ] **Step 2: Pick the releases**

For `prom/prometheus` and `grafana/grafana`, pick the newest release that is at least two weeks old and has a `linux/arm64` image. Write the two tags into Task 1 Step 4 and Task 2 Step 3. Read the release notes of each for changes of the flags and the settings that this plan uses.

- [ ] **Step 3: Make the data folders**

```sh
sudo install -d -o gautam -g gautam -m 700 /var/lib/lifeos-monitoring
install -d -m 700 /var/lib/lifeos-monitoring/prometheus /var/lib/lifeos-monitoring/grafana
```

- [ ] **Step 4: Write `monitoring/prometheus.yml`**

```yaml
# Prometheus of LifeOS. The design doc, section "Monitoring", lists the
# targets that come later; each gets a scrape job here.
global:
  scrape_interval: 60s
  evaluation_interval: 60s

scrape_configs:
  - job_name: prometheus
    static_configs:
      - targets: ["localhost:9090"]
```

- [ ] **Step 5: Add Prometheus to `deploy/docker-compose.yml`**

Add this below `ntfy`:

```yaml
  # Metrics of the Pi and of LifeOS. Grafana reads it on the Compose network.
  prometheus:
    image: prom/prometheus:<tag of Step 2>
    restart: unless-stopped
    user: "1000:1000"
    command:
      - --config.file=/etc/prometheus/prometheus.yml
      - --storage.tsdb.path=/prometheus
      # Delete no data (decision 1). Without this flag Prometheus keeps 15
      # days. No retention.size: a size limit deletes the oldest data.
      - --storage.tsdb.retention.time=100y
    ports:
      - "127.0.0.1:9090:9090"
    volumes:
      - ../monitoring/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - /var/lib/lifeos-monitoring/prometheus:/prometheus
    mem_limit: 512m
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:9090/-/ready"]
      interval: 30s
      timeout: 5s
      retries: 3
```

Check that the image has `wget`. If it has none, remove the healthcheck.

- [ ] **Step 6: Start and check**

```sh
sudo docker compose -f deploy/docker-compose.yml up -d prometheus
curl -sS http://127.0.0.1:9090/-/ready
curl -sS 'http://127.0.0.1:9090/api/v1/query?query=up' | jq '.data.result[] | {job: .metric.job, up: .value[1]}'
ss -ltn | grep 9090
```

Expected: `Prometheus Server is Ready.`, the job `prometheus` with `up` 1, and `127.0.0.1:9090` only. Then check the retention:

```sh
curl -sS http://127.0.0.1:9090/api/v1/status/runtimeinfo | jq '.data.storageRetention'
```

Expected: `100y`, with no size. If Prometheus refuses `100y`, use the largest value that it takes, and write it into decision 1.

- [ ] **Step 7: Commit**

```sh
git add monitoring/prometheus.yml deploy/docker-compose.yml
git commit -m "chore(deploy): run Prometheus"
```

## Task 2: Run Grafana

About 30 minutes.

**Files:**
- Create: `monitoring/grafana/provisioning/datasources/datasources.yaml`
- Modify: `deploy/docker-compose.yml`, `deploy/.env.example`, `deploy/systemd/lifeos.service`

- [ ] **Step 1: Add the keys to the env files**

Add to `deploy/.env.example`:

```sh
# Grafana (docker-compose.yml): password of the built-in user admin. Grafana
# has no login form (Access is the one login), so no person types it. It
# replaces the default password "admin". Grafana reads it only at the first
# start.
GRAFANA_ADMIN_PASSWORD=
```

Add `GRAFANA_ADMIN_PASSWORD` to `UnsetEnvironment` in `deploy/systemd/lifeos.service`. Run `deploy/test.sh`: it passes only with the two changes. Put a value into `deploy/.env` (`openssl rand -hex 24`), and keep a copy in the password manager.

- [ ] **Step 2: Write the data source file**

```yaml
# The data sources of Grafana. The telemetry plan adds Loki and Tempo.
apiVersion: 1
datasources:
  - name: Prometheus
    uid: prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
```

- [ ] **Step 3: Add Grafana to `deploy/docker-compose.yml`**

```yaml
  # The dashboards. The public hostname dash.<domain> has an Access login.
  # Task 3 turns off the login of Grafana, so Access is the one login.
  grafana:
    image: grafana/grafana:<tag of Task 1 Step 2>
    restart: unless-stopped
    user: "1000:1000"
    environment:
      GF_SECURITY_ADMIN_PASSWORD: ${GRAFANA_ADMIN_PASSWORD:?set GRAFANA_ADMIN_PASSWORD in deploy/.env}
      GF_USERS_ALLOW_SIGN_UP: "false"
      GF_ANALYTICS_REPORTING_ENABLED: "false"
      GF_ANALYTICS_CHECK_FOR_UPDATES: "false"
      GF_ANALYTICS_CHECK_FOR_PLUGIN_UPDATES: "false"
      GF_NEWS_NEWS_FEED_ENABLED: "false"
    ports:
      - "127.0.0.1:3000:3000"
    volumes:
      - ../monitoring/grafana/provisioning:/etc/grafana/provisioning:ro
      - /var/lib/lifeos-monitoring/grafana:/var/lib/grafana
    depends_on:
      - prometheus
    mem_limit: 384m
```

- [ ] **Step 4: Start and check**

```sh
sudo docker compose -f deploy/docker-compose.yml up -d grafana
curl -sS http://127.0.0.1:3000/api/health | jq .
```

Expected: `"database": "ok"`. Open an SSH tunnel from the MacBook (`ssh -L 3000:127.0.0.1:3000 <pi>`) and log in at `http://localhost:3000` as `admin`. In Connections, the data source Prometheus shows a successful test. In Explore, the query `up` shows 1. This is the last use of the login form: Task 3 turns it off.

- [ ] **Step 5: Commit**

```sh
git add monitoring/grafana deploy/docker-compose.yml deploy/.env.example deploy/systemd/lifeos.service
git commit -m "chore(deploy): run Grafana with Prometheus as its data source"
```

## Task 3: Open `dash.<domain>` with Access as the one login

About 40 minutes. The order of the steps is the rule of `AGENTS.md`: the Access application exists before the tunnel rule.

**Files:**
- Modify: `/etc/cloudflared/config.yml` (not in git), `deploy/check-access.sh`, `deploy/test.sh`, `deploy/.env.example`, `deploy/systemd/lifeos.service`, `deploy/docker-compose.yml`

- [ ] **Step 1: Make the Access application**

In the Cloudflare dashboard, make the Access application `LifeOS Dash` for `dash.<domain>`. Give it the same login policy as the application of the admin pages (email code). Write down its audience tag.

- [ ] **Step 2: Add the check first**

1. Add `LIFEOS_DASH_HOST=` to `deploy/.env.example`, next to the other hostnames. Add it to `UnsetEnvironment` in `deploy/systemd/lifeos.service`.
2. In `deploy/check-access.sh`, add a block: `guarded login "https://$LIFEOS_DASH_HOST/"`. Add the hostname to the check at the top that stops when no hostname is set, and to the comment that lists the keys.
3. Add the case to `deploy/test.sh`, as the cases of the admin hostname are there. Run `deploy/test.sh` and see the new case fail before the change of `check-access.sh`, then pass after it.
4. Put the hostname into `deploy/.env`. Run `deploy/check-access.sh`. Expected now: the new line fails, because the hostname has no tunnel rule.

- [ ] **Step 3: Add the tunnel rule**

Back up the file first: `sudo cp /etc/cloudflared/config.yml /etc/cloudflared/config.yml.bak-$(date +%Y%m%d-%H%M%S)`. Add this rule before the last `http_status:404` rule:

```yaml
  - hostname: dash.<domain>
    service: http://127.0.0.1:3000
    originRequest:
      access:
        required: true
        teamName: <team>
        audTag:
          - <audience tag of LifeOS Dash>
```

Add the DNS route with `cloudflared tunnel route dns <tunnel> dash.<domain>`. Restart `cloudflared`.

- [ ] **Step 4: Write the failing check**

```sh
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/api/user
curl -sS -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3000/login
```

Expected after Step 5: `200` for `/api/user` (the anonymous user), and no login form. Before Step 5, the first request gets `401`. That is the failure that Step 5 corrects.

- [ ] **Step 5: Turn off the login of Grafana**

Add this to the environment of `grafana` in `deploy/docker-compose.yml`:

```yaml
      GF_SERVER_ROOT_URL: https://${LIFEOS_DASH_HOST:?set LIFEOS_DASH_HOST in deploy/.env}/
      # One login: Cloudflare Access (decision 3). Grafana has no login of
      # its own: each request is the anonymous user, with the role Admin.
      # Grafana listens on 127.0.0.1 only, as the admin pages of lifeos do.
      GF_AUTH_ANONYMOUS_ENABLED: "true"
      GF_AUTH_ANONYMOUS_ORG_ROLE: Admin
      GF_AUTH_DISABLE_LOGIN_FORM: "true"
      GF_AUTH_BASIC_ENABLED: "false"
```

Run `sudo docker compose -f deploy/docker-compose.yml up -d grafana`. Read the release notes of the pinned Grafana for these keys. If the release does not give the role `Admin` to the anonymous user, use `Editor`, and tell Gautam: the data sources then change only through the files of `monitoring/grafana/provisioning`.

- [ ] **Step 6: Check**

```sh
deploy/test.sh
deploy/check-access.sh
```

Expected: both pass. `check-access.sh` prints `ok` for the dash hostname: Access asks for a login. Then:

1. Run the two requests of Step 4 again. Expected: `200`, and no login form.
2. In a browser, `https://dash.<domain>` asks for the email code, then shows Grafana with no second login. Gautam can edit a dashboard and open Connections.

- [ ] **Step 7: Commit**

```sh
git add deploy/check-access.sh deploy/test.sh deploy/.env.example deploy/systemd/lifeos.service deploy/docker-compose.yml
git commit -m "chore(deploy): open Grafana at the dash hostname with Access as its one login"
```

## Task 4: Back up Grafana and document

About 30 minutes.

**Files:**
- Modify: `deploy/backup.sh`, `docs/design.md`, `AGENTS.md`, `README.md` (if it lists the services), `assets/architecture.svg`

- [ ] **Step 1: Back up the Grafana and Prometheus data**

In `deploy/backup.sh`, add `/var/lib/lifeos-monitoring/grafana` and `/var/lib/lifeos-monitoring/prometheus` to the two `restic backup` commands, with a comment as `files_dir` has. Prometheus keeps all data (decision 1), so a lost SD card must not lose it. The blocks of Prometheus do not change after they are written, so the copy of a running Prometheus is good. Only the write-ahead log of the last two hours can be cut; Prometheus repairs it at the start. Run `deploy/test.sh`. Run `deploy/backup.sh` one time and check with `restic ls latest | grep -E 'lifeos-monitoring/(grafana|prometheus)'`.

Do the restore test one time: restore the Prometheus folder of the snapshot to a folder in the scratchpad. Start a second Prometheus container on it, with no published port. Query `up` in it.

- [ ] **Step 2: Update the documents**

- `docs/design.md`, "Deployment and security": add Prometheus (`127.0.0.1:9090`) to the component table. Put the real port of each service. Add Prometheus (about 250 MB) to the memory table.
- `docs/design.md`, "Deployment and security": in the hostname table, the app check of `dash.` is "None: Access is the one login", not "Grafana login".
- `docs/design.md`, "Monitoring": Prometheus deletes no data (decision 1), and the backup has it.
- `AGENTS.md`, "Layout": add `monitoring/` ("the config files of Prometheus and Grafana, and later of the other monitoring services"). "Deploy": the dash hostname has one login, Access; Grafana has no login of its own. Grafana must listen on `127.0.0.1` only, because each request that reaches it is an admin.
- `assets/architecture.svg`: add Prometheus, Grafana and the dash hostname.

- [ ] **Step 3: Commit**

```sh
git add deploy/backup.sh
git commit -m "chore(deploy): back up the Grafana and Prometheus data"
git add docs/design.md AGENTS.md README.md assets/architecture.svg
git commit -m "docs: describe Prometheus and Grafana on the Pi"
```

---

## Time

| Task | Time |
| --- | --- |
| 1. Prometheus | 30 min |
| 2. Grafana | 30 min |
| 3. `dash.<domain>` with one login | 40 min |
| 4. Backup, restore test and docs | 45 min |
| Total | about 2.5 hours |

After Task 3, Grafana opens at `dash.<domain>` from the phone and the MacBook, after the email code of Access only.
