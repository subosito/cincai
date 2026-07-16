# OAuth login — local and remote servers

Cincai vendor OAuth (`cincai credential login`) uses a **loopback HTTP callback** on the machine where the CLI runs. The provider redirects the browser to `http://127.0.0.1:<port>/…` after sign-in; `cincai` must receive that request locally.

Works out of the box on your laptop. On a **remote server** (SSH, VPS, headless host), the browser runs elsewhere — you need **SSH port forwarding** or **manual flow**.

---

## Vendor callback ports

| Profile | Callback URL |
|---------|----------------|
| `xai` | `http://127.0.0.1:56121/callback` |

`cincai credential login` prints these hints automatically (stronger message when `SSH_CONNECTION` is set).

---

## Remote server: SSH port forwarding (recommended)

1. On your **laptop**, open a tunnel **before** login (xAI port `56121`):

```bash
ssh -L 56121:127.0.0.1:56121 user@your-server
```

2. On the **server** (in that SSH session):

```bash
set -a && source config/cincai.dev.env && set +a
cincai credential login xai --config config/cincai.yaml
```

3. Open the printed **auth URL in your laptop browser** (not on the server). After sign-in, the redirect hits `127.0.0.1:56121` on your laptop; SSH forwards it to the server where `cincai` is listening.

Keep the `ssh -L` session open until you see `logged in id=…`.

---

## Alternative: manual flow (no port-forward)

Paste the redirect URL on the machine running `cincai`:

```bash
cincai credential login xai --flow manual --config config/cincai.yaml
```

1. Open the printed URL in any browser (phone, laptop, etc.).
2. After sign-in, copy the **full redirect URL** from the browser address bar (or the page if it errors — the URL still contains `code=`).
3. Paste into the terminal when prompted.

Works over slow links and when port-forward is blocked; slightly more steps.

---

## Prerequisites

```bash
cincai init
set -a && source config/cincai.dev.env && set +a
```

`CINCAI_BROKER_KEY` must be set — see [configuration.md](configuration.md).

---

## Device flow

Some vendor profiles support `--flow device`. Prefer that on truly headless hosts when the provider offers it:

```bash
cincai credential login PROFILE --flow device
```

---

## Token refresh

`cincai serve` refreshes OAuth access tokens automatically on read (proactive near expiry + reactive on upstream 401), so profiles stay live while the gateway runs. To refresh manually:

```bash
cincai credential refresh xai --config config/cincai.yaml
```

**One refresher per broker:** don't run two processes that refresh grants against the same `broker.db` at the same time — they can invalidate each other.

---

## Remote servers today

Use **port-forward** (`ssh -L …`) or **manual flow** (`--flow manual`) when the
gateway runs without a browser on the same machine. A future `cincai credential
login --remote-url` flag may accept an operator-run callback relay; not implemented yet.

---

## See also

- [credential.md](credential.md) — broker, profiles, API keys vs OAuth
- [cli.md](cli.md) — `credential login` flags
- [configuration.md](configuration.md) — `CINCAI_BROKER_KEY` and broker path