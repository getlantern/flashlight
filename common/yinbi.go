package common

var (
	// Default Stellar Horizon address to use
	HorizonAddr        = "https://horizon.stellar.org"
	NetworkName        = "public"
	YinbiAssetName     = "Yinbi"
	YinbiAssetCode     = "YNB"
	YinbiIssuerAccount = "GDTFHBTWLOYSMX54QZKTWWKFHAYCI3NSZADKY3M7PATARUUKVWOAEY2E"
	YinbiServerAddr    = "https://api.yin.bi"
)

func useYinbiStaging() {
	HorizonAddr = "https://horizon-testnet.stellar.org"
	YinbiIssuerAccount = "GCFK6UXOZJ7WRRLO4FRRHGLW7L7JLCSPCHT6M3WULZNGK4C7HGWE7NHY"
	NetworkName = "test"
	YinbiServerAddr = "https://may38fjstaging.yin.bi"
}
