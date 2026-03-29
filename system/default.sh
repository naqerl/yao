#!/usr/bin/env bash

printf 'Today: '
date 2>/dev/null || true

printf 'PWD: '
pwd 2>/dev/null || true

echo '== project tree =='
git ls-files --cached --others --exclude-standard 2>/dev/null || true

echo '== git status =='
git status --short --branch 2>/dev/null || true

echo '== last 5 commits =='
git log --oneline -n 5 2>/dev/null || true

echo '== general instructions =='
cat "$HOME/.agents/AGENTS.md" 2>/dev/null || true

echo '== general skills =='
find "$HOME/.agents/skills" -type f -name "SKILL.md" 2>/dev/null || true
echo 'NOTE: if any word from the users prompt matches with a skill name, ensure that it was read'

echo "== current project instructions =="
cat AGENTS.md 2>/dev/null || true
