package redis

import (
	"context"
	"math/rand"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/getlantern/geo"
	"github.com/getlantern/golog"
	"github.com/getlantern/http-proxy-lantern/v2/throttle"
	"github.com/getlantern/http-proxy-lantern/v2/usage"
	"github.com/getlantern/http-proxy/listeners"
	"github.com/getlantern/measured"
)

const (
	updateUsageScript = `
	local clientKey = KEYS[1]

	local bytesIn = redis.call("hincrby", clientKey, "bytesIn", ARGV[1])
	local bytesOut = redis.call("hincrby", clientKey, "bytesOut", ARGV[2])
	local countryCode = redis.call("hget", clientKey, "countryCode")
	if not countryCode or countryCode == "" then
		countryCode = ARGV[3]
		redis.call("hset", clientKey, "countryCode", countryCode)
		-- record the IP on which we based the countryCode for auditing
		redis.call("hset", clientKey, "clientIP", ARGV[4])
		redis.call("expireat", clientKey, ARGV[5])
	end

	local ttl = redis.call("ttl", clientKey)
	return {bytesIn, bytesOut, countryCode, ttl}
`

	sixtyDays = 60 * 24 * time.Hour
)

var (
	log = golog.LoggerFor("redis")
)

type statsAndContext struct {
	ctx   map[string]interface{}
	stats *measured.Stats
}

func (sac *statsAndContext) add(other *statsAndContext) *statsAndContext {
	newStats := *other.stats
	if sac != nil {
		newStats.SentTotal += sac.stats.SentTotal
		newStats.RecvTotal += sac.stats.RecvTotal
	}
	return &statsAndContext{other.ctx, &newStats}
}

func NewMeasuredReporter(countryLookup geo.CountryLookup, rc *redis.Client, reportInterval time.Duration, throttleConfig throttle.Config) listeners.MeasuredReportFN {
	// Provide some buffering so that we don't lose data while submitting to Redis
	statsCh := make(chan *statsAndContext, 10000)
	go reportPeriodically(countryLookup, rc, reportInterval, throttleConfig, statsCh)
	return func(ctx map[string]interface{}, stats *measured.Stats, deltaStats *measured.Stats, final bool) {
		select {
		case statsCh <- &statsAndContext{ctx, deltaStats}:
			// submitted successfully
		default:
			// data lost, probably because Redis submission is taking longer than expected
		}
	}
}

func reportPeriodically(countryLookup geo.CountryLookup, rc *redis.Client, reportInterval time.Duration, throttleConfig throttle.Config, statsCh chan *statsAndContext) {
	// randomize the interval to evenly distribute traffic to reporting Redis.
	randomized := time.Duration(reportInterval.Nanoseconds()/2 + rand.Int63n(reportInterval.Nanoseconds()))
	log.Debugf("Will report data usage to Redis every %v", randomized)
	ticker := time.NewTicker(randomized)
	statsByDeviceID := make(map[string]*statsAndContext)
	var scriptSHA string
	for {
		select {
		case sac := <-statsCh:
			_deviceID := sac.ctx["deviceid"]
			if _deviceID == nil {
				// ignore
				continue
			}
			deviceID := _deviceID.(string)
			statsByDeviceID[deviceID] = statsByDeviceID[deviceID].add(sac)
		case <-ticker.C:
			if log.IsTraceEnabled() {
				log.Tracef("Submitting %d stats", len(statsByDeviceID))
			}
			if scriptSHA == "" {
				var err error
				scriptSHA, err = rc.ScriptLoad(context.Background(), updateUsageScript).Result()
				if err != nil {
					log.Errorf("Unable to load script, skip submitting stats: %v", err)
					continue
				}
			}

			err := submit(countryLookup, rc, scriptSHA, statsByDeviceID, throttleConfig)
			if err != nil {
				log.Errorf("Unable to submit stats: %v", err)
			}
			// Reset stats
			statsByDeviceID = make(map[string]*statsAndContext)
		}
	}
}

