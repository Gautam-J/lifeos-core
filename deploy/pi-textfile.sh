#!/bin/bash
# Writes facts of the Pi for the textfile collector of node_exporter, which
# runs in a container and cannot see them. lifeos-textfile.timer runs it
# each minute.
#
# pi_throttled.prom  the bits of `vcgencmd get_throttled`: 0 to 3 are the
#                    state now, 16 to 19 the same flags since the boot.
# pi_host.prom       the platform and the private IP address, named as the
#                    sysinfo service (gopsutil) names them.
#
# Each file is written on its own: a failure of one does not stop the other.
# Usage: pi-textfile.sh <textfile-dir>
set -euo pipefail

dir=$1
debian_version=${DEBIAN_VERSION:-/etc/debian_version}
raspi_config=${RASPI_CONFIG:-/usr/bin/raspi-config}
status=0

# write <file>: the lines of stdin become the file. node_exporter must never
# read a file that is half written.
write() {
    local tmp
    tmp=$(mktemp "$dir/.$1.XXXXXX")
    cat > "$tmp"
    chmod 644 "$tmp"
    mv "$tmp" "$dir/$1"
}

# A value that cannot be read must not stay in the file as an old good value.
fail() {
    rm -f "$dir/$1"
    echo "pi-textfile: $2" >&2
    status=1
}

throttled() {
    local answer value i
    local flags=(under_voltage arm_frequency_capped throttled soft_temperature_limit)
    answer=$(vcgencmd get_throttled) || { fail pi_throttled.prom "vcgencmd failed"; return; }
    [[ $answer =~ ^throttled=(0x[0-9a-fA-F]+)$ ]] || { fail pi_throttled.prom "unexpected answer: $answer"; return; }
    value=$((BASH_REMATCH[1]))
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
    } | write pi_throttled.prom
}

host() {
    local platform=debian version ip_line interface address
    [ -e "$raspi_config" ] && platform=raspbian
    version=$(cat "$debian_version") || { fail pi_host.prom "cannot read $debian_version"; return; }
    # The first global IPv4 address, as gopsutil takes the first address
    # that is not a loopback address.
    ip_line=$(ip -4 -o addr show scope global | head -1)
    read -r _ interface _ address _ <<< "$ip_line"
    address=${address%%/*}
    {
        echo "# HELP pi_platform_info The platform of the Pi, as gopsutil names it."
        echo "# TYPE pi_platform_info gauge"
        echo "pi_platform_info{platform=\"$platform\",platform_version=\"$version\"} 1"
        if [ -n "$address" ]; then
            echo "# HELP pi_private_ip_info The first global IPv4 address of the Pi."
            echo "# TYPE pi_private_ip_info gauge"
            echo "pi_private_ip_info{interface=\"$interface\",address=\"$address\"} 1"
        fi
    } | write pi_host.prom
}

host
throttled
exit "$status"
