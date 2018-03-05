package cli

import (
	"fmt"

	"github.com/0xfe/lumen/store"
	"github.com/0xfe/microstellar"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

type CLI struct {
	store store.API
	ms    *microstellar.MicroStellar
	ns    string // namespace
}

// NewCLI
func NewCLI(store store.API, ms *microstellar.MicroStellar) *CLI {
	return &CLI{
		store: store,
		ms:    ms,
		ns:    "default",
	}
}

func (cli *CLI) Install(rootCmd *cobra.Command) {
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Get version of lumen CLI",
		Run:   cli.cmdVersion,
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "set [key] [value]",
		Short: "set variable",
		Args:  cobra.MinimumNArgs(2),
		Run:   cli.cmdSet,
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "get [key]",
		Short: "get variable",
		Args:  cobra.MinimumNArgs(1),
		Run:   cli.cmdGet,
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "del [key]",
		Short: "delete variable",
		Args:  cobra.MinimumNArgs(1),
		Run:   cli.cmdDel,
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "watch [address]",
		Short: "watch the address on the ledger",
		Args:  cobra.MinimumNArgs(1),
		Run:   cli.cmdWatch,
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "pay [source] [target] [amount]",
		Short: "pay [amount] lumens from [source] to [target]",
		Args:  cobra.MinimumNArgs(3),
		Run:   cli.cmdPay,
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "balance [address]",
		Short: "get the balance of [address] in lumens",
		Args:  cobra.MinimumNArgs(1),
		Run:   cli.cmdBalance,
	})

	rootCmd.AddCommand(cli.getAccountsCmd())
}

// SetVar writes the kv pair to the storage backend
func (cli *CLI) SetVar(key string, value string) error {
	key = fmt.Sprintf("%s:%s", cli.ns, key)
	logrus.WithFields(logrus.Fields{"type": "cli", "method": "SetVar"}).Debugf("setting %s: %s", key, value)
	return cli.store.Set(key, value, 0)
}

func (cli *CLI) GetVar(key string) (string, error) {
	key = fmt.Sprintf("%s:%s", cli.ns, key)
	logrus.WithFields(logrus.Fields{"type": "cli", "method": "GetVar"}).Debugf("getting %s", key)
	return cli.store.Get(key)
}

func (cli *CLI) DelVar(key string) error {
	key = fmt.Sprintf("%s:%s", cli.ns, key)
	logrus.WithFields(logrus.Fields{"type": "cli", "method": "DelVar"}).Debugf("deleting %s", key)
	return cli.store.Delete(key)
}

func (cli *CLI) cmdVersion(cmd *cobra.Command, args []string) {
	showSuccess("v0.1\n")
}

func (cli *CLI) cmdSet(cmd *cobra.Command, args []string) {
	key := fmt.Sprintf("vars:%s", args[0])
	val := args[1]

	err := cli.SetVar(key, val)
	if err != nil {
		showError(logrus.Fields{"cmd": "set"}, "set failed: ", err)
		return
	}

	showSuccess("setting %s to %s\n", args[0], args[1])
}

func (cli *CLI) cmdDel(cmd *cobra.Command, args []string) {
	key := fmt.Sprintf("vars:%s", args[0])

	err := cli.DelVar(key)
	if err != nil {
		showError(logrus.Fields{"cmd": "del"}, "del failed: %s\n", err)
	}
}

func (cli *CLI) cmdGet(cmd *cobra.Command, args []string) {
	key := fmt.Sprintf("vars:%s", args[0])

	val, err := cli.GetVar(key)
	if err == nil {
		showSuccess(val + "\n")
	} else {
		showError(logrus.Fields{"cmd": "get"}, "no such variable: %s\n", args[0])
	}
}

func (cli *CLI) cmdWatch(cmd *cobra.Command, args []string) {
	address := args[0]

	watcher, err := cli.ms.WatchPayments(address)

	if err != nil {
		showError(logrus.Fields{"cmd": "watch"}, "can't watch address: %+v\n", err)
		return
	}

	for p := range watcher.Ch {
		showSuccess("%v %v from %v to %v\n", p.Amount, p.AssetCode, p.From, p.To)
	}

	if watcher.Err != nil {
		showError(logrus.Fields{"cmd": "watch"}, "%+v\n", *watcher.Err)
	}
}

func (cli *CLI) cmdPay(cmd *cobra.Command, args []string) {
	fields := logrus.Fields{"cmd": "pay"}
	source := cli.validateAddressOrSeed(fields, args[0], "seed")
	target := cli.validateAddressOrSeed(fields, args[1], "address")

	amount := args[2]
	err := cli.ms.PayNative(source, target, amount)

	if err != nil {
		showError(fields, "payment failed: %v", microstellar.ErrorString(err))
	} else {
		showSuccess("paid\n")
	}
}

func (cli *CLI) cmdBalance(cmd *cobra.Command, args []string) {
	address := args[0]

	account, err := cli.ms.LoadAccount(address)

	if err != nil {
		showError(logrus.Fields{"cmd": "balance"}, "payment failed: %v", err)
	} else {
		showSuccess("%v\n", account.GetNativeBalance())
	}
}