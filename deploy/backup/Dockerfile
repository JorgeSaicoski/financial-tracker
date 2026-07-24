# Sidecar container with cron (not a host systemd timer) — consistent
# with this whole stack being defined in compose.yaml alone (podman-compose
# over `podman kube play`, per INFRA-01's own reasoning): one mechanism,
# no separate host-level unit files to keep in sync with the compose file.
FROM docker.io/library/alpine:3.20

RUN apk add --no-cache postgresql16-client tzdata

COPY backup.sh /usr/local/bin/backup.sh
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/backup.sh /usr/local/bin/entrypoint.sh

# Daily at 03:00 UTC. Output redirected to the container's own stdout
# (PID 1, which crond -f becomes) so `podman logs` captures every run,
# including failures — see deploy/README.md's Backups section.
RUN echo "0 3 * * * /usr/local/bin/backup.sh >>/proc/1/fd/1 2>>/proc/1/fd/2" > /etc/crontabs/root

VOLUME /backups

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
