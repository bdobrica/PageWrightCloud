package config

import (
	"fmt"
	"os"
	"strconv"
)

type Pilot struct {
	DevelopmentSignup                         bool
	UserDaily, SiteDaily, Active, BudgetCents int
	ProxyToken                                string
}

func LoadPilot() (Pilot, error) {
	p := Pilot{UserDaily: 10, SiteDaily: 5, Active: 2, ProxyToken: os.Getenv("PAGEWRIGHT_PROVIDER_TOKEN")}
	mode := os.Getenv("PAGEWRIGHT_SIGNUP_MODE")
	if mode != "" && mode != "closed" && mode != "development" {
		return p, fmt.Errorf("invalid PAGEWRIGHT_SIGNUP_MODE")
	}
	p.DevelopmentSignup = mode == "development"
	for key, dst := range map[string]*int{"PAGEWRIGHT_USER_DAILY_BUILDS": &p.UserDaily, "PAGEWRIGHT_SITE_DAILY_BUILDS": &p.SiteDaily, "PAGEWRIGHT_ACTIVE_BUILDS": &p.Active, "PAGEWRIGHT_AI_ALLOWANCE_CENTS": &p.BudgetCents} {
		if raw, ok := os.LookupEnv(key); ok && raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v < 0 || v > 1000000 {
				return p, fmt.Errorf("invalid %s", key)
			}
			*dst = v
		}
	}
	if p.Active < 1 || p.Active > 16 || p.UserDaily < 1 || p.SiteDaily < 1 {
		return p, fmt.Errorf("pilot quotas must be positive; active builds must be <=16")
	}
	if p.BudgetCents > 0 && len(p.ProxyToken) < 32 {
		return p, fmt.Errorf("paid AI requires PAGEWRIGHT_PROVIDER_TOKEN with at least 32 characters")
	}
	return p, nil
}
