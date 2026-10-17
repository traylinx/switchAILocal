# Managed switchAILocal servers

One enrollment procedure for Linux servers running a **single Compose router
service per profile**. Requires Python 3.10+, Docker Engine, current Compose v2+
and systemd for optional scheduling. No Git checkout or new daemon required.

**Availability:** introduced in v0.6.2. No server is automatically
enrolled by installing or publishing this release. Commands below are operator procedures, not a record
that they have run. There are no placeholder channel manifests: a feed returns
404 until its first reviewed promotion is actually published.

## How releases reach servers

```text
source + tests → versioned GHCR image/digest → canary manifest PR
→ approved canary apply + real client tests → stable manifest PR
→ enrolled stable servers reconcile that exact digest
```

A release builds artifacts. It does **not** deploy or patch a running container.
The old tag-triggered infrastructure pin writer is removed. Neither `latest`
nor `stable` moving image tags are published by the new build workflow. Existing
unenrolled hosts stay on their current pin until an operator changes it.
Existing mutable tags are not deleted by this change.

Promotion changes `channels/canary.json` or `channels/stable.json` on protected
`main`. The canonical HTTPS feeds are:

- `https://raw.githubusercontent.com/traylinx/switchAILocal/main/channels/canary.json`
- `https://raw.githubusercontent.com/traylinx/switchAILocal/main/channels/stable.json`

Before first publication, verify repository visibility, direct raw URL access,
branch protection, required independent channel checks and review ownership.
These are real administrative gates, not protections created merely by naming
an Actions environment. Remove the obsolete cross-repository `INFRA_REPO_PAT`
secret with separate credential/admin approval. Do not publish a tag before the
old auto-pin workflow has been removed on the remote default branch.

### Prepare promotions on the release owner's workstation

Resolve the published **multi-platform index** digest, not a guessed tag or a
platform-specific child manifest. Inspect the image source label on the target
architecture and match the reviewed commit. Example read-only registry command:

```bash
docker buildx imagetools inspect ghcr.io/traylinx/switchailocal:VERSION --format '{{json .Manifest}}'
```

Use its `digest` value and the exact 40-character source SHA. Replace uppercase
placeholders before executing. The tool writes a local JSON file only:

```bash
python3 deploy/managed-server/promote.py --channel canary --generation 1 \
  --version VERSION --image ghcr.io/traylinx/switchailocal@sha256:DIGEST \
  --source SOURCE_SHA --out channels/canary.json
```

Commit that change through review, publish it, then separately authorize and
verify the canary. Stable promotion must use the **already published** canary
image, source and version plus an HTTPS evidence reference:

```bash
python3 deploy/managed-server/promote.py --channel stable --generation 1 \
  --version VERSION --image ghcr.io/traylinx/switchailocal@sha256:DIGEST \
  --source SOURCE_SHA --canary channels/canary.json \
  --evidence https://github.com/traylinx/switchAILocal/issues/EVIDENCE \
  --out channels/stable.json
```

CI compares to the base branch: generations must increase, deletion is refused,
and stable must match a previously published canary, not a new canary in the
same PR. Require up-to-date base checks before merging. An intentional rollback
uses a **new generation** with an older approved image. A quarantined target
still requires an explicit per-host `retry`.

## New server: repeatable enrollment

### 1. Provision the router and preserve the application's contract

Use the application's reviewed Compose/config provisioning first. A router must
already be running and pass its normal client checks before updater enrollment.
Inventory the exact Compose project name, service, ordered files, **all explicit
env files**, port, image ID, mounts, aliases and competing deployment jobs.
Do not create a second project accidentally by choosing a different project name.

The updater disables implicit `.env` loading and ambient Compose variables. Add
that file explicitly to `env_files` if your application uses it. Compose-relative
service `env_file` and bind mount paths retain their existing project-directory
semantics. Root must own trusted directories/files and no group/other may write
these inputs. Secrets-bearing env files and profiles must be mode `0600`.

Keep model/provider configuration in the application's normal source of truth.
The updater never changes it. Keep local embeddings/search/transcription as
configured. For a new provider or model, test chat, streaming, tools/replay,
vision and embedding dimensions before changing the live route. Pinning a router
image does not pin or update upstream model availability.

For Cortex, preserve local embeddings and search; verify actual deployed Pi
client chat/tool replay/vision, not just the generic probe. Provider changes must pass that SDK gate; generic HTTP success is insufficient. For HoCa, preserve its template
renderer and ordered `.env.production`; local faster-whisper is separate from
AIL. Do not edit generated HoCa configs or copy credentials between applications.

