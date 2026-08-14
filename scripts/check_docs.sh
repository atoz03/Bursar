#!/usr/bin/env bash
# Documentation consistency checks.
#
# English is the source language. Every English document under docs/ must have a
# Simplified Chinese counterpart under docs/zh-CN/ with the same file name, and
# every relative link in every Markdown file must resolve.
#
# Usage: bash scripts/check_docs.sh

set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}" || exit 1

failures=0

fail() {
  echo "FAIL: $*" >&2
  failures=$((failures + 1))
}

markdown_files() {
  find . -name '*.md' \
    -not -path './.git/*' \
    -not -path './web/node_modules/*' \
    -not -path './web/dist/*' | sort
}

# ---------- 1. Relative links resolve ----------
echo "==> Checking relative links"
link_problems=0
while IFS= read -r file; do
  dir="$(dirname "${file}")"
  while IFS= read -r target; do
    [[ -z "${target}" ]] && continue
    case "${target}" in
      http://*|https://*|mailto:*|'#'*) continue ;;
    esac
    path="${target%%#*}"
    [[ -z "${path}" ]] && continue
    if [[ ! -e "${dir}/${path}" ]]; then
      fail "broken link in ${file}: ${target}"
      link_problems=$((link_problems + 1))
    fi
  done < <(grep -oE '\]\([^) ]+\)' "${file}" 2>/dev/null | sed -E 's/^\]\(//; s/\)$//')
done < <(markdown_files)
[[ "${link_problems}" -eq 0 ]] && echo "    all relative links resolve"

# ---------- 2. English/Chinese parity under docs/ ----------
echo "==> Checking English/Chinese parity"
parity_problems=0
if [[ ! -d docs/zh-CN ]]; then
  fail "docs/zh-CN/ does not exist"
  parity_problems=$((parity_problems + 1))
else
  while IFS= read -r en; do
    name="$(basename "${en}")"
    if [[ ! -f "docs/zh-CN/${name}" ]]; then
      fail "missing translation: docs/zh-CN/${name}"
      parity_problems=$((parity_problems + 1))
    fi
  done < <(find docs -maxdepth 1 -name '*.md' | sort)

  while IFS= read -r zh; do
    name="$(basename "${zh}")"
    if [[ ! -f "docs/${name}" ]]; then
      fail "translation without an English source: docs/zh-CN/${name}"
      parity_problems=$((parity_problems + 1))
    fi
  done < <(find docs/zh-CN -maxdepth 1 -name '*.md' | sort)
fi
[[ "${parity_problems}" -eq 0 ]] && echo "    every document exists in both languages"

# ---------- 3. Root-level bilingual pairs ----------
echo "==> Checking root-level bilingual pairs"
root_problems=0
for base in README CHANGELOG CONTRIBUTING SECURITY CODE_OF_CONDUCT; do
  if [[ -f "${base}.md" && ! -f "${base}.zh-CN.md" ]]; then
    fail "missing ${base}.zh-CN.md"
    root_problems=$((root_problems + 1))
  fi
  if [[ -f "${base}.zh-CN.md" && ! -f "${base}.md" ]]; then
    fail "missing ${base}.md"
    root_problems=$((root_problems + 1))
  fi
done
[[ "${root_problems}" -eq 0 ]] && echo "    all root documents are bilingual"

# ---------- 4. Language switcher present ----------
echo "==> Checking language switcher headers"
switcher_problems=0
while IFS= read -r f; do
  if ! head -n 6 "${f}" | grep -q 'English'; then
    fail "no language switcher in ${f}"
    switcher_problems=$((switcher_problems + 1))
  fi
done < <(find docs -maxdepth 2 -name '*.md' | sort)
[[ "${switcher_problems}" -eq 0 ]] && echo "    all documents link to their translation"

# ---------- 5. Stale port references ----------
# The 60039/60040 defaults were retired in favour of 8080/8081. Changelogs and the
# troubleshooting guides mention the old values deliberately, to explain the migration.
echo "==> Checking for retired default ports"
port_problems=0
port_allowlist='CHANGELOG|check_docs.sh|docs/troubleshooting.md|docs/zh-CN/troubleshooting.md'
while IFS= read -r hit; do
  fail "retired port reference: ${hit}"
  port_problems=$((port_problems + 1))
done < <(grep -rn '60039\|60040' --include='*.md' --include='*.yaml' --include='*.yml' \
  --include='*.sh' --include='*.go' --include='*.ts' --include='*.vue' --include='*.service' . 2>/dev/null \
  | grep -v '^\./\.git/' | grep -v node_modules | grep -vE "${port_allowlist}")
[[ "${port_problems}" -eq 0 ]] && echo "    no references to the retired 60039/60040 defaults"

echo
if [[ "${failures}" -gt 0 ]]; then
  echo "${failures} documentation problem(s) found." >&2
  exit 1
fi
echo "Documentation checks passed."
