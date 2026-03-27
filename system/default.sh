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

cat <<'EOF'
== edit guide ==
To edit files, you MUST use git-style patches.

First, read the current file with cat filename.

Then output a valid unified diff patch and apply it with:

cat > /tmp/edit.patch << 'EOF' [your full patch here] EOF git apply /tmp/edit.patch && rm /tmp/edit.patch

Always verify with git diff or cat filename after applying.
EOF

echo "== AGENTS.md =="
cat AGENTS.md 2>/dev/null || true

echo '== available skills =='
ls -la "$HOME/.agents/skills" 2>/dev/null || true

cat "$HOME/.agents/AGENTS.md" 2>/dev/null || true
