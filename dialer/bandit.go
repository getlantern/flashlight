package dialer

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	bandit "github.com/alextanhongpin/go-bandit"
)

// banditDialer is responsible for continually choosing the optimized dialer.
type banditDialer struct {
	dialers                       []ProxyDialer
	bandit                        bandit.Bandit
	opts                          *Options
	banditRewardsMutex            *sync.Mutex
	secondsUntilRewardSample      time.Duration
	secondsUntilSaveBanditRewards time.Duration
}

type banditMetrics struct {
	Reward    float64
	Count     int
	UpdatedAt int64
}

// NewBandit creates a new bandit given the available dialers and options with
// callbacks to be called when a dialer is selected, an error occurs, etc.
func NewBandit(opts *Options) (Dialer, error) {
	if opts.OnError == nil {
		opts.OnError = func(error, bool) {}
	}
	if opts.OnSuccess == nil {
		opts.OnSuccess = func(ProxyDialer) {}
	}

	dialers := opts.Dialers
	log.Debugf("Creating bandit with %d dialers", len(dialers))

	var b bandit.Bandit
	var err error
	dialer := &banditDialer{
		dialers:                       dialers,
		opts:                          opts,
		banditRewardsMutex:            &sync.Mutex{},
		secondsUntilRewardSample:      secondsForSample * time.Second,
		secondsUntilSaveBanditRewards: saveBanditRewardsAfter,
	}

	dialerWeights, err := dialer.loadLastBanditRewards()
	if err != nil {
		log.Errorf("unable to load bandit weights: %v", err)
	}
	if dialerWeights != nil {
		log.Debugf("Loading bandit weights from %q", opts.BanditDir)
		counts := make([]int, len(dialers))
		rewards := make([]float64, len(dialers))
		for arm, dialer := range dialers {
			if metrics, ok := dialerWeights[dialer.Name()]; ok {
				rewards[arm] = metrics.Reward
				counts[arm] = metrics.Count
			}
		}
		b, err = bandit.NewEpsilonGreedy(0.1, counts, rewards)
		if err != nil {
			log.Errorf("unable to create weighted bandit: %w", err)
			return nil, err
		}
		dialer.bandit = b
		return dialer, nil
	}

	b, err = bandit.NewEpsilonGreedy(0.1, nil, nil)
	if err != nil {
		log.Errorf("unable to create bandit: %v", err)
		return nil, err
	}
	if err := b.Init(len(dialers)); err != nil {
		log.Errorf("unable to initialize bandit: %v", err)
		return nil, err
	}
	dialer.bandit = b

	return dialer, nil
}

func (bd *banditDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	deadline, _ := ctx.Deadline()
	log.Debugf("bandit::DialContext::time remaining: %v", time.Until(deadline))
	// We can not create a multi-armed bandit with no arms.
	if len(bd.dialers) == 0 {
		return nil, log.Error("Cannot dial with no dialers")
	}

	start := time.Now()
	d, chosenArm := bd.chooseDialerForDomain(network, addr)

	// We have to be careful here about virtual, multiplexed connections, as the
	// initial TCP dial will have different performance characteristics than the
	// subsequent virtual connection dials.
	log.Debugf("bandit::dialer %d: %s at %v", chosenArm, d.Label(), d.Addr())
	conn, failedUpstream, err := d.DialContext(ctx, network, addr)
	if err != nil {
		hasSucceeding := hasSucceedingDialer(bd.dialers)
		bd.opts.OnError(err, hasSucceeding)

		if !failedUpstream {
			log.Errorf("Dialer %v failed in %v seconds: %v", d.Name(), time.Since(start).Seconds(), err)
			if errUpdatingBanditReward := bd.bandit.Update(chosenArm, 0); errUpdatingBanditReward != nil {
				log.Errorf("unable to update bandit: %v", errUpdatingBanditReward)
			}
		} else {
			log.Debugf("Dialer %v failed upstream...", d.Name())
			// This can happen, for example, if the upstream server is down, or
			// if the DNS resolves to localhost, for example. It is also possible
			// that the proxy is blacklisted by upstream sites for some reason,
			// so we have to choose some reasonable value.
			if errUpdatingBanditReward := bd.bandit.Update(chosenArm, 0.00005); errUpdatingBanditReward != nil {
				log.Errorf("unable to update bandit: %v", errUpdatingBanditReward)
			}
		}
		return nil, err
	}
	log.Debugf("Dialer %v dialed in %v seconds", d.Name(), time.Since(start).Seconds())
	// We don't give any special reward for a successful dial here and just rely on
	// the normalized raw throughput to determine the reward. This is because the
	// reward system takes into account how many tries there have been for a given
	// "arm", so giving a reward here would be double-counting.

	// Tell the dialer to update the bandit with it's throughput after 5 seconds.
	var dataRecv atomic.Uint64
	var elapsedTimeReading atomic.Int64
	dt := newDataTrackingConn(conn, &dataRecv, &elapsedTimeReading)
	time.AfterFunc(bd.secondsUntilRewardSample, func() {
		speed := normalizeReceiveSpeed(dataRecv.Load(), elapsedTimeReading.Load())
		// log.Debugf("Dialer %v received %v bytes in %v seconds, normalized speed: %v", d.Name(), dt.dataRecv, secondsForSample, speed)
		if errUpdatingBanditReward := bd.bandit.Update(chosenArm, speed); errUpdatingBanditReward != nil {
			log.Errorf("unable to update bandit: %v", err)
		}
	})

	time.AfterFunc(bd.secondsUntilSaveBanditRewards, func() {
		log.Debugf("saving bandit rewards")
		metrics := make(map[string]banditMetrics)
		rewards := bd.bandit.GetRewards()
		counts := bd.bandit.GetCounts()
		for i, d := range bd.dialers {
			metrics[d.Name()] = banditMetrics{
				Reward:    rewards[i],
				Count:     counts[i],
				UpdatedAt: time.Now().UTC().Unix(),
			}
		}

		errUpdatingBanditReward := bd.updateBanditRewards(metrics)
		if errUpdatingBanditReward != nil {
			log.Errorf("unable to save bandit weights: %v", errUpdatingBanditReward)
		}
	})

	bd.opts.OnSuccess(d)
	return dt, err
}

