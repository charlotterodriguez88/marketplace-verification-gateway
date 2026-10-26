#!/bin/sh
set -eu

curl --fail-with-body --request POST "${PUBLIC_URL:-http://localhost:8080}/signup" \
  --header 'Content-Type: application/json' \
  --data '{"seller_id":"seller-42","email":"seller@example.com","assets":[{"sku":"lamp-1","title":"Desk lamp"}]}'
