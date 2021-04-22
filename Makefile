APP_NAME  = i3tmux
SRC_FILES = $(filter-out %_test.go, $(wildcard *.go))

$(APP_NAME): $(SRC_FILES)
	go build

build: $(APP_NAME)

test:
	go test -v

fmt:
	gofmt -s -w *.go

.PHONY: test
