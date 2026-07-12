package actions

import (
	"errors"
	"fmt"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const sunkenlandLauncherPath = "xylona-sunkenland-launch.ps1"

func (inst *Instance) postSunkenlandInstall(gameServer *models.GameServer) error {
	if gameServer == nil {
		return errors.New("prepare Sunkenland launcher: game server is nil")
	}
	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return fmt.Errorf("prepare Sunkenland launcher: resolve node client: %w", errClient)
	}
	errWorlds := client.CreateFileOrDirectory(
		inst.ctx,
		gameServer.Directory,
		"worlds",
		"",
		true,
		node.ProtectionPolicy{},
	)
	if errWorlds != nil {
		return fmt.Errorf("prepare Sunkenland launcher: create worlds directory: %w", errWorlds)
	}
	errWrite := client.WriteFile(
		inst.ctx,
		gameServer.Directory,
		sunkenlandLauncherPath,
		[]byte(sunkenlandLauncherSource),
		node.ProtectionPolicy{},
	)
	if errWrite != nil {
		return fmt.Errorf("prepare Sunkenland launcher: write launcher: %w", errWrite)
	}
	return nil
}

const sunkenlandLauncherSource = `param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]] $ServerArguments
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$worldNamePattern = '^(?<name>.+)~(?<guid>[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12})$'
$worldsRoot = Join-Path $PSScriptRoot 'worlds'

if (-not (Test-Path -LiteralPath $worldsRoot -PathType Container)) {
    throw "Sunkenland worlds directory is missing: $worldsRoot"
}

$worlds = @(
    Get-ChildItem -LiteralPath $worldsRoot -Directory -Force |
        Where-Object { $_.Name -match $worldNamePattern }
)
if ($worlds.Count -ne 1) {
    throw "Sunkenland requires exactly one complete WorldName~GUID folder under $worldsRoot; found $($worlds.Count)."
}

$world = $worlds[0]
$worldContents = @(Get-ChildItem -LiteralPath $world.FullName -Force | Select-Object -First 1)
if ($worldContents.Count -eq 0) {
    throw "Sunkenland world folder is empty: $($world.FullName)"
}

$match = [regex]::Match($world.Name, $worldNamePattern)
$worldGuid = $match.Groups['guid'].Value
$userProfile = [Environment]::GetFolderPath('UserProfile')
$globalWorldsRoot = Join-Path $userProfile 'AppData\LocalLow\Vector3Studio\Sunkenland\Worlds'
New-Item -ItemType Directory -Path $globalWorldsRoot -Force | Out-Null
$globalWorld = Join-Path $globalWorldsRoot $world.Name

if (Test-Path -LiteralPath $globalWorld) {
    $existing = Get-Item -LiteralPath $globalWorld -Force
    $isReparsePoint = ($existing.Attributes -band [IO.FileAttributes]::ReparsePoint) -ne 0
    if (-not $isReparsePoint) {
        throw "A non-Xylona Sunkenland world already exists at $globalWorld. Move that world into this server's worlds directory or choose a different world; Xylona will not overwrite it."
    }
    $existingTarget = @($existing.Target)[0]
    if ([string]::IsNullOrWhiteSpace($existingTarget)) {
        throw "The existing Sunkenland world link at $globalWorld has no readable target."
    }
    if ([IO.Path]::IsPathRooted($existingTarget)) {
        $resolvedTarget = [IO.Path]::GetFullPath($existingTarget)
    } else {
        $resolvedTarget = [IO.Path]::GetFullPath((Join-Path $existing.Parent.FullName $existingTarget))
    }
    $expectedTarget = [IO.Path]::GetFullPath($world.FullName)
    if (-not [string]::Equals($resolvedTarget, $expectedTarget, [StringComparison]::OrdinalIgnoreCase)) {
        throw "Sunkenland world link collision at $globalWorld. It points to $resolvedTarget instead of $expectedTarget."
    }
} else {
    New-Item -ItemType Junction -Path $globalWorld -Target $world.FullName | Out-Null
    Write-Host "[Xylona] Linked Sunkenland world $($world.Name) into the dedicated-server save directory."
}

$executable = Join-Path $PSScriptRoot 'Sunkenland-DedicatedServer.exe'
if (-not (Test-Path -LiteralPath $executable -PathType Leaf)) {
    throw "Sunkenland dedicated server executable is missing: $executable"
}

$env:SteamAppId = '2667530'
Write-Host "[Xylona] Starting Sunkenland world $($world.Name) on GUID $worldGuid."
& $executable '-nographics' '-batchmode' '-worldGUID' $worldGuid @ServerArguments
exit $LASTEXITCODE
`
