# Development Notes

```bash
# on development machine
make build OS=linux ARCH=amd64
scp bin/gh-ci-webhook-linux-amd64 root@this-is-nats.appscode.ninja:/root


# on production server
> ssh root@this-is-nats.appscode.ninja

chmod +x gh-ci-webhook-linux-amd64
mv gh-ci-webhook-linux-amd64 /usr/local/bin/gh-ci-webhook
systemctl restart gh-ci-webhook
```

```bash
RUNNER_HOST_IP=86.109.9.7

scp bin/gh-ci-webhook-linux-amd64 root@$RUNNER_HOST_IP:/root

> ssh root@$RUNNER_HOST_IP

chmod +x gh-ci-webhook-linux-amd64
mv gh-ci-webhook-linux-amd64 /usr/local/bin/gh-ci-webhook
systemctl restart gh-ci-hostctl
```
