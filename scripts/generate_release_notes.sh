#!/usr/bin/env bash

set -u -o pipefail

if ! [[ "$0" =~ scripts/generate_release_notes.sh ]]; then
	echo "must be run from repository root"
	exit 1
fi

if [[ $# -ne 1 ]]; then
	echo "usage: scripts/generate_release_notes.sh <version>"
	echo "examples:"
	echo "  scripts/generate_release_notes.sh 3.11.0"
	echo "  scripts/generate_release_notes.sh 3.11.0-rc.0"
	echo "  scripts/generate_release_notes.sh 3.11.0-rc.1"
	echo "  scripts/generate_release_notes.sh 3.11.1"
	exit 1
fi

version="${1#v}"
IFS='.' read -r major minor patch_rest <<< "${version}"
# Strip any pre-release suffix (e.g. "0-rc.0" -> "0").
patch="${patch_rest%%-*}"

if [[ -z "${GITHUB_TOKEN:-}" ]]; then
	echo "GITHUB_TOKEN is not set"
	exit 1
fi
if ! command -v release-notes >/dev/null 2>&1; then
	echo "release-notes not found. Install: go install k8s.io/release/cmd/release-notes@v0.20.1"
	exit 1
fi

TMPL='{{- $version := .CurrentRevision -}}
{{- if not $version}}{{- $version = "X.Y.Z" -}}{{end -}}
## {{$version}} / YYYY-MM-DD
{{/* Sort the sections */}}
{{- range .Notes}}{{range $note := .NoteEntries}}{{if eq (slice $note 0 10) "[SECURITY]"}}- {{$note}}
{{end}}{{end}}{{end -}}
{{- range .Notes}}{{range $note := .NoteEntries}}{{if eq (slice $note 0 8) "[CHANGE]"}}- {{$note}}
{{end}}{{end}}{{end -}}
{{- range .Notes}}{{range $note := .NoteEntries}}{{if eq (slice $note 0 9) "[FEATURE]"}}- {{$note}}
{{end}}{{end}}{{end -}}
{{- range .Notes}}{{range $note := .NoteEntries}}{{if eq (slice $note 0 13) "[ENHANCEMENT]"}}- {{$note}}
{{end}}{{end}}{{end -}}
{{- range .Notes}}{{range $note := .NoteEntries}}{{if eq (slice $note 0 6) "[PERF]"}}- {{$note}}
{{end}}{{end}}{{end -}}
{{- range .Notes}}{{range $note := .NoteEntries}}{{if eq (slice $note 0 8) "[BUGFIX]"}}- {{$note}}
{{end}}{{end}}{{end}}
.'

release_branch="release-${major}.${minor}"

if [[ "${patch}" == "0" ]]; then
	# Minor release (any RC or final): cover the full cycle from the previous minor's branch-cut point.
	prev_minor=$(( minor - 1 ))
	start_sha="$(git log -1 --format='%P' "v${major}.${prev_minor}.0-rc.0" | awk '{print $1}')"
	end_rev="${release_branch}"
	echo "Generating release notes for version=${version} major=${major} minor=${minor} patch=${patch} start_sha=${start_sha} end_rev=${end_rev}"
	release-notes generate \
		--start-sha="${start_sha}" \
		--end-rev="${end_rev}" \
		--branch="${release_branch}" \
		--org prometheus --repo prometheus \
		--required-author="" --dependencies=false \
		--go-template="go-template:inline:${TMPL}" \
		--output=/dev/stdout
else
	# Patch release: cover commits since the previous patch tag.
	prev_patch=$(( patch - 1 ))
	start_rev="v${major}.${minor}.${prev_patch}"
	end_rev="${release_branch}"
	echo "Generating release notes for version=${version} major=${major} minor=${minor} patch=${patch} start_rev=${start_rev} end_rev=${end_rev}"
	release-notes generate \
		--start-rev="${start_rev}" \
		--end-rev="${end_rev}" \
		--branch="${release_branch}" \
		--org prometheus --repo prometheus \
		--required-author="" --dependencies=false \
		--go-template="go-template:inline:${TMPL}" \
		--skip-first-commit \
		--output=/dev/stdout
fi
