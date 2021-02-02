package common

import "strconv"

var (
	// Enable yinbi wallet related features via the YINBI env var
	EnableYinbiFeatures = "false"
	EnableYinbi         = false
	// Default Stellar Horizon address to use
	HorizonAddr        = "https://horizon.stellar.org"
	NetworkName        = "public"
	YinbiAssetName     = "Yinbi"
	YinbiAssetCode     = "YNB"
	YinbiIssuerAccount = "GDTFHBTWLOYSMX54QZKTWWKFHAYCI3NSZADKY3M7PATARUUKVWOAEY2E"
	YinbiServerAddr    = "https://api.yin.bi"
)

func useYinbiStaging() {
	EnableYinbi, _ = strconv.ParseBool(EnableYinbiFeatures)

	HorizonAddr = "https://horizon-testnet.stellar.org"
	YinbiIssuerAccount = "GAH3ZXCVTOS6IHKCULBZD7KREA3URTPCDHZQZ7GQBF552PFR77ACCRUY"
	NetworkName = "test"
	YinbiServerAddr = "https://may38fjstaging.yin.bi"
}
