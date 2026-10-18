#!/bin/sh
# Creates .env from .env.example with every secret generated, and leaves an
# existing .env alone except for secrets it is still missing — so running it
# again never changes a secret the installation already uses.
set -eu
cd "$(dirname "$0")"

[ -f .env ] || cp .env.example .env

hex() { od -An -N"$1" -tx1 /dev/urandom | tr -d ' \n'; }
b64() { head -c "$1" /dev/urandom | base64 | tr -d '\n'; }

fill() {
	key=$1 value=$2
	if grep -q "^$key=$" .env; then
		# The value is hex or base64: no character sed would read specially
		# except "/", hence the "|" delimiter.
		sed -i.bak "s|^$key=$|$key=$value|" .env && rm -f .env.bak
		echo "generated $key"
	fi
}

fill AUTHWAY_DATABASE_PASSWORD "$(hex 24)"
fill AUTHWAY_HYDRA_SECRETS_SYSTEM "$(hex 32)"
fill AUTHWAY_HYDRA_TOKEN_HOOK_KEY "$(hex 32)"
fill AUTHWAY_ADMIN_API_KEY "$(hex 32)"
fill AUTHWAY_ADMIN_PASSWORD "$(hex 16)"
fill AUTHWAY_TOTP_ENCRYPTION_KEY "$(b64 32)"
fill AUTHWAY_JWT_ACCESS_TOKEN_SECRET "$(hex 32)"
fill AUTHWAY_JWT_REFRESH_TOKEN_SECRET "$(hex 32)"

chmod 600 .env
mkdir -p certs
echo "next: set AUTHWAY_DOMAIN and the mail settings in .env, then: docker compose up -d"
