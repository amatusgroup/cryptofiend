package orderbook

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mattkanwisher/cryptofiend/currency/pair"
)

// Const values for orderbook package
const (
	ErrOrderbookForExchangeNotFound = "Orderbook for exchange does not exist."
	ErrPrimaryCurrencyNotFound      = "Error primary currency for orderbook not found."
	ErrSecondaryCurrencyNotFound    = "Error secondary currency for orderbook not found."

	Spot = "SPOT"
)

// CalculateTotalBids returns the total amount of bids and the total orderbook
// bids value
func (o *Base) CalculateTotalBids() (float64, float64) {
	amountCollated := float64(0)
	total := float64(0)
	for _, x := range o.Bids {
		amountCollated += x.Amount
		total += x.Amount * x.Price
	}
	return amountCollated, total
}

// CalculateTotalAsks returns the total amount of asks and the total orderbook
// asks value
func (o *Base) CalculateTotalAsks() (float64, float64) {
	amountCollated := float64(0)
	total := float64(0)
	for _, x := range o.Asks {
		amountCollated += x.Amount
		total += x.Amount * x.Price
	}
	return amountCollated, total
}

// Update updates the bids and asks
func (o *Base) Update(Bids, Asks []Item) {
	o.Bids = Bids
	o.Asks = Asks
	o.LastUpdated = time.Now()
}

// Stores the order books, and provides helper methods
type Orderbooks struct {
	m          sync.Mutex
	orderbooks map[pair.CurrencyItem]map[pair.CurrencyItem]map[string]Base
}

// Item stores the amount and price values
type Item struct {
	Amount float64
	Price  float64
}

// Base holds the fields for the orderbook base
type Base struct {
	Pair         pair.CurrencyPair `json:"pair"`
	CurrencyPair string            `json:"CurrencyPair"`
	Bids         []Item            `json:"bids"`
	Asks         []Item            `json:"asks"`
	LastUpdated  time.Time         `json:"last_updated"`
}

// GetOrderbook checks and returns the orderbook given an exchange name and
// currency pair if it exists
func (o *Orderbooks) GetOrderbook(_ string, p pair.CurrencyPair, orderbookType string) (Base, error) {
	o.m.Lock()
	defer o.m.Unlock()

	fp := o.formatCurrencyPair(p)

	if !o.FirstCurrencyExists(fp.GetFirstCurrency()) {
		return Base{}, errors.New(ErrPrimaryCurrencyNotFound)
	}

	if !o.SecondCurrencyExists(fp) {
		err := fmt.Errorf("%s-%s-%s", ErrSecondaryCurrencyNotFound, fp.GetFirstCurrency(), fp.GetSecondCurrency())
		return Base{}, err
	}

	return o.orderbooks[fp.GetFirstCurrency()][fp.GetSecondCurrency()][orderbookType], nil
}

// FirstCurrencyExists checks to see if the first currency of the orderbook map
// exists
func (o *Orderbooks) FirstCurrencyExists(currency pair.CurrencyItem) bool {
	if _, ok := o.orderbooks[o.formatCurrency(currency)]; ok {
		return true
	}
	return false
}

// SecondCurrencyExists checks to see if the second currency of the orderbook
// map exists
func (o *Orderbooks) SecondCurrencyExists(p pair.CurrencyPair) bool {
	fp := o.formatCurrencyPair(p)

	if _, ok := o.orderbooks[fp.GetFirstCurrency()]; ok {
		if _, ok := o.orderbooks[fp.GetFirstCurrency()][fp.GetSecondCurrency()]; ok {
			return true
		}
	}
	return false
}

// ProcessOrderbook processes incoming orderbooks, creating or updating the
// Orderbook list
func (o *Orderbooks) ProcessOrderbook(_ string, p pair.CurrencyPair, orderbookNew Base, orderbookType string) {
	o.m.Lock()
	defer o.m.Unlock()

	// Use a single currency pair format internally regardless of the format used by the exchange/config.
	fp := o.formatCurrencyPair(p)

	orderbookNew.CurrencyPair = fp.Pair().String()
	orderbookNew.LastUpdated = time.Now()

	if o.FirstCurrencyExists(fp.GetFirstCurrency()) {
		if !o.SecondCurrencyExists(fp) {
			b := make(map[string]Base)
			b[orderbookType] = orderbookNew
			o.orderbooks[fp.FirstCurrency][fp.SecondCurrency] = b
			return
		} else {
			o.orderbooks[fp.FirstCurrency][fp.SecondCurrency][orderbookType] = orderbookNew
			return
		}
	}

	a := make(map[pair.CurrencyItem]map[string]Base)
	b := make(map[string]Base)
	b[orderbookType] = orderbookNew
	a[fp.SecondCurrency] = b
	o.orderbooks[fp.FirstCurrency] = a
}

// Returns a new currency pair based on the given one that's formatted using the internal format.
func (o *Orderbooks) formatCurrencyPair(p pair.CurrencyPair) pair.CurrencyPair {
	return p.FormatPair("/", false)
}

func (o *Orderbooks) formatCurrency(currency pair.CurrencyItem) pair.CurrencyItem {
	return currency.Lower()
}

// Init creates a new set of Orderbooks
func Init() Orderbooks {
	obs := Orderbooks{}
	obs.m = sync.Mutex{}
	obs.orderbooks = make(map[pair.CurrencyItem]map[pair.CurrencyItem]map[string]Base)
	return obs
}
