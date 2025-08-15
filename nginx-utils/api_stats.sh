#!/usr/bin/env bash

# Parse command line options
set -e
set -o pipefail
while getopts "p:v:h" opt; do
  case $opt in
    p) API_PORT="$OPTARG"
    ;;
    h) echo "Usage: $0 [-p port]"
       exit 0
    ;;
    \?)  echo "Invalid option -$OPTARG" >&2
    echo "Usage: $0 [-p port]"
        exit 1
    ;;
  esac
done

if [ $OPTIND -eq 1 ]; then
  echo "No options were passed, exiting ..."
  echo "Usage: $(basename "$0") [-p port]"
  exit 1
fi

if [ -z "${API_PORT}" ]; then
  echo 'Missing -p arg' >&2
  exit 1
fi

api_versions=($(curl -s http://127.0.0.1:$API_PORT/api/ | sed -e  's/\[//g' -e 's/\]//g' -e 's/\,/ /g'))
API_VERSION=${api_versions[-1]}
echo "API_VERSION: $API_VERSION"

echo "**** /api/$API_VERSION/nginx ****" ;
curl -s "127.0.0.1:$API_PORT/api/$API_VERSION/nginx" | jq -r '.';
echo "";

for i in /api/$API_VERSION/processes /api/$API_VERSION/connections /api/$API_VERSION/slabs /api/$API_VERSION/http/requests /api/$API_VERSION/http/server_zones /api/$API_VERSION/http/location_zones /api/$API_VERSION/http/caches /api/$API_VERSION/http/upstreams /api/$API_VERSION/http/keyvals; do
  echo "**** $i ****" ;
  curl -s "127.0.0.1:$API_PORT/$i" | jq -r '.';
  echo "";
done