FROM docker.io/ljmf00/archlinux:devel
LABEL maintainer="Jguer,docker@jguer.space"

ENV GO111MODULE=on
WORKDIR /app

RUN sed -i '/^\[community\]/,/^\[/ s/^/#/' /etc/pacman.conf

COPY go.mod .

RUN pacman-key --init && pacman -Sy && pacman -S --overwrite=* --noconfirm archlinux-keyring && \
    pacman -Su --overwrite=* --needed --noconfirm pacman doxygen meson asciidoc go git gcc make sudo base-devel && \
    rm -rfv /var/cache/pacman/* /var/lib/pacman/sync/* && \
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s v1.64.6 && \
    go mod download 
