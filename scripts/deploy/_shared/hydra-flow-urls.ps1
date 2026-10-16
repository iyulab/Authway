# ============================================================
# Hydra flow URLs (URLS_LOGIN / URLS_CONSENT / URLS_LOGOUT / URLS_DEVICE_* / URLS_ERROR)
# ============================================================
# Hydra sends the browser to these addresses during sign-in. Login, consent,
# logout and a device sign-in's code page go to the central API, which hands
# the login UI an opaque flow id; the error screen and the page that ends a
# device sign-in are addressed directly. Both the deploy (publish-hydra)
# and its check (verify-hydra-env) read them from here, so they cannot drift.
#
# Derived from API_URL and AUTH_UI_URL — there is no separate key per URL.
# ============================================================

function Get-HydraFlowUrls {
    param(
        [Parameter(Mandatory = $true)]
        [hashtable]$EnvVars
    )

    foreach ($key in @('API_URL', 'AUTH_UI_URL')) {
        if ([string]::IsNullOrWhiteSpace($EnvVars[$key])) {
            throw "$key is required to derive Hydra's flow URLs"
        }
    }
    $api = $EnvVars['API_URL'].TrimEnd('/')
    $ui = $EnvVars['AUTH_UI_URL'].TrimEnd('/')

    return [ordered]@{
        URLS_LOGIN   = "$api/login"
        URLS_CONSENT = "$api/consent"
        URLS_LOGOUT  = "$api/logout"
        URLS_DEVICE_VERIFICATION = "$api/device"
        URLS_DEVICE_SUCCESS      = "$ui/device/done"
        URLS_ERROR   = "$ui/error"
    }
}

# ============================================================
# Where Hydra's token endpoint and token hook are
# ============================================================
# Discovery names the API's /oauth2/token, which passes every grant Hydra
# knows on to Hydra (WEBFINGER_OIDC_DISCOVERY_TOKEN_URL — Hydra then also
# accepts a client assertion addressed to it). Before it issues a token Hydra
# calls the API's token hook, at the API's Container Apps address
# (API_INTERNAL_URL) so the call stays inside the environment. That address
# may resolve to a private one, which CLIENTS_HTTP_DISALLOW_PRIVATE_IP_RANGES
# refuses: the hook, and only the hook, is the exception.
#
# The key Hydra presents to the hook is a secret and is not here
# (publish-hydra.core.ps1).
# ============================================================

function Get-HydraTokenRoute {
    param(
        [Parameter(Mandatory = $true)]
        [hashtable]$EnvVars
    )

    foreach ($key in @('API_URL', 'API_INTERNAL_URL')) {
        if ([string]::IsNullOrWhiteSpace($EnvVars[$key])) {
            throw "$key is required to derive Hydra's token endpoint and token hook"
        }
    }
    $api = $EnvVars['API_URL'].TrimEnd('/')
    $hook = $EnvVars['API_INTERNAL_URL'].TrimEnd('/') + '/internal/token-hook'

    return [ordered]@{
        WEBFINGER_OIDC_DISCOVERY_TOKEN_URL     = "$api/oauth2/token"
        OAUTH2_TOKEN_HOOK_URL                  = $hook
        CLIENTS_HTTP_PRIVATE_IP_EXCEPTION_URLS = $hook
    }
}
