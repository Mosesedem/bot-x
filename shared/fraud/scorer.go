package fraud

type AccountMetadata struct {
	TwitterID       string
	Handle          string
	AccountAgeDays  int
	FollowerCount   int
	FollowingCount  int
	TweetCount      int
	IsVerified      bool
	HasProfileImage bool
	PhoneVerified   bool
}

const DefaultThreshold = 40.0

// Score calculates a trust score from 0 to 100 based on account metadata.
func Score(meta AccountMetadata) float64 {
	var score float64

	// Account age scoring
	if meta.AccountAgeDays >= 365 {
		score += 30
	} else if meta.AccountAgeDays >= 180 {
		score += 15
	} else if meta.AccountAgeDays >= 30 {
		score += 5
	}

	// Followers count scoring
	if meta.FollowerCount >= 100 {
		score += 20
	} else if meta.FollowerCount >= 10 {
		score += 10
	}

	// Tweet count scoring
	if meta.TweetCount >= 100 {
		score += 10
	} else if meta.TweetCount >= 20 {
		score += 5
	}

	// Verification status
	if meta.IsVerified {
		score += 10
	}

	// Has profile image
	if meta.HasProfileImage {
		score += 5
	}

	// Phone verification
	if meta.PhoneVerified {
		score += 5
	}

	// Follow spam penalty
	if meta.FollowingCount > meta.FollowerCount*10 {
		score -= 20
	}

	// Clamp score between 0 and 100
	if score < 0 {
		score = 0
	} else if score > 100 {
		score = 100
	}

	return score
}

// IsEligible checks if a score is above the required threshold.
func IsEligible(score float64, threshold float64) bool {
	return score >= threshold
}
