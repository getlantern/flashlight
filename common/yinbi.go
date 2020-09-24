package common

var (
	HorizonAddr        = "https://horizon.stellar.org"
	NetworkName        = "public"
	YinbiAssetName     = "Yinbi"
	YinbiAssetCode     = "YNB"
	YinbiIssuerAccount = "GDTFHBTWLOYSMX54QZKTWWKFHAYCI3NSZADKY3M7PATARUUKVWOAEY2E"
	YinbiServerAddr    = "https://api.yin.bi"
)

func useYinbiStaging() {
	HorizonAddr = "https://horizon-testnet.stellar.org"
	YinbiIssuerAccount = "GBHV7FVZILTLVWSJI5TVH25UBNZ2CXWAZERKH4CI4CUQM6HN5IVM3HOS"
	NetworkName = "test"
	YinbiServerAddr = "https://may38fjstaging.yin.bi"
}
