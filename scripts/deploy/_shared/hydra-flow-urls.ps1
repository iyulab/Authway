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
