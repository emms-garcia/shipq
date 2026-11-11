package identity_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/emms-garcia/shipq/internal/domain"
	"github.com/emms-garcia/shipq/internal/domain/identity"
)

func TestReference(t *testing.T) {
	first, err := identity.NewReference("GitHub.COM", "PR_fixture_A")
	if err != nil {
		t.Fatal(err)
	}
	same, err := identity.NewReference("github.com", "PR_fixture_A")
	if err != nil {
		t.Fatal(err)
	}
	otherHost, err := identity.NewReference("other.example", "PR_fixture_A")
	if err != nil {
		t.Fatal(err)
	}
	otherNode, err := identity.NewReference("github.com", "PR_fixture_a")
	if err != nil {
		t.Fatal(err)
	}
	if first != same || first == otherHost || first == otherNode {
		t.Fatal("references must use canonical host and case-sensitive opaque node ID")
	}
	if first.Host() != "github.com" || first.NodeID() != "PR_fixture_A" || first.IsZero() {
		t.Fatalf("unexpected canonical reference: %+v", first)
	}
	if !(identity.Reference{}).IsZero() {
		t.Fatal("zero reference must represent an unresolved identity")
	}
}

func TestReferenceRejectsInvalidInput(t *testing.T) {
	for _, host := range []string{"", " github.com", "https://github.com", "github.com:443", "user@github.com", "github..com", "-github.com", "github-.com", "github.com/path", "github.com?x", "github_com", strings.Repeat("x", 64) + ".com", strings.Repeat("a.", 127) + "com"} {
		t.Run(host, func(t *testing.T) {
			_, err := identity.NewReference(host, "PR_fixture")
			assertValidation(t, err, "host")
		})
	}
	for _, node := range []string{"", " ", " PR_fixture", "PR fixture", "PR\nfixture", "PR\x00fixture"} {
		t.Run(node, func(t *testing.T) {
			_, err := identity.NewReference("github.com", node)
			assertValidation(t, err, "node_id")
		})
	}
}

func TestLocalID(t *testing.T) {
	const value = "ABCDEF01-2345-4678-89AB-CDEF01234567"
	id, err := identity.ParseID(value)
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != strings.ToLower(value) {
		t.Fatalf("UUID did not retain its canonical identity: %s", id)
	}
	for _, value := range []string{"", "not-a-uuid", "00000000-0000-0000-0000-000000000000"} {
		_, err := identity.ParseID(value)
		assertValidation(t, err, "id")
	}
}

func assertValidation(t *testing.T, err error, field string) {
	t.Helper()
	var validation *domain.ValidationError
	if !errors.As(err, &validation) || validation.Field != field || !strings.Contains(err.Error(), field) {
		t.Fatalf("expected typed validation error for %s, got %v", field, err)
	}
}
