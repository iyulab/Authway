#!/bin/sh

# Register ASP.NET sample OAuth client in Azure Hydra
# Admin API is available at /admin/ path via nginx proxy

HYDRA_ADMIN_URL="https://authway-hydra.lemonfield-03f5fb88.koreacentral.azurecontainerapps.io/admin"
CLIENT_ID="authway_yW9iQXu8Ho4ISf6plaHl_w"
CLIENT_SECRET="hPUKZhePS1fk9Lx3LDzZVcQ7TWqbPQWW0-LnGN8Z_10"

echo "Registering OAuth client in Azure Hydra..."
echo "Admin URL: $HYDRA_ADMIN_URL"
echo "Client ID: $CLIENT_ID"

curl -v -X POST "$HYDRA_ADMIN_URL/clients" \
  -H "Content-Type: application/json" \
  -d @- <<EOF
{
  "client_id": "$CLIENT_ID",
  "client_secret": "$CLIENT_SECRET",
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "redirect_uris": [
    "https://localhost:5001/signin-oidc",
    "http://localhost:5000/signin-oidc"
  ],
  "post_logout_redirect_uris": [
    "https://localhost:5001/signout-callback-oidc",
    "http://localhost:5000/signout-callback-oidc"
  ],
  "scope": "openid profile email",
  "token_endpoint_auth_method": "client_secret_post"
}
EOF

echo ""
echo "Client registration completed!"
