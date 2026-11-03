# Puts a swcode shortcut in the Start menu and on the desktop: it opens swcode, starting the daemon in the
# background when it does not run. swcode install runs this, for swcode.exe on Windows or swcode in WSL.
param(
  [Parameter(Mandatory)] [string] $Target,
  [string] $Arguments = '',
  [Parameter(Mandatory)] [string] $Icon
)
$ErrorActionPreference = 'Stop'

$Minimized = 7
$Name = 'swcode.lnk'

$shell = New-Object -ComObject WScript.Shell
foreach ($place in @([Environment]::GetFolderPath('Programs'), [Environment]::GetFolderPath('Desktop'))) {
  $link = $shell.CreateShortcut((Join-Path $place $Name))
  $link.TargetPath = $Target
  $link.Arguments = $Arguments
  $link.WindowStyle = $Minimized
  $link.IconLocation = "$Icon,0"
  $link.Description = 'Opens swcode, starting it when it does not run'
  $link.Save()
  Write-Output "Shortcut: $($link.FullName)"
}

# Explorer keeps the icons it showed: without this, an earlier icon of the same file can stay.
& (Join-Path $env:WINDIR 'System32\ie4uinit.exe') -show
