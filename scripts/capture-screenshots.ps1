[CmdletBinding()]
param(
    [string]$OutputDirectory = (Join-Path $PSScriptRoot '..\docs\images')
)

$ErrorActionPreference = 'Stop'
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$outputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
$temporaryDirectory = Join-Path (Join-Path $repoRoot 'dist') ('.screenshot-build-' + $PID)
$exePath = Join-Path $temporaryDirectory 'IdleTrigger-screenshot.exe'
$files = @('panel-en-light.png', 'panel-en-dark.png', 'panel-zh-light.png', 'panel-zh-dark.png')
$expectedWidth = 472
$expectedHeight = 751

function Get-PngSize([string]$Path) {
    $bytes = [IO.File]::ReadAllBytes($Path)
    if ($bytes.Length -lt 24) { throw "PNG is too small: $Path" }
    $signature = [byte[]](0x89,0x50,0x4e,0x47,0x0d,0x0a,0x1a,0x0a)
    for ($i = 0; $i -lt $signature.Length; $i++) { if ($bytes[$i] -ne $signature[$i]) { throw "Invalid PNG signature: $Path" } }
    $width = ([int]$bytes[16] -shl 24) -bor ([int]$bytes[17] -shl 16) -bor ([int]$bytes[18] -shl 8) -bor [int]$bytes[19]
    $height = ([int]$bytes[20] -shl 24) -bor ([int]$bytes[21] -shl 16) -bor ([int]$bytes[22] -shl 8) -bor [int]$bytes[23]
    if ($width -le 0 -or $height -le 0) { throw "Invalid PNG dimensions: $Path" }
    return ,@($width, $height)
}

try {
    New-Item -ItemType Directory -Force -Path $temporaryDirectory | Out-Null
    Push-Location $repoRoot
    try {
        $env:CGO_ENABLED = '0'
        $env:GOARCH = 'amd64'
		$env:GOCACHE = Join-Path $temporaryDirectory 'gocache'
        go build -trimpath -ldflags '-s -w -H windowsgui -X github.com/JeffioZ/idletrigger/internal/version.Value=screenshot' -o $exePath .
        if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
        & $exePath screenshot --all --output $outputDirectory
        if ($LASTEXITCODE -ne 0) { throw "screenshot command failed with exit code $LASTEXITCODE" }
    } finally { Pop-Location }

    foreach ($name in $files) {
        $path = Join-Path $outputDirectory $name
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { throw "Missing screenshot: $path" }
        $size = Get-PngSize $path
        if ($size[0] -ne $expectedWidth -or $size[1] -ne $expectedHeight) { throw "Unexpected PNG dimensions for ${name}: $($size[0])x$($size[1])" }
        Write-Output "$name $($size[0])x$($size[1])"
    }
} finally {
    for ($attempt = 0; $attempt -lt 10 -and (Test-Path -LiteralPath $temporaryDirectory); $attempt++) {
        try {
            Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force -ErrorAction Stop
        } catch {
            if ($attempt -eq 9) { throw }
            Start-Sleep -Milliseconds 200
        }
    }
}
