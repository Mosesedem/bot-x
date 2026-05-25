package commandparser

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

type EntryRule string

const (
	RuleRandom               EntryRule = "RANDOM"
	RuleMustFollow           EntryRule = "MUST_FOLLOW_HOST"
	RuleMustRetweet          EntryRule = "MUST_RETWEET"
	RuleMustReply            EntryRule = "MUST_REPLY"
	RuleMustFollowAndReply   EntryRule = "MUST_FOLLOW_AND_REPLY"
	RuleMustRetweetAndFollow EntryRule = "MUST_RETWEET_AND_FOLLOW"
)

type Currency string

const (
	CurrencyNGN  Currency = "NGN"
	CurrencyUSD  Currency = "USD"
	CurrencyUSDT Currency = "USDT"
	CurrencyUSDC Currency = "USDC"
)

type GiveawayCommand struct {
	WinnerCount   int
	TotalAmount   float64
	AmountEach    float64
	Currency      Currency
	EntryRule     EntryRule
	SourceTweetID string
	RawText       string
	Confidence    float64
}

type Parser struct{}

func New() *Parser {
	return &Parser{}
}

var wordToNumMap = map[string]int{
	"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8, "nine": 9, "ten": 10,
	"eleven": 11, "twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15, "sixteen": 16, "seventeen": 17,
	"eighteen": 18, "nineteen": 19, "twenty": 20,
}

func (p *Parser) wordToNumber(s string) int {
	if val, ok := wordToNumMap[strings.ToLower(s)]; ok {
		return val
	}
	if n, err := strconv.Atoi(s); err == nil {
		return n
	}
	return 0
}

