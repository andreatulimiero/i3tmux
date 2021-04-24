FROM docker.io/andreatulimiero/i3tmux:i3tmux-client_base
# Build and install i3tmux
WORKDIR ~/i3tmux
COPY . .
RUN go get
RUN go install

CMD [ "/usr/bin/Xvfb" ]
