Go library for OS version discovery

# Usage


```
package main

import (
	"log"

	"github.com/getlantern/osversion"
)

func main() {
	hstr, _ := osversion.GetHumanReadable()
	log.Println(hstr)
  // For OSX, output will be something like "darwin 12.6"
  // For Android, it'll be "Android (API 33 | OS 13 | Arch aarch64)"
}
```

# Testing

## All platforms but Android

Just run `make run-test-cmd`

## Android

```
// Run an emulator or plugin your Android phone to your computer
make build-test-android-app
adb install -r cmd.apk
// Run cmd.apk on your phone
adb logcat | grep PINEAPPLE
// You should see the output in the logs
```
