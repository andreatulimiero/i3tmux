FROM docker.io/ubuntu:18.04

# Install basic packages
ENV DEBIAN_FRONTEND=noninteractive
ENV DISPLAY=:0
RUN apt-get update && \
    apt-get install -y wget xvfb imagemagick x11-apps i3 xterm

# Install golang
RUN wget https://golang.org/dl/go1.16.3.linux-amd64.tar.gz && \
    rm -rf /usr/local/go && \
    tar -C /usr/local -xzf go1.16.3.linux-amd64.tar.gz
ENV PATH=$PATH:/usr/local/go/bin:~/go/bin
ENV GOBIN=/bin

# Load ssh configuration files
COPY assets/test_config /root/.ssh/config
COPY assets/test_key /root/.ssh/
RUN chmod 0700 /root/.ssh && \
    chmod 0600 /root/.ssh/test_key

# Load misc files
COPY assets/test_i3_config /root/.config/i3/config
COPY assets/test_config.yaml /root/.config/i3tmux/config.yaml

# Build and install i3tmux
WORKDIR ~/i3tmux
COPY . .
RUN go get
RUN go install

CMD [ "/usr/bin/Xvfb" ]