func submit(countryLookup geo.CountryLookup, rc *redis.Client, scriptSHA string, statsByDeviceID map[string]*statsAndContext, throttleConfig throttle.Config) error {
	for deviceID, sac := range statsByDeviceID {
		now := time.Now()

		_clientIP := sac.ctx["client_ip"]
		if _clientIP == nil {
			log.Error("Missing client_ip in context, this shouldn't happen. Ignoring.")
			continue
		}
		clientIP := _clientIP.(string)
		countryCode := countryLookup.CountryCode(net.ParseIP(clientIP))

		var platform string
		_platform, ok := sac.ctx["app_platform"]
		if ok {
			platform = _platform.(string)
		}

		var appName string
		_appName, ok := sac.ctx["app"]
		if ok {
			appName = _appName.(string)
		}

		var supportedDataCaps []string
		_supportedDataCaps, ok := sac.ctx["supported_data_caps"]
		if ok {
			supportedDataCaps = _supportedDataCaps.([]string)
		}
		throttleSettings, hasThrottleSettings := throttleConfig.SettingsFor(deviceID, countryCode, platform, appName, supportedDataCaps)

		pl := rc.Pipeline()
		throttleCohort := ""
		var updateUsage *redis.Cmd
		if !hasThrottleSettings {
			throttleCohort = "uncapped"
		} else {
			stats := sac.stats
			throttleCohort = throttleSettings.Label

			timeZone := ""
			_timeZone, hasTimeZone := sac.ctx["time_zone"]
			if hasTimeZone {
				timeZone = _timeZone.(string)
			} else {
				// default timeZone to now
				timeZone = now.Location().String()
			}

			clientKey := "_client:" + deviceID
			updateUsage = pl.EvalSha(context.Background(), scriptSHA, []string{clientKey},
				strconv.Itoa(stats.RecvTotal),
				strconv.Itoa(stats.SentTotal),
				strings.ToLower(countryCode),
				clientIP,
				expirationFor(now, throttleSettings.CapResets, timeZone))
		}
		log.Tracef("device %v on platform %v in country %v with supported data caps %v is in throttle cohort %v", deviceID, platform, countryCode, supportedDataCaps, throttleCohort)
		countryCodeLower := strings.ToLower(countryCode)

		nowUTC := now.In(time.UTC)
		today := nowUTC.Format("2006-01-02")
		uniqueDevicesKey := "_devices:" + countryCodeLower + ":" + today + ":" + throttleCohort
		pl.SAdd(context.Background(), uniqueDevicesKey, deviceID)
		// we don't keep these around forever to save space, however we do need to keep them around for longer than the purchase data from pro-server,
		// to make sure that we can identify the device cohort for all purchases
		pl.ExpireAt(context.Background(), uniqueDevicesKey, daysFrom(nowUTC.In(time.UTC), 4))

		deviceLastSeenKey := "_deviceLastSeen:" + countryCodeLower + ":" + throttleCohort + ":" + deviceID
		pl.Set(context.Background(), deviceLastSeenKey, now.Unix(), 0)
		pl.Expire(context.Background(), deviceLastSeenKey, sixtyDays) // nb: test fails if we try to set expiration in the above Set call

		throttled := sac.ctx["throttled"] == true
		if throttled {
			deviceFirstThrottledKey := "_deviceFirstThrottled:" + deviceID
			pl.Set(context.Background(), deviceFirstThrottledKey, now.Unix(), 0)
			pl.Expire(context.Background(), deviceFirstThrottledKey, sixtyDays) // nb: test fails if we try to set expiration in the above Set call
		}

		_, err := pl.Exec(context.Background())
		if err != nil {
			return err
		}

		if hasThrottleSettings {
			_result, err := updateUsage.Result()
			if err != nil {
				return err
			}
			result := _result.([]interface{})
			bytesIn, _ := result[0].(int64)
			bytesOut, _ := result[1].(int64)
			_countryCode := result[2]
			// In production it should never be nil but LedisDB (for unit testing)
			// has a bug which treats empty string as nil when `EvalSha`.
			if _countryCode == nil {
				countryCode = ""
			} else {
				countryCode = _countryCode.(string)
			}
			ttlSeconds := result[3].(int64)
			usage.Set(deviceID, countryCode, bytesIn+bytesOut, now, ttlSeconds)
		}
	}
	return nil
}

func expirationFor(now time.Time, ttl throttle.CapInterval, timeZoneName string) int64 {
	tz, err := time.LoadLocation(timeZoneName)
	if err == nil {
		// adjust to given timeZone
		now = now.In(tz)
	}
	switch ttl {
	case throttle.Daily:
		return daysFrom(now, 1).Unix()
	case throttle.Weekly:
		daysFromSunday := int(now.Weekday())
		daysToNextMonday := 8 - daysFromSunday
		if daysToNextMonday > 7 {
			// today's Sunday, so next Monday is in just 1 day
			daysToNextMonday = 1
		}
		nextMonday := now.AddDate(0, 0, daysToNextMonday)
		return time.Date(nextMonday.Year(), nextMonday.Month(), nextMonday.Day(), 0, 0, 0, 0, now.Location()).Add(-1 * time.Nanosecond).Unix()
	case throttle.Monthly, throttle.Legacy:
		nextMonth := now.AddDate(0, 1, 0)
		return time.Date(nextMonth.Year(), nextMonth.Month(), 1, 0, 0, 0, 0, now.Location()).Add(-1 * time.Nanosecond).Unix()
	}
	return 0
}

func daysFrom(start time.Time, days int) time.Time {
	next := start.AddDate(0, 0, days)
	return time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, start.Location()).Add(-1 * time.Nanosecond)
}
