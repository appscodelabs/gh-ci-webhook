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
curl -L https://gist.githubusercontent.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7/raw/9ed8c72a0307278a93ec15695b61103d1a1f8c14/build_rootfs.sh | bash -s -- bionic 100G

curl -L https://gist.githubusercontent.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7/raw/9ed8c72a0307278a93ec15695b61103d1a1f8c14/build_rootfs.sh | bash -s -- focal 100G

curl -L https://gist.githubusercontent.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7/raw/9ed8c72a0307278a93ec15695b61103d1a1f8c14/build_rootfs.sh | bash -s -- jammy 100G

curl -L https://gist.githubusercontent.com/tamalsaha/af2f99c80f84410253bd1e532bdfabc7/raw/9ed8c72a0307278a93ec15695b61103d1a1f8c14/build_rootfs.sh | bash -s -- noble 100G
```

## Download firecracker binary

```
release_url="https://github.com/firecracker-microvm/firecracker/releases"
latest=$(basename $(curl -fsSLI -o /dev/null -w  %{url_effective} ${release_url}/latest))
latest=v1.7.0
arch=`uname -m`
curl -L ${release_url}/download/${latest}/firecracker-${latest}-${arch}.tgz \
| tar -xz
mv release-${latest}-$(uname -m)/firecracker-${latest}-$(uname -m) firecracker
```

## Test VMs

```
hostctl --machine-id=0 \
--nats-addr=this-is-nats.appscode.ninja:4222 \
--provider=firecracker \
--firecracker.binary-path=/home/tamal/go/src/github.com/appscodelabs/gh-ci-webhook/fc/firecracker \
--firecracker.os=focal \
--firecracker.image-dir=/home/tamal/go/src/github.com/appscodelabs/gh-ci-webhook/fc/images

hostctl --machine-id=0 \
--nats-addr=this-is-nats.appscode.ninja:4222 \
--provider=dummy

firecracker create-vm \
--firecracker.binary-path=/home/tamal/go/src/github.com/appscodelabs/gh-ci-webhook/fc/firecracker \
--firecracker.os=focal \
--firecracker.image-dir=/home/tamal/go/src/github.com/appscodelabs/gh-ci-webhook/fc/images

sudo /home/tamal/go/bin/gh-ci-webhook firecracker create-tap --name=tap100
ifconfig
```

## Configure label detector

```
apt update
apt upgrade
apt install curl jq

export USER=bytebuilders # GitHub Org name

adduser --disabled-password --gecos "" $USER
usermod -aG sudo $USER
rsync --archive --chown=$USER:$USER ~/.ssh /home/$USER
echo "$USER ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers


su $USER
cd /home/$USER

export RUNNER_OWNER=$USER
export RUNNER_CFG_PAT=***
export RUNNER_NAME=gha-label-detector-0

curl -s https://raw.githubusercontent.com/actions/runner/main/scripts/create-latest-svc.sh | bash -s -- -s ${RUNNER_OWNER} -n ${RUNNER_NAME} -f -l label-detector
```

## Update Webhook Host

```
curl -fsSL -O https://github.com/appscodelabs/gh-ci-webhook/releases/download/v0.0.21/gh-ci-webhook-linux-amd64.tar.gz
tar -xzvf gh-ci-webhook-linux-amd64.tar.gz
chmod +x gh-ci-webhook-linux-amd64
mv gh-ci-webhook-linux-amd64 /usr/local/bin/gh-ci-webhook

systemctl stop gh-ci-webhook
```

## Update Worker hosts

```
curl -fsSL -O https://github.com/appscodelabs/gh-ci-webhook/releases/download/v0.0.21/gh-ci-webhook-linux-amd64.tar.gz
tar -xzvf gh-ci-webhook-linux-amd64.tar.gz
chmod +x gh-ci-webhook-linux-amd64
mv gh-ci-webhook-linux-amd64 /usr/local/bin/gh-ci-webhook
systemctl restart gh-ci-hostctl
```
