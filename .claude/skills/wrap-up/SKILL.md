---
name: wrap-up
description: Wrap up the session so the context can be cleared or the work handed to another agent or session — commit and open PRs for open work, delete merged branches, leave git clean, and update personal-notes/plan.md with where we are and the next step. Use when the user types /wrap-up or asks to wrap up, hand off, or prepare to clear the context. `/wrap-up merge` also merges the PRs once checks pass.
---

# Wrap-up

Goal: the next session starts from `main`, a clean tree and `personal-notes/plan.md`, and needs nothing from this conversation.

Arguments: `merge` — merge the PRs this wrap-up leaves open once their checks pass. Without it, stop at an open PR: merging is the user's decision.

## 1. Inventory

Run `scripts/wrap-up.sh status`. It fetches, then lists: current branch, uncommitted changes, stashes, extra worktrees, unpushed commits, merged branches, unmerged remote branches, open PRs. Exit 1 means git is not clean.

Then recall from the conversation what was done, decided or left open this session. That is the material for step 5.

## 2. Bring open work to a PR

For each piece of unfinished or uncommitted work:

- Never on `main`. Put it on its own branch (`feat/…`, `fix/…`, `perf/…`, `chore/…`); unrelated changes go on separate branches.
- Never commit `.env`, secrets, production data or `personal-notes/`.
- Verify as `AGENTS.md` says (backend: `cd apps/api && go test ./...`; frontend: `cd apps/web && npm run lint && npm test && npm run build`), only for the side that changed. If it fails, fix it or say so in the PR and in the plan; do not hide it.
- Conventional commit subject, lowercase, imperative; the body says why.
- `git push -u origin <branch>`, `gh pr create` with a short description: what changed, why, how it was checked.
- A stash or worktree: find out what it is. If it is work, bring it to a branch like above; if it is clearly stale, ask before dropping it.
- Work that is half done and should not be a PR yet: commit it as a draft (`gh pr create --draft`) so it is not lost, and say in the plan what is missing.

With `merge`: `gh pr checks <n> --watch`, then `git fetch` and check `origin/main` did not move under the PR, then `gh pr merge <n> --merge` (merge commit, no squash). Merge only PRs from this wrap-up and only with green checks.

## 3. Clean up git

- `git checkout main && git pull --ff-only`.
- `scripts/wrap-up.sh cleanup` (dry run), read the list, then `scripts/wrap-up.sh cleanup --apply`. It deletes only branches merged into `origin/main`, local and remote.
- Unmerged remote branches the script lists: do not delete them yourself. Look at each (`git log origin/main..origin/<branch>`, `gh pr list --head <branch> --state all`) and ask the user, one question for all of them, what to do: keep, open a PR, or delete.
- Run `scripts/wrap-up.sh status` again; it must end with `git is clean`. If it cannot (an open PR, a branch the user wants to keep), say why in the report.

## 4. Keep the plan short

`personal-notes/plan.md` is gitignored, in Russian, written for the user (see the memory on it). If it is over ~20 KB, move the «Сделано» section and closed items to `personal-notes/archive/plan-<date>.md` and leave a link, as with `archive/plan-2026-10-16.md`.

## 5. Update the plan

In `personal-notes/plan.md`:

- Header: today's date and the last PR in `main`.
- «Где мы сейчас» and «Дальше по порядку»: strike through or move what is done, with PR numbers; add new items and decisions with their date.
- «Ждёт тебя (руками)»: what only the user can do (deploy, prod checks, answers).
- Section «Передача» right after the header, replaced on every wrap-up (not appended):
  - date, `main` @ short hash, open PRs and what each waits for;
  - what this session did and decided (short, with PRs);
  - open questions and what each one blocks;
  - **next step**: one concrete action, with the files or command to start from;
  - anything a fresh session would get wrong without being told.

## 6. Memory

Save to memory only what holds beyond this task: a rule the user set for how to work, a durable fact about the project. Update an existing memory instead of adding a near duplicate. The plan state goes to the plan, not to memory.

## 7. Report

Short, in the user's language:

- state of git (`git is clean`, `main` @ hash) or what is left and why;
- PRs opened or merged, with links;
- branches deleted (count) and the ones kept;
- the next step from «Передача»;
- one line to paste into the next session, e.g. «Прочитай personal-notes/plan.md, раздел «Передача», и начни со следующего шага.»
