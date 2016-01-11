<!--

Copyright 2015 Comcast Cable Communications Management, LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

End Copyright -->


# Embedded Rules

Summary: Basic tests pass and basic engine functionality verified;
*not* extensively tested.

## Raspberry Pi

Status: Core tests pass; basic engine functionality verified.

We've used unofficial Go
[tarballs](http://dave.cheney.net/unofficial-arm-tarballs) from Dave
Cheney.

The executable built with `go build` is about 15MB.  Doing 

```Shell
go build --ldflags '-s'
```

gives a 10MB executable.

Then doing

```Shell
sudo apt-get install upx
go get github.com/pwaller/goupx/
goupx ./rulesys
```

gives a 2.8MB executable.

## Android

Status: Basic engine functionality verified.

We used the semi-official
[`github.com/golang/mobile`](https://github.com/golang/mobile).  In
particular, we used its
[`libhello`](https://github.com/golang/mobile/tree/master/example/libhello)
approach.

In order to avoid clumsy, tricky native calls, we just used TCP on the
loopback interface.  Perhaps we can instead use
[UNIX domain sockets](http://golang.org/pkg/net/#UnixConn), which
Android
[appears to support](http://developer.android.com/reference/android/net/LocalServerSocket.html),
to bypass the network stack.  Or we would wrap a native Android API
around the Go API.  That approach is also supported by
[`github.com/golang/mobile`](https://github.com/golang/mobile).