### 2. Obtain and inspect the package

Use `managed-server/` from an exact reviewed source commit or the platform
release archive. Verify the archive against release checksums over trusted
transport. This does not substitute for publisher authentication. Review the
installer and configuration before running as root; Docker access is root-equivalent.

```bash
sudo bash managed-server/install.sh
```

This installs versioned tools under `/opt/switchailocal-updater/v1/` and systemd
units. It does **not** enable/start/reload units, enroll, pull or restart anything.
Reinstall is idempotent only for identical files; differing v1 files are refused.
Updater code does not self-update. Future tool upgrades require a reviewed,
versioned package, stopped timer, compatible schema and no in-flight transaction.
Do not erase state to upgrade.

### 3. Configure the profile and verification

Copy `examples/profile.json` to
`/etc/switchailocal-updater/profiles/router-01.json`. Set exact project/files/env
files/service and channel. Keep `allow_recreate: false` until downtime and
rollback have been approved. Use one profile per instance, with different names.
Do not bind two profiles to the same project/service.

Copy `examples/probe.json` to
`/etc/switchailocal-updater/router-01-probe.json`. Store the **router caller key**
in the separately referenced `key_file`, not the provider key, profile, argv,
evidence or shell history. Use a private editor or approved secret tooling.
Root ownership and `0600` are mandatory for all three files.

The probe uses operator-selected chat/vision aliases, token budget and optional
local embedding model/dimension. It sends only synthetic text and an embedded
red PNG. It requires text marker, forced tool arguments, raw and normalized tool
replay, vision color, complete SSE stream and optional embedding dimensionality.
It uses six generation calls, plus one optional embedding call, per verification.
Updates verify before and after, rollback verifies again: budget accordingly.
An unchanged healthy timer tick sends **no paid inference**.

HTTP is allowed only on loopback; elsewhere verified HTTPS is mandatory.
No redirects/proxy environment; responses and keys are never printed. Individual
requests have bounded socket timeout; the updater's `verify_timeout` bounds the
whole process group. Set it large enough for your configured request budgets.
Use a root-owned wrapper as `verify_command` to run this probe **and** application
SDK acceptance tests. The generic probe cannot prove every SDK's behavior.
Optional pre/post argv hooks can quiesce/resume the client. They must be
idempotent and bounded: post may run during recovery. A single direct-port router
can briefly interrupt requests. This is **not zero-downtime failover**.

Validate and inspect without changing the runtime:

```bash
sudo /opt/switchailocal-updater/v1/probe.py --config /etc/switchailocal-updater/router-01-probe.json --validate-only
sudo /opt/switchailocal-updater/v1/managed_update.py --profile /etc/switchailocal-updater/profiles/router-01.json validate-profile
sudo /opt/switchailocal-updater/v1/managed_update.py --profile /etc/switchailocal-updater/profiles/router-01.json check
```

`check` reads runtime identity and fetches the manifest in memory. No file
writes, lock creation, pull, paid probe or restart. HTTPS feed retrieval permits
zero redirects, no auth headers or proxy environment, 64 KiB and 20-second socket
timeout. The operator trusts that host and DNS resolution, including private IPs;
this is not an SSRF boundary. TLS plus a digest proves transport/integrity, not
independent publisher identity.

### 4. Disable competing writers, enroll, then apply explicitly

All operations targeting the router must use the same exclusive `flock(2)`:
`/run/lock/wannolot-gitops.lock`. On Ubuntu `/var/lock` resolves to `/run/lock`;
confirm inode identity before enabling timers. Root owns the regular lock file,
not group/other-writable; existing `0644` is allowed. Kernel lock contention
refuses the update. No PID metadata can bypass it.

For wannolot infrastructure, separately set `AIL_UPDATE_MODE=managed` in the
private `.env.droplet` **after deploying the reviewed fence changes**. This holds
its entire legacy runtime GitOps apply and refuses its old AIL update script;
verify-only still works. The broad hold is intentional because the old renderers,
key reconciliation and rollback couple AIL/app/embedding mutations. Approve a
separate manual path for unrelated infra operations; do not assume they continue
automatically. HoCa needs its own deployment-lock/quiesce adapter before enrollment.
Do not enable both deployers and hope image overrides will win.

After the operator approves recreation/downtime, set `allow_recreate: true`.
Then run, as distinct actions:

