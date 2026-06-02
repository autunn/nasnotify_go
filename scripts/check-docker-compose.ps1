param(
    [switch]$Build,
    [switch]$SmokeTest,
    [switch]$Cleanup,
    [string]$ContainerName = "nasnotify-go",
    [string]$Url = "http://127.0.0.1:5080"
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $repoRoot

function Invoke-Checked {
    param(
        [Parameter(Mandatory = $true)]
        [string]$FilePath,
        [Parameter(ValueFromRemainingArguments = $true)]
        [string[]]$Arguments
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$FilePath $($Arguments -join ' ') failed with exit code $LASTEXITCODE"
    }
}

$composeArgs = @("compose", "--env-file", ".env.example")
if ($Build) {
    $composeArgs += @("-f", "docker-compose.yml", "-f", "docker-compose.local.yml")
}

Write-Host "==> Validate compose"
Invoke-Checked -FilePath docker -Arguments ($composeArgs + @("config")) | Out-Null

if ($Build) {
    Write-Host "==> Build local Docker image"
    Invoke-Checked -FilePath docker -Arguments ($composeArgs + @("build", "nasnotify"))
}

if ($SmokeTest) {
    Write-Host "==> Start nasnotify"
    Invoke-Checked -FilePath docker -Arguments ($composeArgs + @("up", "-d", "nasnotify"))

    Write-Host "==> Wait for healthy container"
    $healthy = $false
    for ($i = 0; $i -lt 60; $i++) {
        $health = docker inspect --format "{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}" $ContainerName 2>$null
        if ($health -eq "healthy" -or $health -eq "running") {
            $healthy = $true
            break
        }
        Start-Sleep -Seconds 2
    }
    if (-not $healthy) {
        docker logs --tail 120 $ContainerName
        throw "$ContainerName did not become healthy"
    }

    Write-Host "==> Smoke test HTTP endpoints"
    $healthResp = Invoke-WebRequest -UseBasicParsing "$Url/healthz"
    if ($healthResp.StatusCode -ne 200) {
        throw "healthz returned $($healthResp.StatusCode)"
    }
    $bootstrapResp = Invoke-WebRequest -UseBasicParsing "$Url/api/bootstrap"
    if ($bootstrapResp.StatusCode -ne 200 -or $bootstrapResp.Headers["Content-Type"] -notmatch "application/json") {
        throw "bootstrap did not return JSON"
    }

    $image = docker inspect --format "{{.Config.Image}}" $ContainerName
    $status = docker inspect --format "{{.State.Status}}" $ContainerName
    Write-Host "==> Container $ContainerName is $status, image=$image, url=$Url"
}

if ($Cleanup) {
    Write-Host "==> Cleanup compose stack"
    Invoke-Checked -FilePath docker -Arguments ($composeArgs + @("down", "--remove-orphans"))
}

Write-Host "Docker compose check passed."
