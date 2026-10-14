# Starts a disposable authorization server (in-memory store) with the login
# UI's pages pointed at a port nothing listens on (the probe plays the UI),
# runs probe.py against it, and removes the container. Needs Docker and
# Python.
#
#   scripts/probes/device-flow/run.ps1

$ErrorActionPreference = 'Stop'
$name = 'authway-probe-device'
docker rm -f $name 2>$null | Out-Null

docker run -d --name $name -p 4644:4444 -p 4645:4445 `
    -e DSN=memory `
    -e SECRETS_SYSTEM=probe-system-secret-not-for-use `
    -e URLS_SELF_ISSUER=http://localhost:4644 `
    -e URLS_LOGIN=http://localhost:9914/login `
    -e URLS_CONSENT=http://localhost:9914/consent `
    -e URLS_DEVICE_VERIFICATION=http://localhost:9914/device `
    -e URLS_DEVICE_SUCCESS=http://localhost:9914/device/done `
    oryd/hydra:v26.2.0 serve all --dev | Out-Null

try {
    python -I (Join-Path $PSScriptRoot 'probe.py')
    $code = $LASTEXITCODE
} finally {
    if ($code -ne 0) { docker logs --tail 30 $name }
    docker rm -f $name | Out-Null
}
exit $code
