package chained

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/getlantern/flashlight/balancer"
	"github.com/getlantern/flashlight/common"
)

var (
	statsTrackingDialers []balancer.Dialer

	statsMx sync.Mutex

	persistOnce sync.Once
)

// TrackStatsFor enables periodic checkpointing of the given proxies' stats to
// disk.
func TrackStatsFor(dialers []balancer.Dialer, configDir string, probeIfNecessary bool) {
	statsMx.Lock()

	statsFilePath := filepath.Join(configDir, "proxystats.csv")
	applyExistingStats(statsFilePath, dialers)
	if probeIfNecessary && len(dialers) > 1 {
		probeIfRequired(dialers)
	}

	statsTrackingDialers = make([]balancer.Dialer, 0, len(dialers))
	for _, d := range dialers {
		statsTrackingDialers = append(statsTrackingDialers, d)
	}

	statsMx.Unlock()

	persistOnce.Do(func() {
		go persistStats(statsFilePath)
	})
}

func probeIfRequired(dialers []balancer.Dialer) {
	if common.InStealthMode() {
		log.Debugf("In stealth mode, not probing")
		return
	}
	sorted := balancer.SortDialers(dialers)
	rttOfTopProxy := sorted[0].EstRTT()
	for i, dialer := range sorted {
		// probe is automatically required for relatively new dialers
		probeRequired := dialer.Attempts() < 20
		if probeRequired {
			log.Debugf("%v is relatively new, will probe for performance", dialer.Label())
		} else if i > 0 && dialer.Successes() > 0 && dialer.EstRTT() < rttOfTopProxy {
			// dialers whose RTT is lower than the top proxy get checked on startup as well
			log.Debugf("%v is lower RTT than %v, will probe for performance", dialer.Label(), sorted[0].Label())
			probeRequired = true
		}
		if probeRequired {
			go dialer.Probe(true)
		}
	}
}

func applyExistingStats(statsFile string, dialers []balancer.Dialer) {
	dialersMap := make(map[string]balancer.Dialer, len(dialers))
	for _, d := range dialers {
		dialersMap[d.Addr()] = d
	}

	in, err := os.Open(statsFile)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Errorf("Error opening stats file, will remove: %v", err)
			os.Remove(statsFile)
		}
		return
	}
	defer in.Close()

	csvIn := csv.NewReader(in)
	rows, err := csvIn.ReadAll()
	if err != nil {
		log.Errorf("Unable to read proxystats.csv, will remove: %v", err)
		os.Remove(statsFile)
		return
	}

	successful := true
	for _, row := range rows {
		d := dialersMap[row[0]]
		if d != nil {
			updateErr := updateStats(d.(*proxy), row)
			if updateErr != nil {
				log.Errorf("Error updating stats, will remove proxystats.csv: %v", updateErr)
				successful = false
				break
			}
			log.Debugf("Loaded stats for %v", row[1])
		}
	}

	in.Close()
	if !successful {
		os.Remove(statsFile)
	}
}

func updateStats(p *proxy, row []string) error {
	if len(row) < 10 {
		return fmt.Errorf("Wrong number of fields in row")
	}

	attempts, err := strconv.ParseInt(row[2], 10, 64)
	if err != nil {
		return err
	}
	successes, err := strconv.ParseInt(row[3], 10, 64)
	if err != nil {
		return err
	}
	consecSuccesses, err := strconv.ParseInt(row[4], 10, 64)
	if err != nil {
		return err
	}
	failures, err := strconv.ParseInt(row[5], 10, 64)
	if err != nil {
		return err
	}
	consecFailures, err := strconv.ParseInt(row[6], 10, 64)
	if err != nil {
		return err
	}
	emaRTT, err := time.ParseDuration(row[7])
	if err != nil {
		return err
	}
	mostRecentABETime, err := time.Parse(time.RFC3339Nano, row[8])
	if err != nil {
		return err
	}
	abe, err := strconv.ParseInt(row[9], 10, 64)
	if err != nil {
		return err
	}

	var emaSuccessRate float64
	if len(row) > 10 {
		emaSuccessRate, err = strconv.ParseFloat(row[10], 64)
		if err != nil {
			return err
		}
	}

	p.setStats(attempts, successes, consecSuccesses, failures, consecFailures, emaRTT, mostRecentABETime, abe, emaSuccessRate)
	return nil
}

func persistStats(statsFilePath string) {
	for {
		time.Sleep(15 * time.Second)
		statsMx.Lock()
		dialers := make([]balancer.Dialer, 0, len(statsTrackingDialers))
		for _, d := range statsTrackingDialers {
			dialers = append(dialers, d)
		}
		statsMx.Unlock()
		doPersistStats(statsFilePath, dialers)
	}
}

func doPersistStats(statsFile string, dialers []balancer.Dialer) {

	out, err := os.OpenFile(fmt.Sprintf("%v.tmp", statsFile), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		log.Errorf("Unable to create temp file to save proxystats.csv: %v", err)
		return
	}
	defer out.Close()

	csvOut := csv.NewWriter(out)
	csvOut.Write([]string{"addr", "label", "attempts", "successes", "consec successes", "failures", "consec failures", "est rtt", "most recent bandwidth estimate", "est bandwidth", "est success rate"})
	for _, d := range dialers {
		p := d.(*proxy)
		p.mx.Lock()
		err = csvOut.Write([]string{d.Addr(), d.Label(), fmt.Sprint(d.Attempts()), fmt.Sprint(d.Successes()), fmt.Sprint(d.ConsecSuccesses()), fmt.Sprint(d.Failures()), fmt.Sprint(d.ConsecFailures()), p.emaRTT.GetDuration().String(), p.mostRecentABETime.Format(time.RFC3339Nano), fmt.Sprint(p.abe), fmt.Sprint(p.emaSuccessRate.Get())})
		p.mx.Unlock()
		if err != nil {
			log.Errorf("Error writing to proxystats.csv: %v", err)
			return
		}
	}

	csvOut.Flush()
	err = out.Close()
	if err != nil {
		log.Errorf("Unable to close temporary proxystats.csv: %v", err)
		return
	}

	err = os.Rename(out.Name(), statsFile)
	if err != nil {
		log.Errorf("Unable to move temporary proxystats.csv to final location: %v", err)
	}

	log.Debugf("Saved proxy stats to %v", statsFile)
}
