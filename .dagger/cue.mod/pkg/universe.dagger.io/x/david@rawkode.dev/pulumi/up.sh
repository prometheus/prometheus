#!/usr/bin/env bash
set -xeo pipefail

if test -v PULUMI_CONFIG_PASSPHRASE || test -v PULUMI_CONFIG_PASSPHRASE_FILE; then
  echo "PULUMI_CONFIG_PASSPHRASE is set, using a local login"
  pulumi login --local
fi

# Using Pulumi SaaS
# We need to check for an existing stack with the name
# If it exists, refresh the config
# If it doesn't, create the stack
if test -v PULUMI_ACCESS_TOKEN; then
  if (pulumi stack ls | grep -e "^${PULUMI_STACK}"); then
    echo "Stack exists, let's refresh"
    pulumi stack select "${PULUMI_STACK}"
    # Could be first deployment, so let's not worry about this failing
    pulumi config refresh --force || true
  else
    echo "Stack does not exist, let's create"
    pulumi stack init "${PULUMI_STACK}"
  fi
else
  # Not using Pulumi SaaS, relying on local stack files
  if test -v PULUMI_STACK_CREATE && test ! -f "Pulumi.${PULUMI_STACK}.yaml"; then
    pulumi stack init "${PULUMI_STACK}"
  fi
fi

case "$PULUMI_RUNTIME" in
  nodejs)
    npm install
    ;;

  *)
    echo -n "unknown"
    ;;
esac

pulumi up --stack "${PULUMI_STACK}" --yes

pulumi stack output -j > /outputs.json || echo '{}' > /outputs.json
