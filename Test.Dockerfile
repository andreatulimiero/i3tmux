FROM docker.io/ubuntu:latest

RUN apt update && \
    apt install -y openssh-server tmux
RUN sed 's/#PasswordAuthentication yes/PasswordAuthentication no/' -i /etc/ssh/sshd_config && \
    mkdir /run/sshd
COPY test_key.pub /root/.ssh/authorized_keys
RUN chmod 0700 /root/.ssh && \
    chmod 0600 /root/.ssh/authorized_keys

EXPOSE 22

CMD [ "/usr/sbin/sshd", "-D" ]
