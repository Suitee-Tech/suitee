# Security policy

## Reporting a vulnerability

Please **do not** disclose security vulnerabilities in public issues, discussions or pull requests.

Send a private report to **security@suitee.com.br** with:

- a description of the issue and affected component;
- reproduction steps or a proof of concept;
- potential impact;
- any suggested mitigation.

We will acknowledge the report, investigate it and coordinate disclosure. Please allow reasonable time for a fix before publishing details.

## Scope

This policy covers the code in this repository. Production infrastructure, provider accounts and customer data are not part of the public repository; reports about them should still be sent privately.

## Repository controls

- `main` and `develop` are protected: pull request, CODEOWNER approval, green CI, resolved conversations, linear history and no force pushes.
- Secret scanning and push protection are enabled.
- Security-sensitive paths have CODEOWNERS review.
- Dependabot security updates are enabled for the default branch.

These controls reduce risk but do not replace rotating a secret after exposure.

## Secret handling

- Never commit `.env`, API keys, tokens, cookies, private keys or customer data.
- Use the provided `.env.example` files as templates with placeholders.
- If a secret is accidentally committed, rotate it immediately and report the incident privately. Removing it from the current commit is not sufficient because Git history retains it.
