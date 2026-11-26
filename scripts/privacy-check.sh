#!/usr/bin/env bash
# Fails if a tracked file contains a value listed in the private deny-list.
# The list lives outside the repo (one extended-regex pattern per line):
#   ${RIGCTLWEB_DENYLIST:-$HOME/.config/rigctlweb-dev/denylist}
# Without a list the check is skipped, so contributors are not affected.
set -euo pipefail
list="${RIGCTLWEB_DENYLIST:-$HOME/.config/rigctlweb-dev/denylist}"
[ -r "$list" ] || { echo "privacy-check: no deny-list at $list, skipped"; exit 0; }
cd "$(git rev-parse --show-toplevel)"
if git ls-files -z | xargs -0 grep -nIiE -f "$list" ; then
  echo "privacy-check: private values found in tracked files" >&2; exit 1
fi
echo "privacy-check: clean"
