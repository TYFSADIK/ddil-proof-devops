#!/bin/sh
# Emulate a DDIL link on the edge NIC with Linux netem. Usage: sudo ./tc-degrade.sh eth0 [clear]
IF=${1:-eth0}
tc qdisc del dev "$IF" root 2>/dev/null
[ "$2" = "clear" ] && exit 0
# 256 kbit/s, 600 ms latency +-200, 5% loss, 1% corruption
tc qdisc add dev "$IF" root netem rate 256kbit delay 600ms 200ms loss 5% corrupt 1%
echo "degraded $IF"; tc qdisc show dev "$IF"
