// Package microstellar is an easy-to-use Go client for the Stellar network.
//
//   go get github.com/0xfe/microstellar
//
// Author: Mohit Muthanna Cheppudira <mohit@muthanna.com>
//
// Usage notes
//
// In Stellar lingo, a private key is called a seed, and a public key is called an address. Seed
// strings start with "S", and address strings start with "G". (Not techincally accurate, but you
// get the picture.)
//
//   Seed:    S6H4HQPE6BRZKLK3QNV6LTD5BGS7S6SZPU3PUGMJDJ26V7YRG3FRNPGA
//   Address: GAUYTZ24ATLEBIV63MXMPOPQO2T6NHI6TQYEXRTFYXWYZ3JOCVO6UYUM
//
// In most the methods below, the first parameter is usually "sourceSeed", which should be the
// seed of the account that signs the transaction.
//
// You can add a *TxOptions struct to the end of many methods, which set extra parameters on the
// submitted transaction. If you add new signers via TxOptions, then sourceSeed will not be used to sign
// the transaction -- and it's okay to use a public address instead of a seed for sourceSeed.
// See examples for how to use TxOptions.
//
// You can use ErrorString(...) to extract the Horizon error from a returned error.
package microstellar

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"github.com/stellar/go/build"
	"github.com/stellar/go/clients/horizon"
	"github.com/stellar/go/keypair"
)

// MicroStellar is the user handle to the Stellar network. Use the New function
// to create a new instance.
type MicroStellar struct {
	networkName string
	params      Params
	fake        bool
}

// Params lets you add optional parameters to New and NewTx.
type Params map[string]interface{}

// New returns a new MicroStellar client connected that operates on the network
// specified by networkName. The supported networks are:
//
//    public: the public horizon network
//    test: the public horizon testnet
//    fake: a fake network used for tests
//    custom: a custom network specified by the parameters
//
// If you're using "custom", provide the URL and Passphrase to your
// horizon network server in the parameters.
//
//    NewTx("custom", Params{
//        "url": "https://my-horizon-server.com",
//        "passphrase": "foobar"})
func New(networkName string, params ...Params) *MicroStellar {
	var p Params

	if len(params) > 0 {
		p = params[0]
	}

	return &MicroStellar{
		networkName: networkName,
		params:      p,
		fake:        networkName == "fake",
	}
}

// CreateKeyPair generates a new random key pair.
func (ms *MicroStellar) CreateKeyPair() (*KeyPair, error) {
	pair, err := keypair.Random()
	if err != nil {
		return nil, err
	}

	return &KeyPair{pair.Seed(), pair.Address()}, nil
}

// FundAccount creates a new account out of addressOrSeed by funding it with lumens
// from sourceSeed. The minimum funding amount today is 0.5 XLM.
func (ms *MicroStellar) FundAccount(sourceSeed string, addressOrSeed string, amount string, options ...*TxOptions) error {
	if !ValidAddressOrSeed(sourceSeed) {
		return errors.Errorf("invalid source address or seed: %s", sourceSeed)
	}

	if !ValidAddressOrSeed(addressOrSeed) {
		return errors.Errorf("invalid target address or seed: %s", addressOrSeed)
	}

	payment := build.CreateAccount(
		build.Destination{AddressOrSeed: addressOrSeed},
		build.NativeAmount{Amount: amount})

	tx := NewTx(ms.networkName, ms.params)

	if len(options) > 0 {
		tx.SetOptions(options[0])
	}

	tx.Build(sourceAccount(sourceSeed), payment)
	tx.Sign(sourceSeed)
	tx.Submit()
	return tx.Err()
}

// LoadAccount loads the account information for the given address.
func (ms *MicroStellar) LoadAccount(address string) (*Account, error) {
	if !ValidAddressOrSeed(address) {
		return nil, errors.Errorf("can't load account: invalid address or seed: %v", address)
	}

	if ms.fake {
		return newAccount(), nil
	}

	tx := NewTx(ms.networkName, ms.params)
	account, err := tx.GetClient().LoadAccount(address)

	if err != nil {
		return nil, errors.Wrap(err, "could not load account")
	}

	return newAccountFromHorizon(account), nil
}

// PayNative makes a native asset payment of amount from source to target.
func (ms *MicroStellar) PayNative(sourceSeed string, targetAddress string, amount string, options ...*TxOptions) error {
	return ms.Pay(sourceSeed, targetAddress, amount, NativeAsset, options...)
}

