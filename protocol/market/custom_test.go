package market

import (
	"math"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRegisterDefinition(t *testing.T) {
	def := Definition{Key: "testex", Exchange: 252, CodePattern: "[A-Z][a-z][0-9]|[A-Z][a-z][0-9]{2}"}
	for _, mutate := range []func(*Definition){
		func(d *Definition) { d.Key = "TestEx" },
		func(d *Definition) { d.Key = "hi" },
		func(d *Definition) { d.Key = "sh" },
		func(d *Definition) { d.Key = "cni" },
		func(d *Definition) { d.Exchange = ExchangeSH },
		func(d *Definition) { d.Exchange = ExchangeCNI },
		func(d *Definition) { d.CodePattern = "[" },
		func(d *Definition) { d.DayVolumeScale = -1 },
		func(d *Definition) { d.AmountScale = math.NaN() },
	} {
		bad := def
		mutate(&bad)
		if err := registerDef(bad); err == nil {
			t.Fatalf("invalid registration accepted %+v", bad)
		}
	}
	if _, exists := def.Key.Exchange(); !exists {
		var successes atomic.Int32
		var tasks sync.WaitGroup
		for i := 0; i < 8; i++ {
			tasks.Add(1)
			go func() {
				defer tasks.Done()
				if registerDef(def) == nil {
					successes.Add(1)
				}
				_, _ = ParseInstrument("testex.Aa12")
				_ = Markets()
			}()
		}
		tasks.Wait()
		if successes.Load() != 1 {
			t.Fatalf("successful registrations = %d", successes.Load())
		}
	}
	for _, input := range []string{"testex.Aa12", "Aa12.TESTEX", "Aa12.252", "testexAa12"} {
		got, err := ParseInstrument(input)
		if err != nil || got != (Instrument{Exchange: 252, Code: "Aa12"}) {
			t.Fatal(input, got, err)
		}
	}
	for _, input := range []string{"testex.Aa123", "testex.xAa12", "testex../Aa12"} {
		if _, err := ParseInstrument(input); err == nil {
			t.Fatal("invalid code accepted", input)
		}
	}
	conflict := def
	conflict.Key = "another"
	if err := registerDef(conflict); err == nil {
		t.Fatal("duplicate wire market accepted")
	}
	if _, exists := conflict.Key.Exchange(); exists {
		t.Fatal("failed registration published partial state")
	}
	info, _ := def.Key.Info()
	info.Routes[0].Name = "changed"
	again, _ := def.Key.Info()
	if again.Routes[0].Name == "changed" {
		t.Fatal("definition was mutated through a returned value")
	}
}

func TestCustomPrefixKeepsExplicitSuffix(t *testing.T) {
	if _, known := (ID("aa")).Exchange(); !known {
		if err := registerDef(Definition{Key: "aa", Exchange: 246, CodePattern: "[A-Za-z.]+"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := ParseInstrument("AAPL.US")
	if err != nil || got.Exchange != ExchangeUS || got.Code != "AAPL" {
		t.Fatal(got, err)
	}
	got, err = ParseInstrument("aa.AAPL.US")
	if err != nil || got.Exchange != 246 || got.Code != "AAPL.US" {
		t.Fatal(got, err)
	}
}

func registerDef(def Definition) error {
	m, err := New(def)
	if err != nil {
		return err
	}
	return Register(m)
}
