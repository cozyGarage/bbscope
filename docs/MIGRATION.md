# Module Namespace Migration Plan

## Overview

This document outlines the plan to migrate the Go module from `github.com/sw33tLie/bbscope/v2` to `github.com/cozyGarage/bbscope/v2`.

## Current State

- **Old Module Path:** `github.com/sw33tLie/bbscope/v2`
- **New Module Path:** `github.com/cozyGarage/bbscope/v2`
- **Repository:** https://github.com/cozyGarage/bbscope (private)

## Files Requiring Changes

### 1. go.mod (1 change)
```go
// Change:
module github.com/sw33tLie/bbscope/v2
// To:
module github.com/cozyGarage/bbscope/v2
```

### 2. All Import Statements (~30+ files)

Every file that imports internal packages needs updating:

| Directory | Files | Import Pattern |
|-----------|-------|----------------|
| `cmd/` | 12 files | `github.com/sw33tLie/bbscope/v2/...` |
| `pkg/ai/` | 2 files | Internal imports |
| `pkg/platforms/*/` | 7 files | Internal imports |
| `pkg/storage/` | 4 files | Internal imports |
| `pkg/whttp/` | 1 file | Internal imports |
| `main.go` | 1 file | `github.com/sw33tLie/bbscope/v2/cmd` |

### 3. Build Configuration Files

| File | Changes Needed |
|------|----------------|
| `Makefile` | Update ldflags paths |
| `.goreleaser.yaml` | Update ldflags paths, GitHub owner |
| `.github/workflows/ci.yml` | Update ldflags paths |
| `.github/workflows/release.yml` | Already uses cozyGarage |

## Migration Steps

### Step 1: Update go.mod
```bash
# Single line change
sed -i 's|github.com/sw33tLie/bbscope/v2|github.com/cozyGarage/bbscope/v2|' go.mod
```

### Step 2: Update All Go Files
```bash
# PowerShell command to replace all occurrences
Get-ChildItem -Recurse -Include "*.go" | ForEach-Object {
    (Get-Content $_.FullName) -replace 'github.com/sw33tLie/bbscope/v2', 'github.com/cozyGarage/bbscope/v2' | Set-Content $_.FullName
}
```

### Step 3: Update Build Files
```bash
# Update Makefile, .goreleaser.yaml, CI workflows
# Same replacement pattern
```

### Step 4: Verify Build
```bash
go mod tidy
go build .
go test ./...
```

### Step 5: Commit and Push
```bash
git add .
git commit -m "Migrate module namespace to github.com/cozyGarage/bbscope/v2"
git push origin main
```

## Detailed File List

### cmd/ Directory
- `cmd/db.go`
- `cmd/db_ignore.go`
- `cmd/dev.go`
- `cmd/get.go`
- `cmd/get_cidrs.go`
- `cmd/get_domains.go`
- `cmd/get_ips.go`
- `cmd/get_urls.go`
- `cmd/get_wildcards.go`
- `cmd/poll.go`
- `cmd/poll_bc.go`
- `cmd/poll_dev.go`
- `cmd/poll_h1.go`
- `cmd/poll_immunefi.go`
- `cmd/poll_it.go`
- `cmd/poll_ywh.go`
- `cmd/root.go`
- `cmd/version.go`

### pkg/ Directory
- `pkg/ai/normalizer.go`
- `pkg/platforms/bugcrowd/bugcrowd.go`
- `pkg/platforms/bugcrowd/poller.go`
- `pkg/platforms/dev/dev.go`
- `pkg/platforms/hackerone/poller.go`
- `pkg/platforms/immunefi/immunefi.go`
- `pkg/platforms/immunefi/poller.go`
- `pkg/platforms/intigriti/poller.go`
- `pkg/platforms/yeswehack/poller.go`
- `pkg/storage/storage.go`
- `pkg/storage/transform.go`
- `pkg/storage/normalize.go`
- `pkg/storage/extra.go`
- `pkg/whttp/whttp.go`

### Root Directory
- `main.go`
- `go.mod`

### Build Configuration
- `Makefile`
- `.goreleaser.yaml`
- `.github/workflows/ci.yml`

## Automated Migration Script

Save this as `migrate-namespace.ps1`:

```powershell
#!/usr/bin/env pwsh
# Module Namespace Migration Script
# Migrates from sw33tLie to cozyGarage

$OldNamespace = "github.com/sw33tLie/bbscope/v2"
$NewNamespace = "github.com/cozyGarage/bbscope/v2"

Write-Host "Migrating module namespace..." -ForegroundColor Cyan
Write-Host "From: $OldNamespace" -ForegroundColor Yellow
Write-Host "To:   $NewNamespace" -ForegroundColor Green

# Files to update
$patterns = @("*.go", "go.mod", "Makefile", ".goreleaser.yaml")
$excludeDirs = @(".git", "vendor", "node_modules")

$count = 0

foreach ($pattern in $patterns) {
    Get-ChildItem -Recurse -Include $pattern -Exclude $excludeDirs | ForEach-Object {
        $content = Get-Content $_.FullName -Raw
        if ($content -match [regex]::Escape($OldNamespace)) {
            $newContent = $content -replace [regex]::Escape($OldNamespace), $NewNamespace
            Set-Content $_.FullName -Value $newContent -NoNewline
            Write-Host "Updated: $($_.FullName)" -ForegroundColor Gray
            $count++
        }
    }
}

# Update CI workflows
Get-ChildItem -Path ".github/workflows" -Include "*.yml","*.yaml" -Recurse -ErrorAction SilentlyContinue | ForEach-Object {
    $content = Get-Content $_.FullName -Raw
    if ($content -match [regex]::Escape($OldNamespace)) {
        $newContent = $content -replace [regex]::Escape($OldNamespace), $NewNamespace
        Set-Content $_.FullName -Value $newContent -NoNewline
        Write-Host "Updated: $($_.FullName)" -ForegroundColor Gray
        $count++
    }
}

Write-Host ""
Write-Host "Migration complete! Updated $count files." -ForegroundColor Green
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Cyan
Write-Host "  1. go mod tidy"
Write-Host "  2. go build ."
Write-Host "  3. go test ./..."
Write-Host "  4. git add . && git commit -m 'Migrate to cozyGarage namespace'"
Write-Host "  5. git push origin main"
```

## Verification Checklist

After migration, verify:

- [ ] `go mod tidy` succeeds
- [ ] `go build .` succeeds
- [ ] `go test ./...` passes
- [ ] `./bbscope version` works
- [ ] `./bbscope --help` works
- [ ] No references to `sw33tLie` remain (except git history)

## Rollback Plan

If issues occur:

```bash
git checkout HEAD~1 -- .
go mod tidy
```

## Notes

1. **Git History:** The git history will still reference sw33tLie. This is normal and expected.

2. **Dependencies:** External dependencies (lib/pq, hashicorp/go-retryablehttp, etc.) don't need changes.

3. **Private Repo:** Since your repo is private, there are no external consumers to worry about.

4. **Version:** Keep `v2` suffix for compatibility with existing internal structure.

## Estimated Time

- Manual migration: 15-30 minutes
- Automated script: 2-5 minutes
- Testing & verification: 10-15 minutes

**Total: ~30 minutes**
