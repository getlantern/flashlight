// +build !darwin darwin,ios

package hellocap

func defaultBrowser() (browser, error) {
	return nil, errors.New("unsupported platform")
}