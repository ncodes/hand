# Git Hooks

Install the repository hooks with:

```sh
make install-hooks
```

This sets `core.hooksPath` to `.githooks` and makes the commit message hook executable.

## Commit Message Format

Commits must use a Conventional Commit title, then one or more body sections.
Each section must end with `:` and contain at least one bullet.

```text
feat(scope): improve something

Changed:
Optional description of the change.
- Mandatory bullet point
- Mandatory bullet point

Verified:
- Mandatory bullet point
```

Rules enforced by `commit-msg`:

- The title must be 72 characters or fewer.
- The title must use a supported Conventional Commit type.
- The title description must start lowercase and must not end with punctuation.
- The body must include at least one section heading, such as `Changed:`.
- Every section must include at least one `- ` bullet.
- Co-author trailers and assistant/tool attribution are not allowed.
