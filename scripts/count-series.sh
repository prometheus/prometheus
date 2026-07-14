#!/usr/bin/env bash
#
# count-series.sh — count series, native-histogram series and native-histogram
# buckets currently in a Prometheus server.
#
# Only the documented HTTP query APIs are used:
#   - GET /api/v1/query                  (instant query)
#   - GET /api/v1/label/__name__/values  (list metric names)
#   - GET /api/v1/labels                 (labels of a metric, for sharding)
#   - GET /api/v1/label/<name>/values    (label values, for sharding)
# See https://prometheus.io/docs/prometheus/latest/querying/api/
#
# Design notes for scale (millions of series):
#   * Total series is one aggregating query -> tiny response.
#   * Native histograms cannot be selected globally: histogram_count() drops the
#     __name__ label, so a query over all series collapses different metrics to
#     the same label set ("vector cannot contain metrics with the same
#     labelset"). Instead we shortlist candidates and confirm each per metric
#     *name* with count(histogram_count({__name__="X"})), concurrently.
#   * Candidate selection is a hybrid: /metadata shortlists histogram-typed
#     metrics (so we do not query every name on large tenants), and any name that
#     has no metadata is also scanned (metadata may be incomplete or capped). The
#     only names trusted as non-native-histogram are those metadata explicitly
#     types as something else — a native histogram exposed with a wrong metadata
#     type would be missed (rare; a broken exporter).
#   * The metric-name list itself can be capped by the server (e.g. Mimir's
#     querier.max-label-values-limit=10000). When that happens we enumerate names
#     by a recursive, UTF-8-safe character-class partition of __name__, so even
#     tenants with far more names than the cap are fully covered.
#   * Bucket counting needs the actual histogram samples (there is no PromQL
#     function for the number of buckets). We fetch raw samples ONLY for
#     native-histogram metrics, and for any metric with more series than the
#     shard threshold (-s) we partition it by a label into bounded shards,
#     including an L="" catch-all shard so series lacking the label are counted.
#     The partition is complete and disjoint, so buckets are neither missed nor
#     double counted. Raw fetches run through a concurrency pool. Leaves that the
#     server refuses (e.g. Mimir "aggregated metrics") are reported as a lower
#     bound rather than silently counted as zero.
#   * All names/label values are embedded into PromQL as escaped string literals
#     (see promquote), so arbitrary UTF-8, quotes and backslashes are handled.
#   * Instant queries use Prometheus' lookback delta (5m by default); the label
#     and metric-name enumerations are scoped to the last 5 minutes via start/end.
#   * Every instant query is pinned to a single evaluation time (time=NOW), so
#     the whole run is one coherent snapshot and concurrent ingestion cannot make
#     the sharded counts drift relative to each other.

set -euo pipefail

# Force C collation so sort/comm agree on byte order (metric names are UTF-8;
# server-side RE2 regexes and jq are unaffected by this).
export LC_ALL=C

PROM_URL="${PROM_URL:-http://localhost:9090}"
JOBS="${JOBS:-16}"       # Concurrency for detection and bucket-fetch pools.
SHARD_MAX="${SHARD_MAX:-20000}"  # Max series per bucket-fetch shard before splitting.
LOOKBACK="${LOOKBACK:-300}"      # Enumeration look-back window, seconds.
RETRIES="${RETRIES:-5}"          # Attempts per request before giving up.
TIMEOUT="${TIMEOUT:-120}"        # Per-request timeout, seconds.

usage() {
	cat >&2 <<EOF
Usage: $0 [-u PROM_URL] [-j JOBS] [-s SHARD_MAX] [-H 'Header: value']... [-- CURL_ARGS...]

  -u PROM_URL   Base URL incl. any path prefix (default: ${PROM_URL}).
                E.g. http://localhost:9090/prometheus
  -j JOBS       Concurrent queries in each phase (default: ${JOBS}).
  -s SHARD_MAX  Max series per bucket-fetch shard; metrics larger than this are
                split by a label (default: ${SHARD_MAX}).
  -H HEADER     Extra HTTP header sent with every request; repeatable. E.g. for
                a Mimir/Cortex tenant:  -H 'X-Scope-OrgID: anonymous'
  -- CURL_ARGS  Everything after -- is passed verbatim to curl, e.g. for auth:
                $0 -- -H 'Authorization: Bearer TOKEN'  --user 'name:pass'
EOF
	exit 1
}

