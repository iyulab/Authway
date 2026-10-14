# Starts a disposable authorization server (in-memory store) configured the
# way the delegation design needs it, runs probe.py against it, and removes
# the container. Needs Docker and Python with the `cryptography` package.
#
#   scripts/probes/engine-delegation/run.ps1

$ErrorActionPreference = 'Stop'
$name = 'authway-probe-delegation'
docker rm -f $name 2>$null | Out-Null

docker run -d --name $name -p 4544:4444 -p 4545:4445 `
    -e DSN=memory `
    -e SECRETS_SYSTEM=probe-system-secret-not-for-use `
    -e URLS_SELF_ISSUER=http://localhost:4544 `
    -e WEBFINGER_OIDC_DISCOVERY_TOKEN_URL=http://localhost:9999/oauth2/token `
    -e OAUTH2_TOKEN_HOOK_URL=http://host.docker.internal:9911/hook `
    -e OAUTH2_TOKEN_HOOK_AUTH_TYPE=api_key `
    -e OAUTH2_TOKEN_HOOK_AUTH_CONFIG_NAME=X-Hook-Key `
    -e OAUTH2_TOKEN_HOOK_AUTH_CONFIG_VALUE=probe-hook-key `
    -e OAUTH2_TOKEN_HOOK_AUTH_CONFIG_IN=header `
    -e OAUTH2_ALLOWED_TOP_LEVEL_CLAIMS=act `
    -e STRATEGIES_ACCESS_TOKEN=jwt `
    oryd/hydra:v26.2.0 serve all --dev | Out-Null

try {
    python -I (Join-Path $PSScriptRoot 'probe.py')
    $code = $LASTEXITCODE
} finally {
    if ($code -ne 0) { docker logs --tail 40 $name }
    docker rm -f $name | Out-Null
}
exit $code
