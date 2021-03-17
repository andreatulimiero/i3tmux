APP_NAME  = i3tmux
SRC_FILES = $(filter-out %_test.go, $(wildcard *.go))
GO_IMAGE  = index.docker.io/library/golang:latest

$(APP_NAME): $(SRC_FILES)
	go build

build: $(APP_NAME)

test:
	go test

.PHONY: test
