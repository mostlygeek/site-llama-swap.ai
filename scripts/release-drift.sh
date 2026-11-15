#!/usr/bin/env bash
# Says whether the live site's release notes are behind llama-swap's newest
# changelog file. The site renders the notes embedded in the llama-swap commit
# it was built with, so "behind" means the site needs a rebuild and nothing
# else.
#
# Prints key=value lines for a GitHub Actions $GITHUB_OUTPUT file:
#
#   upstream=v273  newest changelog file on llama-swap's main
#   live=v272      newest release on the site
#   drift=true     the site is behind and should be rebuilt
#
# Exits 2 when either side could not be read, so an outage stays an outage
# instead of looking like a new release. A readable page with no releases in
# it counts as behind, which rebuilds a broken page.
#
# Usage: scripts/release-drift.sh [site] [org/repo]
set -uo pipefail

site=${1:-https://llama-swap.ai/changelog}
repo=${2:-mostlygeek/llama-swap}
api=${GH_API:-https://api.github.com}

get() { # url
  local args=(--silent --show-error --fail --retry 3 --retry-delay 5
    --retry-all-errors --max-time 60)
  # GitHub allows 60 unauthenticated API requests an hour per address and an
  # Actions runner shares one, so send the workflow's token when there is one.
  local token=${GH_TOKEN:-${GITHUB_TOKEN:-}}
  if [ -n "$token" ]; then
    args+=(--header "authorization: Bearer $token")
  fi
  curl "${args[@]}" "$1"
}

if ! page=$(get "$site"); then
  echo "could not read $site" >&2
  exit 2
fi
if ! notes=$(get "$api/repos/$repo/contents/changelog"); then
  echo "could not list $repo's changelog on GitHub" >&2
  exit 2
fi

# Newest version on the site, from the version links in its release listing.
live=$(printf '%s\n' "$page" | grep -oE '/changelog/v[0-9]+' | grep -oE '[0-9]+$' | sort -n | tail -1)
# Newest changelog file, for example v273.
upstream=$(printf '%s\n' "$notes" | jq -r '
  [ .[] | .name | select(test("^v[0-9]+\\.md$")) ]
  | map(rtrimstr(".md") | ltrimstr("v") | tonumber) | max // empty')

if [ -z "$upstream" ]; then
  echo "no v<number>.md files in $repo's changelog; the layout changed?" >&2
  exit 2
fi

if [ -z "$live" ]; then
  echo "no releases found on $site" >&2
  drift=true
elif [ "$live" -lt "$upstream" ]; then
  drift=true
else
  drift=false
fi

echo "upstream=v$upstream"
echo "live=v${live:-none}"
echo "drift=$drift"
