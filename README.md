# `simpleplumber`

`wireplumber` broke a lot of compatibility from 0.4 to 0.5, while writing scripts for each of them is a big time investment (personally for me). At the same time I have few streaming-studio related workstations where I just need some wiring done no matter what with minimal effort. As a result I decided to write this small ugly service.

## Quick start

Install
```sh
go install github.com/xaionaro-go/simpleplumber/cmd/simpleplumber@main
```

Configure:
```sh
mkdir -p ~/.config/simpleplumber
cat > ~/.config/simpleplumber/config.conf <<EOF
```
```yaml
routes:
    - from: # to link mpv (matched loosely) with RODECaster Duo (left channel)
        node:
            - property: media.name
              values: [' - mpv']
              op: CONTAINS
        port:
            - property: port.name
              values: [output_FL]
      to:
        node:
            - property: node.name
              values: [alsa_output.usb-R__DE_RODECaster_Duo_IR0037235-00.pro-output-0]
        port:
            - property: port.name
              values: [playback_AUX0]
      should_be_linked: true
    - from: # to link mpv (matched loosely) with RODECaster Duo (right channel)
        node:
            - property: media.name
              values: [' - mpv']
              op: CONTAINS
        port:
            - property: port.name
              values: [output_FR]
      to:
        node:
            - property: node.name
              values: [alsa_output.usb-R__DE_RODECaster_Duo_IR0037235-00.pro-output-0]
        port:
            - property: port.name
              values: [playback_AUX1]
      should_be_linked: true
    - from: # to do not unlink from volume meters (e.g. pavucontrol)
        node:
            - property: media.name
              values: [' - mpv']
              op: CONTAINS
      to:
        node:
            - property: media.name
              values: [Peak detect]
      should_be_linked: null # <- means: keep as is; but since we already matched this rule, we won't get to the next one
    - from: # to unlink it from everything else
        node:
            - property: media.name
              values: [' - mpv']
              op: CONTAINS
      should_be_linked: false
```
```sh
EOF
```

Run:
```sh
"$(go env GOPATH)"/bin/simpleplumber
```

An example of output:
```sh
streaming@void:~/go/src/github.com/xaionaro-go/simpleplumber$ ~/go/bin/simpleplumber
INFO[0000]main.go:70 started
INFO[0005]run.go:33 link {"From":{"NodeID":206,"PortID":208},"To":{"NodeID":114,"PortID":145}} created
INFO[0005]run.go:56 link {"From":{"NodeID":206,"PortID":208},"To":{"NodeID":74,"PortID":137}} destroyed
INFO[0005]run.go:56 link {"From":{"NodeID":206,"PortID":208},"To":{"NodeID":319,"PortID":203}} destroyed
```

If you need more info about `property`-ies mentioned in the config, try running (see `props`):
```sh
pw-dump --monitor
```

As an ugly but simple solution to make `simpleplumber` run together with `pipewire`, run (under your normal user account):
```sh
mkdir -p ~/.local/bin
cat > ~/.local/bin/run-simpleplumber.sh <<EOF
#!/bin/bash
killall -9 simpleplumber
~/go/bin/simpleplumber "$@" &
EOF
chmod +x ~/.local/bin/run-simpleplumber.sh
```
then:
```sh
systemctl edit --user pipewire
```
and add there:
```sh
[Service]
ExecStartPost=~/.local/bin/run-simpleplumber.sh
```
This is abuse of `ExecStartPost`, you generally should not do that. But it is very simple, and it works. A better solution would be to write another service file and build a guarantee one is always executed on any `pipewire` restart.
