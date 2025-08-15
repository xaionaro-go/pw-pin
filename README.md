# `simpleplumber`

`wireplumber` broke a lot of compatibility from 0.4 to 0.5, while writing scripts for each of them is a big time investment (personally for me). At the same time I have few streaming-studio related workstations where I just need some wiring done no matter what with minimal effort. As a result I decided to write this small ugly service.

## Quick start

Install
```sh
go install github.com/xaionaro-go/simpleplumber@latest
```

Configure:
```sh
mkdir -p ~/.config/simpleplumber
cat > ~/.config/simpleplumber/config.conf <<EOF
```
```yaml
routes:
    - from:
        node:
            - parameter: media.name
              values: [1.webm - mpv]
            - parameter: media.class
              values: [Stream/Output/Audio]
        port:
            - parameter: port.name
              values: [output_FL]
      to:
        node:
            - parameter: node.name
              values: [alsa_output.usb-R__DE_RODECaster_Duo_IR0037235-00.pro-output-0]
        port:
            - parameter: port.name
              values: [playback_AUX0]
      should_be_linked: true
    - from:
        node:
            - parameter: media.name
              values: [1.webm - mpv]
            - parameter: media.class
              values: [Stream/Output/Audio]
        port:
            - parameter: port.name
              values: [output_FR]
      to:
        node:
            - parameter: node.name
              values: [alsa_output.usb-R__DE_RODECaster_Duo_IR0037235-00.pro-output-0]
        port:
            - parameter: port.name
              values: [playback_AUX1]
      should_be_linked: true
    - from:
        node:
            - parameter: media.name
              values: [1.webm - mpv]
            - parameter: media.class
              values: [Stream/Output/Audio]
      should_be_linked: false
```
```sh
EOF
```

Run:
```sh
"$(go env GOPATH)"/bin/simpleplumber
```
