# WHEP Load Tester

This is a tool for testing WHEP WebRTC playback performance. It launches the simultaneous playback of any number of sessions for a WHEP stream. This way you can test the capacity of your WebRTC WHEP solution and see its performance limits.

It's brought to you by [Softvelum](https://softvelum.com/) and it's part of our [WebRTC bundle](https://softvelum.com/webrtc/).

This tool is used for testing our [WHEP ABR playback support in Nimble Streamer](https://softvelum.com/2024/05/webrtc-whep-abr-nimble-streamer/) as well as [WebRTC adaptive bitrate algorithm](https://softvelum.com/2024/12/webrtc-adaptive-bitrate-algorithm/) implementation.

## Build
WHEP Load Tester is a Go program and in order to build it, you need to get into load tester folder and run go build:
```
cd whep-load-tester
go build
```

## Run
```
whep-load-tester$ ./whep_loader  -whep-addr <URL> -whep-sessions <number>
```

## Parameters

* '-whep-addr' is the URL of WHEP playback stream to test.
* '-whep-sessions' is the number of simultaneous sessions to run for the stream, by default it's 1.
* '--help' parameter provides tool description.

## Example
```
./whep_loader -whep-addr https://yourserver/live_whep/stream/whep.stream -whep-sessions 100
```


## Questions?

Let us know [via our helpdesk](https://wmspanel.com/help) if you have any questions or suggestions.

### Thanks

Special thanks to Sean DuBois and all contributors for creating and maintaining the excellent [Pion framework](https://github.com/pion/webrtc).
