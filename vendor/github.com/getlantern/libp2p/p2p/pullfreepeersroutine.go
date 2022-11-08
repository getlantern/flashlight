package p2p

import (
	"context"
	"time"

	"github.com/getlantern/libp2p/common"
	"github.com/getlantern/libp2p/dhtwrapper"
	"github.com/getlantern/libp2p/logger"
	"github.com/getlantern/libp2p/peersrepository"
	"github.com/pkg/errors"
)

func startPullFreePeersRoutine(
	DHTWrapper dhtwrapper.DHTWrapper,
	targetAndSalt common.Bep44TargetAndSalt,
	waitDurationIfNoErr time.Duration,
	waitDurationIfErr time.Duration,
	doneChan chan struct{},
	peersRepo2 *peersrepository.PeersRepository,
) {
	// Make a cancellable context that dies if the close channel is closed
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-doneChan
		cancel()
	}()

	var lastSuccessfulPayloadChecksum uint32
	var retryDelay time.Duration
	for {
		// TODO <28-07-2022, soltzen> Find a way to report failed DHT get requests to Sentry maybe or GA
		checksum, err := pullFreePeersFromTheDHT(
			ctx,
			DHTWrapper,
			targetAndSalt,
			lastSuccessfulPayloadChecksum,
			peersRepo2,
		)
		if err != nil {
			logger.Log.Errorf("while pulling FreePeers from the DHT: %v", err)
			retryDelay = waitDurationIfErr
		} else {
			lastSuccessfulPayloadChecksum = checksum
			retryDelay = waitDurationIfNoErr
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(retryDelay):
			continue
		}
	}
}

// pullFreePeersFromTheDHT pulls a FreePeerBatch (see README) from the DHT
// using "targetAndSalt". If the checksum of the fetch batch is the same as the
// last successful checksum, return early. Else, parse and send the FreePeers
// from the batch to "C".
//
// It returns the checksum of the last successful payload, regardless of
// whether it sent it to "C" or not. If an error occurs, ignore the result of
// the checksum.
func pullFreePeersFromTheDHT(
	ctx context.Context,
	DHTWrapper dhtwrapper.DHTWrapper,
	targetAndSalt common.Bep44TargetAndSalt,
	lastSuccessfulChecksum uint32,
	peersRepo2 *peersrepository.PeersRepository,
) (uint32, error) {
	logger.Log.Infof(
		"Pulling potentially new FreePeers from %s",
		targetAndSalt,
	)
	// Fetch the BEP44 payload from the DHT
	freePeers, checksum, hasNewPeers, err := DHTWrapper.GetFreePeers(
		ctx, targetAndSalt, &lastSuccessfulChecksum,
	)
	if err != nil {
		return 0, errors.Wrapf(
			err,
			"while running get DHT request %s",
			targetAndSalt,
		)
	}
	// If there aren't any new peers, return early
	if !hasNewPeers {
		logger.Log.Infof(
			"FreePeers from %s are unchanged. Not sending to C",
			targetAndSalt,
		)
		return checksum, nil
	}
	// Else, push it to C
	logger.Log.Infof(
		"Pulled FreePeers from %s: %v",
		targetAndSalt,
		freePeers,
	)
	peersRepo2.PushBatch(freePeers)
	return checksum, nil
}
