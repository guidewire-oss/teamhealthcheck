# Security Policy

## Supported Versions

We release patches for security vulnerabilities for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

If you discover a security vulnerability, please report it by emailing the maintainers directly. You can find the maintainer contact information in the repository.

Please include the following information:
- Type of vulnerability (e.g., buffer overflow, SQL injection, XSS)
- Full paths of source file(s) related to the issue
- Location of the affected source code (tag/branch/commit or direct URL)
- Step-by-step instructions to reproduce the issue
- Proof-of-concept or exploit code (if possible)
- Impact of the issue and how an attacker might exploit it

We will acknowledge receipt of your vulnerability report within 48 hours and will send you regular updates about our progress.

## Security Measures

### Container Security

- All Docker images are built from minimal base images (`scratch` for Go, `node:alpine` for frontend)
- Images run as non-root users
- Images are signed with Sigstore/Cosign
- SBOM (Software Bill of Materials) is generated for each image
- Images are scanned with Trivy for vulnerabilities

### Code Security

- CodeQL analysis (via GitHub's default setup) runs on every PR and push to main
- Dependency review blocks PRs with high/critical vulnerabilities
- TruffleHog scans for leaked secrets
- Dependabot keeps dependencies up to date

### Supply Chain Security

- All GitHub Actions use pinned versions
- Docker images use content-addressable digests where possible
- SLSA provenance attestations are generated for builds

### Verification

You can verify image signatures using Cosign:

```bash
cosign verify ghcr.io/OWNER/teams360-api:TAG \
  --certificate-identity-regexp="https://github.com/OWNER/teams360" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com"
```

## Security Best Practices for Contributors

1. **Never commit secrets** - Use environment variables or secret management
2. **Validate input** - Always validate and sanitize user input
3. **Use parameterized queries** - Prevent SQL injection
4. **Escape output** - Prevent XSS attacks
5. **Keep dependencies updated** - Review and merge Dependabot PRs promptly
6. **Follow least privilege** - Request minimal permissions in code and CI/CD
