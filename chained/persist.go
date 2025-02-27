package chained

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/getlantern/flashlight/v7/dialer"
)

var (
	statsTrackingDialers []dialer.ProxyDialer

	statsMx sync.Mutex

	persistOnce sync.Once
)

// TrackStatsFor enables periodic checkpointing of the given proxies' stats to
// disk.
func TrackStatsFor(dialers []dialer.ProxyDialer, configDir string) {
	statsMx.Lock()

	statsFilePath := filepath.Join(configDir, "proxystats.csv")
	applyExistingStats(statsFilePath, dialers)

	statsTrackingDialers = append(statsTrackingDialers, dialers...)

	statsMx.Unlock()

	persistOnce.Do(func() {
		go persistStats(statsFilePath)
	})
}

func applyExistingStats(statsFile string, dialers []dialer.ProxyDialer) {
	dialersMap := make(map[string]dialer.ProxyDialer, len(dialers))
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
		dialers := make([]dialer.ProxyDialer, 0, len(statsTrackingDialers))
		copy(dialers, statsTrackingDialers)
		statsMx.Unlock()
		doPersistStats(statsFilePath, dialers)
	}
}

func doPersistStats(statsFile string, dialers []dialer.ProxyDialer) {

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
