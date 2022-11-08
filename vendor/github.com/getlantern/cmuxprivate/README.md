cmuxprivate
===========

Contains plugins for https://github.com/getlantern/cmux that reference
private repositories (cmux itself is a public repository)


psmux
-----

example:

```
import (
	"github.com/getlantern/cmux"
	"github.com/getlantern/cmuxprivate"
	"github.com/getlantern/psmux"
)

config := psmux.DefaultConfig()
config.Version = 2
...
protocol := cmuxprivate.NewPsmuxProtocol(config)

dialer := cmux.Dialer(&cmux.DialerOpts{Protocol: protocol, ...}
```