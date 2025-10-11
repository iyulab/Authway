# 🔐 Authway

**Version**: 0.1.0
**Status**: Production Ready

Authway est un serveur d'authentification OAuth 2.0 / OpenID Connect moderne avec support multi-tenant, conçu pour être simple à déployer et à maintenir.

---

## ✨ Fonctionnalités

### 🎯 Core
- **Multi-Tenancy** - Isolation complète des tenants avec gestion centralisée
- **OAuth 2.0 / OpenID Connect** - Protocoles standards d'authentification
- **JWT Tokens** - Access et refresh tokens sécurisés
- **Email Verification** - Vérification d'email avec MailHog en développement
- **Password Reset** - Flux de réinitialisation de mot de passe sécurisé

### 🔌 Intégrations Sociales
- **Google OAuth** - Sign-in avec Google
- **GitHub OAuth** (à venir)

### 🛡️ Sécurité
- **Admin Console** - Interface d'administration sécurisée
- **Session Management** - Gestion de sessions avec Redis
- **CORS** - Configuration CORS sécurisée
- **SSL/TLS** - Support SSL pour PostgreSQL en production

### 📊 Monitoring (à venir)
- **Prometheus** - Métriques système
- **Jaeger** - Distributed tracing
- **AlertManager** - Gestion des alertes

---

## 🚀 Démarrage Rapide

### Prérequis

- **Docker Desktop** (recommandé)
- **Go 1.21+** (pour développement local)
- **Node.js 18+** (pour les UIs)
- **PostgreSQL 15+** (si non-Docker)

### Installation en 1 Minute

```bash
# Cloner le repository
git clone https://github.com/yourusername/authway.git
cd authway

# Démarrer tous les services
.\start-dev.ps1  # Windows
./start-dev.sh   # Linux/Mac
```

**C'est tout!** 🎉

### Accès aux Services

| Service | URL | Description |
|---------|-----|-------------|
| 🎨 **Admin Dashboard** | http://localhost:3000 | Console d'administration |
| 🔑 **Login UI** | http://localhost:3001 | Interface de connexion |
| 🔧 **Backend API** | http://localhost:8080 | API REST |
| 📧 **MailHog** | http://localhost:8025 | Interface email (dev) |

**Mot de passe admin par défaut**: `authway0`

---

## 📚 Documentation

### Guides de Démarrage

- **[START-HERE.md](START-HERE.md)** - Guide de démarrage rapide
- **[DOCKER-GUIDE.md](DOCKER-GUIDE.md)** - Guide Docker complet
- **[CONFIGURATION.md](CONFIGURATION.md)** - Configuration détaillée

### Guides Techniques

- **[TESTING-GUIDE.md](TESTING-GUIDE.md)** - Tests et validation
- **[docs/architecture/multi-tenancy.md](docs/architecture/multi-tenancy.md)** - Architecture multi-tenant
- **[docs/PRODUCTION_SECURITY.md](docs/PRODUCTION_SECURITY.md)** - Sécurité en production

### Documentation API

- **[docs/api/api-verification-report.md](docs/api/api-verification-report.md)** - Rapport de vérification API

---

## 🏗️ Architecture

### Stack Technique

**Backend**:
- Go 1.21+ avec Fiber framework
- PostgreSQL 15 (base de données principale)
- Redis 7 (sessions et cache)
- Ory Hydra (serveur OAuth optionnel)

**Frontend**:
- React 18 avec TypeScript
- Vite (build tool)
- TailwindCSS (styling)
- Zustand (state management)

**Infrastructure**:
- Docker & Docker Compose
- Prometheus & Grafana (monitoring)
- MailHog (development email)

### Structure du Projet

```
authway/
├── src/server/              # Backend Go
│   ├── cmd/main.go         # Point d'entrée
│   ├── internal/           # Code interne
│   └── pkg/                # Packages publics
│       ├── admin/          # Admin console
│       ├── client/         # OAuth clients
│       ├── tenant/         # Multi-tenancy
│       └── user/           # User management
│
├── packages/web/
│   ├── admin-dashboard/    # React admin UI
│   └── login-ui/           # React login UI
│
├── scripts/                # Utilitaires
│   ├── migrations/         # DB migrations
│   └── update-version.*    # Version management
│
├── configs/                # Fichiers de configuration
├── docs/                   # Documentation technique
└── claudedocs/             # Documentation de conception
```

---

## 🔧 Configuration

### Variables d'Environnement Essentielles

```bash
# Application
AUTHWAY_APP_VERSION=0.1.0
AUTHWAY_APP_ENVIRONMENT=development  # development|production
AUTHWAY_APP_PORT=8080

# Database
AUTHWAY_DATABASE_HOST=localhost
AUTHWAY_DATABASE_PASSWORD=authway  # CHANGER EN PRODUCTION!

# Admin
AUTHWAY_ADMIN_PASSWORD=authway0    # CHANGER EN PRODUCTION!

# JWT (générer avec: openssl rand -base64 64)
AUTHWAY_JWT_ACCESS_TOKEN_SECRET=your-secret-key
AUTHWAY_JWT_REFRESH_TOKEN_SECRET=your-refresh-secret-key
```

