param(
    [Parameter(Mandatory = $true)]
    [string]$V26ServerRoot,

    [Parameter(Mandatory = $true)]
    [string]$V3ServerRoot
)

$ErrorActionPreference = "Stop"
$projectDirectory = Split-Path -Parent $PSCommandPath
$v26Root = (Resolve-Path -LiteralPath $V26ServerRoot).Path
$v3Root = (Resolve-Path -LiteralPath $V3ServerRoot).Path
$v26References = Join-Path $v26Root "7DaysToDieServer_Data\Managed"
$v3References = Join-Path $v3Root "7DaysToDieServer_Data\Managed"
$webServerReference = Join-Path $v26Root "Mods\TFP_WebServer\WebServer.dll"
$payloadDirectory = Join-Path $projectDirectory "..\..\internal\controller\api\rpc\seven-days-to-die-land-claims"

$projects = @(
    @{ Name = "2.6"; Project = "LandClaims.v26.csproj"; References = $v26References; Output = "v2.6" },
    @{ Name = "3.x"; Project = "LandClaims.v3.csproj"; References = $v3References; Output = "v3" }
)

foreach ($target in $projects) {
    $referenceDirectory = (Resolve-Path -LiteralPath $target.References).Path
    $arguments = @(
        "msbuild",
        (Join-Path $projectDirectory $target.Project),
        "-restore:false",
        "-property:GameReferences=$referenceDirectory"
    )
    if ($target.Name -eq "2.6") {
        $arguments += "-property:WebServerReference=$webServerReference"
    }

    & dotnet $arguments
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to build the $($target.Name) land-claim mod."
    }

    $payload = Join-Path $payloadDirectory "$($target.Output)\XylonaLandClaims.dll"
    $bytes = [System.IO.File]::ReadAllBytes($payload)
    if ($bytes.Length -lt 2 -or $bytes[0] -ne 0x4d -or $bytes[1] -ne 0x5a) {
        throw "The $($target.Name) land-claim payload is not a valid PE assembly."
    }
}
