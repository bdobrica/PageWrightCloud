package config

import "testing"

func TestPilotConfigurationFailsClosed(t *testing.T) {
	for _, key := range []string{"PAGEWRIGHT_SIGNUP_MODE", "PAGEWRIGHT_USER_DAILY_BUILDS", "PAGEWRIGHT_SITE_DAILY_BUILDS", "PAGEWRIGHT_ACTIVE_BUILDS", "PAGEWRIGHT_AI_ALLOWANCE_CENTS", "PAGEWRIGHT_PROVIDER_TOKEN"} {
		t.Setenv(key, "")
	}
	p, err := LoadPilot()
	if err != nil || p.DevelopmentSignup || p.BudgetCents != 0 || p.UserDaily != 10 || p.SiteDaily != 5 || p.Active != 2 {
		t.Fatalf("defaults %+v %v", p, err)
	}
	for key, value := range map[string]string{"PAGEWRIGHT_SIGNUP_MODE": "open", "PAGEWRIGHT_USER_DAILY_BUILDS": "0", "PAGEWRIGHT_SITE_DAILY_BUILDS": "oops", "PAGEWRIGHT_ACTIVE_BUILDS": "17", "PAGEWRIGHT_AI_ALLOWANCE_CENTS": "-1"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, value)
			if _, err := LoadPilot(); err == nil {
				t.Fatal("invalid setting accepted")
			}
		})
	}
	t.Setenv("PAGEWRIGHT_AI_ALLOWANCE_CENTS", "100")
	if _, err := LoadPilot(); err == nil {
		t.Fatal("missing proxy credential accepted")
	}
}
