#!/bin/bash

# --- Config ---
BASE_URL="https://apis.cscglobal.com/dbs/api/v2"

# --- Configurable Inputs ---
WORKDIR="${1:-.}"                # Default to current directory
ZONE="${2:-}"

cd "$WORKDIR" || {
  echo "Error: Could not change to directory: $WORKDIR"
  exit 1
}

# --- Check required environment variables ---
if [[ -z "$CSCDM_API_KEY" || -z "$CSCDM_API_TOKEN" ]]; then
  echo "Error: CSCDM_API_KEY and CSCDM_API_TOKEN environment variables must be set."
  exit 1
fi

# --- Headers ---
HEADERS=(
  -H "accept: application/json"
  -H "apikey: $CSCDM_API_KEY"
  -H "Authorization: Bearer $CSCDM_API_TOKEN"
)

# --- Fetch zones ---
zones=$(curl -sSL "${HEADERS[@]}" "$BASE_URL/zones" | jq -c '.zones[]')

# --- Filter for a specific zone if ZONE is set ---
if [[ -n "$ZONE" ]]; then
  zones=$(echo "$zones" | jq -c "select(.zoneName == \"$ZONE\")")
  if [[ -z "$zones" ]]; then
    echo "Zone not found: $ZONE"
    exit 1
  fi
fi

# --- Loop through zones ---
while read -r zone; do
  raw_zone_name=$(echo $zone | jq -r .zoneName)
  echo "Processing zone: $raw_zone_name"

  zone_name=${raw_zone_name//./_}

  # --- Reset output file ---
  > "${zone_name}.tf"

  for type in A AAAA CNAME MX NS TXT; do
    records=$(echo "$zone" | jq -c ".${type,,}[]")

    if [[ -n "$records" ]]; then
      while read -r record; do
        id=$(echo "$record" | jq -r '.id')
        key=$(echo "$record" | jq -r '.key')
        value=$(echo "$record" | jq -r '.value')
        ttl=$(echo "$record" | jq -r '.ttl // empty')
        priority=$(echo "$record" | jq -r '.priority // empty')

        name=${zone_name//./_}_${type}_$id

        {
          echo "resource \"cscdm_record\" \"$name\" {"
          echo "  zone_name = \"$raw_zone_name\""
          echo "  type      = \"$type\""
          echo "  key       = \"$key\""
          echo "  value     = \"$value\""
          [[ -n "$ttl" && "$ttl" -ne 0 ]] && echo "  ttl       = $ttl"
          [[ -n "$priority" && "$priority" -ne 0 ]] && echo "  priority  = $priority"
          echo "}"
          echo
        } >> "${zone_name}.tf"

        terraform import "cscdm_record.$name" "$raw_zone_name:$type:$id"

      done <<< "$records"
    fi
  done
done <<< "$zones"

echo "Terraform configuration written to $(realpath -s $WORKDIR)/$OUTFILE"
