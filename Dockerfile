# syntax=docker/dockerfile:experimental
FROM registry.redhat.io/codeready-workspaces/stacks-golang-rhel8

USER root

# command line for this would look something like
# DOCKER_BUILDKIT=1 docker build --progress=plain --secret id=myuser,src=docker-secrets/myuser.txt --secret id=mypass,src=docker-secrets/mypass.txt -f Dockerfile -t quay.io/mhildenb/operator-builder:1.0 .
RUN --mount=type=secret,id=myuser --mount=type=secret,id=mypass \
    subscription-manager register  --username=$(cat /run/secrets/myuser) \
    --password=$(cat /run/secrets/mypass) --auto-attach

RUN yum -y module install container-tools:1.0 && yum -y install mercurial  

# Add the operator sdk
ENV OPERATOR_SDK_VERSION=v0.18.1
RUN curl -LO https://github.com/operator-framework/operator-sdk/releases/download/${OPERATOR_SDK_VERSION}/operator-sdk-${OPERATOR_SDK_VERSION}-x86_64-linux-gnu && \
    chmod +x operator-sdk-${OPERATOR_SDK_VERSION}-x86_64-linux-gnu && mkdir -p /usr/local/bin/ && \
    cp operator-sdk-${OPERATOR_SDK_VERSION}-x86_64-linux-gnu /usr/local/bin/operator-sdk && rm operator-sdk-${OPERATOR_SDK_VERSION}-x86_64-linux-gnu

# overwrite existing oc with the absolute newest version of the openshift client
RUN curl -L https://mirror.openshift.com/pub/openshift-v4/clients/ocp/latest/openshift-client-linux.tar.gz | \
    tar -xvzf - -C /usr/bin/ oc && chmod 755 /usr/bin/oc

RUN wget https://storage.googleapis.com/kubernetes-release/release/$(curl -s https://storage.googleapis.com/kubernetes-release/release/stable.txt)/bin/linux/amd64/kubectl -O /usr/local/bin/kubectl && \
    chmod 755 /usr/local/bin/kubectl

RUN yum -y install zsh jq 

RUN subscription-manager unregister

# for running podman
RUN chmod 4755 /usr/bin/newgidmap && chmod 4755 /usr/bin/newuidmap

USER jboss

# install and configure ohmyzsh for jboss user
RUN wget https://github.com/robbyrussell/oh-my-zsh/raw/master/tools/install.sh -qO - | zsh
COPY base-image-assets/.zshrc.example ~/.zshr

ENV KUBECONFIG=/home/jboss/.kube/config
ENV KUBE_SSH_KEY_PATH=/home/jboss/.ssh/id_rsa 
ENV VERSION_TAG=1.0

# From base container
# ENTRYPOINT ["/home/jboss/entrypoint.sh"]
# WORKDIR /projects/windows-machine-config-operator
# CMD ${HOME}/gopath.sh & tail -f /dev/null

# Run this with:
# docker run -it -u root --privileged -v ~/.kube:/home/jboss/.kube -v ~/.ssh:/home/jboss/.ssh -v ~/.oh-my-zsh:/home/jboss/.oh-my-zsh -v $(pwd):/projects/windows-machine-config-operator -v $(pwd)/containers:/var/lib/containers quay.io/mhildenb/operator-builder:latest /bin/zsh
# Then, once inside the container run:
# cd windows-machine-config-operator