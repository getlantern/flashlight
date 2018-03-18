package client

import (
	"os"
	"testing"

	"github.com/pborman/uuid"
	"github.com/stretchr/testify/assert"
)

func generateDeviceId() string {
	return uuid.New()
}

func generateEmail() string {
	return uuid.New() + `@example.com`
}

func generateUser() User {
	return User{
		Auth: Auth{
			DeviceID: generateDeviceId(),
		},
		//Email: generateEmail(),
	}
}

type fakeCard struct {
	Number string
	Month  string
	Year   string
	CVC    string
}

func generateCard() fakeCard {
	return fakeCard{
		Number: `4242424242424242`,
		Month:  `12`,
		Year:   `2020`,
		CVC:    `123`,
	}
}

var (
	userA User
	userB User
)

var tc *Client

const (
	testStripeKey = "sk_test_4MSPFce4ceaRL1D3pI1NV9Qo"
	testAPIPrefix = "https://lantern-pro-server-staging.herokuapp.com"
)

func init() {
	// Setting to defaults.
	endpointPrefix = testAPIPrefix

	// Overwritten by env?
	if envAPIPrefix := os.Getenv("API_PREFIX"); envAPIPrefix != "" {
		endpointPrefix = envAPIPrefix
	}
}

func TestCreateClient(t *testing.T) {
	tc = NewClient(nil)
}

func TestCreateUserA(t *testing.T) {
	userA = generateUser()

	res, err := tc.UserCreate(userA)
	assert.NoError(t, err)

	assert.True(t, res.User.ID != 0)
	assert.True(t, res.User.Expiration == 0)
	assert.True(t, res.User.Code == "")
	assert.True(t, res.User.Token != "")
	assert.True(t, res.User.Referral != "")

	userA.ID = res.User.ID
	userA.Referral = res.User.Referral
	userA.Token = res.User.Token

	log.Debug(userA)
}

func TestUserAData(t *testing.T) {
	res, err := tc.UserData(userA)
	assert.NoError(t, err)
	assert.Equal(t, "ok", res.Status)
}

func TestUserADataAfterPurchase(t *testing.T) {
	res, err := tc.UserData(userA)
	assert.NoError(t, err)
	assert.Equal(t, "ok", res.Status)
	assert.True(t, res.User.Expiration != 0) // we already made a purchase, so we expect this to be not empty.
	//assert.True(t, res.User.Email != "")
	//assert.True(t, res.User.Subscription != "")
}

func TestCreateUserB(t *testing.T) {
	userB = generateUser()

	res, err := tc.UserCreate(userB)
	assert.NoError(t, err)

	assert.True(t, res.User.ID != 0)
	assert.True(t, res.User.Code == "")
	assert.True(t, res.User.Token != "")
	assert.True(t, res.User.Referral != "")

	userB.ID = res.User.ID
	userB.Referral = res.User.Referral
	userB.Token = res.User.Token

	log.Debug(userB)
}
