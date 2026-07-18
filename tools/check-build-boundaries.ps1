#requires -Version 7.0

[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $false
Set-StrictMode -Version Latest
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))

function Get-GoDependencies([string[]]$BuildArguments) {
    $dependencies = @(& go list @BuildArguments -deps -f '{{.ImportPath}}' ./cmd/idletrigger)
    if ($LASTEXITCODE -ne 0) {
        throw "go list failed with exit code $LASTEXITCODE"
    }
    return $dependencies
}

function Get-DirectPackageImports {
    $lines = @(& go list -f '{{.ImportPath}}|{{join .Imports ","}}' ./...)
    if ($LASTEXITCODE -ne 0) {
        throw "go list package import scan failed with exit code $LASTEXITCODE"
    }
    $result = @()
    foreach ($line in $lines) {
        $parts = $line -split '\|', 2
        $imports = @()
        if ($parts.Count -gt 1 -and $parts[1]) {
            $imports = @($parts[1] -split ',')
        }
        $result += [pscustomobject]@{ Package = $parts[0]; Imports = $imports }
    }
    return $result
}

function Test-LayerBoundary([object[]]$Packages, [string]$SourcePrefix, [string[]]$ForbiddenPrefixes) {
    foreach ($package in $Packages) {
        $isSource = $package.Package -eq $SourcePrefix -or
            $package.Package.StartsWith($SourcePrefix + '/', [StringComparison]::Ordinal)
        if (-not $isSource) {
            continue
        }
        foreach ($import in $package.Imports) {
            foreach ($forbidden in $ForbiddenPrefixes) {
                $isForbidden = $import -eq $forbidden -or
                    $import.StartsWith($forbidden + '/', [StringComparison]::Ordinal)
                if ($isForbidden) {
                    throw "layer boundary violated: $($package.Package) imports $import"
                }
            }
        }
    }
}

Push-Location $repoRoot
try {
    $releaseDependencies = @(Get-GoDependencies @())
    $forbiddenReleaseDependencies = @(
        'github.com/JeffioZ/idletrigger/internal/devtools/inputtrace',
        'github.com/JeffioZ/idletrigger/internal/devtools/screenshot',
        'image/png',
        'compress/flate',
        'compress/zlib',
        'crypto/md5',
        'crypto/sha256',
        'os/exec'
    )
    foreach ($dependency in $forbiddenReleaseDependencies) {
        if ($releaseDependencies -contains $dependency) {
            throw "release dependency boundary violated: $dependency"
        }
    }

    $devtoolsDependencies = @(Get-GoDependencies @('-tags', 'devtools'))
    $requiredDevtoolsDependencies = @(
        'github.com/JeffioZ/idletrigger/internal/devtools/inputtrace',
        'github.com/JeffioZ/idletrigger/internal/devtools/screenshot',
        'image/png'
    )
    foreach ($dependency in $requiredDevtoolsDependencies) {
        if ($devtoolsDependencies -notcontains $dependency) {
            throw "devtools dependency is missing: $dependency"
        }
    }

    $module = 'github.com/JeffioZ/idletrigger/internal/'
    $packages = @(Get-DirectPackageImports)
    Test-LayerBoundary $packages ($module + 'platform/windows') @(
        $module + 'app', $module + 'devtools', $module + 'feature', $module + 'ui'
    )
    Test-LayerBoundary $packages ($module + 'feature') @(
        $module + 'app', $module + 'devtools', $module + 'ui'
    )
    Test-LayerBoundary $packages ($module + 'ui') @(
        $module + 'app', $module + 'devtools', $module + 'feature'
    )
    Test-LayerBoundary $packages ($module + 'config') @(
        $module + 'app', $module + 'devtools', $module + 'feature', $module + 'platform', $module + 'ui'
    )
    Test-LayerBoundary $packages ($module + 'logging') @(
        $module + 'app', $module + 'config', $module + 'devtools', $module + 'feature',
        $module + 'i18n', $module + 'platform', $module + 'ui'
    )

    # Process-triggered automation is metadata-only. Keep this boundary explicit
    # so future changes cannot quietly add injection, memory access, or forced
    # process termination behavior.
    $processSafetyRoots = @(
        (Join-Path $PSScriptRoot '..\internal\platform\windows\processcatalog'),
        (Join-Path $PSScriptRoot '..\internal\feature\autorules')
    )
    $forbiddenProcessAPIs = @(
        'PROCESS_ALL_ACCESS',
        'SeDebugPrivilege',
        'AdjustTokenPrivileges',
        'ReadProcessMemory',
        'WriteProcessMemory',
        'VirtualAllocEx',
        'CreateRemoteThread',
        'TerminateProcess'
    )
    foreach ($root in $processSafetyRoots) {
        foreach ($source in Get-ChildItem -LiteralPath $root -Recurse -Filter '*.go' -File) {
            $text = Get-Content -Raw -Encoding UTF8 -LiteralPath $source.FullName
            foreach ($api in $forbiddenProcessAPIs) {
                if ($text.Contains($api, [StringComparison]::OrdinalIgnoreCase)) {
                    throw "process automation safety boundary violated: $($source.FullName) contains $api"
                }
            }
        }
    }

    Write-Output 'Build dependency boundaries: OK'
} finally {
    Pop-Location
}
