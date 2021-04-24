APP_NAME  = i3tmux
SRC_FILES = $(filter-out %_test.go, $(wildcard *.go))
CONTAINER_IMAGES = i3tmux-client

$(APP_NAME): $(SRC_FILES)
	go build

build: $(APP_NAME)

$(CONTAINER_IMAGES):
	podman build -f $(subst i3tmux-,,$@).Dockerfile -t $@

test: i3tmux-client
	go test -v

fmt:
	gofmt -s -w *.go

.PHONY: test build fmt
