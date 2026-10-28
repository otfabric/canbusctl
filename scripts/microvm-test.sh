#!/usr/bin/env bash
# SPDX-License-Identifier: MIT
#
# microvm-test.sh: run canbusctl's vcan, kernel ISO-TP, bridge and
# end-to-end tests in a microVM booted with a chosen kernel (virtme-ng on
# QEMU/KVM) instead of the host kernel. The guest shares the host's file
# system read-only, so the binaries built on the host run inside unchanged.
#
#   scripts/microvm-test.sh <vmlinuz> <root containing lib/modules/<release>>
set -euo pipefail
kernel=$1 moddir=$2
out=$(mktemp -d)
GOWORK=off go build -o "$out/canbusctl" .
GOWORK=off go test -c -o "$out/cmd.test" ./cmd
GOWORK=off go test -c -o "$out/e2e.test" ./e2e
cat > "$out/guest.sh" <<GUEST
set -e
uname -a
modprobe -a can can-raw vcan can-isotp slcan
for i in vcan0 vcan1; do ip link add \$i type vcan && ip link set \$i mtu 72 up; done
export CANBUS_REQUIRE_VCAN=1 CANBUS_REQUIRE_ISOTP=1 TZ=UTC
# CAN XL exists from Linux 6.2 on: required there, absent before.
if [ "\$(printf '%s\n6.2\n' "\$(uname -r | cut -d- -f1)" | sort -V | head -1)" = 6.2 ]; then
  ip link add vcanxl0 type vcan && ip link set vcanxl0 mtu 2060 up && export CANBUS_REQUIRE_XL=1
fi
cd $PWD/cmd && $out/cmd.test -test.count=1 -test.run 'VCAN'
cd $PWD/e2e && CANBUS_TEST_ADMIN=1 CANBUS_REQUIRE_SLCAN=1 CANBUSCTL_BIN=$out/canbusctl $out/e2e.test -test.count=1 -test.short -test.run 'VCAN|Chaos|XL'
cd $PWD/e2e && CANBUS_TEST_ADMIN=1 CANBUS_REQUIRE_SLCAN=1 CANBUSCTL_BIN=$out/canbusctl $out/e2e.test -test.count=1 -test.v -test.run 'Slcan'
echo MICROVM-OK
GUEST
start=$(date +%s)
vng --run "$kernel" --root / --user root --memory 2G --cpus 2 \
    --rwdir "$out" \
    --exec "MODPROBE_OPTIONS='-d $moddir' bash $out/guest.sh" | tee "$out/log"
echo "microVM run took $(( $(date +%s) - start )) s"
grep -q MICROVM-OK "$out/log"
