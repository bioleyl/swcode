# Lets phones and other machines on private networks reach swcode in WSL. WSL's own firewall (Hyper-V's) drops
# what comes in by default, and Windows never asks about it: swcode install runs this, as an administrator, through a
# UAC prompt. Private networks only: home and office ones, not the internet.
param(
  [Parameter(Mandatory)] [int] $Port
)
$ErrorActionPreference = 'Stop'

$Wsl = '{40E0AC32-46A5-438A-A0B2-2B479E8F2E90}'
$Name = 'swcode'
$PrivateNetworks = @('10.0.0.0/8', '172.16.0.0/12', '192.168.0.0/16')

Remove-NetFirewallHyperVRule -Name $Name -ErrorAction SilentlyContinue
New-NetFirewallHyperVRule -Name $Name -DisplayName 'swcode (phones on the network)' -Direction Inbound `
  -VMCreatorId $Wsl -Protocol TCP -LocalPorts $Port -RemoteAddresses $PrivateNetworks -Action Allow | Out-Null
