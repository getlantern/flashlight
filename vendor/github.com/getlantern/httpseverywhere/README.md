# httpseverywhere
Go implementation of using [HTTPS Everywhere](https://github.com/EFForg/https-everywhere) rule sets to send traffic over HTTPS. Many thanks to the EFF team maintaining such an amazing project!

Example usage:

```go
import "url"
import "github.com/getlantern/httpseverywhere"

...

httpURL, _ := url.Parse("http://name.com")
rewrite := httpseverywhere.Default()
httpsURL, changed := rewrite(httpURL)
if changed {
	// Redirect to httpsURL
	...
}
```

Please note that this library does not support any rules that include backtracking, specifically any rules with `(?!` or `(?=`, because those are not supported in Go's regular expressions packages for performance reasons. That excludes approximately 6,000 out of around 22,000 rule sets.
