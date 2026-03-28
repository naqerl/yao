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

PREFERRED METHOD: Use the write tool

The write tool supports multiple operations:

1. REPLACE - Replace exact text:
   write:0 {
     "path": "filename.go",
     "old_string": "foo",
     "new_string": "bar"
   }

2. INSERT AFTER - Insert after existing text (keeps old_string):
   write:1 {
     "path": "filename.go",
     "old_string": "line before insertion",
     "new_string": "new inserted line\n",
     "insert_after": true
   }

3. INSERT AT LINE - Insert at specific line number:
   write:2 {
     "path": "filename.go",
     "new_string": "// New comment",
     "insert_line": 1
   }
   # insert_line: 1 = beginning, 5 = after line 4, 9999 = end

4. APPEND TO END - Add to end of file:
   write:3 {
     "path": "filename.go",
     "new_string": "// End of file",
     "append": true
   }

5. REPLACE ALL - Replace all occurrences:
   write:4 {
     "path": "filename.go",
     "old_string": "oldValue",
     "new_string": "newValue",
     "replace_all": true
   }

READING FILES SAFELY:

Before editing, use read to track file state:

   read:0 {
     "path": "filename.go"
   }

Or read a specific range:
   read:1 {
     "path": "filename.go",
     "offset": 10,
     "limit": 20
   }

IMPORTANT: The write tool will FAIL if the file was modified after your last 
read. This prevents editing stale content. If you get a "file changed" 
error, re-read the file with read and retry with updated old_string.

CRITICAL RULES:
- Use read (not cat) before editing - it enables change detection
- Whitespace matters - copy exact tabs/spaces/newlines
- For multi-line strings, include \n explicitly
- If old_string matches multiple places, the tool will fail and suggest using replace_all

---

FALLBACK METHODS (when tools unavailable):

1. SINGLE-LINE or EXACT BLOCK REPLACEMENT - Use sed:
   # Verify the exact line first
   grep -n "exact line content" filename

   # Replace single line
   sed -i 's/const OldValue = 10/const NewValue = 20/' filename

   # Insert after a line
   sed -i '/pattern/a\new line content' filename

   # Insert at beginning
   sed -i '1i\new first line' filename

   # Append to end
   echo "new last line" >> filename

   # Delete exact line
   sed -i '/^func unusedFunc/d' filename

2. MULTI-LINE BLOCK REPLACEMENT - Use ed:
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
- ALWAYS verify exact content with cat -n first
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