const (
	dialerNameCSVHeader = iota
	rewardCSVHeader
	countCSVHeader
	updatedAtCSVHeader

	unusedBanditDialerIgnoredAfter = 7 * 24 * time.Hour
)

// loadLastBanditRewards is a function that returns the last bandit rewards
// for each dialer. If this is set, the bandit will be initialized with the
// last metrics.
func (bd *banditDialer) loadLastBanditRewards() (map[string]banditMetrics, error) {
	bd.banditRewardsMutex.Lock()
	defer bd.banditRewardsMutex.Unlock()
	if bd.opts.BanditDir == "" {
		return nil, log.Error("bandit directory is not set")
	}

	file := filepath.Join(bd.opts.BanditDir, "rewards.csv")
	data, err := os.Open(file)
	if err != nil {
		return nil, err
	}

	reader := csv.NewReader(data)
	// Skip the header, but read it so the csv reader know the expected number of columns
	_, err = reader.Read()
	if err != nil {
		return nil, log.Errorf("unable to skip headers from bandit rewards csv: %w", err)
	}
	metrics := make(map[string]banditMetrics)
	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, log.Errorf("unable to read line from bandit rewards csv: %w", err)
		}

		// load updatedAt unix time and check if it's older than 7 days
		updatedAt, err := strconv.ParseInt(line[updatedAtCSVHeader], 10, 64)
		if err != nil {
			return nil, log.Errorf("unable to parse updated at from %s: %w", line[0], err)
		}
		if time.Since(time.Unix(updatedAt, 0)) > unusedBanditDialerIgnoredAfter {
			log.Debugf("Ignoring bandit dialer %s as it's older than 7 days", line[0])
			continue
		}
		reward, err := strconv.ParseFloat(line[rewardCSVHeader], 64)
		if err != nil {
			return nil, log.Errorf("unable to parse reward from %s: %w", line[0], err)
		}
		count, err := strconv.Atoi(line[countCSVHeader])
		if err != nil {
			return nil, log.Errorf("unable to parse count from %s: %w", line[0], err)
		}

		metrics[line[dialerNameCSVHeader]] = banditMetrics{
			Reward:    reward,
			Count:     count,
			UpdatedAt: updatedAt,
		}
	}
	return metrics, nil
}

