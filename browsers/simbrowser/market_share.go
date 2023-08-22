package simbrowser

import (
	"fmt"
	"sync"
)

// CountryCode is a 2-letter ISO country code.
type CountryCode string

var globally CountryCode = "**"

func (cc CountryCode) String() string {
	if cc == globally {
		return "global region"
	}
	return string(cc[:])
}

// MarketShare is a value between 0 and 1 representing a fraction of the global market.
type MarketShare float64

// BrowserType specifies a type of web browser.
type BrowserType string

// Possible browser types.
const (
	Chrome                  BrowserType = "Chrome"
	Firefox                             = "Firefox"
	Safari                              = "Safari"
	Edge                                = "Edge"
	ThreeSixtySecureBrowser             = "ThreeSixtySecure"
	QQBrowser                           = "QQBrowser"
)

func browserByType(t BrowserType) (*Browser, error) {
	switch t {
	case Chrome:
		return &chrome, nil
	case Firefox:
		return &firefox, nil
	case Safari:
		return &safari, nil
	case Edge:
		return &edge, nil
	case ThreeSixtySecureBrowser:
		return &threeSixty, nil
	case QQBrowser:
		return &qq, nil
	default:
		return nil, fmt.Errorf("unsupported browser %s", string(t))
	}
}

// MarketShareData encapsulates market share information for a region.
type MarketShareData map[BrowserType]MarketShare

type browserChoice struct {
	Browser
	marketShare MarketShare
}

// Implements the deterministic.WeightedChoice interface.
func (bc browserChoice) Weight() int { return int(bc.marketShare * 100) }

var (
	marketShareLock sync.RWMutex
	marketShareData = map[CountryCode][]browserChoice{
		// https://gs.statcounter.com/browser-market-share/desktop/worldwide/#monthly-202301-202308-bar
		globally: {
			{chrome, 0.65},
			{safari, 0.12},
			{edge, 0.11},
			{firefox, 0.06},
		},
		// https://gs.statcounter.com/browser-market-share/desktop/china/#monthly-202301-202308-bar
		// We switched Chrome and 360 because we felt that was more accurate. Sogou Explorer is not
		// represented because it is not supported by utls.
		"CN": {
			{threeSixty, 0.38},
			{edge, 0.21},
			{chrome, 0.19},
			{qq, 0.09},
			{firefox, 0.03},
		},
	}
)

// Bounds for data accepted by SetMarketShareData.
const (
	AcceptableMinTotalMarketShare MarketShare = 0.85
	AcceptableMaxTotalMarketShare MarketShare = 1.05
)

// SetMarketShareData sets the data used by ChooseForUser. The total share of the global market and
// each regional market must fall within the bounds established by AcceptableMinTotalMarketShare and
// AcceptableMaxTotalMarketShare.
func SetMarketShareData(global MarketShareData, regional map[CountryCode]MarketShareData) error {
	if len(global) == 0 {
		return fmt.Errorf("global MarketShareData parameter must not be empty")
	}
	msd := map[CountryCode][]browserChoice{}
	totals := map[CountryCode]MarketShare{}
	for browserType, marketShare := range global {
		b, err := browserByType(browserType)
		if err != nil {
			return fmt.Errorf("%w", err)
		}
		msd[globally] = append(msd[globally], browserChoice{*b, marketShare})
		totals[globally] += marketShare
	}
	for countryCode, regionalData := range regional {
		for browserType, marketShare := range regionalData {
			b, err := browserByType(browserType)
			if err != nil {
				return fmt.Errorf("%w", err)
			}
			msd[countryCode] = append(msd[countryCode], browserChoice{*b, marketShare})
			totals[countryCode] += marketShare
		}
	}
	for region, total := range totals {
		if total < AcceptableMinTotalMarketShare || total > AcceptableMaxTotalMarketShare {
			return fmt.Errorf(
				"total market share for %s, %f,  is not without accepted bounds [%f, %f]",
				region, total, AcceptableMinTotalMarketShare, AcceptableMaxTotalMarketShare,
			)
		}
	}

	marketShareLock.Lock()
	marketShareData = msd
	marketShareLock.Unlock()
	return nil
}
