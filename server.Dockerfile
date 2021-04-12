FROM docker.io/ubuntu:18.04

# Install basic packages
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && \
    apt-get install -y openssh-server tmux

# Load ssh configuration files
RUN sed 's/#PasswordAuthentication yes/PasswordAuthentication no/' -i /etc/ssh/sshd_config && \
    mkdir /run/sshd
COPY assets/test_key.pub /root/.ssh/authorized_keys
RUN chmod 0700 /root/.ssh && \
    chmod 0600 /root/.ssh/authorized_keys

EXPOSE 22
CMD [ "/usr/sbin/sshd", "-D" ]
