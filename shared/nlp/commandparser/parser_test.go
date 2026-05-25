package commandparser

import (
	"testing"
)

func TestParse(t *testing.T) {
	parser := New()

	tests := []struct {
		name          string
		tweetText     string
		sourceTweetID string
		wantCount     int
		wantTotal     float64
		wantEach      float64
		wantCurrency  Currency
		wantRule      EntryRule
	}{
		{
			name:          "Total NGN, follow host",
			tweetText:     "@bot select 10 followers, ₦50,000 total",
			sourceTweetID: "12345",
			wantCount:     10,
			wantTotal:     50000,
			wantEach:      5000,
			wantCurrency:  CurrencyNGN,
			wantRule:      RuleMustFollow,
		},
		{
			name:          "USD each, follow host",
			tweetText:     "@bot pick 3 people who follow me, $50 each",
			sourceTweetID: "67890",
			wantCount:     3,
			wantTotal:     150,
			wantEach:      50,
			wantCurrency:  CurrencyUSD,
			wantRule:      RuleMustFollow,
		},
		{
			name:          "Random replies NGN",
			tweetText:     "@bot randomly pick 1 winner from replies, ₦20,000",
			sourceTweetID: "11111",
			wantCount:     1,
			wantTotal:     20000,
			wantEach:      20000,
			wantCurrency:  CurrencyNGN,
			wantRule:      RuleMustReply,
		},
		{
			name:          "Retweet rule NGN each",
			tweetText:     "@bot choose 10 retweeters, ₦100 each",
			sourceTweetID: "22222",
			wantCount:     10,
			wantTotal:     1000,
			wantEach:      100,
			wantCurrency:  CurrencyNGN,
			wantRule:      RuleMustRetweet,
		},
		{
			name:          "Compound follow and reply NGN total",
			tweetText:     "@bot select 2 winners who follow me and replied, total ₦5,000",
			sourceTweetID: "33333",
			wantCount:     2,
			wantTotal:     5000,
			wantEach:      2500,
			wantCurrency:  CurrencyNGN,
			wantRule:      RuleMustFollowAndReply,
		},
		{
			name:          "Word numbers and K suffix",
			tweetText:     "@bot select five people, ₦10k total",
			sourceTweetID: "44444",
			wantCount:     5,
			wantTotal:     10000,
			wantEach:      2000,
			wantCurrency:  CurrencyNGN,
			wantRule:      RuleRandom,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parser.Parse(tt.tweetText, tt.sourceTweetID)
			if err != nil {
				t.Fatalf("Parse() unexpected error: %v", err)
			}
			if got.WinnerCount != tt.wantCount {
				t.Errorf("WinnerCount = %d, want %d", got.WinnerCount, tt.wantCount)
			}
			if got.TotalAmount != tt.wantTotal {
				t.Errorf("TotalAmount = %f, want %f", got.TotalAmount, tt.wantTotal)
			}
			if got.AmountEach != tt.wantEach {
				t.Errorf("AmountEach = %f, want %f", got.AmountEach, tt.wantEach)
			}
			if got.Currency != tt.wantCurrency {
				t.Errorf("Currency = %s, want %s", got.Currency, tt.wantCurrency)
			}
			if got.EntryRule != tt.wantRule {
				t.Errorf("EntryRule = %s, want %s", got.EntryRule, tt.wantRule)
			}
			if got.SourceTweetID != tt.sourceTweetID {
				t.Errorf("SourceTweetID = %s, want %s", got.SourceTweetID, tt.sourceTweetID)
			}
		})
	}
}