**⚠️ Sécurité**: Voir [CONFIGURATION.md](CONFIGURATION.md) pour la configuration complète et les bonnes pratiques de sécurité.

---

## 🧪 Tests

```bash
# Backend tests
cd src/server
go test ./...

# Frontend tests (Admin Dashboard)
cd packages/web/admin-dashboard
npm test

# Frontend tests (Login UI)
cd packages/web/login-ui
npm test

# Integration tests
cd scripts
go run test_integration.go
```

Voir [TESTING-GUIDE.md](TESTING-GUIDE.md) pour plus de détails.

---

## 🚢 Déploiement Production

### Checklist Sécurité

- [ ] Changer **TOUS** les mots de passe et secrets par défaut
- [ ] Générer des secrets JWT uniques (64+ caractères)
- [ ] Activer SSL pour PostgreSQL (`ssl_mode=require`)
- [ ] Utiliser HTTPS pour toutes les URLs
- [ ] Configurer CORS pour les domaines de production uniquement
- [ ] Configurer un service SMTP de production
- [ ] Activer les backups automatiques de la base de données

### Déploiement Docker

```bash
# 1. Copier et configurer .env.production
cp .env.production.example .env.production
nano .env.production  # Éditer avec vos valeurs

# 2. Démarrer en production
docker-compose -f docker-compose.prod.yml up -d
```

Voir [DOCKER-GUIDE.md](DOCKER-GUIDE.md) et [docs/PRODUCTION_SECURITY.md](docs/PRODUCTION_SECURITY.md) pour le guide complet.

---

## 🛠️ Développement

### Setup Développement

```bash
# 1. Installer les dépendances
cd src/server && go mod download
cd packages/web/admin-dashboard && npm install
cd packages/web/login-ui && npm install

# 2. Démarrer l'infrastructure
docker-compose -f docker-compose.dev.yml up -d postgres redis mailhog

# 3. Démarrer le backend
cd src/server && go run cmd/main.go

# 4. Démarrer le frontend
cd packages/web/admin-dashboard && npm run dev
```

### Conventions de Code

- **Go**: `gofmt`, `go vet`, `golint`
- **TypeScript**: ESLint, Prettier
- **Commits**: Conventional Commits (`feat:`, `fix:`, `docs:`, etc.)

### Gestion des Versions

```bash
# Mettre à jour la version du projet
.\scripts\update-version.ps1 -Version "0.2.0"
```

---

## 📦 Multi-Tenancy

Authway supporte deux modes d'opération:

### Mode Multi-Tenant (Par défaut)

Plusieurs tenants isolés dans une seule instance:

```bash
AUTHWAY_TENANT_SINGLE_TENANT_MODE=false
```

- Chaque tenant a ses propres utilisateurs et clients OAuth
- Isolation complète au niveau de la base de données
- API d'administration pour gérer les tenants

### Mode Single-Tenant

Un seul tenant dédié:

```bash
AUTHWAY_TENANT_SINGLE_TENANT_MODE=true
AUTHWAY_TENANT_TENANT_NAME="My Company"
AUTHWAY_TENANT_TENANT_SLUG="my-company"
```

- Simplicité de configuration
- Pas de surcharge multi-tenant
- Idéal pour les déploiements dédiés

Voir [docs/architecture/multi-tenancy.md](docs/architecture/multi-tenancy.md) pour plus de détails.

---

## 🤝 Contributing

Les contributions sont les bienvenues!

1. Fork le projet
2. Créer une branche feature (`git checkout -b feature/amazing-feature`)
3. Commit les changements (`git commit -m 'feat: add amazing feature'`)
4. Push la branche (`git push origin feature/amazing-feature`)
5. Ouvrir une Pull Request

---

## 📄 License

MIT License - voir [LICENSE](LICENSE) pour plus de détails.

---

## 🙏 Remerciements

- [Ory Hydra](https://www.ory.sh/hydra/) - OAuth 2.0 Server
- [Fiber](https://gofiber.io/) - Go Web Framework
- [React](https://react.dev/) - UI Library
- [PostgreSQL](https://www.postgresql.org/) - Database
- [Redis](https://redis.io/) - Caching & Sessions

---

## 📞 Support

- 📖 **Documentation**: Voir les fichiers dans `docs/`
- 🐛 **Bug Reports**: [GitHub Issues](https://github.com/yourusername/authway/issues)
- 💬 **Discussions**: [GitHub Discussions](https://github.com/yourusername/authway/discussions)

---

**Fait avec ❤️ par l'équipe Authway**
