[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

function Get-GoDependencies([string[]]$BuildArguments) {
    $dependencies = @(& go list @BuildArguments -deps -f '{{.ImportPath}}' .)
    if ($LASTEXITCODE -ne 0) {
        throw "go list failed with exit code $LASTEXITCODE"
    }
    return $dependencies
}

$releaseDependencies = @(Get-GoDependencies @())
$forbiddenReleaseDependencies = @(
    'github.com/JeffioZ/idletrigger/internal/inputdiag',
    'github.com/JeffioZ/idletrigger/internal/screenshot',
    'image/png',
    'compress/flate',
    'compress/zlib',
    'crypto/md5',
    'os/exec'
)
foreach ($dependency in $forbiddenReleaseDependencies) {
    if ($releaseDependencies -contains $dependency) {
        throw "release dependency boundary violated: $dependency"
    }
}

$devtoolsDependencies = @(Get-GoDependencies @('-tags', 'devtools'))
$requiredDevtoolsDependencies = @(
    'github.com/JeffioZ/idletrigger/internal/inputdiag',
    'github.com/JeffioZ/idletrigger/internal/screenshot',
    'image/png'
)
foreach ($dependency in $requiredDevtoolsDependencies) {
    if ($devtoolsDependencies -notcontains $dependency) {
        throw "devtools dependency is missing: $dependency"
    }
}

Write-Output 'Build dependency boundaries: OK'
