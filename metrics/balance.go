package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	AccountBalance = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "account_balance", Help: "Account balances"},
		[]string{"account", "chainID", "chainFamily", "denomination"},
	)
)
