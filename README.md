[![GitHub Actions CI](https://github.com/andreatulimiero/i3tmux/actions/workflows/go.yml/badge.svg)](https://github.com/andreatulimiero/i3tmux/actions/workflows/go.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/andreatulimiero/i3tmux)](https://goreportcard.com/report/github.com/andreatulimiero/i3tmux)
# i3tmux
_i3wm_ and _tmux_ in a seamless experience.

## Rationale
Lately, I found myself working a lot with _tmux_ on remote servers.  
Although a custom _tmux_ configuration let me manage panes in a semi-productive way, I still felt slow without _i3wm_.
> I was looking for a way to manage _tmux_ panes with _i3wm_ as if they were local terminal windows.

_i3tmux_ lets you do exactly this! Plus some goodies like sessions **multiplexing**, to keep the experience **lag free**<sup>1</sup>, and **layout resumption**.  
You can check out the wiki to learn more about how i3tmux works (coming soon).

## Get started
### Indicate your terminal preference
You can specify your preferred terminal emulator -- to spawn sessions windows -- with a dotfile at `~/.config/i3tmux/config.yaml`like this:
```yaml
terminal:
  bin: xterm
  nameFlag: -name
```
### Add i3 shortcuts
To perform the main actions like `add` and `kill` a session or `detach` a group, you can add the following shortcuts to your _i3wm_ config file.
Here is an example<sup>2</sup>:
```
bindsym $caps+Shift+Return exec i3tmux --add
bindsym $caps+Shift+q exec i3tmux --kill
bindsym $caps+Shift+d exec i3tmux --detach
```
### Start Using It!
Host options are parsed from your `~/.ssh/config` file, so you are ready to go!
##### Create a new group
Each session is part of a group. You can create a new group with the following command:
```
i3tmux -host <host> -create <group_name>
```
To confirm that the group was created, you can list existing groups with the following:
```
i3tmux -host <host> -list
```
As you should see from the output, the _create_ command also creates a session in the group.
#### Resume A Group
To resume a group of sessions, you can use the following:
```
i3tmux -host <host> -resume <group_name>
```
When a group gets detached and resumed, its layout reestablished too.
#### Add And Kill Sessions
You can quickly _add_ (or _kill_) a session to a group by having the focus on a session window and using the shortcuts defined above.  
Killing a window means also closing it remotely on the server.
#### Detach A Group
You can simply detach a group by having the focus on a session window of the group and using the shortcut defined above.  
Detaching means (locally) closing all the windows that belong to it and save its layout.

## Build and install
To install _i3tmux_ you can either run `make build`, and place the `i3tmux` executable in a folder contained in `$PATH`, or use `go install`, and make sure that `$GOBIN` is in `$PATH`.

## Testing
To run the tests just run `make test`.
`Podman` (or `Docker`) is required to spawn isolated environments for tests.

## State Of The Project
This project is in alpha stage.

### Footnotes
<sup>1</sup>: Being a network based interaction this is limited to RTT.  
<sup>2</sup>: I remapped my caps lock to `$caps`.
