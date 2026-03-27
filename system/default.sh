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

cat <<'GUIDEEOF'
== how to edit files  ==
Use old_string/new_string exact matching.

First, read the file: cat filename

Then apply surgical edits using one of these methods:

1. SINGLE-LINE or EXACT BLOCK REPLACEMENT - Use sed with exact match:
   # Verify the exact line first
   grep -n "exact line content" filename

   # Replace single line
   sed -i 's/const OldValue = 10/const NewValue = 20/' filename

   # Delete exact line
   sed -i '/^func unusedFunc/d' filename

2. MULTI-LINE BLOCK REPLACEMENT - Use ed with exact strings:
   ed filename << 'EDCMDS'
   /^func Init.*error {$/  # find exact function signature
   /^}/                    # go to closing brace
   c                        # change entire block
   func Init(ctx context.Context) error {
       if ctx == nil {
           return fmt.Errorf("context required")
       }
       return nil
   }
   .
   w
   q
   EDCMDS

3. EXACT PATCH with context - Use patch (most reliable for multi-file):
   cat > /tmp/fix.patch << 'PATCH'
   --- a/state/state.go
   +++ b/state/state.go
   @@ -45,8 +45,8 @@
    func (s *State) String() string {
        if s == nil {
   -		return "<nil>"
   +		return "nil state"
        }

        systemSource := "default"
   PATCH
   patch -p1 < /tmp/fix.patch

4. NEW FILES or COMPLETE REWRITES - Use cat:
   cat > filename.go << 'EOF'
   package main
   // ... full content ...
   EOF

CRITICAL RULES:
- ALWAYS verify exact content with cat/grep first
- Whitespace matters - use tabs/exactly as shown
- If sed/ed fails, use cat for full rewrite
- Never use git apply (whitespace fragile)

ANTI-PATTERNS to avoid:
- Inserting lines without context (fragile)
- Pattern matching that could match multiple places
- Complex multi-step sed chains
- Leaving backup files (.orig, .rej) in the tree
GUIDEEOF

echo '== general instructions =='
cat "$HOME/.agents/AGENTS.md" 2>/dev/null || true

echo '== general skills =='
find "$HOME/.agents/skills" -type f -name "SKILL.md" 2>/dev/null || true

echo "== current project instructions =="
cat AGENTS.md 2>/dev/null || true
