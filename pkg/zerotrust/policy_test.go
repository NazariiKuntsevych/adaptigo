package zerotrust_test

import (
	"errors"
	"testing"

	"adaptigo/pkg/zerotrust"
)

func TestPolicyEngine_DefaultDeny(t *testing.T) {
	pe := zerotrust.NewPolicyEngine()

	if err := pe.Authorize("service-a", "service-b", "GET", "/api/work"); !errors.Is(err, zerotrust.ErrAccessDenied) {
		t.Errorf("Authorize() = %v, want %v", err, zerotrust.ErrAccessDenied)
	}
}

func TestPolicyEngine_Authorize(t *testing.T) {
	pe := zerotrust.NewPolicyEngine()
	pe.AddRule("service-a", "service-b", "GET", "/api/work")

	if err := pe.Authorize("service-a", "service-b", "GET", "/api/work/item-1"); err != nil {
		t.Errorf("Authorize() = %v, want nil", err)
	}
}

func TestPolicyEngine_UnauthorizedVerb(t *testing.T) {
	pe := zerotrust.NewPolicyEngine()
	pe.AddRule("service-a", "service-b", "GET", "/api/work")

	if err := pe.Authorize("service-a", "service-b", "POST", "/api/work"); !errors.Is(err, zerotrust.ErrAccessDenied) {
		t.Errorf("Authorize() = %v, want %v", err, zerotrust.ErrAccessDenied)
	}
}

func TestPolicyEngine_UnauthorizedCaller(t *testing.T) {
	pe := zerotrust.NewPolicyEngine()
	pe.AddRule("service-a", "service-b", "GET", "/api/work")

	if err := pe.Authorize("service-intruder", "service-b", "GET", "/api/work"); !errors.Is(err, zerotrust.ErrAccessDenied) {
		t.Errorf("Authorize() = %v, want %v", err, zerotrust.ErrAccessDenied)
	}
}

func TestPolicyEngine_UnauthorizedPath(t *testing.T) {
	pe := zerotrust.NewPolicyEngine()
	pe.AddRule("service-a", "service-b", "GET", "/api/work")

	if err := pe.Authorize("service-a", "service-b", "GET", "/admin/metrics"); !errors.Is(err, zerotrust.ErrAccessDenied) {
		t.Errorf("Authorize() = %v, want %v", err, zerotrust.ErrAccessDenied)
	}
}

func TestPolicyEngine_Wildcard(t *testing.T) {
	pe := zerotrust.NewPolicyEngine()
	pe.AddRule("*", "service-b", "GET", "/public")

	if err := pe.Authorize("any-caller", "service-b", "GET", "/public/health"); err != nil {
		t.Errorf("Authorize() = %v, want nil", err)
	}
}
