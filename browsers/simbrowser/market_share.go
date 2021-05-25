package simbrowser

import (
	"fmt"
	"sync"

	"github.com/getlantern/common"
)

type CountryCode common.CountryCode

var globally CountryCode = "**"

func (cc CountryCode) String() string {
	if cc == globally {
		return "global region"
	}
	return string(cc[:])
}

// Possible browser types.
const (
	Chrome                  common.BrowserType = "Chrome"
	Firefox                                    = "Firefox"
	Safari                                     = "Safari"
	Edge                                       = "Edge"
	InternetExplorer                           = "InternetExplorer"
	ThreeSixtySecureBrowser                    = "ThreeSixtySecure"
	QQBrowser                                  = "QQBrowser"
)

func browserByType(t common.BrowserType) (*Browser, error) {
	switch t {
	case Chrome:
		return &chrome, nil
	case Firefox:
		return &firefox, nil
	case Safari:
		return &safari, nil
	case Edge:
		return &edge, nil
	case InternetExplorer:
		return &explorer, nil
	case ThreeSixtySecureBrowser:
		return &threeSixty, nil
	case QQBrowser:
		return &qq, nil
	default:
		return nil, fmt.Errorf("unsupported browser %s", string(t))
	}
}

type browserChoice struct {
	Browser
	marketShare common.MarketShare
}

// Implements the deterministic.WeightedChoice interface.
func (bc browserChoice) Weight() int { return int(bc.marketShare * 100) }

var (
	marketShareLock sync.RWMutex
	marketShareData = map[CountryCode][]browserChoice{
		// https://gs.statcounter.com/browser-market-share/desktop/worldwide#monthly-201910-202009-bar
		globally: {
			{chrome, 0.70},
			{firefox, 0.08},
			{safari, 0.08},
			{edge, 0.05},
		},
		// https://gs.statcounter.com/browser-market-share/desktop/china#monthly-201910-202009-bar
		// We switched Chrome and 360 because we felt that was more accurate. Sogou Explorer is not
		// represented because it is not supported by utls.
		"CN": {
			{threeSixty, 0.39},
			{chrome, 0.25},
			{firefox, 0.08},
			{qq, 0.07},
			{explorer, 0.05},
		},
	}
)

// Bounds for data accepted by SetMarketShareData.
const (
	AcceptableMinTotalMarketShare common.MarketShare = 0.85
	AcceptableMaxTotalMarketShare common.MarketShare = 1.05
)

// SetMarketShareData sets the data used by ChooseForUser. The total share of the global market and
// each regional market must fall within the bounds established by AcceptableMinTotalMarketShare and
// AcceptableMaxTotalMarketShare.
func SetMarketShareData(global common.MarketShareData, regional map[CountryCode]common.MarketShareData) error {
	if len(global) == 0 {
		return fmt.Errorf("global MarketShareData parameter must not be empty")
	}
	msd := map[CountryCode][]browserChoice{}
	totals := map[CountryCode]common.MarketShare{}
	for browserType, marketShare := range global {
		b, err := browserByType(browserType)
		if err != nil {
			return fmt.Errorf("%w", err)
		}
		msd[globally] = append(msd[globally], browserChoice{*b, marketShare})
		totals[globally] += marketShare
	}
	for CountryCode, regionalData := range regional {
		for browserType, marketShare := range regionalData {
			b, err := browserByType(browserType)
			if err != nil {
				return fmt.Errorf("%w", err)
			}
			msd[CountryCode] = append(msd[CountryCode], browserChoice{*b, marketShare})
			totals[CountryCode] += marketShare
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
