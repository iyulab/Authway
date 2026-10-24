#!/bin/sh
# Assembles an installation bundle in <out-dir>: the compose file, the
# authorization server settings every deployment shares, the settings template
# and setup script, and — unless --no-images — every image saved to one
# images.tar, so a host without registry access needs only `docker load`.
#
#   sh deploy/onprem/pack.sh <out-dir> [--no-images]
#
# Run from anywhere in the repository; images are built from the working tree.
set -eu

out=${1:?usage: pack.sh <out-dir> [--no-images]}
images=yes
[ "${2:-}" = "--no-images" ] && images=no

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
mkdir -p "$out"

cp "$here/docker-compose.yml" "$here/.env.example" "$here/setup.sh" "$here/README.md" "$out/"
# One source for the shared settings: the file every other deployment reads.
cp "$root/configs/hydra.shared.env" "$out/hydra.shared.env"

if [ "$images" = yes ]; then
	version=$(git -C "$root" describe --tags --match 'v[0-9]*' --always --dirty 2>/dev/null || date +%Y%m%d)
	docker build -t authway-api:latest --build-arg APP_VERSION="$version" -f "$root/Dockerfile" "$root"
	docker build -t authway-web:latest -f "$here/web.Dockerfile" "$root"
	set -- authway-api:latest authway-web:latest
	# The third-party images the compose file names, at the versions it names.
	for ref in $(sed -n 's/.*image: \${[A-Z_]*:-\([^}]*\)}.*/\1/p' "$here/docker-compose.yml" | sort -u); do
		case $ref in authway-*) ;; *) docker pull "$ref" && set -- "$@" "$ref" ;; esac
	done
	docker save -o "$out/images.tar" "$@"
	echo "saved: $*"
fi
echo "bundle: $out"
