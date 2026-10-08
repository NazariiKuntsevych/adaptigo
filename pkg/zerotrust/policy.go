package zerotrust

import (
	"errors"
	"path"
	"strings"
	"sync"
)

// ErrAccessDenied indicates that the inter-service call was rejected under the Zero-Trust default-deny policy.
var ErrAccessDenied = errors.New("access denied by zero-trust security policy")

// rule defines an explicit authorization grant permitting a caller service to access a downstream resource.
type rule struct {
	// caller is the identity of the invoking service (or "*" for wildcard permission).
	caller string

	// target is the recipient service exposing the API endpoint (or "*" for any).
	target string

	// method is the permitted HTTP verb (e.g., "GET", "POST", or "*").
	method string

	// pathPrefix is the URI boundary permitted for access.
	pathPrefix string
}

// PolicyEngine acts as a Policy Decision Point (PDP) enforcing a strict Default-Deny authorization model.
type PolicyEngine struct {
	mu    sync.RWMutex
	rules []rule
}

// NewPolicyEngine initializes a PolicyEngine with an empty rule repository and default-deny stance.
func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{
		rules: make([]rule, 0),
	}
}

// AddRule registers an explicit access grant into the policy repository.
func (pe *PolicyEngine) AddRule(caller, target, method, pathPrefix string) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	pe.rules = append(pe.rules, rule{
		caller:     caller,
		target:     target,
		method:     strings.ToUpper(method),
		pathPrefix: path.Clean(pathPrefix),
	})
}

// Authorize determines whether the authenticated caller has permission to perform the requested operation.
// Returns nil if an explicit grant matches the request, or ErrAccessDenied otherwise.
func (pe *PolicyEngine) Authorize(caller, target, method, rawPath string) error {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	normalizedMethod := strings.ToUpper(method)
	cleanPath := path.Clean(rawPath)

	for _, rule := range pe.rules {
		if rule.caller != "*" && rule.caller != caller {
			continue
		}
		if rule.target != "*" && rule.target != target {
			continue
		}
		if rule.method != "*" && rule.method != normalizedMethod {
			continue
		}
		if rule.pathPrefix != "*" && cleanPath != rule.pathPrefix && !strings.HasPrefix(cleanPath, strings.TrimSuffix(rule.pathPrefix, "/")+"/") {
			continue
		}
		return nil
	}

	return ErrAccessDenied
}
