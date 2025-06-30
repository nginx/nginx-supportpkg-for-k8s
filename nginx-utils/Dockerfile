FROM alpine:latest

LABEL org.opencontainers.image.description="Container image including various troubleshooting tools such as curl, tcpdump, iperf, netcat to name a few not available in the target container"
LABEL org.opencontainers.image.vendor="NGINX"
LABEL org.opencontainers.image.authors="NGINX <plus-support@nginx.com>"
LABEL org.opencontainers.image.url="https://github.com/nginx/nginx-supportpkg-for-k8s/pkgs/container/nginx-utils"
LABEL org.opencontainers.image.documentation="https://github.com/nginx/nginx-supportpkg-for-k8s/pkgs/container/nginx-utils"
LABEL org.opencontainers.image.source="https://github.com/nginx/nginx-supportpkg-for-k8s/tree/main/nginx-utils"
LABEL org.opencontainers.image.licenses="Apache-2.0"

COPY --chmod=744 nginx-utils/api_stats.sh /root/api_stats.sh
COPY --chmod=744 nginx-utils/memory_stats.sh /root/memory_stats.sh

RUN set -ex \
  && apk --update add --no-cache \
  bind-tools curl netcat-openbsd iproute2 \
  iperf tcpdump tshark bash jq \
  && rm -rf /var/cache/apk/* \
  && ln -s /usr/bin/iperf /usr/local/bin/iperf \
  && ls -altrh /usr/local/bin/iperf

# Setting User and Home
USER root
WORKDIR /root
ENV HOSTNAME=nginx-utils

CMD ["bash"]