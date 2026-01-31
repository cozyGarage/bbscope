# bbscope Security Audit Report

## Executive Summary

This document provides a security assessment of the bbscope codebase, identifying potential vulnerabilities, security concerns, and recommendations for improvement. The analysis covers authentication, data handling, network security, and code quality from a security perspective.

### Fixed Issues Summary

The following security issues identified in this audit have been addressed:

| Issue | Severity | Status |
|-------|----------|--------|
| Insecure TLS configuration (TLS 1.1, weak ciphers) | High | ✅ Fixed |
| Debug mode exposes Authorization headers | Medium | ✅ Fixed |
| Unlimited HTTP retries (99999) | Medium | ✅ Fixed |
| Docker runs as root | Low | ✅ Fixed |
| Config file permissions not checked | Low | ✅ Fixed |
| Database name not validated (SQL injection risk) | Medium | ✅ Fixed |
| DB password exposed in error messages | Medium | ✅ Fixed |
| No graceful shutdown handling | Low | ✅ Fixed |

---

## 1. Authentication & Credentials

### 1.1 Credential Storage

**Finding: Credentials stored in plaintext config file**
- **Severity**: Medium
- **Location**: `~/.bbscope.yaml`
- **Issue**: All platform credentials (passwords, API tokens, OTP secrets) are stored in plaintext
- **Code Reference**: [cmd/root.go](cmd/root.go#L107-L124)

```yaml
# Current storage format
hackerone:
  username: "user"
  token: "api_token"  # Plaintext!
bugcrowd:
  password: "secret"   # Plaintext!
  otpsecret: "base32"  # Plaintext 2FA secret!
```

**Recommendations**:
1. ~~Warn users about file permissions on first creation~~ ✅ **IMPLEMENTED** - Config file now created with 0600 permissions and a warning is shown if permissions are too open
2. ~~Document recommended `chmod 600 ~/.bbscope.yaml`~~ ✅ **IMPLEMENTED** - Automatically enforced
3. Consider supporting OS keychain integration (macOS Keychain, Windows Credential Manager)
4. Support environment variable fallbacks for all credentials (partially implemented)

### 1.2 OTP Secret Handling

**Finding: 2FA secrets stored and transmitted in memory**
- **Severity**: Low-Medium
- **Location**: [pkg/otp/otp.go](pkg/otp/otp.go)
- **Issue**: OTP secrets remain in memory; no secure wiping after use

**Recommendations**:
1. Zero out OTP secrets after code generation
2. Consider using secure memory allocation for sensitive data

### 1.3 Basic Auth Credentials in Memory

**Finding: HackerOne credentials base64-encoded but not encrypted**
- **Severity**: Low
- **Location**: [pkg/platforms/hackerone/poller.go](pkg/platforms/hackerone/poller.go#L21-L25)

```go
func NewPoller(username, token string) *Poller {
    raw := username + ":" + token
    return &Poller{authB64: base64.StdEncoding.EncodeToString([]byte(raw))}
}
```

**Note**: Base64 is not encryption; this is standard HTTP Basic Auth behavior.

---

## 2. Network Security

### 2.1 TLS Configuration

**Finding: ~~Insecure TLS configuration for Bugcrowd proxy~~ ✅ FIXED**
- **Severity**: High → **Resolved**
- **Location**: [pkg/platforms/bugcrowd/bugcrowd.go](pkg/platforms/bugcrowd/bugcrowd.go)

**Previous Issues** (now fixed):
1. ~~Certificate verification disabled~~ → Now only disabled when proxy is explicitly used
2. ~~Forces TLS 1.1~~ → Now uses TLS 1.2 minimum
3. ~~Uses CBC cipher suites~~ → Now uses Go's default secure cipher suites

**Current secure configuration:**
```go
TLSClientConfig: &tls.Config{
    InsecureSkipVerify: proxy != "",  // Only when proxy explicitly set
    MinVersion:         tls.VersionTLS12,
}
```

### 2.2 HTTP Client Configuration

**Finding: ~~Global debug mode exposes sensitive data~~ ✅ FIXED**
- **Severity**: Medium → **Resolved**
- **Location**: [pkg/whttp/whttp.go](pkg/whttp/whttp.go)

**Previous issue**: Debug mode printed Authorization headers to stdout in plaintext.

**Fix implemented**: Sensitive headers are now redacted in debug output:
```go
// Headers like Authorization, Cookie, X-Csrf-Token, X-Api-Key now show [REDACTED]
func isSensitiveHeader(name string) bool {
    sensitiveHeaders := []string{"authorization", "cookie", "x-csrf-token", "x-api-key", "x-auth-token"}
    for _, h := range sensitiveHeaders {
        if strings.EqualFold(name, h) {
            return true
        }
    }
    return false
}
```

### 2.3 SQL Injection Risk

**Finding: ~~Dynamic SQL in database creation~~ ✅ FIXED**
- **Severity**: Medium → **Resolved**
- **Location**: [pkg/storage/storage.go](pkg/storage/storage.go)

**Fix implemented:**
- Added `validateDatabaseName()` function with strict regex pattern `^[a-zA-Z_][a-zA-Z0-9_]*$`
- Database name length limited to 1-63 characters
- Using `pq.QuoteIdentifier()` for safe SQL quoting
- Connection string passwords are now redacted in error messages

```go
func validateDatabaseName(name string) error {
    if len(name) == 0 || len(name) > 63 {
        return fmt.Errorf("%w: length must be 1-63 characters", ErrInvalidDatabaseName)
    }
    if !validDBNameRegex.MatchString(name) {
        return fmt.Errorf("%w: got %q", ErrInvalidDatabaseName, name)
    }
    return nil
}
```

---

## 3. Data Security

### 3.1 Sensitive Data in Logs

**Finding: ~~Potential sensitive data exposure in error messages~~ ✅ FIXED**
- **Severity**: Low-Medium → **Resolved**
- **Locations**: `pkg/storage/storage.go`, `internal/utils/utils.go`

**Fix implemented:**
- Added `redactConnectionString()` function that removes passwords from database URLs
- Error messages now show `postgres://user:****@host/db` instead of the actual password
- Added `RedactURL()` helper in `internal/utils/utils.go` for general use

Error messages may still include:
- Program handles and URLs (not sensitive)
- Target scope information (public data)

### 3.2 AI API Key Exposure

**Finding: API key fallback from environment**
- **Severity**: Low
- **Location**: [cmd/poll.go](cmd/poll.go#L151-L154)

```go
if cfg.APIKey == "" {
    cfg.APIKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
}
```

**Note**: This is actually a security-positive pattern (environment > config file).

### 3.3 Session Cookie Handling

**Finding: Bugcrowd session cookies stored in memory**
- **Severity**: Low
- **Location**: [pkg/platforms/bugcrowd/bugcrowd.go](pkg/platforms/bugcrowd/bugcrowd.go#L74-L77)

Session cookies remain in memory throughout program execution.

---

## 4. Input Validation

### 4.1 URL Validation

**Finding: Limited URL validation before processing**
- **Severity**: Low
- **Location**: [pkg/storage/normalize.go](pkg/storage/normalize.go)

While URLs are normalized, there's limited validation of URL structure.

**Recommendations**:
1. Validate URL schemes (only allow http/https)
2. Sanitize or reject URLs with unusual characters

### 4.2 Category Validation

**Finding: Unknown categories handled gracefully**
- **Severity**: None (positive finding)
- **Location**: [pkg/scope/scope.go](pkg/scope/scope.go#L84-L86)

Unknown categories are logged as warnings but don't crash the application.

---

## 5. Dependency Security

### 5.1 Known Vulnerable Dependencies

Run `go list -m all | nancy sleuth` or `govulncheck` to check for known vulnerabilities.

**Current Dependencies of Note**:

| Dependency | Version | Notes |
|------------|---------|-------|
| `golang.org/x/net` | v0.44.0 | Keep updated |
| `golang.org/x/crypto` | (indirect) | Keep updated |
| `lib/pq` | v1.10.9 | PostgreSQL driver |

**Recommendations**:
1. Set up automated dependency vulnerability scanning (Dependabot, Snyk)
2. Regular `go get -u` for security patches
3. Review Go security advisories

### 5.2 Supply Chain

**Finding: No vendoring or checksum verification beyond go.sum**
- **Severity**: Low
- The project relies on Go modules with checksums (go.sum)
- No additional supply chain protections

---

## 6. Container Security

### 6.1 Docker Image

**Finding: Running as root in container**
- **Severity**: Medium
- **Location**: [Dockerfile](Dockerfile)

```dockerfile
FROM alpine:latest
WORKDIR /root/
# Runs as root by default
```

**Recommendations**:
1. Create non-root user in Dockerfile
2. Use specific Alpine version (not `latest`)
3. Add `USER` directive

```dockerfile
# Recommended changes
FROM alpine:3.19

RUN adduser -D -g '' bbscope
USER bbscope
WORKDIR /home/bbscope
```

### 6.2 Sensitive Data in Docker

**Issue**: Config file mounted from host may expose credentials if container is compromised

**Recommendations**:
1. Document secure mounting practices
2. Consider secrets management (Docker secrets, K8s secrets)

---

## 7. Rate Limiting & Abuse Prevention

### 7.1 Bugcrowd Rate Limiting

**Finding: Rate limiting implemented to avoid WAF bans**
- **Severity**: None (positive finding)
- **Location**: [pkg/platforms/bugcrowd/bugcrowd.go](pkg/platforms/bugcrowd/bugcrowd.go#L41-L58)

The global rate limiter (1 req/sec) prevents abuse and WAF bans.

### 7.2 API Retry Logic

**Finding: ~~Aggressive retry could mask issues~~ ✅ FIXED**
- **Severity**: Low → **Resolved**
- **Location**: [pkg/whttp/whttp.go](pkg/whttp/whttp.go)

**Previous issue:**
```go
retryClient.RetryMax = 99999  // Near-infinite retries
```

**Fix implemented:**
```go
retryClient.RetryMax = 10
retryClient.RetryWaitMin = 1 * time.Second
retryClient.RetryWaitMax = 30 * time.Second
```

This prevents resource exhaustion and properly surfaces errors after reasonable retry attempts.

---

## 8. Error Handling Security

### 8.1 Error Information Leakage

**Finding: Detailed errors exposed to users**
- **Severity**: Low
- Various locations return raw error messages that may contain internal details

**Recommendations**:
1. Wrap errors with user-friendly messages
2. Log detailed errors internally; show summary to users

### 8.2 Panic Recovery

**Finding: No global panic recovery**
- **Severity**: Low
- Unhandled panics will crash with stack traces

**Recommendations**:
1. Add panic recovery in main execution path
2. Log panics without exposing stack traces to end users

---

## 9. Security Best Practices Compliance

| Practice | Status | Notes |
|----------|--------|-------|
| Principle of Least Privilege | ⚠️ Partial | Container runs as root |
| Defense in Depth | ⚠️ Partial | Single auth layer per platform |
| Secure Defaults | ⚠️ Partial | Debug mode off by default ✓, but weak TLS config |
| Input Validation | ✓ Mostly | URL normalization, category validation |
| Output Encoding | ✓ Good | Proper escaping in output |
| Error Handling | ⚠️ Partial | Some info leakage |
| Logging Security | ⚠️ Partial | Sensitive data in debug mode |
| Dependency Management | ✓ Good | Go modules with checksums |

---

## 10. Recommendations Summary

### ✅ Completed Fixes

| # | Issue | Status |
|---|-------|--------|
| 1 | Fix TLS configuration in Bugcrowd proxy mode | ✅ Fixed |
| 2 | Add non-root user to Docker container | ✅ Fixed |
| 3 | Redact sensitive headers in debug output | ✅ Fixed |
| 4 | Reduce retry limits from 99999 to 10 | ✅ Fixed |
| 5 | Validate database name in auto-creation | ✅ Fixed |
| 6 | Set config file permissions on creation | ✅ Fixed |
| 7 | Redact database passwords in error logs | ✅ Fixed |
| 8 | Pin Docker base image versions (Alpine 3.19) | ✅ Fixed |
| 9 | Add graceful shutdown support | ✅ Fixed |
| 10 | Add input validation package | ✅ Fixed |

### Remaining Low Priority
- **Add keychain integration** for credential storage
- **Implement panic recovery** in main path
- **Zero sensitive data** after use

---

## 11. Security Testing Recommendations

1. **Static Analysis**: Run `gosec` and `staticcheck`
   ```bash
   gosec ./...
   staticcheck ./...
   ```

2. **Dependency Scanning**: Run `govulncheck`
   ```bash
   govulncheck ./...
   ```

3. **Secret Scanning**: Ensure no hardcoded secrets
   ```bash
   trufflehog git file://.
   ```

4. **Container Scanning**: Scan Docker image
   ```bash
   trivy image ghcr.io/cozygarage/bbscope:latest
   ```

---

## Appendix: STRIDE Threat Model

| Threat | Applicable | Mitigation Status |
|--------|------------|-------------------|
| **S**poofing | Yes - credentials | Config file, basic auth |
| **T**ampering | Yes - network | ✅ TLS fixed (uses TLS 1.2+) |
| **R**epudiation | Partial | Change logging in DB |
| **I**nformation Disclosure | Yes | ✅ Debug headers redacted |
| **D**enial of Service | Low | ✅ Rate limiting + retry limits fixed |
| **E**levation of Privilege | Low | ✅ Docker runs as non-root |

---

*Last Updated: January 2025*
