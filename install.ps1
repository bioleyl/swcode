# swcode's installer for Windows: downloads the desktop app from the latest release, checks it against the release's
# checksums, and opens it, which installs it in %LOCALAPPDATA%\Programs\swcode with its shortcuts and starts it.
#
#   irm https://raw.githubusercontent.com/bioleyl/swcode/main/install.ps1 | iex
#
# SWCODE_RELEASES downloads from elsewhere: a release of your own, or a folder served for a test.
$ErrorActionPreference = 'Stop'
# Windows PowerShell's progress bar slows a download down many times over.
$ProgressPreference = 'SilentlyContinue'

$releases = if ($env:SWCODE_RELEASES) { $env:SWCODE_RELEASES } else { 'https://github.com/bioleyl/swcode/releases/latest/download' }
$program = 'swcode-desktop.exe'
$checksums = 'SHA256SUMS'
# A program that runs cannot be replaced: the app's, or the daemon's alone, both swcode-desktop.exe.
if (Get-Process -Name 'swcode-desktop' -ErrorAction SilentlyContinue) {
    Write-Host 'swcode is open: quit it (Quit swcode, in its icon''s menu), then run this again.'
    return
}
$folder = Join-Path ([IO.Path]::GetTempPath()) ('swcode-' + [Guid]::NewGuid())
New-Item -ItemType Directory -Path $folder | Out-Null

try {
    Write-Host "Downloading $program..."
    $download = Join-Path $folder $program
    $sums = Join-Path $folder $checksums
    Invoke-WebRequest -Uri "$releases/$program" -OutFile $download -UseBasicParsing
    Invoke-WebRequest -Uri "$releases/$checksums" -OutFile $sums -UseBasicParsing
    $line = Get-Content $sums | Where-Object { $_ -match " $([regex]::Escape($program))$" } | Select-Object -First 1
    $actual = (Get-FileHash -Algorithm SHA256 -Path $download).Hash
    if (-not $line -or $line.Split(' ')[0] -ne $actual.ToLower()) {
        throw "The download does not match the release's checksum: try again."
    }
    # It installs itself in the programs folder, starts from there, and ends: then its download can go.
    Start-Process -FilePath $download -Wait
    Write-Host 'swcode is installed: it opens from the Start menu.'
}
finally {
    Remove-Item -Recurse -Force $folder -ErrorAction SilentlyContinue
}
