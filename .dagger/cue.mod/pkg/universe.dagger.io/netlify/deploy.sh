#!/bin/bash

set -e -o pipefail

NETLIFY_AUTH_TOKEN="$(cat /run/secrets/token)"
export NETLIFY_AUTH_TOKEN

create_site() {
	netlify api createSite -d "{ \"body\": {\"name\": \"${NETLIFY_SITE_NAME}\", \"custom_domain\": \"${NETLIFY_DOMAIN}\"} }" > body

	# shellcheck disable=SC2181
	if [ $? -ne 0 ]; then
		>&2 echo "Error creating site [${NETLIFY_SITE_NAME}] for account [${NETLIFY_ACCOUNT}]"
		cat body >&2
		exit 1
	fi

	jq -r '.site_id' body
}

site_id=$(netlify api listSites | jq -r ".[] | select(.name==\"${NETLIFY_SITE_NAME}\") | .id")
if [ -z "$site_id" ]; then
	if [ "${NETLIFY_SITE_CREATE:-}" != 1 ]; then
		echo "Site $NETLIFY_SITE_NAME does not exist"
		exit 1
	fi
	site_id=$(create_site)
	if [ -z "$site_id" ]; then
		echo "create site failed"
		exit 1
	else
		echo "clean create site API response..."
		rm -f body
	fi
fi

netlify link --id "$site_id"

netlify deploy \
	--build \
	--site="$site_id" \
	--prod |
	tee /tmp/stdout

url="$(grep </tmp/stdout Website | grep -Eo 'https://[^ >]+' | head -1)"
deployUrl="$(grep </tmp/stdout Unique | grep -Eo 'https://[^ >]+' | head -1)"
logsUrl="$(grep </tmp/stdout Logs | grep -Eo 'https://[^ >]+' | head -1)"

# Write output files
mkdir -p /netlify
echo -n "$url" >/netlify/url
echo -n "$deployUrl" >/netlify/deployUrl
echo -n "$logsUrl" >/netlify/logsUrl
