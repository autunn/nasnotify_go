param(
    [string]$Version = "",
    [int]$Build = 1,
    [ValidateSet("all", "amd64", "arm64")]
    [string]$Arch = "all",
    [string]$UgcliPath = ""
)

$ErrorActionPreference = "Stop"

$repoRoot = Split-Path -Parent $PSScriptRoot
$packageRoot = Join-Path $repoRoot "packaging\ugreen-native-app"
$frontendProject = Join-Path $repoRoot "frontend\ugreen-app"
$buildDir = Join-Path $packageRoot "build_dir"
$startLocation = (Get-Location).Path
$targets = @(
    @{
        Arch         = "amd64"
        Output       = Join-Path $packageRoot "rootfs_amd64\bin\nasnotify"
        LegacyOutput = Join-Path $packageRoot "rootfs_amd64\sbin\nasnotify"
    },
    @{
        Arch         = "arm64"
        Output       = Join-Path $packageRoot "rootfs_arm64\bin\nasnotify"
        LegacyOutput = Join-Path $packageRoot "rootfs_arm64\sbin\nasnotify"
    }
)

if (-not $UgcliPath) {
    $UgcliPath = Join-Path $repoRoot "tools\ugcli\ugcli-v1.1.0.12-windows-amd64.exe"
}

if (-not (Test-Path -LiteralPath $UgcliPath)) {
    throw "ugcli not found: $UgcliPath"
}

$originalEnv = @{
    GOOS = $env:GOOS
    GOARCH = $env:GOARCH
    CGO_ENABLED = $env:CGO_ENABLED
}

try {
    Push-Location $repoRoot

    if (Test-Path $frontendProject) {
        $viteBin = Join-Path $frontendProject "node_modules\.bin\vite.cmd"
        $ugosCoreDir = Join-Path $frontendProject "node_modules\@ugreen-nas\core"
        $builderOpenDir = Join-Path $frontendProject "node_modules\@ugreen-nas\builder-open"
        if (-not (Test-Path -LiteralPath $viteBin) -or -not (Test-Path -LiteralPath $ugosCoreDir) -or -not (Test-Path -LiteralPath $builderOpenDir)) {
            $packageLock = Join-Path $frontendProject "package-lock.json"
            if (Test-Path -LiteralPath $packageLock) {
                npm.cmd --prefix $frontendProject ci
            } else {
                npm.cmd --prefix $frontendProject install
            }
            if ($LASTEXITCODE -ne 0) {
                throw "frontend dependency install failed"
            }
        }
        npm.cmd --prefix $frontendProject run build
        if ($LASTEXITCODE -ne 0) {
            throw "frontend build failed"
        }

        node (Join-Path $repoRoot "scripts\build-ugreen-frontend.mjs")
        if ($LASTEXITCODE -ne 0) {
            throw "frontend sync failed"
        }
    }

    $selectedTargets = if ($Arch -eq "all") {
        $targets
    } else {
        $targets | Where-Object { $_.Arch -eq $Arch }
    }

    foreach ($target in $selectedTargets) {
        New-Item -ItemType Directory -Force -Path (Split-Path -Parent $target.Output) | Out-Null
        if ($target.LegacyOutput -and (Test-Path -LiteralPath $target.LegacyOutput)) {
            Remove-Item -LiteralPath $target.LegacyOutput -Force
        }
        $legacyBinDir = Split-Path -Parent $target.LegacyOutput
        if ($legacyBinDir -and (Test-Path -LiteralPath $legacyBinDir)) {
            Remove-Item -LiteralPath $legacyBinDir -Recurse -Force
        }

        $env:GOOS = "linux"
        $env:GOARCH = $target.Arch
        $env:CGO_ENABLED = "0"

        $ldflags = "-s -w"
        if ($Version) {
            $ldflags = "$ldflags -X main.Version=$Version"
        }

        go build -buildvcs=false -trimpath -ldflags $ldflags -o $target.Output ./cmd/nasnotify
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed for $($target.Arch)"
        }

    }

    if (Test-Path -LiteralPath $buildDir) {
        Remove-Item -LiteralPath $buildDir -Recurse -Force
    }

    node (Join-Path $repoRoot "scripts\pack-ugreen-native-app.mjs") --build $Build --arch $Arch --ugcli $UgcliPath
    if ($LASTEXITCODE -ne 0) {
        throw "pack-ugreen-native-app.mjs failed"
    }
}
finally {
    Set-Location -LiteralPath $startLocation

    foreach ($name in $originalEnv.Keys) {
        if ($null -eq $originalEnv[$name]) {
            Remove-Item "Env:$name" -ErrorAction SilentlyContinue
        } else {
            Set-Item "Env:$name" $originalEnv[$name]
        }
    }
}

Write-Host "UGREEN PC-only UPK files generated under $buildDir\pkgs\upk"