func (bd *banditDialer) updateBanditRewards(newRewards map[string]banditMetrics) error {
	if err := os.MkdirAll(bd.opts.BanditDir, 0755); err != nil {
		return log.Errorf("unable to create bandit directory: %v", err)
	}

	previousRewards, err := bd.loadLastBanditRewards()
	if err != nil && !os.IsNotExist(err) {
		return log.Errorf("couldn't load previous bandit rewards: %w", err)
	}
	bd.banditRewardsMutex.Lock()
	defer bd.banditRewardsMutex.Unlock()

	// if there's previous rewards, we must overwrite current values
	if previousRewards != nil {
		for dialer, metrics := range newRewards {
			previousRewards[dialer] = metrics
		}
	} else {
		previousRewards = newRewards
	}

	if bd.opts.BanditDir == "" {
		return log.Error("bandit directory is not set")
	}

	file := filepath.Join(bd.opts.BanditDir, "rewards.csv")

	headers := []string{"dialer", "reward", "count", "updated at"}
	f, err := os.OpenFile(file, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return log.Errorf("unable to open bandit rewards file: %v", err)
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err = w.Write(headers); err != nil {
		return log.Errorf("unable to write headers to bandit rewards file: %v", err)
	}

	for dialerName, metric := range previousRewards {
		if err = w.Write([]string{dialerName, fmt.Sprintf("%f", metric.Reward), fmt.Sprintf("%d", metric.Count), fmt.Sprintf("%d", metric.UpdatedAt)}); err != nil {
			return log.Errorf("unable to write bandit rewards to file: %v", err)
		}
	}

	return nil
}

func (bd *banditDialer) chooseDialerForDomain(network, addr string) (ProxyDialer, int) {
	// Loop through the number of dialers we have and select the one that is best
	// for the given domain.
	chosenArm := bd.bandit.SelectArm(rand.Float64())
	var d ProxyDialer
	notAllFailing := hasNotFailing(bd.dialers)
	for i := 0; i < (len(bd.dialers) * 4); i++ {
		d = bd.dialers[chosenArm]
		readyChan := d.Ready()
		if readyChan != nil {
			select {
			case err := <-readyChan:
				if err != nil {
					log.Errorf("dialer %q failed to initialize with error %w, chossing different arm", d.Name(), err)
					chosenArm = differentArm(chosenArm, len(bd.dialers))
					continue
				}
			default:
				log.Debugf("dialer %q is not ready, chossing different arm", d.Name())
				chosenArm = differentArm(chosenArm, len(bd.dialers))
				continue
			}
		}
		if (d.ConsecFailures() > 0 && notAllFailing) || !d.SupportsAddr(network, addr) {
			// If the chosen dialer has consecutive failures and there are other
			// dialers that are succeeding, we should choose a different dialer.
			//
			// If the chosen dialer does not support the address, we should also
			// choose a different dialer.
			chosenArm = differentArm(chosenArm, len(bd.dialers))
			continue
		}
		break
	}
	return d, chosenArm
}

// Choose a different arm than the one we already have, if possible.
func differentArm(existingArm, numDialers int) int {
	// This selects a new arm randomly, which is preferable to just choosing
	// the next one in the list because that will always be the next dialer
	// after whatever dialer is currently best.
	for i := 0; i < 20; i++ {
		newArm := rand.Intn(numDialers)
		if newArm != existingArm {
			return newArm
		}
	}

	// If random selection doesn't work, just choose the next one.
	log.Debugf("Choosing next dialer after %d", existingArm)
	return (existingArm + 1) % numDialers
}

const secondsForSample = 6

const saveBanditRewardsAfter = 30 * time.Second

// A reasonable upper bound for the top expected bytes to receive per second.
// Anything over this will be normalized to over 1.
const topExpectedBps = 125000

func normalizeReceiveSpeed(dataRecv uint64, elapsedTimeReading int64) float64 {
	// Record the bytes in relation to the top expected speed.
	return (float64(dataRecv) / (float64(elapsedTimeReading) / 1000)) / topExpectedBps
}

func (bd *banditDialer) Close() {
	log.Debug("Closing all dialers")
	for _, d := range bd.dialers {
		d.Stop()
	}
}

func newDataTrackingConn(conn net.Conn, dataRecv *atomic.Uint64, elapsedTimeReading *atomic.Int64) *dataTrackingConn {
	return &dataTrackingConn{
		Conn:               conn,
		dataRecv:           dataRecv,
		elapsedTimeReading: elapsedTimeReading,
	}
}

type dataTrackingConn struct {
	net.Conn
	dataRecv           *atomic.Uint64
	elapsedTimeReading *atomic.Int64 // elapsedTimeReading store in milliseconds the time the connection took to read data
}

func (c *dataTrackingConn) Read(b []byte) (int, error) {
	startedReading := time.Now()
	n, err := c.Conn.Read(b)
	c.dataRecv.Add(uint64(n))
	c.elapsedTimeReading.Add(time.Since(startedReading).Milliseconds())
	return n, err
}
