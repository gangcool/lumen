package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/0xfe/microstellar"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func showSuccess(msg string, args ...interface{}) {
	fmt.Printf(msg+"\n", args...)
}

func showError(fields logrus.Fields, msg string, args ...interface{}) {
	logrus.WithFields(fields).Errorf(msg, args...)
}

func (cli *CLI) help(cmd *cobra.Command, args []string) {
	fmt.Fprint(os.Stderr, cmd.UsageString())

	if !cli.testing {
		os.Exit(-1)
	} else {
		fmt.Println("error")
	}
}

func debugf(fields logrus.Fields, msg string, args ...interface{}) {
	logrus.WithFields(fields).Debugf(msg, args...)
}

func (cli *CLI) error(logFields logrus.Fields, msg string, args ...interface{}) {
	showError(logFields, msg, args...)

	if !cli.testing {
		os.Exit(-1)
	} else {
		fmt.Println("error")
	}
}

func buildFlagsForTxOptions(cmd *cobra.Command) {
	cmd.Flags().Bool("nosign", false, "don't sign transaction")
	cmd.Flags().String("memotext", "", "memo text")
	cmd.Flags().String("memoid", "", "memo ID")
	cmd.Flags().StringSlice("signers", []string{}, "alternate signers (comma separated)")
}

func (cli *CLI) genTxOptions(cmd *cobra.Command, logFields logrus.Fields) (*microstellar.Options, error) {
	opts := microstellar.Opts()

	if memotext, err := cmd.Flags().GetString("memotext"); err == nil && memotext != "" {
		opts = opts.WithMemoText(memotext)
	}

	if memoid, err := cmd.Flags().GetString("memoid"); err == nil && memoid != "" {
		id, err := strconv.ParseUint(memoid, 10, 64)
		if err != nil {
			logrus.WithFields(logFields).Debugf("error parsing memoid: %v", err)
			return nil, errors.Errorf("bad memoid: %s", memoid)
		}
		opts = opts.WithMemoID(id)
	}

	if signers, err := cmd.Flags().GetStringSlice("signers"); err == nil && len(signers) > 0 {
		for _, signer := range signers {
			logrus.WithFields(logFields).Debugf("adding signer: %s", signer)
			address, err := cli.ResolveAccount(logFields, signer, "seed")

			if err != nil {
				logrus.WithFields(logFields).Debugf("bad signer %s: %v", signer, err)
				return nil, errors.Errorf("bad signer: %s", signer)
			}

			opts = opts.WithSigner(address)
		}
	}

	if nosubmit, _ := cli.rootCmd.Flags().GetBool("nosubmit"); nosubmit {
		handler := func(args ...interface{}) (bool, error) {
			showSuccess(args[0].(string))
			return false, nil
		}

		txHandler := microstellar.TxHandler(handler)
		logrus.WithFields(logFields).Debugf("sign-only transaction")
		opts = opts.On(microstellar.EvBeforeSubmit, &txHandler)
	}

	if nosign, err := cmd.Flags().GetBool("nosign"); err == nil && nosign {
		opts = opts.SkipSignatures()
	}

	return opts, nil
}

// ResolveAccount returns an address or seed (depending on keyType), by looking up lookupKey
// in the local store (or in federation servers.)
func (cli *CLI) ResolveAccount(fields logrus.Fields, lookupKey string, keyType string) (string, error) {
	var err error
	addressOrSeed := lookupKey

	if strings.Contains(lookupKey, "*") {
		logrus.WithFields(fields).Debugf("resolving federation address: %s", lookupKey)
		resolvedAddr, err := cli.ms.Resolve(lookupKey)

		if err == nil {
			logrus.WithFields(fields).Debugf("got address: %s = %s", lookupKey, resolvedAddr)
			addressOrSeed = resolvedAddr
			lookupKey = resolvedAddr
		}
	}

	if !microstellar.ValidAddressOrSeed(lookupKey) {
		addressOrSeed, err = cli.GetAccountOrSeed(lookupKey, keyType)
		if err != nil {
			logrus.WithFields(fields).Debugf("invalid address, seed, or account name: %s", lookupKey)
			return "", err
		}

		if strings.Contains(addressOrSeed, "*") {
			return cli.ResolveAccount(fields, addressOrSeed, keyType)
		}
	}

	return addressOrSeed, nil
}

