package binance

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/mattkanwisher/cryptofiend/common"
	"github.com/mattkanwisher/cryptofiend/currency/pair"
	exchange "github.com/mattkanwisher/cryptofiend/exchanges"
)

const (
	binanceBaseURL          = "https://www.binance.com/"
	binanceExchangeInfoPath = "api/v1/exchangeInfo"
	binanceAccountPath      = "api/v3/account"
	binanceOpenOrdersPath   = "api/v3/openOrders"
	binanceOrderPath        = "api/v3/order"
	binanceOrderTestPath    = "api/v3/order/test"
	binanceDepthPath        = "api/v1/depth"
)

// BinanceErrCode enum represents a frequently encountered subset of the error codes documented at:
// https://github.com/binance-exchange/binance-official-api-docs/blob/master/errors.md
type BinanceErrCode int32

const (
	InvalidTimestampErrCode BinanceErrCode = -1021 // fix: sync your computer clock to internet time
)

type rateLimitInfo struct {
	StartTime    int64
	RequestCount uint
}

type Binance struct {
	exchange.Base
	rateLimits map[string]*rateLimitInfo
	// Maps symbol (exchange specific market identifier) to currency pair info
	currencyPairs map[pair.CurrencyItem]*exchange.CurrencyPairInfo
}

// CurrencyPairToSymbol converts a currency pair to a symbol (exchange specific market identifier).
func (b *Binance) CurrencyPairToSymbol(p pair.CurrencyPair) string {
	return p.
		Display(b.RequestCurrencyPairFormat.Delimiter, b.RequestCurrencyPairFormat.Uppercase).
		String()
}

// SymbolToCurrencyPair converts a symbol (exchange specific market identifier) to a currency pair.
func (b *Binance) SymbolToCurrencyPair(symbol string) (pair.CurrencyPair, error) {
	if p, exists := b.currencyPairs[pair.CurrencyItem(symbol)]; exists {
		return p.Currency.FormatPair(
			b.RequestCurrencyPairFormat.Delimiter, b.RequestCurrencyPairFormat.Uppercase), nil
	}
	return pair.CurrencyPair{}, fmt.Errorf("no currency pair found for '%s' symbol", symbol)
}

// FetchExchangeInfo fetches current exchange trading rules and symbol information.
func (b *Binance) FetchExchangeInfo() (*ExchangeInfo, error) {
	response := ExchangeInfo{}
	err := common.SendHTTPGetRequest(binanceBaseURL+binanceExchangeInfoPath, true, b.Verbose, &response)
	return &response, err
}

// FetchAccountInfo fetches current account information.
func (b *Binance) FetchAccountInfo() (*AccountInfo, error) {
	response := AccountInfo{}
	_, err := b.SendHTTPRequest(http.MethodGet, binanceAccountPath, nil, true, &response)
	return &response, err
}

// FetchOpenOrders fetches all currently open orders.
func (b *Binance) FetchOpenOrders() ([]Order, error) {
	response := []Order{}
	// TODO: This endpoint takes an optional list of symbols to return orders for, it's cheaper
	// to query only a few symbols rather than all of them (from a rate limiting standpoint).
	_, err := b.SendHTTPRequest(http.MethodGet, binanceOpenOrdersPath, nil, true, &response)
	return response, err
}

type PostOrderParams struct {
	Symbol           string
	Side             OrderSide
	Type             OrderType
	TimeInForce      TimeInForce
	Quantity         float64
	Price            float64
	NewClientOrderID string
	StopPrice        float64
	IcebergQty       float64
	// Set to true to submit the order to the test endpoint for validation,
	// it won't be sent to the exchange matching engine.
	ValidateOnly bool
}

func (b *Binance) PostOrderAck(params *PostOrderParams) (*PostOrderAckResponse, error) {
	v := url.Values{}
	v.Set("symbol", params.Symbol)
	v.Set("side", string(params.Side))
	v.Set("type", string(params.Type))
	v.Set("timeInForce", string(params.TimeInForce))
	v.Set("quantity", strconv.FormatFloat(params.Quantity, 'f', -1, 64))
	v.Set("price", strconv.FormatFloat(params.Price, 'f', -1, 64))
	if params.NewClientOrderID != "" {
		v.Set("newClientOrderId", params.NewClientOrderID)
	}
	if params.StopPrice != 0 {
		v.Set("stopPrice", strconv.FormatFloat(params.StopPrice, 'f', -1, 64))
	}
	if params.IcebergQty != 0 {
		v.Set("icebergQty", strconv.FormatFloat(params.IcebergQty, 'f', -1, 64))
	}
	v.Set("newOrderRespType", "ACK")

	response := PostOrderAckResponse{}
	path := binanceOrderPath
	if params.ValidateOnly {
		path = binanceOrderTestPath
	}
	_, err := b.SendHTTPRequest(http.MethodPost, path, v, true, &response)
	return &response, err
}

// FetchOrder fetches an order from the exchange, either orderID or clientOrderID must be provided.
func (b *Binance) FetchOrder(symbol string, orderID int64, clientOrderID string) (*Order, error) {
	v := url.Values{}
	v.Set("symbol", symbol)
	if orderID != 0 {
		v.Set("orderId", strconv.FormatInt(orderID, 10))
	}
	if clientOrderID != "" {
		v.Set("origClientOrderId", clientOrderID)
	}
	response := Order{}
	_, err := b.SendHTTPRequest(http.MethodGet, binanceOrderPath, v, true, &response)
	return &response, err
}

