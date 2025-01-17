package services

/*
func TestFetchConfig(t *testing.T) {
	t.Run("no change", func(t *testing.T) {
		cs := newTestConfigService()
		want := fmt.Sprintf("%v", cs.configHandler.GetConfig())
		cs.opts.RoundTripper.(*mockRoundTripper).status = http.StatusNoContent

		_, err := cs.fetchConfig()
		require.NoError(t, err)

		got := fmt.Sprintf("%v", cs.configHandler.GetConfig())
		assert.Equal(t, want, got, "config should not have changed")
	})

	t.Run("new config", func(t *testing.T) {
		cs := newTestConfigService()
		_, err := cs.fetchConfig()
		require.NoError(t, err)

		rt := cs.opts.RoundTripper.(*mockRoundTripper)
		want := fmt.Sprintf("%v", rt.config)
		got := fmt.Sprintf("%v", cs.configHandler.GetConfig())
		assert.Equal(t, want, got, "new config not set")
	})
}

type mockConfigHandler struct {
	config *apipb.ConfigResponse
}

func (m *mockConfigHandler) GetConfig() *apipb.ConfigResponse    { return m.config }
func (m *mockConfigHandler) SetConfig(new *apipb.ConfigResponse) { m.config = new }

func newTestConfigService() *configService {
	clientInfo := &apipb.ConfigRequest_ClientInfo{
		ProToken: "the gray",
		Country:  "shire",
	}
	return &configService{
		opts: &ConfigOptions{
			OriginURL:  "http://middle.earth",
			UserConfig: common.NullUserConfig{},
			RoundTripper: &mockRoundTripper{
				config: &apipb.ConfigResponse{
					ProToken: "the white",
					Country:  "shire",
					Proxy: &apipb.ConfigResponse_Proxy{
						Proxies: []*apipb.ProxyConnectConfig{{Name: "mines of moria"}},
					},
				},
			},
		},
		clientInfo: clientInfo,
		configHandler: &mockConfigHandler{
			config: &apipb.ConfigResponse{
				ProToken: clientInfo.ProToken,
				Country:  clientInfo.Country,
				Proxy: &apipb.ConfigResponse_Proxy{
					Proxies: []*apipb.ProxyConnectConfig{{Name: "misty mountain"}},
				},
			},
		},
		sender: &sender{},
		done:   make(chan struct{}),
	}
}
*/