```bash
sudo /opt/switchailocal-updater/v1/managed_update.py --profile /etc/switchailocal-updater/profiles/router-01.json enroll
sudo /opt/switchailocal-updater/v1/managed_update.py --profile /etc/switchailocal-updater/profiles/router-01.json apply
sudo /opt/switchailocal-updater/v1/managed_update.py --profile /etc/switchailocal-updater/profiles/router-01.json status
```

Enrollment performs baseline application verification and writes state plus an
independent marker under `/etc/switchailocal-updater/enrolled/`. Deleting runtime
state after enrollment fails closed. Do not delete the marker to reset generations.
Changing channel/project/service/files/feed/lock binding also requires explicit
operator reconciliation, not a silent profile edit or automatic migration.

Apply pulls only the immutable approved digest, checks its OCI source revision,
journals the old image ID, and recreates **only** the named service using
`--no-deps --no-build --pull never --force-recreate`. Config, env, models, keys,
volumes, engine, embeddings and other routers are not modified. The image-only
JSON override is in `/var/lib/switchailocal-updater/PROFILE/image.override.json`.
Any future manual Compose command for this service must include that override
and acquire the same lock. Do not run a bare application-wide `compose up/down`.

### 5. Enable periodic reconciliation only after successful manual verification

This is a **separate live scheduling approval**, not part of installation:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now switchailocal-update@router-01.timer
systemctl list-timers 'switchailocal-update@*'
```

Ticks run about every five minutes plus up to 90 seconds jitter. Missing feed,
invalid state, lock contention, hold, quarantine or incompatible config fail
closed. Use `systemctl status`, `journalctl -u switchailocal-update@router-01.service`
and the updater `status` command; there is no central telemetry/dashboard.
Fleet inventory and staged enrollment remain operator responsibilities. Servers
do not become enrolled merely by installing a new application release.

## Failure, rollback and recovery

- Pull/baseline failure: running container unchanged; failure code in job output.
- New-image verification failure: restore previous **local image ID** with pull
  disabled, verify application, quarantine failed digest. Persist pending journal
  if rollback/resume fails. A generation bump cannot evade quarantine.
- Crash: next apply acquires lock and restores the journal before any new update.
  Recovery returns nonzero so an operator sees it; a later tick can proceed only
  if target is not quarantined. No remote manifest is trusted for rollback.
- Config/env hash drift: held, not silently re-rendered. Resolve against the
  recorded transaction under the shared lock; preserve restricted evidence.
- State corruption/loss/old schema: held. Restore a trusted matching state and
  marker, reconcile actual image/config, and review before resuming. No automatic
  reset, generation downgrade or in-flight schema migration command exists.

Commands with the same `--profile` prefix:

| Command | Effect |
| --- | --- |
| `status` | Saved result and current identity, no paid probe |
| `check` | Read-only current desired/running/hold/quarantine report |
| `retry` | Explicitly retry CURRENT feed target despite its quarantine/hold; full verification; only removes that digest from quarantine after success |
| `rollback` | Restore last successful update's retained previous image; profile-wide operator hold prevents all timer reapply, even a different newly promoted digest |

A manual rollback is not a fleet-channel rollback. Use a reviewed **new generation**
for fleet rollback. To release a local manual hold, inspect `check`, approve the
current target, then `retry`. It does not silently clear other quarantined digests.

### Retention and disk space

A never-started, network-none container labelled
`io.switchailocal.rollback=PROFILE` anchors the previous local image; it has no
Compose ownership labels, customer mounts or credentials. A tag alone would not
protect against `docker image prune -a`. Keep the anchor: exclude this label
from any container prune policy. Removing it may invalidate rollback retention.

The rollback window lasts until the next completed update replaces its recorded
previous image. At most two guards are needed during a transition; completed
updates remove only exact owned non-current guards, never images. A crash during
cleanup can leave an extra guard until the next completed update. Missing cached
image refuses rollback; never substitute a moved tag or pull an unknown image.
Provision headroom for old and new images. This tool does not run global prune.

## Local verification and held live gates

```bash
python3 -m unittest discover -s deploy/managed-server/tests -v
bash -n deploy/managed-server/install.sh
```

Tests use fake Docker, temporary installation roots, synthetic probe responses
and no production keys. Real Compose config rendering is a separate local gate;
it does not prove actual recreation from a local `sha256:` ID. Before first
canary, prove real pull-disabled apply/rollback, guard retention, systemd/lock
interoperation and application-client behavior on a disposable target. No live
server commands, timer activation or fleet deployment are implied by these tests.
