#!/usr/bin/env bash
set -euo pipefail

report_path='.agentproof/report.md'
summary_path="${GITHUB_STEP_SUMMARY:?GITHUB_STEP_SUMMARY is required}"

if [[ -L "$report_path" ]]; then
  printf 'AgentProof report must not be a symbolic link\n' >&2
  exit 1
fi
if [[ ! -e "$report_path" ]]; then
  exit 0
fi
if [[ ! -f "$report_path" || ! -r "$report_path" ]]; then
  printf 'AgentProof report must be a readable regular file\n' >&2
  exit 1
fi
if ! cat -- "$report_path" >/dev/null; then
  printf 'Unable to read AgentProof report\n' >&2
  exit 1
fi

if ! {
  printf '## AgentProof\n\n'
  cat -- "$report_path"
} >> "$summary_path"; then
  printf 'Unable to write AgentProof step summary\n' >&2
  exit 1
fi
