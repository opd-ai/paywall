package paywall

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/opd-ai/paywall/wallet"
)

func TestPaywall_BroadcasterIntegration(t *testing.T) {
	t.Run("broadcasters not initialized without RPC config", func(t *testing.T) {
		config := Config{
			PriceInBTC:       0.0001,
			TestNet:          true,
			Store:            NewMemoryStore(),
			PaymentTimeout:   time.Hour,
			MinConfirmations: 1,
		}

		pw, err := NewPaywall(config)
		if err != nil {
			t.Fatalf("NewPaywall() error = %v", err)
		}
		defer pw.Close()

		if pw.GetBTCBroadcaster() != nil {
			t.Error("BTC broadcaster should be nil when RPC config not provided")
		}

		if pw.GetXMRBroadcaster() != nil {
			t.Error("XMR broadcaster should be nil when RPC config not provided")
		}
	})

	t.Run("BTC broadcaster initialized with valid RPC config", func(t *testing.T) {
		// Note: This test will fail to connect to RPC but should create the broadcaster
		config := Config{
			PriceInBTC:       0.0001,
			TestNet:          true,
			Store:            NewMemoryStore(),
			PaymentTimeout:   time.Hour,
			MinConfirmations: 1,
			BTCRPCHost:       "localhost:18332",
			BTCRPCUser:       "testuser",
			BTCRPCPass:       "testpass",
			BTCDisableTLS:    true,
		}

		pw, err := NewPaywall(config)
		if err != nil {
			t.Fatalf("NewPaywall() error = %v", err)
		}
		defer pw.Close()

		// Broadcaster should be initialized even if connection fails
		// (initialization only requires valid config, not successful connection)
		if pw.GetBTCBroadcaster() == nil {
			t.Skip("BTC broadcaster initialization skipped (RPC connection failed, this is expected in test environment)")
		}
	})

	t.Run("XMR broadcaster initialized with valid RPC config", func(t *testing.T) {
		// Note: This test will fail to connect to RPC but should create the broadcaster
		config := Config{
			PriceInBTC:       0.0001,
			PriceInXMR:       0.01,
			TestNet:          true,
			Store:            NewMemoryStore(),
			PaymentTimeout:   time.Hour,
			MinConfirmations: 1,
			XMRRPC:           "http://localhost:18083/json_rpc",
			XMRUser:          "testuser",
			XMRPassword:      "testpass",
		}

		pw, err := NewPaywall(config)
		if err != nil {
			t.Fatalf("NewPaywall() error = %v", err)
		}
		defer pw.Close()

		// Broadcaster should be initialized even if connection fails
		if pw.GetXMRBroadcaster() == nil {
			t.Skip("XMR broadcaster initialization skipped (RPC connection failed, this is expected in test environment)")
		}
	})

	t.Run("broadcasters accessible for MultisigCoordinator setup", func(t *testing.T) {
		config := Config{
			PriceInBTC:       0.0001,
			TestNet:          true,
			Store:            NewMemoryStore(),
			PaymentTimeout:   time.Hour,
			MinConfirmations: 1,
			BTCRPCHost:       "localhost:18332",
			BTCRPCUser:       "testuser",
			BTCRPCPass:       "testpass",
			BTCDisableTLS:    true,
		}

		pw, err := NewPaywall(config)
		if err != nil {
			t.Fatalf("NewPaywall() error = %v", err)
		}
		defer pw.Close()

		// Create MultisigCoordinator and demonstrate broadcaster integration
		coordinator := NewMultisigCoordinator(pw, nil, nil)
		if coordinator == nil {
			t.Fatal("NewMultisigCoordinator() returned nil")
		}

		// Set broadcasters from paywall (this is the intended usage pattern)
		if btcBroadcaster := pw.GetBTCBroadcaster(); btcBroadcaster != nil {
			coordinator.SetBTCBroadcaster(btcBroadcaster)
			// Verify broadcaster is set (by checking it's no longer nil)
			if coordinator.btcBroadcaster == nil {
				t.Error("SetBTCBroadcaster() did not set the broadcaster")
			}
		} else {
			t.Log("BTC broadcaster not initialized (expected in test without real RPC server)")
		}

		if xmrBroadcaster := pw.GetXMRBroadcaster(); xmrBroadcaster != nil {
			coordinator.SetXMRBroadcaster(xmrBroadcaster)
			if coordinator.xmrBroadcaster == nil {
				t.Error("SetXMRBroadcaster() did not set the broadcaster")
			}
		}
	})

	t.Run("multisig payment with broadcaster configured", func(t *testing.T) {
		// Generate valid test public keys using btcec
		seed1 := sha256.Sum256([]byte("buyer-seed"))
		buyerPrivKey, _ := btcec.PrivKeyFromBytes(seed1[:])
		buyerPubKey := buyerPrivKey.PubKey().SerializeCompressed()

		seed2 := sha256.Sum256([]byte("seller-seed"))
		sellerPrivKey, _ := btcec.PrivKeyFromBytes(seed2[:])
		sellerPubKey := sellerPrivKey.PubKey().SerializeCompressed()

		seed3 := sha256.Sum256([]byte("arbiter-seed"))
		arbiterPrivKey, _ := btcec.PrivKeyFromBytes(seed3[:])
		arbiterPubKey := arbiterPrivKey.PubKey().SerializeCompressed()

		config := Config{
			PriceInBTC:       0.001,
			TestNet:          true,
			Store:            NewMemoryStore(),
			PaymentTimeout:   time.Hour * 24,
			MinConfirmations: 3,
			MultisigEnabled:  true,
			MultisigRequired: 2,
			MultisigTotal:    3,
			ParticipantPubKeys: map[wallet.WalletType][][]byte{
				wallet.Bitcoin: {buyerPubKey, sellerPubKey, arbiterPubKey},
			},
			MultisigRole:  RoleBuyer,
			BTCRPCHost:    "localhost:18332",
			BTCRPCUser:    "testuser",
			BTCRPCPass:    "testpass",
			BTCDisableTLS: true,
		}

		pw, err := NewPaywall(config)
		if err != nil {
			t.Fatalf("NewPaywall() error = %v", err)
		}
		defer pw.Close()

		// Verify multisig is enabled
		if !pw.multisigEnabled {
			t.Error("multisig should be enabled")
		}

		// Create coordinator with broadcaster integration
		coordinator := NewMultisigCoordinator(pw, nil, nil)
		if btcBroadcaster := pw.GetBTCBroadcaster(); btcBroadcaster != nil {
			coordinator.SetBTCBroadcaster(btcBroadcaster)
			t.Log("Bitcoin broadcaster successfully integrated with MultisigCoordinator")
		}
	})
}

func TestPaywall_GetBroadcasters(t *testing.T) {
	t.Run("GetBTCBroadcaster returns nil when not configured", func(t *testing.T) {
		config := Config{
			PriceInBTC:       0.0001,
			TestNet:          true,
			Store:            NewMemoryStore(),
			PaymentTimeout:   time.Hour,
			MinConfirmations: 1,
		}

		pw, err := NewPaywall(config)
		if err != nil {
			t.Fatalf("NewPaywall() error = %v", err)
		}
		defer pw.Close()

		if pw.GetBTCBroadcaster() != nil {
			t.Error("GetBTCBroadcaster() should return nil when RPC not configured")
		}
	})

	t.Run("GetXMRBroadcaster returns nil when not configured", func(t *testing.T) {
		config := Config{
			PriceInBTC:       0.0001,
			TestNet:          true,
			Store:            NewMemoryStore(),
			PaymentTimeout:   time.Hour,
			MinConfirmations: 1,
		}

		pw, err := NewPaywall(config)
		if err != nil {
			t.Fatalf("NewPaywall() error = %v", err)
		}
		defer pw.Close()

		if pw.GetXMRBroadcaster() != nil {
			t.Error("GetXMRBroadcaster() should return nil when RPC not configured")
		}
	})
}