// Pay lets you make payments with credit assets.
//
//   ms.Pay("source_seed", "target_address", "3", NativeAsset, microstellar.Opts().WithMemoText("for shelter"))
func (ms *MicroStellar) Pay(sourceSeed string, targetAddress string, amount string, asset *Asset, options ...*TxOptions) error {
	if err := asset.Validate(); err != nil {
		return errors.Wrap(err, "can't pay")
	}

	if !ValidAddressOrSeed(sourceSeed) {
		return errors.Errorf("can't pay: invalid source address or seed: %s", sourceSeed)
	}

	if !ValidAddressOrSeed(targetAddress) {
		return errors.Errorf("can't pay: invalid address: %v", targetAddress)
	}

	paymentMuts := []interface{}{
		build.Destination{AddressOrSeed: targetAddress},
	}

	if asset.IsNative() {
		paymentMuts = append(paymentMuts, build.NativeAmount{Amount: amount})
	} else {
		paymentMuts = append(paymentMuts,
			build.CreditAmount{Code: asset.Code, Issuer: asset.Issuer, Amount: amount})
	}

	tx := NewTx(ms.networkName, ms.params)

	if len(options) > 0 {
		tx.SetOptions(options[0])
	}

	tx.Build(sourceAccount(sourceSeed), build.Payment(paymentMuts...))
	tx.Sign(sourceSeed)
	tx.Submit()
	return tx.Err()
}

// CreateTrustLine creates a trustline from sourceSeed to asset, with the specified trust limit. An empty
// limit string indicates no limit.
func (ms *MicroStellar) CreateTrustLine(sourceSeed string, asset *Asset, limit string, options ...*TxOptions) error {
	if !ValidAddressOrSeed(sourceSeed) {
		return errors.Errorf("can't create trust line: invalid source address or seed: %s", sourceSeed)
	}

	if err := asset.Validate(); err != nil {
		return errors.Wrap(err, "can't create trust line")
	}

	tx := NewTx(ms.networkName, ms.params)

	if len(options) > 0 {
		tx.SetOptions(options[0])
	}

	if limit == "" {
		tx.Build(sourceAccount(sourceSeed), build.Trust(asset.Code, asset.Issuer))
	} else {
		tx.Build(sourceAccount(sourceSeed), build.Trust(asset.Code, asset.Issuer, build.Limit(limit)))
	}

	tx.Sign(sourceSeed)
	tx.Submit()
	return tx.Err()
}

// RemoveTrustLine removes an trustline from sourceSeed to an asset.
func (ms *MicroStellar) RemoveTrustLine(sourceSeed string, asset *Asset, options ...*TxOptions) error {
	if !ValidAddressOrSeed(sourceSeed) {
		return errors.Errorf("can't remove trust line: invalid source address or seed: %s", sourceSeed)
	}

	if err := asset.Validate(); err != nil {
		return errors.Wrapf(err, "can't remove trust line")
	}

	tx := NewTx(ms.networkName, ms.params)

	if len(options) > 0 {
		tx.SetOptions(options[0])
	}

	tx.Build(sourceAccount(sourceSeed), build.RemoveTrust(asset.Code, asset.Issuer))
	tx.Sign(sourceSeed)
	tx.Submit()
	return tx.Err()
}

// SetMasterWeight changes the master weight of sourceSeed.
func (ms *MicroStellar) SetMasterWeight(sourceSeed string, weight uint32, options ...*TxOptions) error {
	if !ValidAddressOrSeed(sourceSeed) {
		return errors.Errorf("can't set master weight: invalid source address or seed: %s", sourceSeed)
	}

	tx := NewTx(ms.networkName, ms.params)

	if len(options) > 0 {
		tx.SetOptions(options[0])
	}

	tx.Build(sourceAccount(sourceSeed), build.MasterWeight(weight))
	tx.Sign(sourceSeed)
	tx.Submit()
	return tx.Err()
}

// AddSigner adds signerAddress as a signer to sourceSeed's account with weight signerWeight.
func (ms *MicroStellar) AddSigner(sourceSeed string, signerAddress string, signerWeight uint32, options ...*TxOptions) error {
	if !ValidAddressOrSeed(sourceSeed) {
		return errors.Errorf("can't add signer: invalid source address or seed: %s", sourceSeed)
	}

	if !ValidAddressOrSeed(signerAddress) {
		return errors.Errorf("can't add signer: invalid signer address or seed: %s", signerAddress)
	}

	tx := NewTx(ms.networkName, ms.params)

	if len(options) > 0 {
		tx.SetOptions(options[0])
	}

	tx.Build(sourceAccount(sourceSeed), build.AddSigner(signerAddress, signerWeight))
	tx.Sign(sourceSeed)
	tx.Submit()
	return tx.Err()
}