CURL_EXTRA=()
while [ $# -gt 0 ]; do
	case "$1" in
	-u) PROM_URL="$2"; shift 2 ;;
	-j) JOBS="$2"; shift 2 ;;
	-s) SHARD_MAX="$2"; shift 2 ;;
	-H) CURL_EXTRA+=(-H "$2"); shift 2 ;;
	-h | --help) usage ;;
	--) shift; CURL_EXTRA+=("$@"); break ;;
	*) echo "Unknown argument: $1" >&2; usage ;;
	esac
done

for cmd in curl jq date; do
	command -v "$cmd" >/dev/null 2>&1 || { echo "Error: required command '$cmd' not found." >&2; exit 1; }
done

PROM_URL="${PROM_URL%/}"
API="${PROM_URL}/api/v1"
NOW="$(date +%s)"
START="$((NOW - LOOKBACK))"

# promquote STR -> a PromQL double-quoted string literal for STR.
# Escapes backslash and double quote; UTF-8 bytes pass through unchanged (they
# need no escaping inside a PromQL string). Control characters (newline/tab) in
# names are not expected and not handled.
promquote() {
	local s=$1
	s=${s//\\/\\\\}
	s=${s//\"/\\\"}
	printf '"%s"' "$s"
}

# labelmatcher NAME VALUE -> `name="value"`, quoting the label name too when it
# is not a legacy identifier (UTF-8 label names).
labelmatcher() {
	local n=$1 v; v=$(promquote "$2")
	if [[ $n =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
		printf '%s=%s' "$n" "$v"
	else
		printf '%s=%s' "$(promquote "$n")" "$v"
	fi
}

# add_matcher SELECTOR NAME VALUE -> SELECTOR with `, name="value"` inserted.
add_matcher() {
	printf '%s, %s}' "${1%\}}" "$(labelmatcher "$2" "$3")"
}

# uenc STR -> URL-encoded STR (for path segments).
uenc() { jq -rn --arg s "$1" '$s|@uri'; }

# retry_curl CURL_ARGS... -> response body on stdout, retrying transient
# transport errors (connection reset, timeout, DNS) with linear backoff. Returns
# nonzero if all attempts fail. Used for GET endpoints (labels/metadata) where a
# transient failure would otherwise silently drop data. Runs safely in background
# subshells (they inherit RETRIES/TIMEOUT/CURL_EXTRA).
retry_curl() {
	local attempt=1 body rc
	while :; do
		body=$(curl -sS -m "$TIMEOUT" "$@" 2>/dev/null); rc=$?
		[ "$rc" -eq 0 ] && { printf '%s' "$body"; return 0; }
		[ "$attempt" -ge "$RETRIES" ] && { printf '%s' "$body"; return 1; }
		sleep "$attempt"; attempt=$((attempt + 1))
	done
}

# do_query EXPR -> instant-query JSON body pinned to time=NOW. Retries TRANSIENT
# failures (curl transport error, HTTP 5xx, 429) with backoff; returns permanent
# responses (HTTP 2xx/4xx, including 422 query errors such as Mimir's aggregated-
# metric rejection) to the caller to classify. Returns nonzero only when transient
# failures exhaust all retries.
do_query() {
	local q="$1" attempt=1 body code rc
	while :; do
		body=$(curl -sS -m "$TIMEOUT" -w $'\n%{http_code}' -G "${CURL_EXTRA[@]}" \
			--data-urlencode "time=$NOW" --data-urlencode "query=$q" "${API}/query" 2>/dev/null); rc=$?
		code=${body##*$'\n'}; body=${body%$'\n'*}
		# Accept only a real API response: transport ok, HTTP < 500 and not 429,
		# and a body that parses as JSON with a .status (a genuine query error such
		# as Mimir's aggregated-metric rejection returns HTTP 422 + valid JSON, so
		# it is accepted here and classified by the caller — not retried). An empty
		# or garbled body (e.g. a flaky kubectl port-forward) is treated as transient.
		if [ "$rc" -eq 0 ] && [ "${code:-500}" -lt 500 ] && [ "${code:-500}" != "429" ] &&
			[ -n "$body" ] && jq -e '.status' >/dev/null 2>&1 <<<"$body"; then
			printf '%s' "$body"; return 0
		fi
		[ "$attempt" -ge "$RETRIES" ] && { printf '%s' "$body"; return 1; }
		sleep "$attempt"; attempt=$((attempt + 1))
	done
}

# q_query EXPR -> JSON body of an instant query; aborts on any error (used in the
# foreground for total series and shard-count queries).
q_query() {
	local resp
	resp=$(do_query "$1") || { echo "Error: request failed (after retries) for query: $1" >&2; exit 1; }
	if [ "$(jq -r '.status // "nonjson"' <<<"$resp" 2>/dev/null)" != "success" ]; then
		echo "Error: Prometheus error for query: $1" >&2
		jq -r '.error // .errorType // .' <<<"$resp" 2>/dev/null >&2 || printf '%s\n' "$resp" >&2
		exit 1
	fi
	printf '%s' "$resp"
}

# q_scalar EXPR -> scalar/first-sample value of an instant query (0 if empty).
q_scalar() { q_query "$1" | jq -r '.data.result[0].value[1] // "0"'; }

# q_label_values LABEL SELECTOR -> newline-separated values, scoped to lookback.
q_label_values() {
	local resp
	resp=$(retry_curl -G "${CURL_EXTRA[@]}" \
		--data-urlencode "match[]=$2" \
		--data-urlencode "start=$START" --data-urlencode "end=$NOW" \
		"${API}/label/$(uenc "$1")/values") ||
		{ echo "Error: label-values request failed for $1" >&2; exit 1; }
	jq -r '.data[]?' <<<"$resp"
}

# q_label_names SELECTOR -> newline-separated label names (excluding __name__).
q_label_names() {
	local resp
	resp=$(retry_curl -G "${CURL_EXTRA[@]}" \
		--data-urlencode "match[]=$1" \
		--data-urlencode "start=$START" --data-urlencode "end=$NOW" \
		"${API}/labels") ||
		{ echo "Error: labels request failed" >&2; exit 1; }
	jq -r '.data[]? | select(. != "__name__")' <<<"$resp"
}

# _names_json REGEX -> raw JSON of metric-name values whose __name__ matches the
# (RE2, auto-anchored) REGEX, scoped to the lookback window.
_names_json() {
	retry_curl -G "${CURL_EXTRA[@]}" "${API}/label/__name__/values" \
		--data-urlencode "start=$START" --data-urlencode "end=$NOW" \
		--data-urlencode "match[]={__name__=~\"$1\"}"
}

# NAMES_FILE accumulates the complete metric-name list; EFFLIMIT is the server's
# enforced label-values result cap (0 = none). A query returning exactly EFFLIMIT
# names is truncated (e.g. Mimir's querier.max-label-values-limit=10000) and must
# be partitioned further.
NAMES_FILE=""
EFFLIMIT=0

# explore REGEX_PREFIX -> append every metric name whose start matches the
# char-class prefix REGEX_PREFIX (a sequence of [..] atoms) to NAMES_FILE,
# recursively splitting the next character into four complete, disjoint classes
# when the server truncates the result. The classes cover every possible leading
# rune (including all multi-byte UTF-8), so no name — ASCII or not — is missed.
explore() {
	local re=$1 j len
	j=$(_names_json "${re}.*")
	len=$(jq '.data|length' <<<"$j")
	if [ "$EFFLIMIT" -eq 0 ] || [ "$len" -lt "$EFFLIMIT" ]; then
		jq -r '.data[]?' <<<"$j" >>"$NAMES_FILE"; return
	fi
	if [ "${#re}" -gt 80 ]; then
		echo "  warning: name partition too deep at /${re}/; some names may be missed" >&2
		jq -r '.data[]?' <<<"$j" >>"$NAMES_FILE"; return
	fi
	# Truncated: emit names of exactly this length, then recurse on the next char.
	_names_json "${re}" | jq -r '.data[]?' >>"$NAMES_FILE"
	local c
	for c in '[a-z]' '[A-Z]' '[0-9]' '[^a-zA-Z0-9]'; do explore "${re}${c}"; done
}

# collect_names FILE -> write the complete, sorted metric-name list to FILE,
# partitioning around the server's result cap only when it is actually hit.
collect_names() {
	NAMES_FILE=$1; : >"$NAMES_FILE"
	local probe len
	probe=$(_names_json ".+")
	len=$(jq '.data|length' <<<"$probe")
	EFFLIMIT=$(jq -r 'first((.warnings // [])[] | capture("enforced: (?<n>[0-9]+)").n) // "0"' <<<"$probe")
	if [ "$EFFLIMIT" -eq 0 ] || [ "$len" -lt "$EFFLIMIT" ]; then
		jq -r '.data[]?' <<<"$probe" >>"$NAMES_FILE"
	else
		echo "  metric-name list capped at ${EFFLIMIT}; enumerating by UTF-8-safe partition ..." >&2
		local c
		for c in '[a-z]' '[A-Z]' '[0-9]' '[^a-zA-Z0-9]'; do explore "$c"; done
	fi
	LC_ALL=C sort -u "$NAMES_FILE" -o "$NAMES_FILE"
}

echo "Querying ${PROM_URL} ..." >&2

# Preflight: fail fast with a clear message if the endpoint is unreachable (e.g.
# a dead kubectl port-forward), instead of grinding through TIMEOUT*RETRIES on
# every subsequent request.
if ! curl -sS -m 10 -o /dev/null -G "${CURL_EXTRA[@]}" \
	--data-urlencode "query=1" "${API}/query" 2>/dev/null; then
	echo "Error: cannot reach ${API}/query (10s). Check the URL, credentials, and" >&2
	echo "       that any port-forward/tunnel is up, then retry." >&2
	exit 1
fi

# Temp files (single cleanup trap).
names_all=$(mktemp); meta_file=$(mktemp); meta_hist=$(mktemp); meta_all=$(mktemp)
cand_file=$(mktemp); detect_file=$(mktemp); detfail_file=$(mktemp); worklist=$(mktemp)
buckets_file=$(mktemp); fails_file=$(mktemp)
trap 'rm -f "$names_all" "$meta_file" "$meta_hist" "$meta_all" "$cand_file" "$detect_file" "$detfail_file" "$worklist" "$buckets_file" "$fails_file"' EXIT

# 1) Total number of series (with a sample in the lookback window).
total_series=$(q_scalar 'count({__name__=~".+"})')

# 2) Enumerate ALL metric names (partitioning around any server-side result cap,
#    UTF-8 safe), then find native histograms. To avoid a query per name on large
#    tenants, use /metadata to shortlist histogram-typed metrics; ADDITIONALLY
#    scan any name that has no metadata (metadata may be incomplete/capped), so
#    the only names trusted as non-native-histogram are those metadata explicitly
#    types as something else. count(histogram_count({__name__="X"})) then confirms
#    each candidate and yields its NH series count (0/empty if not a native
#    histogram; classic-histogram base names have no series and drop out).
collect_names "$names_all"
n_names=$(wc -l <"$names_all")

retry_curl -G "${CURL_EXTRA[@]}" "${API}/metadata" >"$meta_file" 2>/dev/null || : >"$meta_file"
jq -r '(.data // {}) | to_entries[] | select(any(.value[]?; .type=="histogram" or .type=="gaugehistogram")) | .key' "$meta_file" 2>/dev/null | LC_ALL=C sort -u >"$meta_hist"
jq -r '(.data // {}) | keys[]?' "$meta_file" 2>/dev/null | LC_ALL=C sort -u >"$meta_all"

# Candidates = histogram-typed names that exist as series  UNION  names lacking metadata.
comm -12 "$meta_hist" "$names_all" >"$cand_file"
comm -23 "$names_all" "$meta_all" >>"$cand_file"
LC_ALL=C sort -u "$cand_file" -o "$cand_file"
n_cand=$(wc -l <"$cand_file")
n_nometa=$(comm -23 "$names_all" "$meta_all" | wc -l)
echo "Names: ${n_names} total; scanning ${n_cand} candidate(s) (${n_nometa} without metadata) for native histograms (${JOBS} concurrent) ..." >&2

# Emits "<nh_series_count>\t<name>" for native-histogram metrics. Runs in a
# background subshell; inherits CURL_EXTRA/API and the helper functions. A
# transient failure that exhausts retries is recorded (detfail_file) rather than
# silently dropping a potential native histogram from the counts.
detect_one() {
	local name="$1" sel body cnt
	sel="{$(labelmatcher __name__ "$name")}"
	if ! body=$(do_query "count(histogram_count(${sel}))"); then
		printf '%s\n' "$name" >>"$detfail_file"
		echo "detect failed (after retries): $name" >&2
		return 0
	fi
	cnt=$(jq -r 'if .status=="success" then (.data.result[0].value[1] // empty) else empty end' <<<"$body" 2>/dev/null || true)
	[ -n "${cnt:-}" ] && printf '%s\t%s\n' "$cnt" "$name"
	return 0
}

active=0 scanned=0
while IFS= read -r name; do
	[ -z "$name" ] && continue
	detect_one "$name" >>"$detect_file" &
	active=$((active + 1)); scanned=$((scanned + 1))
	if [ "$active" -ge "$JOBS" ]; then wait -n 2>/dev/null || wait; active=$((active - 1)); fi
	[ $((scanned % 200)) -eq 0 ] && printf '  scanned %d/%d\r' "$scanned" "$n_cand" >&2
done <"$cand_file"
wait
printf '  scanned %d/%d   \n' "$n_cand" "$n_cand" >&2

nh_series=$(jq -Rrn '[inputs | split("\t")[0] | tonumber] | add // 0' "$detect_file" 2>/dev/null || echo 0)

# 3a) Build the flat list of leaf selectors to fetch. Small metrics contribute
#     one leaf; metrics larger than SHARD_MAX are partitioned by a label.
declare -A CARD              # Per-metric: label -> value cardinality.

# choose_label TARGET LABEL... -> label whose cardinality is the smallest that is
# still >= TARGET (fewest shards that stay under SHARD_MAX); falls back to the
# highest-cardinality label. Empty if nothing can split (all cardinalities <= 1).
choose_label() {
	local target=$1; shift
	local best="" bestc=0 pick="" pickc=0 L cc
	for L in "$@"; do
		cc=${CARD[$L]:-0}
		[ "$cc" -le 1 ] && continue
		if [ "$cc" -gt "$bestc" ]; then bestc=$cc; best=$L; fi
		if [ "$cc" -ge "$target" ] && { [ -z "$pick" ] || [ "$cc" -lt "$pickc" ]; }; then
			pick=$L; pickc=$cc
		fi
	done
	[ -n "$pick" ] && { printf '%s' "$pick"; return; }
	printf '%s' "$best"
}

# expand SELECTOR LABEL... -> append bounded leaf selectors for SELECTOR to the
# worklist, recursively splitting on the given candidate labels as needed.
expand() {
	local sel=$1; shift
	local c; c=$(q_scalar "count(histogram_count(${sel}))")
	if [ "$c" -le "$SHARD_MAX" ] || [ $# -eq 0 ]; then
		printf '%s\n' "$sel" >>"$worklist"; return
	fi
	local target=$(((c + SHARD_MAX - 1) / SHARD_MAX))
	local L; L=$(choose_label "$target" "$@")
	if [ -z "$L" ]; then printf '%s\n' "$sel" >>"$worklist"; return; fi
	local rest=() x
	for x in "$@"; do [ "$x" != "$L" ] && rest+=("$x"); done
	local vals=() v
	mapfile -t vals < <(q_label_values "$L" "$sel")
	for v in "${vals[@]}"; do
		expand "$(add_matcher "$sel" "$L" "$v")" "${rest[@]}"
	done
	# Catch-all shard: series that do not have label L at all.
	expand "$(add_matcher "$sel" "$L" "")" "${rest[@]}"
}

n_nh=0 n_sharded=0
while IFS=$'\t' read -r cnt name; do
	[ -z "$name" ] && continue
	n_nh=$((n_nh + 1))
	sel="{$(labelmatcher __name__ "$name")}"
	if [ "${cnt%.*}" -le "$SHARD_MAX" ]; then
		printf '%s\n' "$sel" >>"$worklist"
	else
		n_sharded=$((n_sharded + 1))
		CARD=()
		while IFS= read -r lbl; do
			[ -z "$lbl" ] && continue
			CARD[$lbl]=$(q_label_values "$lbl" "$sel" | wc -l)
		done < <(q_label_names "$sel")
		mapfile -t cand < <(printf '%s\n' "${!CARD[@]}")
		expand "$sel" "${cand[@]}"
	fi
done <"$detect_file"

n_leaves=$(wc -l <"$worklist")
echo "Native-histogram metrics: ${n_nh} (${n_sharded} sharded) -> ${n_leaves} fetch(es)" >&2

# 3b) Fetch each leaf selector concurrently and sum bucket counts. Float series
#     (if any share the name) have .value not .histogram and contribute nothing.
#     Failures are classified so the total is an honest lower bound, never a
#     silent undercount: "perm" = the server refuses the query (e.g. Mimir
#     "aggregated metrics"); "trans" = transient failure that survived all
#     retries. Both are recorded in fails_file with a leading tag.
fetch_leaf() {
	local sel="$1" body b err
	if body=$(do_query "$sel"); then
		if [ "$(jq -r '.status // "err"' <<<"$body" 2>/dev/null)" = "success" ]; then
			b=$(jq -r '[.data.result[]? | (.histogram[1].buckets // []) | length] | add // 0' <<<"$body" 2>/dev/null)
			printf '%s\n' "${b:-0}"; return
		fi
		err=$(jq -r '.error // .errorType // "error"' <<<"$body" 2>/dev/null | head -c 200)
		printf 'perm\t%s\n' "$sel" >>"$fails_file"
		echo "leaf query error (permanent): $sel -> $err" >&2
		printf '0\n'; return
	fi
	printf 'trans\t%s\n' "$sel" >>"$fails_file"
	echo "leaf request failed after ${RETRIES} attempts: $sel" >&2
	printf '0\n'
}

active=0 done_leaves=0
while IFS= read -r sel; do
	[ -z "$sel" ] && continue
	fetch_leaf "$sel" >>"$buckets_file" &
	active=$((active + 1)); done_leaves=$((done_leaves + 1))
	if [ "$active" -ge "$JOBS" ]; then wait -n 2>/dev/null || wait; active=$((active - 1)); fi
	[ $((done_leaves % 50)) -eq 0 ] && printf '  fetched %d/%d\r' "$done_leaves" "$n_leaves" >&2
done <"$worklist"
wait
[ "$n_leaves" -gt 0 ] && printf '  fetched %d/%d   \n' "$n_leaves" "$n_leaves" >&2

total_buckets=$(awk '{s+=$1} END{print s+0}' "$buckets_file")
n_perm=$(awk -F'\t' '$1=="perm"{c++} END{print c+0}' "$fails_file")
n_trans=$(awk -F'\t' '$1=="trans"{c++} END{print c+0}' "$fails_file")
n_detfail=$(wc -l <"$detfail_file"); n_detfail=$((n_detfail + 0))

echo
echo "Prometheus:                 ${PROM_URL}"
echo "Total series:               ${total_series}"
if [ "$n_detfail" -gt 0 ]; then
	echo "Native histogram series:    ~${nh_series}  (>= ; ${n_detfail} name(s) undetermined)"
	echo "  note: ${n_detfail} candidate(s) could not be checked after ${RETRIES} retries; increase"
	echo "        RETRIES/TIMEOUT or lower -j. See stderr for the affected names."
else
	echo "Native histogram series:    ${nh_series}"
fi
if [ $((n_perm + n_trans)) -gt 0 ]; then
	echo "Native histogram buckets:   >= ${total_buckets}  (lower bound)"
	[ "$n_perm" -gt 0 ] && echo "  note: ${n_perm} metric(s) reject raw selection (e.g. Mimir aggregated metrics);"
	[ "$n_perm" -gt 0 ] && echo "        their series are counted above but their buckets cannot be fetched."
	[ "$n_trans" -gt 0 ] && echo "  note: ${n_trans} fetch(es) still failed after ${RETRIES} retries; increase"
	[ "$n_trans" -gt 0 ] && echo "        RETRIES/TIMEOUT or lower -j. See stderr for the affected selectors."
else
	echo "Native histogram buckets:   ${total_buckets}"
fi