func (p *Parser) Parse(tweetText string, sourceTweetID string) (*GiveawayCommand, error) {
	rawText := tweetText
	textLower := strings.ToLower(tweetText)

	// Clean bot mentions
	words := strings.Fields(textLower)
	var cleaned []string
	for _, w := range words {
		if !strings.HasPrefix(w, "@") {
			cleaned = append(cleaned, w)
		}
	}
	cleanText := strings.Join(cleaned, " ")

	// 1. Extract winner count
	var winnerCount int
	countConfidence := false

	// Regex for "select 5", "pick 3", "choose 10"
	reActionCount := regexp.MustCompile(`(?:select|pick|choose|draw|giveaway)\s+(\d+|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty)`)
	matches := reActionCount.FindStringSubmatch(cleanText)
	if len(matches) > 1 {
		winnerCount = p.wordToNumber(matches[1])
		countConfidence = winnerCount > 0
	}

	// Regex fallback for "5 winners", "10 followers", "three people"
	if winnerCount == 0 {
		reCountNoun := regexp.MustCompile(`(\d+|one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty)\s+(?:winner|people|person|follower|user|participant|retweeter|rt|repl|member)`)
		matches = reCountNoun.FindStringSubmatch(cleanText)
		if len(matches) > 1 {
			winnerCount = p.wordToNumber(matches[1])
			countConfidence = winnerCount > 0
		}
	}

	// Double fallback: search for any number or word number in the clean text
	if winnerCount == 0 {
		reAnyNum := regexp.MustCompile(`\b(\d+)\b`)
		allMatches := reAnyNum.FindAllStringSubmatch(cleanText, -1)
		for _, m := range allMatches {
			n := p.wordToNumber(m[1])
			if n > 0 && n <= 100 { // reasonable default winner limit
				winnerCount = n
				break
			}
		}
	}

	// 2. Extract currency
	currency := CurrencyNGN // Default currency
	currencyConfidence := false
	if strings.Contains(cleanText, "₦") || strings.Contains(cleanText, "ngn") {
		currency = CurrencyNGN
		currencyConfidence = true
	} else if strings.Contains(cleanText, "$") || strings.Contains(cleanText, "usd") {
		currency = CurrencyUSD
		currencyConfidence = true
	} else if strings.Contains(cleanText, "usdt") {
		currency = CurrencyUSDT
		currencyConfidence = true
	} else if strings.Contains(cleanText, "usdc") {
		currency = CurrencyUSDC
		currencyConfidence = true
	}

	// 3. Extract amount
	var rawAmount float64
	amountConfidence := false

	// First attempt: look for numbers explicitly bounded by currency symbols or names
	reStrictAmount := regexp.MustCompile(`(?:₦|\$|ngn|usd|usdt|usdc)\s*(\d+(?:,\d+)*(?:\.\d+)?k?)\b|\b(\d+(?:,\d+)*(?:\.\d+)?k?)\s*(?:ngn|usd|usdt|usdc)`)
	strictMatches := reStrictAmount.FindAllStringSubmatch(cleanText, -1)
	for _, match := range strictMatches {
		valStr := ""
		if match[1] != "" {
			valStr = match[1]
		} else if match[2] != "" {
			valStr = match[2]
		}
		valStr = strings.ToLower(valStr)
		if strings.Contains(valStr, "k") {
			numPart := strings.ReplaceAll(valStr, "k", "")
			numPart = strings.ReplaceAll(numPart, ",", "")
			if f, err := strconv.ParseFloat(numPart, 64); err == nil {
				rawAmount = f * 1000
				amountConfidence = true
				break
			}
		} else {
			numPart := strings.ReplaceAll(valStr, ",", "")
			if f, err := strconv.ParseFloat(numPart, 64); err == nil {
				rawAmount = f
				amountConfidence = true
				break
			}
		}
	}

	// Second attempt: fallback to general numbers if no currency-bound numbers found
	if rawAmount == 0 {
		reAmount := regexp.MustCompile(`\b\d+(?:,\d+)*(?:\.\d+)?k?\b`)
		amountMatches := reAmount.FindAllString(cleanText, -1)
		for _, valStr := range amountMatches {
			valStrLower := strings.ToLower(valStr)
			var f float64
			var err error
			if strings.Contains(valStrLower, "k") {
				numPart := strings.ReplaceAll(valStrLower, "k", "")
				numPart = strings.ReplaceAll(numPart, ",", "")
				f, err = strconv.ParseFloat(numPart, 64)
				f = f * 1000
			} else {
				numPart := strings.ReplaceAll(valStrLower, ",", "")
				f, err = strconv.ParseFloat(numPart, 64)
			}
			if err == nil {
				// Ignore numbers that equal the winner count or are too small
				if int(f) != winnerCount && f >= 10 {
					rawAmount = f
					amountConfidence = true
					break
				}
			}
		}
	}

	if winnerCount == 0 {
		return nil, errors.New("could not parse winner count")
	}
	if rawAmount == 0 {
		return nil, errors.New("could not parse giveaway amount")
	}

	// 4. Determine total vs. per-winner
	isPerWinner := strings.Contains(cleanText, "each") || strings.Contains(cleanText, "per") || strings.Contains(cleanText, "/person")

	var totalAmount, amountEach float64
	if isPerWinner {
		amountEach = rawAmount
		totalAmount = rawAmount * float64(winnerCount)
	} else {
		totalAmount = rawAmount
		amountEach = totalAmount / float64(winnerCount)
	}

	// 5. Extract entry rules
	hasFollow := strings.Contains(cleanText, "follow")
	hasRetweet := strings.Contains(cleanText, "retweet") || strings.Contains(cleanText, "rt") || strings.Contains(cleanText, "repost")
	hasReply := strings.Contains(cleanText, "repl") || strings.Contains(cleanText, "comment")

	var entryRule EntryRule = RuleRandom
	if hasFollow && hasRetweet {
		entryRule = RuleMustRetweetAndFollow
	} else if hasFollow && hasReply {
		entryRule = RuleMustFollowAndReply
	} else if hasFollow {
		entryRule = RuleMustFollow
	} else if hasRetweet {
		entryRule = RuleMustRetweet
	} else if hasReply {
		entryRule = RuleMustReply
	}

	// 6. Compute confidence score
	var confidence float64 = 0.4
	if countConfidence && amountConfidence && currencyConfidence {
		confidence = 1.0
	} else if countConfidence && amountConfidence {
		confidence = 0.7
	}

	return &GiveawayCommand{
		WinnerCount:   winnerCount,
		TotalAmount:   totalAmount,
		AmountEach:    amountEach,
		Currency:      currency,
		EntryRule:     entryRule,
		SourceTweetID: sourceTweetID,
		RawText:       rawText,
		Confidence:    confidence,
	}, nil
}