// DeleteOrder cancels an active order on the exchange, either orderID or clientOrderID must be provided.
func (b *Binance) DeleteOrder(symbol string, orderID int64, clientOrderID string) error {
	v := url.Values{}
	v.Set("symbol", symbol)
	if orderID != 0 {
		v.Set("orderId", strconv.FormatInt(orderID, 10))
	}
	if clientOrderID != "" {
		v.Set("origClientOrderId", clientOrderID)
	}
	response := DeleteOrderResponse{}
	_, err := b.SendHTTPRequest(http.MethodDelete, binanceOrderPath, v, true, &response)
	return err
}

// FetchMarketData fetches the orderbooks for the given symbol.
// The limit parameter can be -1, 0, 5, 10, 20, 50, 100, 200, 1000.
// Set the limit to -1 to use the default value (currently 100), or to 0 to disable the limit
// (this can return a lot of data, so should avoided).
func (b *Binance) FetchMarketData(symbol string, limit int64) (*MarketData, error) {
	v := url.Values{}
	v.Set("symbol", symbol)
	if limit > -1 {
		v.Set("limit", strconv.FormatInt(limit, 10))
	}
	response := MarketData{}
	_, err := b.SendHTTPRequest(http.MethodGet, binanceDepthPath, v, false, &response)
	return &response, err
}

// SendAuthenticatedHTTPRequest sends a POST request to an authenticated endpoint, the response is
// decoded into the result object.
// Returns the Binance error code and error message (if any).
func (b *Binance) SendHTTPRequest(method, path string, params url.Values, sign bool,
	result interface{}) (int, error) {
	if !b.AuthenticatedAPISupport {
		return 0, fmt.Errorf(exchange.WarningAuthenticatedRequestWithoutCredentialsSet, b.Name)
	}

	if b.Verbose {
		log.Printf("Request params: %v\n", params)
	}

	headers := make(http.Header)
	headers["Accept"] = []string{"application/json"}

	var payload string
	if len(params) > 0 {
		payload = params.Encode()
	}

	if sign {
		recvWindow := 5000
		timestamp := time.Now().UnixNano() / (1000 * 1000) // must be in milliseconds
		timeWindow := fmt.Sprintf("timestamp=%v&recvWindow=%d", timestamp, recvWindow)
		if payload != "" {
			payload += "&" + timeWindow
		} else {
			payload = timeWindow
		}
		hmac := common.GetHMAC(common.HashSHA256, []byte(payload), []byte(b.APISecret))
		payload = fmt.Sprintf("%s&signature=%s", payload, hex.EncodeToString(hmac))
		headers["X-MBX-APIKEY"] = []string{b.APIKey}
	}

	var resp string
	var statusCode int
	var err error
	if method == http.MethodGet {
		resp, statusCode, err = common.SendHTTPRequest2(
			method, fmt.Sprintf("%s%s?%s", binanceBaseURL, path, payload), headers, nil)
	} else {
		headers["Content-Type"] = []string{"application/x-www-form-urlencoded"}
		resp, statusCode, err = common.SendHTTPRequest2(method,
			binanceBaseURL+path, headers, strings.NewReader(payload))
	}

	if err != nil {
		return 0, err
	}

	if b.Verbose {
		log.Printf("Received raw: \n%s\n", resp)
	}

	if 200 <= statusCode && statusCode <= 299 {
		if err = common.JSONDecode([]byte(resp), &result); err != nil {
			return statusCode, errors.New("failed to unmarshal response")
		}
	} else {
		var errInfo ErrorInfo
		if err = common.JSONDecode([]byte(resp), &errInfo); err != nil {
			return 0, errors.New("failed to unmarshal error info")
		}
		return int(errInfo.Code), errors.New(errInfo.Message)
	}

	return 0, nil
}

// SendRateLimitedHTTPRequest sends an HTTP request if the given number of requests per minute
// hasn't been exceeded for the specified method & path and unmarshals the response into the
// result parameter. If the number of requests per minute has been exceeded this method will
// set the result to the default value (which can be a pointer, but must not be nil).
func (b *Binance) SendRateLimitedHTTPRequest(requestsPerMin uint, method string, path string,
	params url.Values, result interface{}, defaultValue interface{}) error {
	rateLimit := b.rateLimits[method+path]
	if rateLimit == nil {
		rateLimit = &rateLimitInfo{}
		b.rateLimits[method+path] = rateLimit
	}

	curTimeStamp := time.Now().Unix()
	if (rateLimit.StartTime == 0) || ((curTimeStamp - rateLimit.StartTime) > 90) {
		rateLimit.RequestCount = 0
		rateLimit.StartTime = curTimeStamp
	}
	if rateLimit.RequestCount < requestsPerMin {
		rateLimit.RequestCount++
	} else {
		// set result to default value
		rv := reflect.ValueOf(result)
		if rv.Kind() != reflect.Ptr || rv.IsNil() {
			return errors.New("result must be a non-nil pointer")
		}
		dv := reflect.ValueOf(defaultValue)
		if !dv.IsValid() {
			return errors.New("default value must be not be nil")
		}
		if dv.Kind() == reflect.Ptr {
			reflect.Indirect(rv).Set(dv.Elem())
		} else {
			reflect.Indirect(rv).Set(dv)
		}
		return nil
	}

	_, err := b.SendHTTPRequest(method, path, params, true, result)
	return err
}
