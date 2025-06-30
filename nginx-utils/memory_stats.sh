#!/usr/bin/env bash
set -e
echo ""
echo " **** Output of memory.stat ****"
cat /sys/fs/cgroup/memory.stat

echo ""
echo " **** Output of pmap for nginx and nginx-ingress processes ****"
for p in $(pidof nginx nginx-ingress); do pmap ${p} -x; done

echo ""
echo " **** Output of /proc/pid/status for nginx and nginx-ingress processes ****"
for p in $(pidof nginx nginx-ingress); do cat /proc/${p}/status; done