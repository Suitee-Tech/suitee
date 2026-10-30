# Git flow and branch protection

This repository uses a lightweight Git Flow model with protected integration branches. The goal is to keep `main` releasable, keep changes reviewable and prevent unreviewed code from reaching production.

## Branches

| Branch | Purpose | Who writes |
|---|---|---|
| `main` | Released, deployable code. Tags are created here. | Only through a pull request from `release/*` or `hotfix/*`. |
| `develop` | Integration branch for the next release. | Only through a pull request. |
| `feature/<short-name>` | New capability or refactor. | Branched from `develop`; PR back to `develop`. |
| `fix/<short-name>` | Non-urgent bug fix. | Branched from `develop`; PR back to `develop`. |
| `release/<version>` | Stabilization and release preparation. | Branched from `develop`; PR to `main`, then merged back to `develop`. |
| `hotfix/<version>` | Urgent production correction. | Branched from `main`; PR to `main`, then merged back to `develop`. |

Keep branches short-lived. Rebase or update your branch before requesting review when `develop` moves.

## Pull requests

Every change to `main` and `develop` goes through a pull request. Direct pushes, force pushes, branch deletion and merge commits are blocked by branch protection.

A pull request is mergeable only when:

- at least **1 approval** is recorded;
- the approval is from a **CODEOWNER**;
- the last push has been reviewed (`require_last_push_approval`);
- all required CI checks are green;
- the branch is up to date with the target branch;
- every conversation is resolved;
- the history is linear.

Use **Squash and merge** for feature/fix PRs. Use **Rebase and merge** when preserving individual commits materially helps review. Merge commits are disabled by the linear-history rule.

## Recommended commit style

Use [Conventional Commits](https://www.conventionalcommits.org/):

```text
feat(site): permite marca própria no painel
fix(auth): expira links de recuperação
docs: documenta o fluxo de release
test: cobre reenvio de contrato
chore(ci): atualiza ações fixadas
```

Keep commits focused. Do not mix formatting-only changes with behavior changes.

## Release flow

```text
develop ──► release/1.4.0 ──► main ──► tag v1.4.0
   ▲                              │
   └──────── merge back ──────────┘
```

1. Create `release/<version>` from `develop` when the scope is frozen.
2. Stabilize it through review and CI; only fixes for the release are accepted.
3. Open a PR to `main`; after merge, tag the merge commit with the semantic version.
4. Merge `main` back into `develop` so the release commit is not lost.
5. Delete the release branch.

## Hotfix flow

1. Create `hotfix/<version>` from `main`.
2. Open a PR to `main` with the incident context and rollback/verification notes.
3. Tag the resulting commit.
4. Merge `main` back into `develop`.

## Security rules

- Never commit `.env`, credentials, tokens, private keys, customer data or production dumps.
- Use the `.env.example` files with placeholders.
- Keep `main` and `develop` protected; all changes should use pull requests.
- Security-sensitive paths require CODEOWNER review: gateway, authentication, migrations and security policy.
- Secret scanning and push protection are enabled. If a secret is ever exposed, rotate it first, then treat the repository history as compromised.
- Report vulnerabilities privately according to [SECURITY.md](../../SECURITY.md), never in a public issue.
- Do not add a dependency without checking its license, maintenance status and transitive footprint.

## Merge checklist

- [ ] The PR has one clear problem and a test proving the change.
- [ ] CI is green on the latest commit.
- [ ] Migration/API/event impacts are described.
- [ ] Security and tenant isolation were considered.
- [ ] No secret, personal data or generated artifact is included.
- [ ] Documentation was updated when behavior or operations changed.
