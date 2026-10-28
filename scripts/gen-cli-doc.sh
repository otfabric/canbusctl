#!/bin/sh
# SPDX-License-Identifier: MIT
# Regenerates docs/cli.md from the --help of every command.
set -eu
cd "$(dirname "$0")/.."
GOWORK=off go build -o bin/canbusctl .
cmds="interfaces info configure up down monitor send send-fd record replay stats
dbc dbc_inspect dbc_validate dbc_decode dbc_encode dbc_generate dbc_convert
isotp isotp_send isotp_receive isotp_monitor trace trace_convert trace_info
simulate inject bridge xl xl_send xl_monitor version completion"
{
	echo "# canbusctl command reference"
	echo
	echo "Generated from \`canbusctl <command> --help\` by scripts/gen-cli-doc.sh; do not edit."
	echo "Exit codes: [exitcodes.md](exitcodes.md). JSON output: [json-output.md](json-output.md)."
	echo
	echo '```text'
	bin/canbusctl --help
	echo '```'
	for c in $cmds; do
		args=$(echo "$c" | tr _ ' ')
		echo
		echo "## canbusctl $args"
		echo
		echo '```text'
		# shellcheck disable=SC2086
		bin/canbusctl $args --help
		echo '```'
	done
} > docs/cli.md