// RemoveSigner removes signerAddress as a signer from sourceSeed's account.
func (ms *MicroStellar) RemoveSigner(sourceSeed string, signerAddress string, options ...*TxOptions) error {
	if !ValidAddressOrSeed(sourceSeed) {
		return errors.Errorf("can't remove signer: invalid source address or seed: %s", sourceSeed)
	}

	if !ValidAddressOrSeed(signerAddress) {
		return errors.Errorf("can't remove signer: invalid signer address or seed: %s", signerAddress)
	}

	tx := NewTx(ms.networkName, ms.params)

	if len(options) > 0 {
		tx.SetOptions(options[0])
	}

	tx.Build(sourceAccount(sourceSeed), build.RemoveSigner(signerAddress))
	tx.Sign(sourceSeed)
	tx.Submit()
	return tx.Err()
}

// SetThresholds sets the signing thresholds for the account.
func (ms *MicroStellar) SetThresholds(sourceSeed string, low, medium, high uint32, options ...*TxOptions) error {
	if !ValidAddressOrSeed(sourceSeed) {
		return errors.Errorf("can't set thresholds: invalid source address or seed: %s", sourceSeed)
	}

	tx := NewTx(ms.networkName, ms.params)

	if len(options) > 0 {
		tx.SetOptions(options[0])
	}

	tx.Build(sourceAccount(sourceSeed), build.SetThresholds(low, medium, high))
	tx.Sign(sourceSeed)
	tx.Submit()
	return tx.Err()
}

// Payment represents a finalized payment in the ledger. You can subscribe to payments
// on the stellar network via the WatchPayments call.
type Payment horizon.Payment

// NewPaymentFromHorizon converts a horizon JSON payment struct to Payment
func NewPaymentFromHorizon(p *horizon.Payment) *Payment {
	payment := Payment(*p)
	return &payment
}

// PaymentWatcher is returned by WatchPayments, which watches the ledger for payments
// to and from an address.
type PaymentWatcher struct {
	// Ch gets a *Payment everytime there's a new entry in the ledger.
	Ch chan *Payment

	// Call Cancelfunc to stop watching the ledger. This closes Ch.
	CancelFunc func()

	// This is set if the stream terminates unexpectedly. Safe to check
	// after Ch is closed.
	Err *error
}

// WatchPayments watches the ledger for payments to and from address and streams them on a channel . Use
// TxOptions.WithContext to set a context.Context, and TxOptions.WithCursor to set a cursor.
func (ms *MicroStellar) WatchPayments(address string, options ...*TxOptions) (*PaymentWatcher, error) {
	if err := ValidAddress(address); err != nil {
		return nil, errors.Errorf("can't watch payments, invalid address: %s", address)
	}

	tx := NewTx(ms.networkName, ms.params)

	var cursor *horizon.Cursor
	var ctx context.Context
	var cancelFunc func()

	if len(options) > 0 {
		tx.SetOptions(options[0])
		if options[0].hasCursor {
			// Ugh! Why do I have to do this?
			c := horizon.Cursor(options[0].cursor)
			cursor = &c
		}
		ctx = options[0].ctx
	}

	if ctx == nil {
		ctx, cancelFunc = context.WithCancel(context.Background())
	} else {
		ctx, cancelFunc = context.WithCancel(ctx)
	}

	var streamError error
	ch := make(chan *Payment)

	go func(ch chan *Payment, streamError *error) {
		if tx.fake {
		out:
			for {
				select {
				case <-ctx.Done():
					break out
				default:
					// continue
				}
				ch <- &Payment{From: "FAKESOURCE", To: "FAKEDEST", Type: "payment", AssetCode: "QBIT", Amount: "5"}
				time.Sleep(200 * time.Millisecond)
			}
		} else {
			err := tx.GetClient().StreamPayments(ctx, address, cursor, func(payment horizon.Payment) {
				ch <- NewPaymentFromHorizon(&payment)
			})

			if err != nil {
				*streamError = errors.Wrapf(err, "payment stream disconnected")
				cancelFunc()
			}
		}

		close(ch)
	}(ch, &streamError)

	return &PaymentWatcher{ch, cancelFunc, &streamError}, nil
}