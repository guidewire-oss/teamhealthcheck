# Repository rename migration runbook

The GitHub repository has moved:

- Old repository: `guidewire-oss/teams360`
- New repository: `guidewire-oss/teamhealthcheck`
- New repository URL: <https://github.com/guidewire-oss/teamhealthcheck>
- Documentation URL: <https://guidewire-oss.github.io/teamhealthcheck/>

The documentation URL will become available after the GitHub Pages deployment
finishes.

GitHub redirects requests from the old repository URL, but local clones and
external integrations should use the new name directly. This migration does
not rename application internals such as databases, containers, Kubernetes
resources, local hostnames, binaries, or environment variables.

## Local clones

Check the name of the remote that points to the Guidewire repository:

```bash
git remote -v
```

Most clones call this remote `origin`. Update it over SSH:

```bash
git remote set-url origin git@github.com:guidewire-oss/teamhealthcheck.git
git remote -v
git fetch origin --prune
```

If the clone uses HTTPS, run:

```bash
git remote set-url origin https://github.com/guidewire-oss/teamhealthcheck.git
git remote -v
git fetch origin --prune
```

If the Guidewire repository has a different remote name, replace `origin` in
these commands with that name. Updating a remote does not change local
branches, commits, stashes, or uncommitted work.

Verify the remote resolves to the renamed repository:

```bash
git remote get-url origin
git ls-remote origin HEAD
```

## Forks

Existing forks keep their current repository name. GitHub updates their parent
relationship to `guidewire-oss/teamhealthcheck`, but does not rename the fork.

In a typical fork-based clone, `origin` points to the personal fork and
`upstream` points to Guidewire. Keep the personal fork URL unchanged and update
only `upstream`:

```bash
git remote -v
git remote set-url upstream git@github.com:guidewire-oss/teamhealthcheck.git
git fetch upstream --prune
```

For HTTPS:

```bash
git remote set-url upstream https://github.com/guidewire-oss/teamhealthcheck.git
git fetch upstream --prune
```

Fork owners may rename their own fork separately. If they do, they must also
update the corresponding local remote.

## New clones

Use one of the new clone URLs:

```bash
git clone git@github.com:guidewire-oss/teamhealthcheck.git
```

```bash
git clone https://github.com/guidewire-oss/teamhealthcheck.git
```

The local directory will be named `teamhealthcheck`. Existing local directories
can remain named `teams360`; the directory name does not affect Git.

## Automation and integrations

Owners of automation that refers to the repository should check for the exact
string `guidewire-oss/teams360` and replace it with
`guidewire-oss/teamhealthcheck`. Review at least:

- CI/CD pipelines outside GitHub Actions
- GitHub App installations and repository allowlists
- Webhooks and deployment systems
- Code quality, coverage, and security scanners
- Scripts that call the GitHub API or `gh --repo`
- Dependency references, badges, bookmarks, and documentation links
- Repository-specific secrets or variables stored in external systems

GitHub redirects many web and Git operations from the old name. Do not rely on
that redirect for long-running automation, API clients, allowlists, or signed
webhook configuration.

Container image names and package names have not changed as part of this
migration. Do not rename them unless a separate migration says to do so.

## Verification checklist

After updating a clone or integration, confirm:

1. `git fetch` succeeds without a redirect warning or authentication error.
2. `git remote -v` contains `guidewire-oss/teamhealthcheck` for the upstream
   repository.
3. Existing local branches and uncommitted changes are still present.
4. Pull requests target `guidewire-oss/teamhealthcheck`.
5. Automated jobs can read or write the renamed repository as intended.
6. Repository links use <https://github.com/guidewire-oss/teamhealthcheck>.
7. Documentation links use
   <https://guidewire-oss.github.io/teamhealthcheck/>.

## Troubleshooting

### `remote origin already exists`

Do not add another `origin`. Update the existing remote with
`git remote set-url` as shown above.

### `No such remote 'upstream'`

Find the correct remote name with `git remote -v`. If the clone does not yet
have an upstream remote, add it:

```bash
git remote add upstream git@github.com:guidewire-oss/teamhealthcheck.git
git fetch upstream --prune
```

### Authentication or permission errors

The rename does not grant new access. Confirm that the SSH key, personal access
token, GitHub App, or service account has access to
`guidewire-oss/teamhealthcheck`. Organization SSO authorization may also need
to be renewed.

### An integration still uses the old name

Update the integration to the new repository identifier rather than depending
on GitHub's redirect. If the integration is managed by another team, send them
the old and new repository names listed at the top of this runbook.

### Return to the previous remote temporarily

If the new URL was entered incorrectly, restore the old URL while diagnosing
the problem:

```bash
git remote set-url origin git@github.com:guidewire-oss/teams360.git
```

GitHub currently redirects that URL to the renamed repository. This is a
temporary fallback, not the completed migration state.
