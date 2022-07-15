# Development Notes

```bash
# on development machine
make build OS=linux ARCH=amd64
scp bin/gh-ci-webhook-linux-amd64 root@gh-ci-webhook.appscode.com:/root


# on production server
> ssh root@gh-ci-webhook.appscode.com

chmod +x gh-ci-webhook-linux-amd64
mv gh-ci-webhook-linux-amd64 /usr/local/bin/gh-ci-webhook
sudo systemctl restart gh-ci-webhook
```
