# Security Policy

## Supported Versions

bbscope follows a rolling release model. Security fixes are applied to the
latest released version and to `main`. Please make sure you are running the most
recent release before reporting an issue.

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Instead, report privately through GitHub's built-in vulnerability reporting:

1. Go to the repository's **Security** tab.
2. Click **Report a vulnerability** (GitHub Private Vulnerability Reporting).
3. Provide a clear description, reproduction steps, and impact assessment.

If private reporting is unavailable, contact the maintainers privately before
disclosing details publicly.

### What to include
- Affected version / commit.
- A description of the vulnerability and its impact.
- Reproduction steps or a proof of concept.
- Any suggested remediation, if known.

### Our commitment
- We will acknowledge your report as soon as reasonably possible.
- We will keep you informed about remediation progress.
- We will credit reporters in the release notes unless you prefer to remain
  anonymous.

## Scope notes

bbscope is a security research tool that talks to third-party bug bounty
platforms. A few behaviors are intentional and are **not** vulnerabilities:

- TLS certificate verification is skipped **only** when the user explicitly
  passes `--proxy` (to support intercepting proxies such as Burp/ZAP).
- `db shell` launches a local `psql` process; the database password is passed
  via the `PGPASSWORD` environment variable rather than the command line.
- SHA-1 is used solely for TOTP (RFC 6238) two-factor authentication.
