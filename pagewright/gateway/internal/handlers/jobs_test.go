package handlers

import "testing"

func TestPublicHistoryCodesDoNotExposeRawDiagnostics(t *testing.T) {
	for _, raw := range []string{"private provider credentials", "worker output\nsecret", "future_code"} {
		if publicFailureCode(raw) != "build_failed" || publicRecoveryCode(raw) != "recovery_attention_required" {
			t.Fatal("unknown diagnostic exposed")
		}
	}
	for _, code := range []string{"", "job_busy", "spawn_failed", "job_conflict", "manager_unavailable", "worker_exit", "artifact_incomplete"} {
		if publicFailureCode(code) != code {
			t.Fatal("known code lost")
		}
	}
	if publicRecoveryCode("manager_evidence_missing_operator_required") != "manager_evidence_missing_operator_required" {
		t.Fatal("recovery hint lost")
	}
}
