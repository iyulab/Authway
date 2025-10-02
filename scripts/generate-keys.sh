#!/bin/bash

# Generate JWT Keys Script for Authway
# This script generates RSA key pairs for JWT signing

set -e

KEYS_DIR="./keys"
PRIVATE_KEY="$KEYS_DIR/jwt_private.pem"
PUBLIC_KEY="$KEYS_DIR/jwt_public.pem"

echo "🔑 Generating JWT keys for Authway..."

# Create keys directory if it doesn't exist
mkdir -p "$KEYS_DIR"

# Generate private key (4096-bit RSA)
echo "📄 Generating private key..."
openssl genrsa -out "$PRIVATE_KEY" 4096

# Generate public key from private key
echo "🔓 Generating public key..."
openssl rsa -in "$PRIVATE_KEY" -pubout -out "$PUBLIC_KEY"

# Set proper permissions (readable only by owner)
chmod 600 "$PRIVATE_KEY"
chmod 644 "$PUBLIC_KEY"

echo "✅ JWT keys generated successfully!"
echo "   Private key: $PRIVATE_KEY"
echo "   Public key:  $PUBLIC_KEY"
echo ""
echo "⚠️  IMPORTANT SECURITY NOTES:"
echo "   - Keep the private key secure and never commit it to version control"
echo "   - Add keys/ to .gitignore"
echo "   - In production, store keys in a secure key management system"
echo "   - Rotate keys periodically for security"

# Create .gitignore entry if it doesn't exist
if [ ! -f .gitignore ]; then
    echo "keys/" > .gitignore
    echo "📄 Created .gitignore with keys/ entry"
elif ! grep -q "^keys/" .gitignore; then
    echo "keys/" >> .gitignore
    echo "📄 Added keys/ to existing .gitignore"
fi

echo ""
echo "🚀 Keys are ready for use in production configuration!"