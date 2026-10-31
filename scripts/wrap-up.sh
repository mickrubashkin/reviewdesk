#!/usr/bin/env bash
# Git side of /wrap-up (.claude/skills/wrap-up): reports what is left before a
# session can end, and removes branches that are already merged into main.
#
# Usage:
#   scripts/wrap-up.sh status            report; exit 1 if git is not clean
#   scripts/wrap-up.sh cleanup           show what cleanup would delete
#   scripts/wrap-up.sh cleanup --apply   delete it
#
# Clean means: on main, level with origin/main, no changes, no stash, no extra
# worktrees, no unpushed commits, no merged branches left (local or remote).
# Open PRs and pushed unmerged branches are reported, not counted against it:
# they are work waiting for review.
set -euo pipefail

root=$(git rev-parse --show-toplevel)
cd "$root"

main=main
remote=origin

git fetch --quiet --prune "$remote"

# Merged branches, without main itself and the current branch.
merged_local() {
  current=$(git branch --show-current)
  git for-each-ref --merged "$remote/$main" --format='%(refname:short)' refs/heads |
    grep -vx -e "$main" -e "$current" || true
}

merged_remote() {
  git for-each-ref --merged "$remote/$main" --format='%(refname:lstrip=3)' "refs/remotes/$remote" |
    grep -vx -e "$main" -e HEAD || true
}

# Local branches with commits that are not on their upstream (or no upstream).
unpushed() {
  git for-each-ref --format='%(refname:short) %(upstream:short) %(upstream:track)' refs/heads |
    while read -r name upstream track; do
      [ "$name" = "$main" ] && continue
      if [ -z "$upstream" ]; then
        git merge-base --is-ancestor "$name" "$remote/$main" || echo "$name (no upstream)"
      elif [[ "$track" == *ahead* ]]; then
        echo "$name $track"
      fi
    done
}

unmerged_remote() {
  git for-each-ref --no-merged "$remote/$main" --sort=-committerdate \
    --format='%(refname:lstrip=3)  %(committerdate:short)  %(subject)' "refs/remotes/$remote"
}

section() {
  # $1 title, $2 body; prints nothing for an empty body.
  [ -n "$2" ] || return 0
  printf '\n%s\n' "$1"
  printf '%s\n' "$2" | sed 's/^/  /'
}

status() {
  dirty=0
  branch=$(git branch --show-current)
  head=$(git rev-parse --short HEAD)
  echo "branch: ${branch:-detached} @ $head"

  if [ "$branch" != "$main" ]; then
    echo "  not on $main"
    dirty=1
  else
    counts=$(git rev-list --left-right --count "$main...$remote/$main")
    ahead=${counts%%[[:space:]]*}
    behind=${counts##*[[:space:]]}
    if [ "$ahead" != 0 ] || [ "$behind" != 0 ]; then
      echo "  $main is $ahead ahead, $behind behind $remote/$main"
      dirty=1
    fi
  fi

  changes=$(git status --short)
  stashes=$(git stash list)
  worktrees=$(git worktree list | tail -n +2)
  pending=$(unpushed)
  local_merged=$(merged_local)
  remote_merged=$(merged_remote)

  for v in "$changes" "$stashes" "$worktrees" "$pending" "$local_merged" "$remote_merged"; do
    [ -z "$v" ] || dirty=1
  done

  section "uncommitted changes:" "$changes"
  section "stashes:" "$stashes"
  section "extra worktrees:" "$worktrees"
  section "unpushed commits:" "$pending"
  section "merged local branches (cleanup deletes):" "$local_merged"
  section "merged remote branches (cleanup deletes):" "$remote_merged"
  section "unmerged remote branches (waiting or abandoned, decide by hand):" "$(unmerged_remote)"
  if command -v gh >/dev/null; then
    section "open PRs:" "$(gh pr list --state open --json number,headRefName,title \
      --template '{{range .}}#{{.number}} {{.headRefName}}  {{.title}}{{"\n"}}{{end}}')"
  fi

  echo
  if [ "$dirty" = 0 ]; then
    echo "git is clean"
  else
    echo "git is not clean"
    return 1
  fi
}

cleanup() {
  apply=0
  [ "${1:-}" = "--apply" ] && apply=1

  local_merged=$(merged_local)
  remote_merged=$(merged_remote)
  if [ -z "$local_merged" ] && [ -z "$remote_merged" ]; then
    echo "nothing to delete"
    return 0
  fi

  section "local branches merged into $remote/$main:" "$local_merged"
  section "remote branches merged into $remote/$main:" "$remote_merged"

  if [ "$apply" = 0 ]; then
    echo
    echo "dry run; delete with: scripts/wrap-up.sh cleanup --apply"
    return 0
  fi

  # -d, not -D: git refuses a branch that turns out not to be merged.
  [ -z "$local_merged" ] || printf '%s\n' "$local_merged" | xargs git branch -d
  [ -z "$remote_merged" ] || printf '%s\n' "$remote_merged" | xargs git push --quiet "$remote" --delete
  git fetch --quiet --prune "$remote"
  echo "deleted"
}

case "${1:-}" in
  status) status ;;
  cleanup) cleanup "${2:-}" ;;
  *)
    echo "usage: $0 status | cleanup [--apply]" >&2
    exit 2
    ;;
esac