// ResolveAsset looks up name and returns a microstellar Asset
func (cli *CLI) ResolveAsset(name string) (*microstellar.Asset, error) {
	if name == "" || name == "native" {
		return microstellar.NativeAsset, nil
	}

	var code, issuer, assetType string
	if strings.Contains(name, ":") {
		var issuerName string
		parts := strings.Split(name, ":")
		if len(parts) < 2 {
			return nil, errors.Errorf("bad asset: %s", name)
		}

		code = parts[0]
		issuerName = parts[1]

		if len(parts) > 2 {
			assetType = parts[2]
		} else {
			if len(code) <= 5 {
				assetType = string(microstellar.Credit4Type)
			} else {
				assetType = string(microstellar.Credit12Type)
			}
		}

		var err error
		issuer, err = cli.ResolveAccount(logrus.Fields{"method": "ResolveAsset"}, issuerName, "address")
		if err != nil {
			return nil, errors.Errorf("bad asset issuer: %v", issuerName)
		}
	} else {
		readField := func(field string) (string, error) {
			key := fmt.Sprintf("asset:%s:%s", name, field)
			val, err := cli.GetVar(key)
			if err != nil {
				return "", err
			}

			return val, nil
		}

		var err1, err2, err3 error
		code, err1 = readField("code")
		issuer, err2 = readField("issuer")
		assetType, err3 = readField("type")

		if err1 != nil || err2 != nil || err3 != nil {
			return nil, errors.Errorf("could not read asset: %v, %v, %v", err1, err2, err3)
		}
	}

	var asset *microstellar.Asset

	if assetType == string(microstellar.Credit4Type) {
		asset = microstellar.NewAsset(code, issuer, microstellar.Credit4Type)
	} else if assetType == string(microstellar.Credit12Type) {
		asset = microstellar.NewAsset(code, issuer, microstellar.Credit12Type)
	} else {
		asset = microstellar.NativeAsset
	}

	logrus.Debugf("got asset: %+v", asset)
	return asset, nil
}

// GetAccount returns the account address or seed for "name". Set keyType
// to "address" or "seed" to specify the return value.
func (cli *CLI) GetAccount(name, keyType string) (string, error) {
	if keyType != "address" && keyType != "seed" {
		return name, errors.Errorf("invalid key type: %s", keyType)
	}

	key := fmt.Sprintf("account:%s:%s", name, keyType)

	code, err := cli.GetVar(key)

	if err != nil {
		return name, err
	}

	return code, nil
}

// GetAccountOrSeed returns the account address or seed for "name". It prefers
// keyType ("address" or "seed")
func (cli *CLI) GetAccountOrSeed(name, keyType string) (string, error) {
	code, err := cli.GetAccount(name, keyType)

	if err != nil {
		if keyType == "address" {
			keyType = "seed"
		} else {
			keyType = "address"
		}

		code, err = cli.GetAccount(name, keyType)
	}

	return code, err
}

// LoadAccount loads information for "name" from horizon.
func (cli *CLI) LoadAccount(logFields logrus.Fields, name string) *microstellar.Account {
	address, err := cli.ResolveAccount(logFields, name, "address")

	if err != nil {
		cli.error(logFields, "invalid address: %s", name)
		return nil
	}

	account, err := cli.ms.LoadAccount(address)

	if err != nil {
		cli.error(logFields, "can't load account: %v", microstellar.ErrorString(err))
		return nil
	}

	return account
}
