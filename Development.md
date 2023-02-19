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

## build_rootfs

script gist: https://gist.github.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7

```
curl -L https://gist.githubusercontent.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7/raw/c1b3647e43a28d17d24cbcb2db2063d61bebf455/build_rootfs.sh | bash -s -- bionic 25G

curl -L https://gist.githubusercontent.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7/raw/c1b3647e43a28d17d24cbcb2db2063d61bebf455/build_rootfs.sh | bash -s -- focal 25G

curl -L https://gist.githubusercontent.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7/raw/c1b3647e43a28d17d24cbcb2db2063d61bebf455/build_rootfs.sh | bash -s -- jammy 25G
```

## Download firecracker binary

```
release_url="https://github.com/firecracker-microvm/firecracker/releases"
latest=$(basename $(curl -fsSLI -o /dev/null -w  %{url_effective} ${release_url}/latest))
arch=`uname -m`
curl -L ${release_url}/download/${latest}/firecracker-${latest}-${arch}.tgz \
| tar -xz
mv release-${latest}-$(uname -m)/firecracker-${latest}-$(uname -m) firecracker
```
