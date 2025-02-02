FROM 639159760825.dkr.ecr.eu-west-1.amazonaws.com/python:alpine3.20-amd64
LABEL authors="Anubhav Mishra, Luke Kysow"
LABEL maintainer="anubhav.mishra@hootsuite.com,luke.kysow@hootsuite.com" 

# create atlantis user
RUN addgroup atlantis && \
    adduser -S -G atlantis atlantis

ENV ATLANTIS_HOME_DIR=/home/atlantis

# install atlantis dependencies
ENV DUMB_INIT_VERSION=1.2.0
ENV GOSU_VERSION=1.17
RUN apk add --no-cache ca-certificates gnupg curl git unzip bash openssh libcap openssl py3-boto3 && \
    [ ! -e /usr/bin/python ] && ln -s /usr/bin/python3 /usr/bin/python || true && \
    wget -O /bin/dumb-init https://github.com/Yelp/dumb-init/releases/download/v${DUMB_INIT_VERSION}/dumb-init_${DUMB_INIT_VERSION}_amd64 && \
    chmod +x /bin/dumb-init && \
    mkdir -p /tmp/build && \
    cd /tmp/build && \
    wget -O gosu "https://github.com/tianon/gosu/releases/download/${GOSU_VERSION}/gosu-amd64" && \
    wget -O gosu.asc "https://github.com/tianon/gosu/releases/download/${GOSU_VERSION}/gosu-amd64.asc" && \
    gpg --keyserver hkps://keys.openpgp.org --recv-keys B42F6819007F00F88E364FD4036A9C25BF357DD4 && \
    gpg --batch --verify gosu.asc gosu && \
    chmod +x gosu && \
    cp gosu /bin && \
    cd /tmp && \
    rm -rf /tmp/build && \
    apk del gnupg openssl && \
    rm -rf /root/.gnupg && rm -rf /var/cache/apk/*

RUN apk upgrade curl libcurl

# install terraform binaries
ENV DEFAULT_TERRAFORM_VERSION=1.7.4

RUN AVAILABLE_TERRAFORM_VERSIONS="1.7.4" && \
    for VERSION in ${AVAILABLE_TERRAFORM_VERSIONS}; do curl -LOk https://releases.hashicorp.com/terraform/${VERSION}/terraform_${VERSION}_linux_amd64.zip && \
    mkdir -p /usr/local/bin/tf/versions/${VERSION} && \
    unzip terraform_${VERSION}_linux_amd64.zip -d /usr/local/bin/tf/versions/${VERSION} && \
    ln -s /usr/local/bin/tf/versions/${VERSION}/terraform /usr/local/bin/terraform${VERSION};rm terraform_${VERSION}_linux_amd64.zip;done && \
    ln -s /usr/local/bin/tf/versions/${DEFAULT_TERRAFORM_VERSION}/terraform /usr/local/bin/terraform
RUN pip3 install boto3
# copy binary
COPY atlantis /usr/local/bin/atlantis

# copy docker entrypoint
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["server"]
