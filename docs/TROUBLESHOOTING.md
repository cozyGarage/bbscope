# bbscope Troubleshooting Guide

This guide helps you diagnose and resolve common issues when using bbscope.

---

## Table of Contents

- [Installation Issues](#installation-issues)
- [Configuration Issues](#configuration-issues)
- [Keychain Issues](#keychain-issues)
- [Database Issues](#database-issues)
- [Platform-Specific Issues](#platform-specific-issues)
- [Authentication Issues](#authentication-issues)
- [AI Normalization Issues](#ai-normalization-issues)
- [Performance Issues](#performance-issues)
- [Docker Issues](#docker-issues)
- [Common Error Messages](#common-error-messages)

---

## Installation Issues

### `go install` fails

**Error:**
```
go: module github.com/sw33tLie/bbscope/v2: no matching versions
```

**Solutions:**
1. Update Go to version 1.24+:
   ```bash
   go version  # Check current version
   ```
2. Clear module cache:
   ```bash
   go clean -modcache
   ```
3. Try with explicit version:
   ```bash
   go install github.com/cozyGarage/bbscope/v2@latest
   ```

### Binary not found after install

**Problem:** `bbscope: command not found`

**Solutions:**
1. Add Go bin to PATH:
   ```bash
   # Add to ~/.bashrc or ~/.zshrc
   export PATH=$PATH:$(go env GOPATH)/bin
   source ~/.bashrc
   ```
2. Verify installation:
   ```bash
   ls $(go env GOPATH)/bin/bbscope
   ```

---

## Configuration Issues

### Config file not found

**Error:**
```
Error reading config file: Config File ".bbscope" Not Found
```

**Solution:**
Run any bbscope command once to auto-create the config:
```bash
bbscope --help
# Then edit ~/.bbscope.yaml
```

### Config file permissions

**Problem:** Config contains sensitive credentials

**Solution:**
Use the secure keychain storage instead:
```bash
# Store credentials in OS keychain
bbscope config set hackerone.token
bbscope config set bugcrowd.password

# Migrate existing config to keychain
bbscope config migrate
```

For legacy config file:
```bash
chmod 600 ~/.bbscope.yaml
```

### Environment variables not working

**Problem:** Credentials in environment not picked up

**Note:** Currently, only `OPENAI_API_KEY` is read from environment. Platform credentials should be stored in the OS keychain using `bbscope config set`.

**Workaround for scripts:**
```bash
bbscope poll h1 --user "$H1_USER" --token "$H1_TOKEN"
```

---

## Keychain Issues

### Keychain not available (Linux)

**Error:**
```
secret service not available
```

**Solutions:**
1. Install and start a secret service:
   ```bash
   # GNOME Keyring
   sudo apt install gnome-keyring
   # Or KDE Wallet
   sudo apt install kwalletmanager
   ```
2. Ensure D-Bus is running:
   ```bash
   eval $(dbus-launch --sh-syntax)
   ```
3. Fall back to config file if keychain unavailable

### Keychain access denied (macOS)

**Error:**
```
keychain access denied
```

**Solutions:**
1. Grant terminal access to Keychain in System Preferences > Security & Privacy
2. Unlock Keychain:
   ```bash
   security unlock-keychain ~/Library/Keychains/login.keychain-db
   ```

### Keychain credential not found

**Error:**
```
credential not found in keychain
```

**Solutions:**
1. Check if credential exists:
   ```bash
   bbscope config list
   bbscope config get <key>
   ```
2. Store the credential:
   ```bash
   bbscope config set hackerone.token
   ```
3. Migrate from config file:
   ```bash
   bbscope config migrate
   ```

### Windows Credential Manager issues

**Error:**
```
The specified item could not be found in the keychain
```

**Solutions:**
1. Open Credential Manager (Control Panel > User Accounts > Credential Manager)
2. Check "Generic Credentials" for "bbscope" entries
3. Re-add credential:
   ```powershell
   bbscope config set hackerone.token
   ```

---

## Database Issues

### Connection refused

**Error:**
```
dial tcp 127.0.0.1:5432: connect: connection refused
```

**Solutions:**
1. Check PostgreSQL is running:
   ```bash
   docker ps | grep postgres
   # Or
   systemctl status postgresql
   ```
2. Start the database:
   ```bash
   docker start bbscope-db
   ```
3. Verify connection string in config:
   ```yaml
   db_url: "postgres://user:password@localhost:5432/bbscope?sslmode=disable"
   ```

### Database does not exist

**Error:**
```
pq: database "bbscope" does not exist
```

**Solution:**
bbscope will attempt to create the database automatically. If this fails:
```bash
# Manual creation
psql -U postgres -c "CREATE DATABASE bbscope;"

# Or with Docker
docker exec -it bbscope-db psql -U bbscope -c "CREATE DATABASE bbscope;"
```

### Permission denied

**Error:**
```
pq: permission denied for database bbscope
```

**Solution:**
Grant permissions:
```sql
GRANT ALL PRIVILEGES ON DATABASE bbscope TO your_user;
```

### SSL certificate error

**Error:**
```
pq: SSL is not enabled on the server
```

**Solution:**
Add `sslmode=disable` to connection string:
```yaml
db_url: "postgres://user:pass@localhost:5432/bbscope?sslmode=disable"
```

### Database migration errors

**Error:**
```
migrating schema: <error details>
```

**Solution:**
If schema is corrupted, you may need to reset:
```bash
# WARNING: This deletes all data
psql $DB_URL -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
```

---

## Platform-Specific Issues

### HackerOne

#### Invalid credentials

**Error:**
```
fetching failed. Got status Code: 401
```

**Solutions:**
1. Verify token is valid: [HackerOne API Tokens](https://docs.hackerone.com/en/articles/8410331-api-token)
2. Check username matches the account that created the token
3. Ensure token has proper permissions

#### No programs returned

**Possible causes:**
- No programs accessible to your account
- Using `--private-only` but no private program invitations
- Using `--bbp-only` but no bounty programs

### Bugcrowd

#### WAF banned

**Error:**
```
you are temporarily WAF banned, change IP or wait a few hours
```

**Solutions:**
1. Wait several hours before retrying
2. Use a different IP (VPN, different network)
3. Use session token instead of email/password login

#### Login fails with OTP

**Error:**
```
Bugcrowd auth failed: <error>
```

**Solutions:**
1. Verify OTP secret is the raw base32 string, not a URL
2. Check system clock is synchronized:
   ```bash
   # Linux
   timedatectl status
   # macOS
   sntp time.apple.com
   ```
3. Use `--token` with session cookie instead

#### Getting session cookie

1. Log into Bugcrowd in browser
2. Open DevTools → Application → Cookies
3. Copy `_crowdcontrol_session_key` value
4. Use: `bbscope poll bc --token "VALUE"`

### Intigriti

#### Token expired

**Error:**
```
Intigriti auth failed: unauthorized
```

**Solution:**
Generate new token: [Intigriti Personal Access Tokens](https://app.intigriti.com/researcher/personal-access-tokens)

### YesWeHack

#### Authentication issues

**Solutions:**
1. Use bearer token directly if available:
   ```bash
   bbscope poll ywh --token "YOUR_JWT"
   ```
2. Verify email/password/OTP secret are correct
3. Check 2FA is properly configured

### Immunefi

**Note:** Immunefi doesn't require authentication as it scrapes public data.

#### Slow or failing

Immunefi polling may be slower due to web scraping nature. If failing:
1. Check if Immunefi website is accessible
2. Website structure may have changed (requires code update)

---

## Authentication Issues

### 2FA/OTP not working

**Common causes:**
1. **Clock drift:** Ensure system time is accurate
2. **Wrong secret format:** Use raw base32, not otpauth:// URL

**Verify OTP secret:**
```bash
# Test OTP generation
oathtool --totp -b YOUR_SECRET
# Should match authenticator app
```

**Supported OTP formats:**
- Raw base32: `JBSWY3DPEHPK3PXP`
- With digits: `6 JBSWY3DPEHPK3PXP`
- otpauth URL: `otpauth://totp/Label?secret=JBSWY3DPEHPK3PXP`

### Session expired during polling

**Symptom:** Polling starts but fails midway

**Solutions:**
1. Get fresh session token
2. Reduce concurrency to avoid rate limits:
   ```bash
   bbscope poll bc --concurrency 1
   ```

---

## AI Normalization Issues

### API key not found

**Error:**
```
ai normalization requires an API key
```

**Solutions:**
1. Add to config:
   ```yaml
   ai:
     api_key: "sk-..."
   ```
2. Or use environment variable:
   ```bash
   export OPENAI_API_KEY="sk-..."
   ```

### API errors

**Error:**
```
openai request failed: 429 Too Many Requests
```

**Solution:**
Reduce concurrency:
```yaml
ai:
  max_batch: 10
  max_concurrency: 2
```

### AI has no effect

**Symptoms:** `--ai` flag doesn't change anything

**Requirements:**
1. Must use with `--db` flag
2. Must have targets that need normalization
3. API key must be configured

**Check:**
```bash
bbscope poll h1 --db --ai -l debug
```

---

## Performance Issues

### Slow polling

**Solutions:**
1. Increase concurrency (carefully):
   ```bash
   bbscope poll --concurrency 10
   ```
2. Filter to specific platforms:
   ```bash
   bbscope poll h1  # Instead of all platforms
   ```
3. Filter by category:
   ```bash
   bbscope poll --category wildcard
   ```

### High memory usage

**Possible causes:**
- Very large number of programs
- AI normalization with high batch size

**Solutions:**
1. Poll platforms individually
2. Reduce AI batch size:
   ```yaml
   ai:
     max_batch: 10
   ```

### Database queries slow

**Solutions:**
1. Check database indexes exist (auto-created)
2. Vacuum database:
   ```sql
   VACUUM ANALYZE;
   ```
3. Check for connection issues

---

## Docker Issues

### Container can't reach database

**Error:**
```
dial tcp 127.0.0.1:5432: connect: connection refused
```

**Problem:** Container localhost ≠ Host localhost

**Solutions:**

For Docker Desktop (macOS/Windows):
```yaml
db_url: "postgres://user:pass@host.docker.internal:5432/bbscope?sslmode=disable"
```

For Linux:
```bash
docker run --network host bbscope poll --db
```

Or use Docker network:
```bash
# Create network
docker network create bbscope-net

# Run database
docker run --name bbscope-db --network bbscope-net ...

# Run bbscope
docker run --network bbscope-net \
  -e "DB_URL=postgres://user:pass@bbscope-db:5432/bbscope?sslmode=disable" \
  bbscope poll --db
```

### Config file not mounted

**Error:**
```
Config File ".bbscope" Not Found
```

**Solution:**
Mount the config file:
```bash
docker run -v ~/.bbscope.yaml:/root/.bbscope.yaml bbscope poll
```

### Permission denied on mounted file

**Solution:**
Check file permissions on host:
```bash
chmod 644 ~/.bbscope.yaml
```

---

## Common Error Messages

### "aborting update to prevent wiping out all targets"

**Cause:** Platform returned empty scope for a program that has existing targets

**What happened:** Safety check prevents accidental data loss

**Solutions:**
1. This may indicate a platform API issue - try again later
2. Check if the program was removed from the platform
3. If intentional, you may need to manually clean up database

### "unknown command"

**Error:**
```
Error: unknown command "h1" for "bbscope"
```

**Cause:** Using v1 syntax in v2

**Solution:**
Use new command structure:
```bash
# Old (v1)
bbscope h1

# New (v2)
bbscope poll h1
```

### "db_url not set in config"

**Cause:** Using `--db` flag without database configuration

**Solution:**
Add `db_url` to `~/.bbscope.yaml`:
```yaml
db_url: "postgres://user:pass@localhost:5432/bbscope?sslmode=disable"
```

### "context deadline exceeded"

**Cause:** Network timeout

**Solutions:**
1. Check network connectivity
2. Check if platform API is responding
3. Try with proxy to debug:
   ```bash
   bbscope poll --proxy http://127.0.0.1:8080
   ```

---

## Debugging Steps

### Enable Debug Logging

```bash
bbscope -l debug poll h1
```

### Enable HTTP Debugging

```bash
bbscope --debug-http poll h1
```

### Use Proxy for Inspection

```bash
# Start mitmproxy or Burp Suite
bbscope --proxy http://127.0.0.1:8080 poll bc
```

### Check Database State

```bash
bbscope db stats
bbscope db changes --limit 10
```

### Verify Configuration

```bash
cat ~/.bbscope.yaml
```

### Check PostgreSQL Logs

```bash
docker logs bbscope-db
```

---

## Getting Help

If you can't resolve an issue:

1. Check existing [GitHub Issues](https://github.com/sw33tLie/bbscope/issues)
2. Create a new issue with:
   - bbscope version (`bbscope --version` or commit)
   - Full error message
   - Steps to reproduce
   - Debug log output (sanitize credentials!)
   - OS and Go version
