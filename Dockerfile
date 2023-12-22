FROM openeuler/openeuler:23.03 as BUILDER
RUN dnf update -y && \
    dnf install -y golang && \
    go env -w GOPROXY=https://goproxy.cn,direct

MAINTAINER zengchen1024<chenzeng765@gmail.com>

# build binary
WORKDIR /go/src/github.com/opensourceways/robot-gitee-scavenger
COPY . .
RUN GO111MODULE=on CGO_ENABLED=0 go build -a -o robot-gitee-scavenger -buildmode=pie --ldflags "-s -linkmode 'external' -extldflags '-Wl,-z,now'" .

# copy binary config and utils
FROM openeuler/openeuler:22.03
RUN dnf -y update && \
    dnf in -y shadow && \
    dnf remove -y gdb-gdbserver && \
    groupadd -g 1000 scavenger && \
    useradd -u 1000 -g scavenger -s /sbin/nologin -m scavenger && \
    echo > /etc/issue && echo > /etc/issue.net && echo > /etc/motd && \
    mkdir /home/scavenger -p && \
    chmod 700 /home/scavenger && \
    chown scavenger:scavenger /home/scavenger && \
    echo 'set +o history' >> /root/.bashrc && \
    sed -i 's/^PASS_MAX_DAYS.*/PASS_MAX_DAYS   90/' /etc/login.defs && \
    rm -rf /tmp/*

USER scavenger

WORKDIR /opt/app

COPY  --chown=scavenger --from=BUILDER /go/src/github.com/opensourceways/robot-gitee-scavenger/robot-gitee-scavenger /opt/app/robot-gitee-scavenger

RUN chmod 550 /opt/app/robot-gitee-scavenger && \
    echo "umask 027" >> /home/scavenger/.bashrc && \
    echo 'set +o history' >> /home/scavenger/.bashrc

ENTRYPOINT ["/opt/app/robot-gitee-scavenger"]
