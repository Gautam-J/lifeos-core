#!/bin/bash
# Writes the undervoltage and throttle flags of the Pi for the textfile
# collector of node_exporter. lifeos-throttled.timer runs it each minute.
# The bits of `vcgencmd get_throttled`: 0 to 3 are the state now, 16 to 19
# the same flags since the boot.
# Usage: pi-throttled.sh <textfile-dir>
set -euo pipefail

dir=$1
out=$dir/pi_throttled.prom

# A value that cannot be read must not stay in the file as an old good value.
fail() {
    rm -f "$out"
    echo "pi-throttled: $1" >&2
    exit 1
}

answer=$(vcgencmd get_throttled) || fail "vcgencmd failed"
[[ $answer =~ ^throttled=(0x[0-9a-fA-F]+)$ ]] || fail "unexpected answer: $answer"
value=$((BASH_REMATCH[1]))

flags=(under_voltage arm_frequency_capped throttled soft_temperature_limit)
tmp=$(mktemp "$dir/.pi_throttled.XXXXXX")
{
    echo "# HELP pi_throttled A flag of vcgencmd get_throttled that is set now."
    echo "# TYPE pi_throttled gauge"
    for i in "${!flags[@]}"; do
        echo "pi_throttled{flag=\"${flags[$i]}\"} $(((value >> i) & 1))"
    done
    echo "# HELP pi_throttled_since_boot A flag of vcgencmd get_throttled that was set since the boot."
    echo "# TYPE pi_throttled_since_boot gauge"
    for i in "${!flags[@]}"; do
        echo "pi_throttled_since_boot{flag=\"${flags[$i]}\"} $(((value >> (i + 16)) & 1))"
    done
} > "$tmp"
chmod 644 "$tmp"
# node_exporter must never read a file that is half written.
mv "$tmp" "$out"
