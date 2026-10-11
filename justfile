set shell := ["bash", "-euo", "pipefail", "-c"]

default:
    @just --list

gen-mocks dir="./...":
    go generate {{ dir }}

test dir="./...": gen-mocks
    go test -short {{ dir }}

auth credentials="credentials.json" token="token.json":
    GOOGLE_CREDENTIALS_FILE="{{ credentials }}" GOOGLE_TOKEN_FILE="{{ token }}" go run ./cmd/lunch-notification auth
    chmod 600 "{{ credentials }}" "{{ token }}"

pi-run-once host="raspberry" dir="~/apps/lunch-notification":
    ssh {{ host }} 'cd {{ dir }} && docker compose run --rm lunch-notification run-once'
