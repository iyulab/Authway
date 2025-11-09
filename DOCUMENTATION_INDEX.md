# 📚 Authway Documentation Index

**Version**: 0.1.1
**Last Updated**: 2025-10-29
**Status**: Production Ready

---

## 🎯 Start Here

**All documentation has been consolidated and updated.**

### Main Documentation Hub

**📖 [Complete Documentation Index →](docs/README.md)**

The comprehensive documentation is located in `/docs/README.md` with:
- Learning paths for all skill levels
- Complete API reference
- SDK guides for @authway/client and @authway/react
- Language-specific quickstart guides
- Sample applications and code examples

---

## 🚀 Quick Links

| Resource | Description |
|----------|-------------|
| **[Project README](README.md)** | Project overview and quick start |
| **[Documentation Hub](docs/README.md)** | Complete documentation index |
| **[API Reference](docs/API_INTRODUCTION.md)** | Full REST API documentation |
| **[SDK Guide](docs/sdk/README.md)** | @authway/client & @authway/react |
| **[Integration Guide](docs/INTEGRATION_GUIDE.md)** | OAuth 2.0 integration |
| **[Samples](samples/)** | Working example applications |

---

## 📦 By Role

### 👨‍💻 Developers

**Getting Started**:
1. [Documentation Hub](docs/README.md) - Complete guide
2. [SDK Documentation](docs/sdk/README.md) - Client libraries
3. [JavaScript Quickstart](docs/quickstart/JAVASCRIPT_QUICKSTART.md) - 5-minute integration
4. [ASP-SPA Sample](samples/asp-spa/) - React + Backend example

**Key Features**:
- [Dynamic Claims](docs/features/DYNAMIC_CLAIMS.md) - Workspace switching
- [Multi-Tenancy](docs/architecture/multi-tenancy.md) - Tenant isolation
- [API Reference](docs/API_INTRODUCTION.md) - Complete endpoint docs

---

### 🏗️ System Administrators

**Deployment**:
1. [Azure Architecture](docs/deployment/azure-architecture.md) - Production setup
2. [Azure CI/CD](docs/deployment/azure-cicd.md) - Automated deployment
3. [Application Insights](docs/monitoring/application-insights.md) - Monitoring

**Configuration**:
- [Local Setup](docs/development/local-setup.md) - Development environment
- Environment variables and configuration options

---

### 📚 Technical Writers

**Documentation Structure**:
- Main docs: `/docs/`
- SDK docs: `/docs/sdk/`
- Samples: `/samples/`
- Package READMEs: `/packages/*/README.md`

**Update Process**: See [docs/README.md](docs/README.md#documentation-updates)

---

## 🔍 Search by Topic

### Authentication & OAuth
- [Integration Guide](docs/INTEGRATION_GUIDE.md)
- [API Introduction](docs/API_INTRODUCTION.md)
- [Quickstart Guides](docs/quickstart/)

### SDKs & Client Libraries
- [SDK Overview](docs/sdk/README.md)
- [@authway/client](packages/client/README.md)
- [@authway/react](packages/react/README.md)

### Features
- [Dynamic Claims](docs/features/DYNAMIC_CLAIMS.md)
- [Auto Migration](docs/features/auto-migration.md)
- [Claims Testing](docs/features/claims-testing-guide.md)

### Deployment & Operations
- [Azure Deployment](docs/deployment/azure-architecture.md)
- [CI/CD Pipeline](docs/deployment/azure-cicd.md)
- [Monitoring](docs/monitoring/application-insights.md)

---

## 📁 Documentation Structure

```
authway/
├── README.md                           ⭐ Project overview
├── DOCUMENTATION_INDEX.md              ⭐ You are here
├── CHANGELOG.md                        📝 Version history
│
├── docs/                               📚 Main documentation
│   ├── README.md                       📖 Complete documentation hub
│   ├── API_INTRODUCTION.md             🔌 API reference
│   ├── INTEGRATION_GUIDE.md            🎯 OAuth guide
│   │
│   ├── quickstart/                     🚀 5-minute guides
│   │   ├── JAVASCRIPT_QUICKSTART.md
│   │   ├── PYTHON_QUICKSTART.md
│   │   ├── DOTNET_QUICKSTART.md
│   │   └── GO_QUICKSTART.md
│   │
│   ├── sdk/                            📦 SDK documentation
│   │   └── README.md                   Complete SDK guide
│   │
│   ├── features/                       🔥 Feature guides
│   │   └── DYNAMIC_CLAIMS.md           Workspace switching
│   │
│   ├── architecture/                   🏗️ System design
│   ├── deployment/                     🚀 Production setup
│   ├── development/                    💻 Dev environment
│   └── monitoring/                     📊 Observability
│
├── samples/                            💡 Example applications
│   ├── asp-spa/                        ASP.NET + React
│   └── react-sdk-sample/               React SDK demo
│
└── packages/                           📦 Source code & SDKs
    ├── client/                         @authway/client
    └── react/                          @authway/react
```

---

## 🆕 Recent Updates (2025-10-29)

### Documentation Consolidation

**✅ Completed**:
- Unified all documentation in `/docs/` directory
- Created comprehensive [docs/README.md](docs/README.md) index
- Consolidated SDK documentation in [docs/sdk/README.md](docs/sdk/README.md)
- Removed duplicate files and outdated content
- Replaced admin-dashboard docs with reference file

**Removed Duplicates**:
- ❌ `docs/quick-start.md` (Korean) → Use language-specific quickstarts
- ❌ `docs/features/dynamic-claims.md` (Korean) → Kept English version
- ❌ `packages/web/admin-dashboard/public/docs/*` → Replaced with reference

**New Documentation**:
- ✅ [docs/README.md](docs/README.md) - Comprehensive documentation hub
- ✅ [docs/sdk/README.md](docs/sdk/README.md) - Complete SDK guide
- ✅ Updated sample documentation with migration guide

---

## 💡 Documentation Best Practices

### For Contributors

- ✅ Update [docs/README.md](docs/README.md) when adding features
- ✅ Include working code examples in all guides
- ✅ Test all commands and code snippets
- ✅ Use consistent terminology across docs
- ✅ Date technical documentation

### For Users

- 📖 Start with [docs/README.md](docs/README.md)
- 🎯 Follow learning path for your level
- 💡 Check [samples/](samples/) for working examples
- ❓ Open [GitHub issues](https://github.com/iyulab/authway/issues) for problems

---

## 📞 Getting Help

- 📖 **Missing Documentation?** [Open an issue](https://github.com/iyulab/authway/issues)
- 🐛 **Error in Docs?** [Submit a PR](https://github.com/iyulab/authway/pulls)
- 💬 **Questions?** [GitHub Discussions](https://github.com/iyulab/authway/discussions)
- 📧 **Email**: support@iyulab.com

---

*For complete documentation, see [docs/README.md](docs/README.md)*
