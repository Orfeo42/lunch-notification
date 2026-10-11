set shell := ["bash", "-euo", "pipefail", "-c"]

default:
    @just --list

auth credentials="credentials.json" token="token.json":
    GOOGLE_CREDENTIALS_FILE="{{ credentials }}" GOOGLE_TOKEN_FILE="{{ token }}" go run ./cmd/lunch-notification auth
    chmod 600 "{{ credentials }}" "{{ token }}"
