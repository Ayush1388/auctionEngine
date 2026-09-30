# Workflow

How work moves through this repo. The plan lives in GitHub, not in a file.

## Where things are tracked

| What | Where |
|---|---|
| Upcoming work | [Issues](https://github.com/Ayush1388/auctionEngine/issues), one per vertical slice |
| A release's scope | [Milestones](https://github.com/Ayush1388/auctionEngine/milestones), e.g. `v0.2 – Auction management` |
| Why something is built a certain way | [`docs/decisions/`](decisions/) |
| What the system does today | [`README.md`](../README.md) |

## The loop

1. **Pick an issue** from the current milestone.
2. **Branch** from an up-to-date `main`:
   `git switch main && git pull && git switch -c feat/3-create-auction`
   Prefixes: `feat/`, `fix/`, `chore/`, `docs/`, `test/`, followed by the issue number.
3. **Commit small**, in the imperative: `Add auction repository`, `Validate auction end time`.
4. **Open a PR** early, even as a draft. Write `Closes #3` in the description.
5. **Review your own diff** on GitHub before merging. Read it as if someone else wrote it.
6. **Squash and merge.** The issue closes automatically; delete the branch.
7. If the PR involved a real design choice, **add a decision note** in the same PR.

## Rules of thumb

- One issue, one branch, one PR. If a PR grows past a few hundred lines, split the issue.
- `main` always builds and passes tests.
- Build vertical slices (migration → repository → service → handler → tests) rather than one layer at a time.
- Write the core logic yourself first, then use AI to review it.
