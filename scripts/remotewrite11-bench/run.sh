#!/bin/sh
set -e
trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

# CONFIGURABLES
# ~~~~~~~~~~~~~
declare -a INSTANCES
# (sender,receiver) pairs to run: (sender_name; sender_flags; receiver_name; receiver_flags)
INSTANCES+=('sender-v1;;receiver-v1;')
INSTANCES+=('sender-v11;--enable-feature rw-1-1-sender;receiver-v11;--enable-feature rw-1-1-receiver')


# ~~~~~~~~~~~~~

# append two ports to all instances
PROM_PORT=9090
for i in "${!INSTANCES[@]}"; do
  INSTANCES[$i]="${INSTANCES[$i]};$PROM_PORT"
  PROM_PORT=$((PROM_PORT+1))
  INSTANCES[$i]="${INSTANCES[$i]};$PROM_PORT"
  PROM_PORT=$((PROM_PORT+1))
done

# Check if all required variables are set
: "${CONTEXT:?}"
: "${NAMESPACE:?}"

TEMP_DIR=$(mktemp -d)
echo "Working on dir: $TEMP_DIR"
SCRAPE_CONFIGS=""
declare -a COMMANDS
PORT=40000
# Get all pods from namespace, create scrape configs and port forward them
while read pod ; do
  COMMANDS+=("kubectl --context $CONTEXT -n $NAMESPACE port-forward $pod $PORT:http-metrics")
  SCRAPE_CONFIGS="${SCRAPE_CONFIGS}
  - job_name: '$pod'
    static_configs:
      - targets: ['localhost:$PORT']"

  PORT=$((PORT+1))
done < <(kubectl --context $CONTEXT -n $NAMESPACE get pods | awk '{print $1}' | tail +2 )


# Run all prometheus instances and add them to the scrape configs
for instance in "${INSTANCES[@]}"
do
  IFS=";" read -r -a arr <<< "${instance}"

  sender="${arr[0]}"
  sender_flags="${arr[1]}"
  receiver="${arr[2]}"
  receiver_flags="${arr[3]}"
  sender_port="${arr[4]}"
  receiver_port="${arr[5]}"

  SCRAPE_CONFIGS="${SCRAPE_CONFIGS}
  - job_name: '$sender'
    static_configs:
      - targets: ['localhost:$sender_port']
  - job_name: '$receiver'
    static_configs:
      - targets: ['localhost:$receiver_port']"

  COMMANDS+=("./prometheus --config.file=$TEMP_DIR/$sender.yml --web.listen-address=0.0.0.0:$sender_port --storage.tsdb.path=$TEMP_DIR/tsdb/$sender/data/ $sender_flags 2>&1 | awk '{print \"[$sender]\",\$0}'")
  COMMANDS+=("./prometheus --config.file=$TEMP_DIR/$receiver.yml --web.listen-address=0.0.0.0:$receiver_port --storage.tsdb.path=$TEMP_DIR/tsdb/$receiver/data/ $receiver_flags --web.enable-remote-write-receiver 2>&1 | awk '{print \"[$receiver]\",\$0}'")
done

# Create the yml configs for the prometheus instances
for instance in "${INSTANCES[@]}"
do
  IFS=";" read -r -a arr <<< "${instance}"
  export SENDER_NAME="${arr[0]}"
  export RECEIVER_NAME="${arr[2]}"
  export REMOTE_WRITE_PORT="${arr[5]}"
  export SCRAPE_CONFIGS="$SCRAPE_CONFIGS"
  envsubst < receiver-template.yml > $TEMP_DIR/$RECEIVER_NAME.yml
  envsubst < sender-template.yml > $TEMP_DIR/$SENDER_NAME.yml
done

# Actually run all commands
for cmd in "${COMMANDS[@]}"
do
  eval $cmd &
done

echo Running...
read -r -d '' _ </dev/tty
