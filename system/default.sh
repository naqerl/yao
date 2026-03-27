#!/usr/bin/env bash

printf '== date ==\n'
date 2>/dev/null || true
printf '\n== pwd ==\n'
pwd 2>/dev/null || true
printf '\n== project tree ==\n'
if git rev-parse --show-toplevel >/dev/null 2>&1; then
  git ls-files --cached --others --exclude-standard 2>/dev/null || true
elif command -v tree >/dev/null 2>&1; then
  tree -a -L 3 . 2>/dev/null || true
else
  find . -maxdepth 3 -print 2>/dev/null | sort || true
fi
printf '\n== git status ==\n'
git status --short --branch 2>/dev/null || true
printf '\n== last 5 commits ==\n'
git log --oneline -n 5 2>/dev/null || true
printf '\n== root AGENTS.md ==\n'
if [ -f AGENTS.md ]; then
  cat AGENTS.md 2>/dev/null || true
else
  printf 'AGENTS.md not found\n'
fi
printf '\n== $HOME/.agents/skills ==\n'
if [ -d "$HOME/.agents/skills" ]; then
  ls -la "$HOME/.agents/skills" 2>/dev/null || true
else
  printf '%s\n' "$HOME/.agents/skills not found"
fi
printf '\n== $HOME/.agents/AGENTS.md ==\n'
if [ -f "$HOME/.agents/AGENTS.md" ]; then
  cat "$HOME/.agents/AGENTS.md" 2>/dev/null || true
else
  printf '%s\n' "$HOME/.agents/AGENTS.md not found"
fi
