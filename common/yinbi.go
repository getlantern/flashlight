package common

var (
	TestNetwork = "test"
	NetworkName = "public"

	YinbiAssetName = "Yinbi"
	YinbiAssetCode = "YNB"

	// Default Stellar Horizon address to use
	HorizonAddr            = "https://horizon.stellar.org"
	HorizonStagingAddr     = "https://horizon-testnet.stellar.org"
	YinbiServerAddr        = "https://api.yin.bi"
	YinbiServerStagingAddr = "https://may38fjstaging.yin.bi"
	YinbiProdIssuer        = "GDTFHBTWLOYSMX54QZKTWWKFHAYCI3NSZADKY3M7PATARUUKVWOAEY2E"
	YinbiStagingIssuer     = "GCFK6UXOZJ7WRRLO4FRRHGLW7L7JLCSPCHT6M3WULZNGK4C7HGWE7NHY"
	YinbiIssuerAccount     = YinbiProdIssuer
)

func useYinbiStaging() {
	HorizonAddr = HorizonStagingAddr
	YinbiIssuerAccount = YinbiStagingIssuer
	NetworkName = TestNetwork
	YinbiServerAddr = YinbiServerStagingAddr
}
