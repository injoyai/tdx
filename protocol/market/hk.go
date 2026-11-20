package market

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	MarketHK       ID       = "hk/stock"
	MarketHKFund   ID       = "hk/fund"
	MarketHKIndex  ID       = "hk/index"
	ExchangeHK     Exchange = 31
	ExchangeHKFund Exchange = 49
	ExchangeHI     Exchange = 27
)

type hk struct {
	mu    sync.RWMutex
	file  string
	codes map[string]Exchange
}

var _ Market = (*hk)(nil)
var _ instrumentMarketLoader = (*hk)(nil)

func (*hk) Info() MarketInfo {
	return MarketInfo{
		Prefix: "hk", Name: "香港交易所", TimeZone: "Asia/Hong_Kong", CloseTime: "16:10",
		Routes: []RouteInfo{
			{ID: MarketHK, Exchange: ExchangeHK, Name: "香港交易所", Protocol: ProtocolExtended, CodeSource: CodeListExtended, Snapshot: true, QuoteListCategory: 2},
			{ID: MarketHKFund, Exchange: ExchangeHKFund, Name: "香港基金", Protocol: ProtocolExtended, CodeSource: CodeListExtended, Snapshot: true, QuoteListCategory: 2},
			{ID: MarketHKIndex, Exchange: ExchangeHI, Name: "港股指数", Protocol: ProtocolExtended, CodeSource: CodeListExtended, BareSymbols: []string{"HSI", "HKL", "VHSI", "CES100", "GEM"}},
		},
	}
}

func (m *hk) Resolve(code Code, snapshot InstrumentSnapshot) (Instrument, error) {
	switch code.Exchange {
	case ExchangeHK, ExchangeHKFund:
		if len(code.Symbol) == 5 && isDigits(code.Symbol) {
			if !code.explicit {
				return snapshot.resolve(InstrumentLookup{
					Code: code.Symbol, Exchanges: []Exchange{ExchangeHK, ExchangeHKFund},
					prefix: "hk", knownExchange: m.knownExchange(code.Symbol),
				})
			}
			return Instrument{Exchange: code.Exchange, Code: code.Symbol}, nil
		}
		if code.Exchange == ExchangeHKFund || code.explicit {
			return validateInstrument(code.Exchange, code, false)
		}
		code.Exchange = ExchangeHI
	case ExchangeHI:
	default:
		return Instrument{}, fmt.Errorf("行情市场 %d 不属于 hk", code.Exchange)
	}
	letter, _, valid := symbolParts(code.Symbol, "")
	code.Symbol = strings.ToUpper(code.Symbol)
	return validateInstrument(ExchangeHI, code, valid && letter)
}

func (*hk) Rules(i Instrument) (InstrumentRules, error) {
	switch i.Exchange {
	case ExchangeHK, ExchangeHKFund:
		rules := equityRules(SharesPerLot, 10_000)
		rules.Security = SecurityStock
		if i.Exchange == ExchangeHKFund {
			rules.Security = SecurityFund
		}
		rules.Trade.Nature = TradeHK
		return rules, nil
	case ExchangeHI:
		rules := extendedRules(SecurityIndex)
		rules.extendedKlineAmountScale = 1_000_000
		rules.DayFields = Fields{Price: true, Amount: true}
		rules.IntradayFields, rules.LongFields = rules.DayFields, rules.DayFields
		return rules, nil
	default:
		return InstrumentRules{}, fmt.Errorf("行情市场 %d 不属于 hk", i.Exchange)
	}
}

func (m *hk) knownExchange(code string) Exchange {
	m.mu.RLock()
	exchange, ok := m.codes[code]
	m.mu.RUnlock()
	if ok {
		return exchange
	}
	return knownHKExchange(code)
}

