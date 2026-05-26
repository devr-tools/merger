#!/usr/bin/env bash
set -euo pipefail

base_ref="${1:-origin/main}"
changed_files="$(git diff --name-only "${base_ref}"...HEAD)"

code_changes="$(printf '%s\n' "$changed_files" | grep -E '^(cmd/|internal/|pkg/).+\.go$' | grep -Ev '(^|/).+_test\.go$|(^|/)doc\.go$' || true)"
test_changes="$(printf '%s\n' "$changed_files" | grep -E '(^tests/|_test\.go$)' || true)"

if [ -z "$code_changes" ]; then
  echo "No Go source changes that require a test presence check."
  exit 0
fi

echo "Go source files changed:"
printf '%s\n' "$code_changes"

if [ -n "$test_changes" ]; then
  echo
  echo "Matching test updates detected:"
  printf '%s\n' "$test_changes"
  exit 0
fi

echo "::error title=Missing test update::Go source changed without any updates under tests/ or *_test.go files"
exit 1
