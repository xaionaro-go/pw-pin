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