func (m *hk) loadInstrumentMarkets(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	file := filepath.Join(dir, "hk-instruments.json")
	codes := make(map[string]Exchange)
	data, err := os.ReadFile(file)
	switch {
	case err == nil:
		if err := json.Unmarshal(data, &codes); err != nil {
			return err
		}
		if codes == nil {
			return fmt.Errorf("%s: 证券归属必须是 JSON 对象", file)
		}
		for code, exchange := range codes {
			if !isHKStockOrFund(code, exchange) {
				return fmt.Errorf("%s: 无效的证券归属 %s / %d", file, code, exchange)
			}
		}
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	m.file, m.codes = file, codes
	return nil
}

func (m *hk) Import(items []Instrument) error {
	changes := make(map[string]Exchange)
	for _, item := range items {
		if isHKStockOrFund(item.Code, item.Exchange) {
			changes[item.Code] = item.Exchange
		}
	}
	if len(changes) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.file == "" {
		return fmt.Errorf("请先调用 LoadInstrumentMarkets 配置证券市场归属目录")
	}
	for code, exchange := range changes {
		if m.codes[code] == exchange {
			delete(changes, code)
		}
	}
	if len(changes) == 0 {
		return nil
	}
	codes := maps.Clone(m.codes)
	maps.Copy(codes, changes)
	if err := writeHKInstrumentMarkets(m.file, codes); err != nil {
		return err
	}
	m.codes = codes
	return nil
}

func isHKStockOrFund(code string, exchange Exchange) bool {
	return (exchange == ExchangeHK || exchange == ExchangeHKFund) && len(code) == 5 && isDigits(code)
}

func writeHKInstrumentMarkets(file string, codes map[string]Exchange) error {
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".hk-instruments-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := json.NewEncoder(tmp).Encode(codes); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

// knownHKExchange lists numeric codes observed in Tongdaxin responses on 2026-11-19.
// See output/hk-market-resolution-20261119 and output/all-type-crosscheck-20261119.
// A code missing from this list may still exist on the server.
func knownHKExchange(code string) Exchange {
	switch code {
	case "00001", "00002", "00003", "00004", "00005", "00006", "00008", "00010", "00012",
		"00013", "00014", "00016", "00017", "00018", "00019", "00020", "00021", "00022",
		"00023", "00025", "00026", "00027", "00028", "00029", "00030", "00031", "00032",
		"00033", "00034", "00035", "00036", "00037", "00038", "00039", "00040", "00041",
		"00042", "00045", "00046", "00048", "00050", "00051", "00052", "00053", "00054",
		"00055", "00057", "00058", "00059", "00060", "00061", "00062", "00063", "00064",
		"00065", "00066", "00068", "00069", "00070", "00071", "00072", "00073", "00075",
		"00076", "00077", "00078", "00079", "00080", "00081", "00082", "00083", "00084",
		"00085", "00086", "00087", "00088", "00089", "00090", "00092", "00093", "00094",
		"00095", "00096", "00097", "00098", "00099", "00100", "00101", "00103", "00104",
		"00105", "00106", "00107", "00108", "00110", "00111", "00113", "00114", "00115",
		"00116", "00117", "00118", "00119", "00120", "00122", "00123", "00124", "00125",
		"00126", "00127", "00128", "00129", "00130", "00131", "00132", "00133", "00135",
		"00136", "00137", "00138", "00139", "00142", "00144", "00145", "00146", "00147",
		"00148", "00149", "00150", "00151", "00152", "00154", "00156", "00157", "00158",
		"00159", "00160", "00162", "00163", "00164", "00165", "00166", "00167", "00168",
		"00169", "00171", "00173", "00174", "00175", "00176", "00177", "00178", "00179",
		"00180", "00181", "00182", "00183", "00184", "00185", "00186", "00700":
		return ExchangeHK
	case "00405", "00435", "00778", "00808", "00823", "01426", "01503", "01881", "02191",
		"02778", "02800", "02801", "02802", "02803", "02804", "02805", "02806", "02807",
		"02809", "02810", "02811", "02812", "02813", "02814", "02815", "02817", "02818",
		"02819", "02820", "02821", "02822", "02823", "02824", "02825", "02826", "02827",
		"02828", "02829", "02830", "02831", "02832", "02834", "02835", "02836", "02837",
		"02838", "02839", "02840", "02841", "02843", "02845", "02846", "02848", "03001",
		"03002", "03003", "03004", "03005", "03006", "03007", "03008", "03009", "03010",
		"03011", "03012", "03014", "03015", "03020", "03024", "03029", "03030", "03031",
		"03032", "03033", "03034", "03036", "03037", "03038", "03039", "03040", "03041",
		"03042", "03046", "03047", "03050", "03053", "03056", "03059", "03064", "03066",
		"03067", "03068", "03069", "03070", "03071", "03074", "03075", "03076", "03077",
		"03081", "03084", "03085", "03086", "03087", "03088", "03093", "03096", "03097",
		"03101", "03102", "03104", "03108", "03109", "03110", "03111", "03112", "03115",
		"03116", "03118", "03119", "03121", "03122", "03128", "03129", "03132", "03133",
		"03134", "03135", "03136", "03137", "03139", "03140", "03141", "03143", "03145",
		"03146", "03147", "03149", "03150", "03151", "03152", "03153", "03156", "03158",
		"03160", "03161", "03165", "03167", "03169", "03170", "03171", "03173", "03174",
		"03175", "03176", "03179", "03181", "03182", "03184", "03185":
		return ExchangeHKFund
	default:
		return 0
	}
}

func init() { registerMarket(&hk{}) }
