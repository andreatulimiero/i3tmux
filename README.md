# i3tmux
_i3wm_ and _tmux_ in a seamless experience.

## Rationale
Lately, I found myself working a lot with _tmux_ on remote servers.  
Although a custom _tmux_ configuration let me manage panes in a semi-productive way, I still felt slow without _i3wm_.
> I was looking for a way to manage _tmux_ panes with _i3wm_ as if they were local terminal windows.

i3tmux lets you do exactly this!  
You can check out the wiki to learn more about how i3tmux works (coming soon).

## Start Using It
_i3tmux_ is not reinventing the wheel -- you could perform all _i3tmux_ actions manually.  
Here are the main commands to get you started

**N.B.**: in the `--nameFlag` parameter, you need to pass the flag used by your terminal of choice to set the name part of the `WM_CLASS` property
(e.g., for [kitty](https://github.com/kovidgoyal/kitty) it would be `--name`).
### Create a new group
Each session is part of a group. You can create a new group with the following command:
```
i3tmux -host <host_address> -new <group_name>
```
This command also creates a session in the new group.
### Resume A Group
To resume a group of sessions, you can use the following:
```
i3tmux -host <host_address> -resume <group_name> -terminal <terminal> -nameFlag <terminal_name_flag>
```
The layout of the group, before detachment, is reestablished too.
### Add And Kill Sessions
To add and kill sessions, you can specify shortcuts like in the following example.  
Although you don't need to use "i3wm" bindings, I recommended you do so.
```
bindsym $caps+Shift+Return exec i3tmux --add --terminal <terminal> --nameFlag <terminal_name_flag>
bindsym $caps+Shift+q exec i3tmux --kill
```
Killing a window means also closing it remotely on the server.
### Detach A Group
To detach a group (i.e., locally closing all the windows that belong to it) you can specify a shortcut like in the following example:
```
bindsym $caps+Shift+q exec i3tmux --detach
```
When you detach a group, "i3tmux" also saves its layout, so when you resume it, i3tmux will arrange the windows in the same way.

### Demo
Here is a demo of the following commands:
- resume a group,
- add two windows,
- kill one window,
- detach the group and resume it again.  

![i3tmux demo](https://media.giphy.com/media/s1A2PG0k8oDLJhlsGu/giphy.gif)

## Build and install
Run `make build` and place the `i3tmux` executable in a folder contained in `$PATH`.
### Specify remote host configuration
Currently, part of the configuration is still hardcoded (this is to change very soon).  
To specify your ssh server options, you can change them in `main.go::main()`.

## Testing
To run the tests just run `make test`.
`Podman` (or `Docker`) is required to spawn fresh, OpenSSH server instances for tests.

## State Of The Project
This project is in alpha stage.
