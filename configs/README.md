# Authway Configuration Files

Configuration templates pour différents environnements.

## Fichiers

### `config.production.yaml`
**Configuration de production de base**

Fichier de configuration simplifié pour le déploiement en production. Copier ce fichier vers `config.yaml` et remplir les valeurs de production.

**Structure:**
- App configuration (nom, version, port, URL de base)
- Database (PostgreSQL)
- Redis (cache et sessions)
- JWT (tokens d'accès et de rafraîchissement)
- OAuth 2.0
- Hydra (serveur OAuth externe)
- CORS
- Email (SMTP)
- Google OAuth

**Usage:**
```bash
cp configs/config.production.yaml config.yaml
# Editer config.yaml avec vos valeurs de production
```

### `production.advanced.yaml.example`
**Configuration avancée (référence future)**

Exemple de configuration complète avec fonctionnalités avancées. Ce fichier sert de référence pour les fonctionnalités qui seront implémentées dans le futur.

**Fonctionnalités incluses:**
- Rate limiting
- Monitoring et métriques (Prometheus)
- Tracing (Jaeger)
- Audit logging
- Account lockout
- Backups automatiques
- Cache configuration avancée
- Password policies
- Session security

**Note:** Cette configuration n'est pas encore supportée par le code actuel. Elle sert de roadmap pour les fonctionnalités futures.

### Configuration Hydra

#### `hydra.yml`
Configuration pour le serveur Ory Hydra OAuth2.

### Configuration Monitoring

#### `prometheus.yml`
Configuration Prometheus pour la collecte de métriques.

#### `alertmanager.yml`
Configuration AlertManager pour les notifications.

#### `alerting_rules.yml`
Règles d'alerte Prometheus.

## Variables d'Environnement

Les fichiers de configuration utilisent des variables d'environnement pour les valeurs sensibles:

- `${POSTGRES_PASSWORD}` - Mot de passe PostgreSQL
- `${REDIS_PASSWORD}` - Mot de passe Redis (optionnel)
- `${JWT_ACCESS_SECRET}` - Secret JWT pour les tokens d'accès
- `${JWT_REFRESH_SECRET}` - Secret JWT pour les tokens de rafraîchissement
- `${SMTP_HOST}`, `${SMTP_USER}`, `${SMTP_PASSWORD}` - Configuration SMTP
- `${GOOGLE_CLIENT_ID}`, `${GOOGLE_CLIENT_SECRET}` - Google OAuth

## Ordre de Priorité

Le serveur charge la configuration dans cet ordre (du plus haut au plus bas):

1. Variables d'environnement (`AUTHWAY_*`)
2. Fichier de configuration (si spécifié avec `--config`)
3. Valeurs par défaut (définies dans `config.go`)

## Sécurité

⚠️ **IMPORTANT:**
- Ne jamais committer les fichiers de configuration avec des valeurs réelles
- Toujours utiliser des variables d'environnement pour les secrets
- Activer SSL/TLS en production (`ssl_mode: require`)
- Utiliser des secrets forts (minimum 32 caractères aléatoires)
- Régulièrement faire tourner les secrets JWT

## Exemples

### Développement

Utiliser les variables d'environnement du fichier `.env`:

```bash
# Aucune configuration YAML nécessaire
# Les valeurs par défaut + .env sont suffisantes
go run cmd/main.go
```

### Production

Utiliser un fichier de configuration YAML:

```bash
# Copier et éditer la configuration
cp configs/config.production.yaml config.yaml

# Démarrer avec la configuration
go run cmd/main.go --config config.yaml
```

Ou utiliser uniquement les variables d'environnement:

```bash
export AUTHWAY_APP_ENVIRONMENT=production
export AUTHWAY_DATABASE_HOST=prod-db.example.com
export AUTHWAY_DATABASE_PASSWORD=secret
# ... autres variables

go run cmd/main.go
```
