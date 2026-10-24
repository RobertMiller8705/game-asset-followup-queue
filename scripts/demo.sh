#!/bin/sh
set -eu

curl --fail-with-body -X POST http://localhost:8080/assets \
  -H 'Content-Type: application/json' \
  -d '{"asset_id":"skin-482","player_id":"player-19","event_id":"summer-cup","delay_hours":6}'
curl --fail-with-body -X POST http://localhost:8080/work/run
