# Security Policy

## Reporting a vulnerability

Please report security issues **privately** — do not open a public issue or pull
request for anything security-sensitive.

Use GitHub's private vulnerability reporting: open the repository's **Security** tab
and click **Report a vulnerability**. Include the affected version or commit, a
description of the impact, and steps to reproduce if you can.

We aim to acknowledge a report within a few days and will keep you updated as we work
on a fix. With your consent, we're glad to credit you in the advisory once it's
resolved.

## Supported versions

Cincai is under active development. Security fixes land on `main` and the latest
release; please reproduce against the latest version before reporting.

## Deployment notes

Cincai is a self-hosted gateway. A few defaults are worth knowing when assessing a
deployment:

- The **data plane binds loopback** (`127.0.0.1`) by default. Exposing it beyond the
  local host is an explicit operator choice — bind a specific interface and put TLS in
  front, since gateway API keys travel on this listener.
- **Gateway API keys** are high-entropy random tokens; only their hashes are stored.
- Upstream provider credentials are held in an **encrypted broker** (AES-256-GCM); the
  broker file is created `0600`. Manage credentials and keys with the `cincai` CLI.

Reports about the default, out-of-the-box configuration are especially welcome.
