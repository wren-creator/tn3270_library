# Publishing Guide

This library publishes to two registries simultaneously on every version tag:
- **npmjs.org** — public, installable with `npm install node-tn3270e`
- **GitHub Packages** — installable as `@YOUR_USERNAME/node-tn3270e`

---

## One-time Setup

### 1. npm Token

1. Log in at [npmjs.com](https://www.npmjs.com) and go to **Access Tokens**
2. Create a new **Automation** token
3. In your GitHub repo → **Settings → Secrets and variables → Actions**
4. Create a secret named `NPM_TOKEN` with that value

### 2. Update placeholders in CI workflow

Edit `.github/workflows/ci.yml` — replace both instances of `YOUR_GITHUB_USERNAME`
with your actual GitHub username or org name.

### 3. Update package.json

Replace `YOUR_USERNAME` in the `repository.url` and `bugs.url` fields.

---

## Releasing a New Version

```bash
# 1. Update the version in package.json
npm version patch   # 1.0.0 → 1.0.1  (bug fixes)
npm version minor   # 1.0.0 → 1.1.0  (new features, backward compatible)
npm version major   # 1.0.0 → 2.0.0  (breaking changes)

# 2. Push the commit and the generated tag
git push origin main --tags
```

The GitHub Actions workflow (`ci.yml`) automatically:
1. Runs tests on Node 18, 20, and 22
2. On success, publishes to npm
3. On success, publishes to GitHub Packages

---

## Manual Publishing (if needed)

```bash
# npm
npm publish --access public

# GitHub Packages (requires scoped name)
npm publish --registry=https://npm.pkg.github.com
```

---

## Versioning Policy

This library follows [Semantic Versioning](https://semver.org/):

- **Patch** — bug fixes, documentation improvements, additional constants
- **Minor** — new API methods, new code pages, new examples
- **Major** — breaking changes to the public API (method signatures, event payloads, exports)

The protocol implementation itself (Telnet negotiation, 3270 datastream parsing) is
considered stable — changes there that affect observable behavior are **minor** at minimum,
**major** if they change what consumers receive in events.
