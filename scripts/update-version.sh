#!/bin/bash
# ============================================================
# Authway Version Update Script
# ============================================================
# Updates version across all project files
# Usage: ./scripts/update-version.sh 0.1.0
# ============================================================

set -e

VERSION=$1

# Check if version provided
if [ -z "$VERSION" ]; then
    echo "❌ Error: Version required"
    echo "Usage: $0 <version>"
    echo "Example: $0 0.1.0"
    exit 1
fi

# Validate version format (semver)
if ! [[ "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "❌ Invalid version format. Use semantic versioning (e.g., 0.1.0)"
    exit 1
fi

echo "🔄 Updating Authway version to $VERSION..."
echo ""

updated_files=0

# ============================================================
# Function: Update file with sed
# ============================================================
update_file() {
    local file=$1
    local pattern=$2
    local replacement=$3
    local description=$4

    if [ -f "$file" ]; then
        echo "  📝 Updating: $file ($description)"
        sed -i "s|$pattern|$replacement|g" "$file"
        ((updated_files++))
    else
        echo "  ⚠️  Not found: $file"
    fi
}

# ============================================================
# Update Package Files
# ============================================================
echo "📦 Updating package.json files..."
update_file "packages/web/admin-dashboard/package.json" '"version": "[0-9.]*"' "\"version\": \"$VERSION\"" "admin dashboard"
update_file "packages/web/login-ui/package.json" '"version": "[0-9.]*"' "\"version\": \"$VERSION\"" "login UI"

# ============================================================
# Update Environment Files
# ============================================================
echo ""
echo "⚙️  Updating environment files..."
update_file ".env" 'AUTHWAY_APP_VERSION=[0-9.]*' "AUTHWAY_APP_VERSION=$VERSION" ".env"
update_file ".env.example" 'AUTHWAY_APP_VERSION=[0-9.]*' "AUTHWAY_APP_VERSION=$VERSION" ".env.example"
update_file ".env.production.example" 'AUTHWAY_APP_VERSION=[0-9.]*' "AUTHWAY_APP_VERSION=$VERSION" ".env.production.example"

# ============================================================
# Update Docker Compose Files
# ============================================================
echo ""
echo "🐳 Updating Docker Compose files..."
update_file "docker-compose.dev.yml" 'AUTHWAY_APP_VERSION: [0-9.]*' "AUTHWAY_APP_VERSION: $VERSION" "dev"
update_file "docker-compose.prod.yml" 'AUTHWAY_APP_VERSION: [0-9.]*' "AUTHWAY_APP_VERSION: $VERSION" "prod"

# ============================================================
# Update Go Config
# ============================================================
echo ""
echo "🔧 Updating Go configuration..."
update_file "src/server/internal/config/config.go" 'viper.SetDefault("app.version", "[0-9.]*")' "viper.SetDefault(\"app.version\", \"$VERSION\")" "config.go"

# ============================================================
# Update Config YAML files
# ============================================================
echo ""
echo "📄 Updating YAML config files..."
update_file "configs/config.production.yaml" 'version: [0-9.]*' "version: $VERSION" "production config"
update_file "configs/production.yaml" 'version: [0-9.]*' "version: $VERSION" "production config"

# ============================================================
# Summary
# ============================================================
echo ""
echo "✅ Version update complete!"
echo "   Updated $updated_files files to version $VERSION"
echo ""
echo "📋 Next steps:"
echo "   1. Review changes: git diff"
echo "   2. Update package-lock.json files: npm install in each package"
echo "   3. Commit changes: git add . && git commit -m 'chore: bump version to $VERSION'"
echo "   4. Tag release: git tag v$VERSION && git push --tags"
echo ""
