#!/usr/bin/env pwsh
# Module Namespace Migration Script
# Migrates bbscope from github.com/sw33tLie/bbscope/v2 to github.com/cozyGarage/bbscope/v2

param(
    [switch]$DryRun = $false,
    [switch]$Verbose = $false
)

$OldNamespace = "github.com/sw33tLie/bbscope/v2"
$NewNamespace = "github.com/cozyGarage/bbscope/v2"

Write-Host "============================================" -ForegroundColor Cyan
Write-Host "  bbscope Module Namespace Migration" -ForegroundColor Cyan
Write-Host "============================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "From: $OldNamespace" -ForegroundColor Yellow
Write-Host "To:   $NewNamespace" -ForegroundColor Green
Write-Host ""

if ($DryRun) {
    Write-Host "[DRY RUN MODE - No changes will be made]" -ForegroundColor Magenta
    Write-Host ""
}

# Track statistics
$stats = @{
    FilesScanned = 0
    FilesUpdated = 0
    TotalReplacements = 0
}

# Function to process a single file
function Update-FileNamespace {
    param(
        [string]$FilePath
    )
    
    $stats.FilesScanned++
    
    try {
        $content = Get-Content $FilePath -Raw -ErrorAction Stop
        
        if ($null -eq $content) {
            return
        }
        
        # Count occurrences
        $matches = [regex]::Matches($content, [regex]::Escape($OldNamespace))
        $matchCount = $matches.Count
        
        if ($matchCount -gt 0) {
            $newContent = $content -replace [regex]::Escape($OldNamespace), $NewNamespace
            
            if (-not $DryRun) {
                Set-Content $FilePath -Value $newContent -NoNewline
            }
            
            $stats.FilesUpdated++
            $stats.TotalReplacements += $matchCount
            
            $relativePath = $FilePath -replace [regex]::Escape((Get-Location).Path + "\"), ""
            Write-Host "  [UPDATED] $relativePath ($matchCount replacements)" -ForegroundColor Green
            
            if ($Verbose) {
                foreach ($match in $matches) {
                    Write-Host "    - Line containing: $($match.Value)" -ForegroundColor Gray
                }
            }
        }
    }
    catch {
        Write-Host "  [ERROR] Failed to process: $FilePath - $_" -ForegroundColor Red
    }
}

# Process Go files
Write-Host "Processing Go files..." -ForegroundColor Cyan
Get-ChildItem -Recurse -Include "*.go" -File | Where-Object {
    $_.FullName -notmatch "[\\/]\.git[\\/]" -and 
    $_.FullName -notmatch "[\\/]vendor[\\/]"
} | ForEach-Object {
    Update-FileNamespace -FilePath $_.FullName
}

# Process go.mod
Write-Host ""
Write-Host "Processing go.mod..." -ForegroundColor Cyan
if (Test-Path "go.mod") {
    Update-FileNamespace -FilePath (Resolve-Path "go.mod").Path
}

# Process Makefile
Write-Host ""
Write-Host "Processing Makefile..." -ForegroundColor Cyan
if (Test-Path "Makefile") {
    Update-FileNamespace -FilePath (Resolve-Path "Makefile").Path
}

# Process .goreleaser.yaml
Write-Host ""
Write-Host "Processing .goreleaser.yaml..." -ForegroundColor Cyan
if (Test-Path ".goreleaser.yaml") {
    Update-FileNamespace -FilePath (Resolve-Path ".goreleaser.yaml").Path
}

# Process GitHub workflows
Write-Host ""
Write-Host "Processing GitHub workflows..." -ForegroundColor Cyan
if (Test-Path ".github/workflows") {
    Get-ChildItem -Path ".github/workflows" -Include "*.yml","*.yaml" -File | ForEach-Object {
        Update-FileNamespace -FilePath $_.FullName
    }
}

# Summary
Write-Host ""
Write-Host "============================================" -ForegroundColor Cyan
Write-Host "  Migration Summary" -ForegroundColor Cyan
Write-Host "============================================" -ForegroundColor Cyan
Write-Host ""
Write-Host "Files scanned:     $($stats.FilesScanned)" -ForegroundColor White
Write-Host "Files updated:     $($stats.FilesUpdated)" -ForegroundColor Green
Write-Host "Total replacements: $($stats.TotalReplacements)" -ForegroundColor Green
Write-Host ""

if ($DryRun) {
    Write-Host "[DRY RUN] No changes were made. Run without -DryRun to apply changes." -ForegroundColor Magenta
    Write-Host ""
}

# Next steps
if (-not $DryRun -and $stats.FilesUpdated -gt 0) {
    Write-Host "Next steps:" -ForegroundColor Yellow
    Write-Host "  1. Run: go mod tidy" -ForegroundColor White
    Write-Host "  2. Run: go build ." -ForegroundColor White
    Write-Host "  3. Run: go test ./..." -ForegroundColor White
    Write-Host "  4. Run: git add . && git commit -m 'Migrate to cozyGarage namespace'" -ForegroundColor White
    Write-Host "  5. Run: git push origin main" -ForegroundColor White
    Write-Host ""
}

# Verification
if (-not $DryRun) {
    Write-Host "Checking for remaining references..." -ForegroundColor Cyan
    $remaining = Get-ChildItem -Recurse -Include "*.go","go.mod","Makefile",".goreleaser.yaml" -File | Where-Object {
        $_.FullName -notmatch "[\\/]\.git[\\/]" -and
        $_.FullName -notmatch "[\\/]vendor[\\/]" -and
        $_.Name -ne "MIGRATION.md"
    } | ForEach-Object {
        $content = Get-Content $_.FullName -Raw -ErrorAction SilentlyContinue
        if ($content -match [regex]::Escape($OldNamespace)) {
            $_.FullName
        }
    }
    
    if ($remaining) {
        Write-Host ""
        Write-Host "[WARNING] Old namespace still found in:" -ForegroundColor Yellow
        $remaining | ForEach-Object {
            $relativePath = $_ -replace [regex]::Escape((Get-Location).Path + "\"), ""
            Write-Host "  - $relativePath" -ForegroundColor Yellow
        }
    } else {
        Write-Host "  No remaining references found. Migration complete!" -ForegroundColor Green
    }
}

Write-Host ""
